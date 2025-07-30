/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package crds

import (
	"fmt"
	"net/url"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

// VirtualServerRouteConverter converts a VirtualServer to HTTPRoute and/or GRPCRoute based on upstream types
type VirtualServerRouteConverter struct {
	vs               nginxv1.VirtualServer
	resolver         *RouteResolver
	virtualServerMap map[string][]gatewayListenerKey
	notificationList *[]notifications.Notification
	listenerMap      map[string]gatewayv1.Listener
	upstreamConfigs  map[string]*UpstreamConfig
}

// NewVirtualServerRouteConverter creates a new converter
func NewVirtualServerRouteConverter(vs nginxv1.VirtualServer, resolver *RouteResolver, virtualServerMap map[string][]gatewayListenerKey, notifs *[]notifications.Notification, listenerMap map[string]gatewayv1.Listener, upstreamConfigs map[string]*UpstreamConfig) *VirtualServerRouteConverter {
	return &VirtualServerRouteConverter{
		vs:               vs,
		resolver:         resolver,
		virtualServerMap: virtualServerMap,
		notificationList: notifs,
		listenerMap:      listenerMap,
		upstreamConfigs:  upstreamConfigs,
	}
}

// ConvertToRoutes converts the VirtualServer to HTTPRoute and/or GRPCRoute based on upstream types
func (c *VirtualServerRouteConverter) ConvertToRoutes() (map[types.NamespacedName]intermediate.HTTPRouteContext, map[types.NamespacedName]gatewayv1.GRPCRoute) {
	httpRoutes := make(map[types.NamespacedName]intermediate.HTTPRouteContext)
	grpcRoutes := make(map[types.NamespacedName]gatewayv1.GRPCRoute)

	// Resolve all routes for this VirtualServer
	resolvedRoutes, resolveNotifications, err := c.resolver.ResolveRoutesForVirtualServer(c.vs)
	if err != nil {
		c.addNotification(notifications.ErrorNotification,
			fmt.Sprintf("Failed to resolve routes for VirtualServer %s: %v", c.vs.Name, err))
		return httpRoutes, grpcRoutes
	}
	*c.notificationList = append(*c.notificationList, resolveNotifications...)

	var rules []gatewayv1.HTTPRouteRule
	for _, resolvedRoute := range resolvedRoutes {
		routeRules := c.convertResolvedRouteToRules(resolvedRoute)
		rules = append(rules, routeRules...)
	}

	var httpRules []gatewayv1.HTTPRouteRule
	var grpcRules []gatewayv1.GRPCRouteRule

	for _, rule := range rules {
		if c.isRouteGRPC(&rule) {
			grpcRule := c.convertHTTPRuleToGRPCRule(rule)
			grpcRules = append(grpcRules, grpcRule)
		} else {
			httpRules = append(httpRules, rule)
		}
	}

	// Convert upstream names to service names after separating HTTP/gRPC rules
	c.convertUpstreamNamesToServiceNames(httpRules)
	c.convertGRPCUpstreamNamesToServiceNames(grpcRules)

	if len(httpRules) > 0 {
		httpRoute, httpRouteKey := c.createHTTPRoute(httpRules)
		httpRoutes[httpRouteKey] = httpRoute
	}

	if len(grpcRules) > 0 {
		grpcRoute, grpcRouteKey := c.createGRPCRoute(grpcRules)
		grpcRoutes[grpcRouteKey] = grpcRoute
	}
	return httpRoutes, grpcRoutes
}

// createParentRefs creates ParentRefs for HTTPRoute based on VirtualServer listener configuration
func (c *VirtualServerRouteConverter) createParentRefs() []gatewayv1.ParentReference {
	var parentRefs []gatewayv1.ParentReference
	for _, listener := range c.virtualServerMap[c.vs.Name] {
		parentRefs = append(parentRefs, gatewayv1.ParentReference{
			Name:        gatewayv1.ObjectName(listener.gatewayName),
			SectionName: (*gatewayv1.SectionName)(&listener.listenerName),
		})
	}
	return parentRefs
}

// generateHTTPRouteName creates a consistent name for the HTTPRoute
func (c *VirtualServerRouteConverter) generateHTTPRouteName() string {
	return c.vs.Name + "-httproute"
}

// convertResolvedRouteToRules converts a resolved route to multiple HTTPRoute rules
// Each NGINX match+action becomes a separate Gateway API rule, ordered by specificity
func (c *VirtualServerRouteConverter) convertResolvedRouteToRules(resolvedRoute ResolvedRoute) []gatewayv1.HTTPRouteRule {
	route := resolvedRoute.Route
	vs := resolvedRoute.VirtualServer

	var rules []gatewayv1.HTTPRouteRule

	var basePathMatch gatewayv1.HTTPRouteMatch

	if strings.HasPrefix(route.Path, "~") {
		basePathMatch = gatewayv1.HTTPRouteMatch{
			Path: &gatewayv1.HTTPPathMatch{
				Type:  Ptr(gatewayv1.PathMatchRegularExpression),
				Value: Ptr(route.Path),
			},
		}
	} else {

		basePathMatch = gatewayv1.HTTPRouteMatch{
			Path: &gatewayv1.HTTPPathMatch{
				Type:  Ptr(gatewayv1.PathMatchPathPrefix),
				Value: Ptr(route.Path),
			},
		}
	}

	// Process each match with its specific action (ordered by specificity)
	for _, match := range route.Matches {
		if match.Action != nil {

			m := c.createMatch(basePathMatch, match, vs)
			if len(m.Headers) == 0 && len(m.QueryParams) == 0 {
				continue
			}

			rule := gatewayv1.HTTPRouteRule{
				Matches: []gatewayv1.HTTPRouteMatch{
					m,
				},
			}

			c.handleRouteActions(vs, match.Action, &rule)

			c.handleTrafficSplits(vs, match.Splits, &rule)

			rules = append(rules, rule)
		}
	}

	// Add default rule for route-level action (catch-all, comes last)
	if route.Action != nil || len(route.Splits) > 0 {
		defaultRule := gatewayv1.HTTPRouteRule{
			Matches: []gatewayv1.HTTPRouteMatch{basePathMatch},
		}

		c.handleRouteActions(vs, route.Action, &defaultRule)

		c.handleTrafficSplits(vs, route.Splits, &defaultRule)

		rules = append(rules, defaultRule)
	}

	return rules
}

// createMatch combines base path match with specific match conditions
func (c *VirtualServerRouteConverter) createMatch(basePathMatch gatewayv1.HTTPRouteMatch, match nginxv1.Match, vs nginxv1.VirtualServer) gatewayv1.HTTPRouteMatch {
	specificMatch := basePathMatch

	// Process match conditions and add to the base path match
	if len(match.Conditions) > 0 {
		headerMatches, queryMatches := processConditions(match.Conditions, vs, c.notificationList)

		if len(headerMatches) > 0 {
			specificMatch.Headers = headerMatches
		}

		if len(queryMatches) > 0 {
			specificMatch.QueryParams = queryMatches
		}
		return specificMatch
	}

	return specificMatch
}

// handleRouteActions processes different route action types
func (c *VirtualServerRouteConverter) handleRouteActions(vs nginxv1.VirtualServer, action *nginxv1.Action, rule *gatewayv1.HTTPRouteRule) {
	if action == nil {
		return
	}

	if action.Pass != "" {
		backendRef := c.handlePassAction(vs, action)
		if backendRef != nil {
			rule.BackendRefs = []gatewayv1.HTTPBackendRef{*backendRef}
		}
	} else if action.Redirect != nil {
		rule.Filters = append(rule.Filters, c.handleRedirectAction(vs, action))
	} else if action.Return != nil {
		c.handleReturnAction(vs, action, rule)
	} else {
		backendRef, filters := handleAdvancedProxyAction(vs, action, c.notificationList)
		if backendRef != nil {
			rule.BackendRefs = []gatewayv1.HTTPBackendRef{*backendRef}
		}
		if len(filters) > 0 {
			rule.Filters = append(rule.Filters, filters...)
		}
	}
}

// handlePassAction handles proxy pass actions to upstreams
func (c *VirtualServerRouteConverter) handlePassAction(vs nginxv1.VirtualServer, action *nginxv1.Action) *gatewayv1.HTTPBackendRef {
	upstream := findUpstream(vs.Spec.Upstreams, action.Pass)
	if upstream != nil {
		return &gatewayv1.HTTPBackendRef{
			BackendRef: gatewayv1.BackendRef{
				BackendObjectReference: gatewayv1.BackendObjectReference{
					Name: gatewayv1.ObjectName(action.Pass), // Need the name of the upstream for GRPC checking later
					Port: Ptr(gatewayv1.PortNumber(upstream.Port)),
				},
			},
		}
	}

	c.addNotification(notifications.WarningNotification,
		fmt.Sprintf("Upstream '%s' not found for route", action.Pass))
	return nil
}

// handleRedirectAction handles HTTP redirect actions
func (c *VirtualServerRouteConverter) handleRedirectAction(
	_ nginxv1.VirtualServer,
	action *nginxv1.Action,
) gatewayv1.HTTPRouteFilter {
	rr := &gatewayv1.HTTPRequestRedirectFilter{
		StatusCode: Ptr(301),
	}

	// Parse URL and set appropriate fields
	if action.Redirect.URL != "" {
		parsedURL := parseRedirectURL(action.Redirect.URL)

		if parsedURL.Scheme != "" {
			rr.Scheme = &parsedURL.Scheme
		}

		if parsedURL.Hostname != "" {
			rr.Hostname = Ptr(gatewayv1.PreciseHostname(parsedURL.Hostname))
		}

		if parsedURL.Path != "" {
			rr.Path = &gatewayv1.HTTPPathModifier{
				Type:            gatewayv1.FullPathHTTPPathModifier,
				ReplaceFullPath: &parsedURL.Path,
			}
		}
	}

	// override status code if the user set one
	switch action.Redirect.Code {
	case 0:
		// nothing to do, keep default 301
	case 307:
		rr.StatusCode = Ptr(302)
	case 308:
		rr.StatusCode = Ptr(301)
	default:
		// 301 or 302 assuming its valid for NIC
		rr.StatusCode = Ptr(action.Redirect.Code)
	}

	return gatewayv1.HTTPRouteFilter{
		Type:            gatewayv1.HTTPRouteFilterRequestRedirect,
		RequestRedirect: rr,
	}
}

// handleReturnAction handles direct return responses
func (c *VirtualServerRouteConverter) handleReturnAction(_ nginxv1.VirtualServer, action *nginxv1.Action, _ *gatewayv1.HTTPRouteRule) {
	c.addNotification(notifications.WarningNotification,
		fmt.Sprintf("Return action with code %d not directly supported in Gateway API", action.Return.Code))
}

// handleTrafficSplits handles weighted traffic distribution
func (c *VirtualServerRouteConverter) handleTrafficSplits(vs nginxv1.VirtualServer, splits []nginxv1.Split, rule *gatewayv1.HTTPRouteRule) {
	if len(splits) == 0 {
		return
	}

	for _, split := range splits {
		if split.Action == nil || split.Weight == 0 {
			continue
		}

		if split.Action.Pass != "" {
			backendRef := c.handlePassAction(vs, split.Action)
			if backendRef != nil {
				backendRef.Weight = Ptr(int32(split.Weight))
				rule.BackendRefs = append(rule.BackendRefs, *backendRef)
			}
		} else if split.Action.Redirect != nil {
			backendRef := gatewayv1.HTTPBackendRef{
				BackendRef: gatewayv1.BackendRef{
					Weight: Ptr(int32(split.Weight)),
				},
			}
			backendRef.Filters = append(backendRef.Filters, c.handleRedirectAction(vs, split.Action))
			rule.BackendRefs = append(rule.BackendRefs, backendRef)
		} else if split.Action.Return != nil {
			c.handleReturnAction(vs, split.Action, rule)
		} else {
			backendRef, filters := handleAdvancedProxyAction(vs, split.Action, c.notificationList)
			if backendRef != nil {
				backendRef.Weight = Ptr(int32(split.Weight))
				if len(filters) > 0 {
					backendRef.Filters = append(backendRef.Filters, filters...)
				}
				rule.BackendRefs = append(rule.BackendRefs, *backendRef)
			}
		}
	}

	c.addNotification(notifications.InfoNotification,
		"Traffic splitting configuration converted to weighted backend refs")
}

// isRouteGRPC determines if a route should be treated as gRPC based on its referenced upstreams
func (c *VirtualServerRouteConverter) isRouteGRPC(rule *gatewayv1.HTTPRouteRule) bool {
	if rule.BackendRefs != nil {
		for _, backendRef := range rule.BackendRefs {
			upstreamName := string(backendRef.BackendObjectReference.Name)
			if config, exists := c.upstreamConfigs[upstreamName]; exists && config.Type == "grpc" {
				return true
			}
		}
	}
	return false
}

// convertUpstreamNamesToServiceNames converts upstream names to service names in backend refs
func (c *VirtualServerRouteConverter) convertUpstreamNamesToServiceNames(rules []gatewayv1.HTTPRouteRule) {
	for i := range rules {
		for j := range rules[i].BackendRefs {
			upstreamName := string(rules[i].BackendRefs[j].BackendObjectReference.Name)
			if config, exists := c.upstreamConfigs[upstreamName]; exists {
				rules[i].BackendRefs[j].BackendObjectReference.Name = gatewayv1.ObjectName(config.Service)
			}
		}
	}
}

// convertGRPCUpstreamNamesToServiceNames converts upstream names to service names in gRPC backend refs
func (c *VirtualServerRouteConverter) convertGRPCUpstreamNamesToServiceNames(rules []gatewayv1.GRPCRouteRule) {
	for i := range rules {
		for j := range rules[i].BackendRefs {
			upstreamName := string(rules[i].BackendRefs[j].BackendObjectReference.Name)
			if config, exists := c.upstreamConfigs[upstreamName]; exists {
				rules[i].BackendRefs[j].BackendObjectReference.Name = gatewayv1.ObjectName(config.Service)
			}
		}
	}
}

// createHTTPRoute creates an HTTPRoute with the given rules
func (c *VirtualServerRouteConverter) createHTTPRoute(rules []gatewayv1.HTTPRouteRule) (intermediate.HTTPRouteContext, types.NamespacedName) {
	httpRouteName := c.generateHTTPRouteName()
	httpRouteKey := types.NamespacedName{
		Namespace: c.vs.Namespace,
		Name:      httpRouteName,
	}

	// Create HTTPRoute
	httpRoute := gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1.GroupVersion.String(),
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      httpRouteName,
			Namespace: c.vs.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ingress2gateway",
				"ingress2gateway.io/source":    "nginx-virtualserver",
				"ingress2gateway.io/vs-name":   c.vs.Name,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: c.createParentRefs(),
			},
			Hostnames: []gatewayv1.Hostname{
				gatewayv1.Hostname(c.vs.Spec.Host),
			},
			Rules: rules,
		},
	}

	// Add notification about HTTPRoute creation
	c.addNotification(notifications.InfoNotification,
		fmt.Sprintf("Created HTTPRoute '%s' with %d HTTP rules for host '%s'",
			httpRouteName, len(rules), c.vs.Spec.Host))

	return intermediate.HTTPRouteContext{
		HTTPRoute: httpRoute,
	}, httpRouteKey
}

