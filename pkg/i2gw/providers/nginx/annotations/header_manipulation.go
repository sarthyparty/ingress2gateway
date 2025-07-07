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

package annotations

import (
	"strings"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// headerManipulationFeature converts header manipulation annotations to HTTPRoute filters
func HeaderManipulationFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		// Process proxy-hide-headers annotation
		if hideHeaders, exists := ingress.Annotations[nginxProxyHideHeadersAnnotation]; exists && hideHeaders != "" {
			filter := createResponseHeaderModifier(hideHeaders)
			if filter != nil {
				errs = append(errs, addFilterToIngressRoutes(ingress, *filter, ir)...)
			}
		}

		// Process proxy-set-headers annotation
		if setHeaders, exists := ingress.Annotations[nginxProxySetHeadersAnnotation]; exists && setHeaders != "" {
			filter := createRequestHeaderModifier(setHeaders)
			if filter != nil {
				errs = append(errs, addFilterToIngressRoutes(ingress, *filter, ir)...)
			}
		}
	}

	return errs
}

// addFilterToIngressRoutes adds a filter to all HTTPRoutes associated with an ingress
func addFilterToIngressRoutes(ingress networkingv1.Ingress, filter gatewayv1.HTTPRouteFilter, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}

		routeName := common.RouteName(ingress.Name, rule.Host)
		routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}

		httpRouteContext, exists := ir.HTTPRoutes[routeKey]
		if !exists {
			continue
		}

		if len(httpRouteContext.HTTPRoute.Spec.Rules) > 0 {
			if httpRouteContext.HTTPRoute.Spec.Rules[0].Filters == nil {
				httpRouteContext.HTTPRoute.Spec.Rules[0].Filters = []gatewayv1.HTTPRouteFilter{}
			}
			httpRouteContext.HTTPRoute.Spec.Rules[0].Filters = append(httpRouteContext.HTTPRoute.Spec.Rules[0].Filters, filter)
		}

		ir.HTTPRoutes[routeKey] = httpRouteContext
	}

	return errs
}

// createResponseHeaderModifier creates a ResponseHeaderModifier filter from comma-separated header names
func createResponseHeaderModifier(hideHeaders string) *gatewayv1.HTTPRouteFilter {
	headersToRemove := parseCommaSeparatedHeaders(hideHeaders)
	if len(headersToRemove) == 0 {
		return nil
	}

	return &gatewayv1.HTTPRouteFilter{
		Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
		ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
			Remove: headersToRemove,
		},
	}
}

// createRequestHeaderModifier creates a RequestHeaderModifier filter from proxy-set-headers annotation
func createRequestHeaderModifier(setHeaders string) *gatewayv1.HTTPRouteFilter {
	headers := parseSetHeaders(setHeaders)
	if len(headers) == 0 {
		return nil
	}

	var headersToSet []gatewayv1.HTTPHeader
	for name, value := range headers {
		if value != "" && !strings.Contains(value, "$") {
			headersToSet = append(headersToSet, gatewayv1.HTTPHeader{
				Name:  gatewayv1.HTTPHeaderName(name),
				Value: value,
			})
		}
		// Note: Headers with NGINX variables cannot be converted to Gateway API
		// as Gateway API doesn't support dynamic header values
	}

	if len(headersToSet) == 0 {
		return nil
	}

	return &gatewayv1.HTTPRouteFilter{
		Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
			Set: headersToSet,
		},
	}
}

// parseCommaSeparatedHeaders parses a comma-separated list of header names
func parseCommaSeparatedHeaders(headersList string) []string {
	if headersList == "" {
		return nil
	}

	var result []string
	headers := strings.Split(headersList, ",")
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header != "" {
			result = append(result, header)
		}
	}

	return result
}

// parseSetHeaders parses nginx.org/proxy-set-headers annotation format
// Supports both header names and header:value pairs
func parseSetHeaders(setHeaders string) map[string]string {
	headers := make(map[string]string)

	if setHeaders == "" {
		return headers
	}

	parts := strings.Split(setHeaders, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, ":") {
			// Format: "Header-Name: value"
			kv := strings.SplitN(part, ":", 2)
			if len(kv) == 2 {
				headerName := strings.TrimSpace(kv[0])
				headerValue := strings.TrimSpace(kv[1])
				if headerName != "" {
					headers[headerName] = headerValue
				}
			}
		} else {
			// Format: "Header-Name" (use default value pattern)
			headerName := strings.TrimSpace(part)
			if headerName != "" {
				// For Gateway API, we can't use NGINX variables like $http_*
				// Instead, we'll use a placeholder that indicates the header should pass through
				// Note: This is a limitation of Gateway API vs NGINX capabilities
				headers[headerName] = "" // Empty value means "pass through from client"
			}
		}
	}

	return headers
}
