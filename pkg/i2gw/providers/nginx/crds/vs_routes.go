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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

// convertRouteToHTTPRoute creates an HTTPRoute from VirtualServer route
func convertRouteToHTTPRoute(vs nginxv1.VirtualServer, route nginxv1.Route, routeIndex int, gatewayRef types.NamespacedName) (intermediate.HTTPRouteContext, []notifications.Notification) {
	var notificationList []notifications.Notification
	var rules []gatewayv1.HTTPRouteRule

	// If route has matches, create separate rules for each match
	if len(route.Matches) > 0 {
		rules = createHTTPRouteRulesForMatches(vs, route, &notificationList)
	} else {
		// Create single rule for route without matches
		rule := createHTTPRouteRule(vs, route, &notificationList)
		
		// Handle route actions
		handleRouteActions(vs, route, &rule, &notificationList)
		
		// Handle traffic splits
		handleTrafficSplits(vs, route, &rule, &notificationList)
		
		rules = []gatewayv1.HTTPRouteRule{rule}
	}

	// Create HTTPRoute
	httpRoute := gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1.GroupVersion.String(),
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateHTTPRouteName(vs, routeIndex),
			Namespace: vs.Namespace,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name: gatewayv1.ObjectName(gatewayRef.Name),
					},
				},
			},
			Rules: rules,
		},
	}

	// Create nginx-specific HTTPRoute IR
	nginxHTTPRouteIR := createNginxHTTPRouteIR(vs, route, &notificationList)

	return intermediate.HTTPRouteContext{
		HTTPRoute: httpRoute,
		ProviderSpecificIR: intermediate.ProviderSpecificHTTPRouteIR{
			Nginx: nginxHTTPRouteIR,
		},
	}, notificationList
}

// createHTTPRouteRule creates HTTPRoute rule with advanced matching support
func createHTTPRouteRule(vs nginxv1.VirtualServer, route nginxv1.Route, notifs *[]notifications.Notification) gatewayv1.HTTPRouteRule {
	// Create matches with advanced matching support
	matches := createAdvancedMatches(vs, route, notifs)

	rule := gatewayv1.HTTPRouteRule{
		Matches: matches,
	}

	// If route has matches, extract backend refs and filters from match actions
	if len(route.Matches) > 0 {
		handleMatchActions(vs, route, &rule, notifs)
	}

	return rule
}

// createHTTPRouteRulesForMatches creates separate HTTPRoute rules for each match
func createHTTPRouteRulesForMatches(vs nginxv1.VirtualServer, route nginxv1.Route, notifs *[]notifications.Notification) []gatewayv1.HTTPRouteRule {
	var rules []gatewayv1.HTTPRouteRule
	
	for i, match := range route.Matches {
		if match.Action == nil {
			continue
		}
		
		// Create match for this specific condition
		pathMatch := gatewayv1.HTTPRouteMatch{
			Path: &gatewayv1.HTTPPathMatch{
				Type:  Ptr(gatewayv1.PathMatchPathPrefix),
				Value: Ptr(route.Path),
			},
		}
		
		// Add specific match conditions for this match
		headerMatches, queryMatches := processConditions(match.Conditions, vs, notifs)
		if len(headerMatches) > 0 {
			pathMatch.Headers = headerMatches
		}
		if len(queryMatches) > 0 {
			pathMatch.QueryParams = queryMatches
		}
		
		// Create rule for this specific match
		rule := gatewayv1.HTTPRouteRule{
			Matches: []gatewayv1.HTTPRouteMatch{pathMatch},
		}
		
		// Create temporary route to process the match action
		tempRoute := nginxv1.Route{
			Path:   route.Path,
			Action: match.Action,
		}
		
		// Handle the action for this specific match
		handleRouteActions(vs, tempRoute, &rule, notifs)
		
		// Handle splits if present in the match
		if len(match.Splits) > 0 {
			handleMatchSplits(vs, match, &rule, notifs)
		}
		
		rules = append(rules, rule)
		
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Created separate HTTPRoute rule for match %d with conditions", i), &vs)
	}
	
	// Note: Default actions (top-level route actions) are not converted for routes with matches
	// because Gateway API doesn't have a direct equivalent to NGINX's return/redirect actions
	// as catch-all rules. This is stored in provider-specific IR for future policy-based handling.
	if route.Action != nil {
		addNotification(notifs, notifications.InfoNotification,
			"Default action for routes with matches stored in provider-specific IR - Gateway API fallback behavior may differ", &vs)
	}
	
	return rules
}

// handleMatchSplits processes splits within a match
func handleMatchSplits(vs nginxv1.VirtualServer, match nginxv1.Match, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	var backendRefs []gatewayv1.HTTPBackendRef
	
	for _, split := range match.Splits {
		if split.Action == nil {
			continue
		}
		
		// Create temporary route to process split action
		tempRoute := nginxv1.Route{
			Action: split.Action,
		}
		
		// Create a temporary rule to extract backend ref
		tempRule := gatewayv1.HTTPRouteRule{}
		handleRouteActions(vs, tempRoute, &tempRule, notifs)
		
		// Add backend ref with weight
		if len(tempRule.BackendRefs) > 0 {
			backendRef := tempRule.BackendRefs[0]
			backendRef.Weight = Ptr(int32(split.Weight))
			backendRefs = append(backendRefs, backendRef)
		}
	}
	
	if len(backendRefs) > 0 {
		rule.BackendRefs = backendRefs
	}
}