// createGRPCRoute creates a GRPCRoute with the given rules
func (c *VirtualServerRouteConverter) createGRPCRoute(rules []gatewayv1.GRPCRouteRule) (gatewayv1.GRPCRoute, types.NamespacedName) {
	grpcRouteName := c.generateGRPCRouteName()
	grpcRouteKey := types.NamespacedName{
		Namespace: c.vs.Namespace,
		Name:      grpcRouteName,
	}

	// Create GRPCRoute
	grpcRoute := gatewayv1.GRPCRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1.GroupVersion.String(),
			Kind:       "GRPCRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      grpcRouteName,
			Namespace: c.vs.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ingress2gateway",
				"ingress2gateway.io/source":    "nginx-virtualserver",
				"ingress2gateway.io/vs-name":   c.vs.Name,
			},
		},
		Spec: gatewayv1.GRPCRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: c.createParentRefs(),
			},
			Hostnames: []gatewayv1.Hostname{
				gatewayv1.Hostname(c.vs.Spec.Host),
			},
			Rules: rules,
		},
	}

	// Add notification about GRPCRoute creation
	c.addNotification(notifications.InfoNotification,
		fmt.Sprintf("Created GRPCRoute '%s' with %d gRPC rules for host '%s'",
			grpcRouteName, len(rules), c.vs.Spec.Host))

	return grpcRoute, grpcRouteKey
}

// generateGRPCRouteName creates a consistent name for the GRPCRoute
func (c *VirtualServerRouteConverter) generateGRPCRouteName() string {
	return c.vs.Name + "-grpcroute"
}

// convertHTTPRuleToGRPCRule converts an HTTPRoute rule to a GRPCRoute rule
func (c *VirtualServerRouteConverter) convertHTTPRuleToGRPCRule(httpRule gatewayv1.HTTPRouteRule) gatewayv1.GRPCRouteRule {
	grpcRule := gatewayv1.GRPCRouteRule{}

	// Convert HTTPBackendRefs to GRPCBackendRefs
	for _, httpBackendRef := range httpRule.BackendRefs {
		grpcBackendRef := gatewayv1.GRPCBackendRef{
			BackendRef: httpBackendRef.BackendRef,
		}

		// Convert HTTP filters to gRPC filters
		grpcBackendRef.Filters = c.convertHTTPFiltersToGRPCFilters(httpBackendRef.Filters)

		grpcRule.BackendRefs = append(grpcRule.BackendRefs, grpcBackendRef)
	}

	// Convert rule-level filters
	grpcRule.Filters = c.convertHTTPFiltersToGRPCFilters(httpRule.Filters)

	// Convert HTTP matches to gRPC matches
	grpcRule.Matches = c.convertHTTPMatchesToGRPCMatches(httpRule.Matches)

	return grpcRule
}