// handleMatchActions processes actions within VirtualServer matches
func handleMatchActions(vs nginxv1.VirtualServer, route nginxv1.Route, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	for _, match := range route.Matches {
		if match.Action != nil {
			// Process match-specific actions and add to rule
			processMatchAction(vs, match, rule, notifs)
		}
	}
}

// processMatchAction processes a specific match action
func processMatchAction(vs nginxv1.VirtualServer, match nginxv1.Match, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	if match.Action == nil {
		return
	}

	// Create a temporary route to reuse existing action processing
	tempRoute := nginxv1.Route{
		Path:   "/", // placeholder
		Action: match.Action,
	}

	// Delegate to existing action handlers
	handleRouteActions(vs, tempRoute, rule, notifs)
}

// handleRouteActions processes different route action types
func handleRouteActions(vs nginxv1.VirtualServer, route nginxv1.Route, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	if route.Action == nil {
		return
	}

	// Handle advanced proxy actions (path rewriting, header manipulation)
	if route.Action.Proxy != nil {
		handleAdvancedProxyAction(vs, route, rule, notifs)
		return
	}

	// Handle basic actions
	if route.Action.Pass != "" {
		handlePassAction(vs, route, rule, notifs)
	} else if route.Action.Redirect != nil {
		handleRedirectAction(vs, route, rule, notifs)
	} else if route.Action.Return != nil {
		handleReturnAction(vs, route, rule, notifs)
	}
}

// handlePassAction handles proxy pass actions to upstreams
func handlePassAction(vs nginxv1.VirtualServer, route nginxv1.Route, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	upstream := findUpstream(vs.Spec.Upstreams, route.Action.Pass)
	if upstream != nil {
		rule.BackendRefs = []gatewayv1.HTTPBackendRef{
			{
				BackendRef: gatewayv1.BackendRef{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: gatewayv1.ObjectName(upstream.Service),
						Port: Ptr(gatewayv1.PortNumber(upstream.Port)),
					},
				},
			},
		}
	} else {
		addNotification(notifs, notifications.WarningNotification,
			fmt.Sprintf("Upstream '%s' not found for route", route.Action.Pass), &vs)
	}
}

// handleRedirectAction handles HTTP redirect actions
func handleRedirectAction(vs nginxv1.VirtualServer, route nginxv1.Route, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	redirectFilter := gatewayv1.HTTPRouteFilter{
		Type: gatewayv1.HTTPRouteFilterRequestRedirect,
		RequestRedirect: Ptr(gatewayv1.HTTPRequestRedirectFilter{
			StatusCode: Ptr(302), // Default redirect status
		}),
	}

	// Handle custom redirect URL
	if route.Action.Redirect.URL != "" {
		addNotification(notifs, notifications.InfoNotification,
			"URL redirect converted to Gateway API redirect filter", &vs)
	}

	// Handle custom status code
	if route.Action.Redirect.Code != 0 {
		redirectFilter.RequestRedirect.StatusCode = Ptr(route.Action.Redirect.Code)
	}

	rule.Filters = []gatewayv1.HTTPRouteFilter{redirectFilter}
}

// handleReturnAction handles direct return responses
func handleReturnAction(vs nginxv1.VirtualServer, route nginxv1.Route, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	addNotification(notifs, notifications.WarningNotification,
		fmt.Sprintf("Return action with code %d not directly supported in Gateway API", route.Action.Return.Code), &vs)
}

// handleTrafficSplits handles weighted traffic distribution
func handleTrafficSplits(vs nginxv1.VirtualServer, route nginxv1.Route, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	if len(route.Splits) == 0 {
		return
	}

	var backendRefs []gatewayv1.HTTPBackendRef
	for _, split := range route.Splits {
		upstream := findUpstream(vs.Spec.Upstreams, split.Action.Pass)
		if upstream != nil {
			backendRefs = append(backendRefs, gatewayv1.HTTPBackendRef{
				BackendRef: gatewayv1.BackendRef{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: gatewayv1.ObjectName(upstream.Service),
						Port: Ptr(gatewayv1.PortNumber(upstream.Port)),
					},
					Weight: Ptr(int32(split.Weight)),
				},
			})
		}
	}
	rule.BackendRefs = backendRefs

	addNotification(notifs, notifications.InfoNotification,
		"Traffic splitting configuration converted to weighted backend refs", &vs)
}

// createNginxHTTPRouteIR creates nginx-specific HTTPRoute IR
func createNginxHTTPRouteIR(vs nginxv1.VirtualServer, route nginxv1.Route, notifs *[]notifications.Notification) *intermediate.NginxHTTPRouteIR {
	nginxHTTPRouteIR := &intermediate.NginxHTTPRouteIR{}

	// Handle advanced proxy configuration
	if route.Action != nil && route.Action.Proxy != nil {
		storeAdvancedProxyConfig(route.Action.Proxy, route, nginxHTTPRouteIR)
	}

	// Handle nginx-specific features that don't map to Gateway API
	if route.Action != nil && route.Action.Pass != "" {
		upstream := findUpstream(vs.Spec.Upstreams, route.Action.Pass)
		if upstream != nil {
			if upstream.LBMethod != "" && upstream.LBMethod != "round_robin" {
				addNotification(notifs, notifications.InfoNotification,
					fmt.Sprintf("Load balancing method '%s' stored in provider-specific IR", upstream.LBMethod), &vs)
			}
		}
	}

	return nginxHTTPRouteIR
}