// convertHTTPMatchesToGRPCMatches converts HTTPRoute matches to GRPCRoute matches
// Converts path-based matches to gRPC service/method format
func (c *VirtualServerRouteConverter) convertHTTPMatchesToGRPCMatches(httpMatches []gatewayv1.HTTPRouteMatch) []gatewayv1.GRPCRouteMatch {
	var grpcMatches []gatewayv1.GRPCRouteMatch

	for _, httpMatch := range httpMatches {
		grpcMatch := gatewayv1.GRPCRouteMatch{}

		// Convert path to gRPC service/method
		if httpMatch.Path != nil && httpMatch.Path.Value != nil {
			pathValue := *httpMatch.Path.Value

			// Parse gRPC service/method from path
			// Expected format: /package.Service/Method or /package.Service
			service, method := parseGRPCServiceMethod(pathValue)
			if service != "" {
				grpcMatch.Method = &gatewayv1.GRPCMethodMatch{
					Service: &service,
				}

				if method != "" {
					grpcMatch.Method.Method = &method
				}
			}
		}

		// Convert headers (gRPC supports header matching)
		if len(httpMatch.Headers) > 0 {
			grpcMatch.Headers = make([]gatewayv1.GRPCHeaderMatch, len(httpMatch.Headers))
			for i, httpHeader := range httpMatch.Headers {
				grpcMatch.Headers[i] = gatewayv1.GRPCHeaderMatch{
					Type:  (*gatewayv1.HeaderMatchType)(httpHeader.Type),
					Name:  gatewayv1.GRPCHeaderName(httpHeader.Name),
					Value: httpHeader.Value,
				}
			}
		}

		// Note: Query parameters don't apply to gRPC, so we skip them

		grpcMatches = append(grpcMatches, grpcMatch)
	}

	return grpcMatches
}

// convertHTTPFiltersToGRPCFilters converts HTTPRoute filters to GRPCRoute filters
// Only converts filters that are actually created by the HTTPRoute conversion logic
func (c *VirtualServerRouteConverter) convertHTTPFiltersToGRPCFilters(httpFilters []gatewayv1.HTTPRouteFilter) []gatewayv1.GRPCRouteFilter {
	var grpcFilters []gatewayv1.GRPCRouteFilter

	for _, httpFilter := range httpFilters {
		switch httpFilter.Type {
		case gatewayv1.HTTPRouteFilterRequestHeaderModifier:
			if httpFilter.RequestHeaderModifier != nil {
				grpcFilter := gatewayv1.GRPCRouteFilter{
					Type:                  gatewayv1.GRPCRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: httpFilter.RequestHeaderModifier,
				}
				grpcFilters = append(grpcFilters, grpcFilter)
			}

		case gatewayv1.HTTPRouteFilterResponseHeaderModifier:
			if httpFilter.ResponseHeaderModifier != nil {
				grpcFilter := gatewayv1.GRPCRouteFilter{
					Type:                   gatewayv1.GRPCRouteFilterResponseHeaderModifier,
					ResponseHeaderModifier: httpFilter.ResponseHeaderModifier,
				}
				grpcFilters = append(grpcFilters, grpcFilter)
			}

		case gatewayv1.HTTPRouteFilterRequestRedirect:
			c.addNotification(notifications.InfoNotification,
				"HTTP redirect filter not applicable to gRPC, skipping")

		case gatewayv1.HTTPRouteFilterURLRewrite:
			c.addNotification(notifications.InfoNotification,
				"HTTP URL rewrite filter not applicable to gRPC, skipping")

		default:
			c.addNotification(notifications.WarningNotification,
				fmt.Sprintf("Unknown HTTP filter type %s encountered during gRPC conversion", httpFilter.Type))
		}
	}

	return grpcFilters
}

// addNotification adds a notification to the notification list
func (c *VirtualServerRouteConverter) addNotification(messageType notifications.MessageType, message string) {
	addNotification(c.notificationList, messageType, message, &c.vs)
}

// ParsedURL represents the components of a parsed redirect URL
type ParsedURL struct {
	Scheme   string
	Hostname string
	Path     string
}

// parseRedirectURL parses a redirect URL and extracts scheme, hostname, and path
func parseRedirectURL(redirectURL string) ParsedURL {
	parsed := ParsedURL{}

	// Parse the URL
	u, err := url.Parse(redirectURL)
	if err != nil {
		// If parsing fails, treat the entire URL as a path
		parsed.Path = redirectURL
		return parsed
	}

	// Extract components
	if u.Scheme != "" {
		parsed.Scheme = u.Scheme
	}

	if u.Host != "" {
		parsed.Hostname = u.Host
	}

	// For path, we want the full path including query and fragment if present
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		path := u.Path
		if u.RawQuery != "" {
			path += "?" + u.RawQuery
		}
		if u.Fragment != "" {
			path += "#" + u.Fragment
		}
		parsed.Path = path
	}

	return parsed
}

// parseGRPCServiceMethod parses gRPC service and method from a path-like string
// Expected formats:
//   - "/package.Service/Method" -> service="package.Service", method="Method"
//   - "/package.Service" -> service="package.Service", method=""
//   - "package.Service/Method" -> service="package.Service", method="Method"
func parseGRPCServiceMethod(path string) (service, method string) {
	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	// Split on the last slash to separate service from method
	if lastSlash := strings.LastIndex(path, "/"); lastSlash != -1 {
		service = path[:lastSlash]
		method = path[lastSlash+1:]
	} else {
		// No method specified, entire path is the service
		service = path
		method = ""
	}

	return service, method
}
