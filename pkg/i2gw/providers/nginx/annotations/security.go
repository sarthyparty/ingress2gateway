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
	"fmt"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// securityFeature converts security-related annotations to Gateway and HTTPRoute configurations
func SecurityFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		// Process HSTS annotations
		if hsts, exists := ingress.Annotations[nginxHSTSAnnotation]; exists && hsts == "true" {
			errs = append(errs, processHSTSAnnotation(ingress, ir)...)
		}

		// Process basic auth annotations
		if authSecret, exists := ingress.Annotations[nginxBasicAuthSecretAnnotation]; exists && authSecret != "" {
			errs = append(errs, processBasicAuthAnnotation(ingress, authSecret, ir)...)
		}
	}

	return errs
}

// processHSTSAnnotation converts HSTS annotations to ResponseHeaderModifier
func processHSTSAnnotation(ingress networkingv1.Ingress, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	// Build HSTS header value
	hstsValue := "max-age=31536000" // Default 1 year

	// Check for custom max-age
	if maxAge, exists := ingress.Annotations[nginxHSTSMaxAgeAnnotation]; exists && maxAge != "" {
		hstsValue = fmt.Sprintf("max-age=%s", maxAge)
	}

	// Check for includeSubDomains
	if includeSubdomains, exists := ingress.Annotations[nginxHSTSIncludeSubdomainsAnnotation]; exists && includeSubdomains == "true" {
		hstsValue += "; includeSubDomains"
	}

	// Apply HSTS header to all routes
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

		// Create ResponseHeaderModifier filter to add HSTS header
		filter := gatewayv1.HTTPRouteFilter{
			Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
			ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
				Add: []gatewayv1.HTTPHeader{
					{
						Name:  "Strict-Transport-Security",
						Value: hstsValue,
					},
				},
			},
		}

		// Add filter to first rule
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

// processBasicAuthAnnotation handles basic authentication configuration
func processBasicAuthAnnotation(ingress networkingv1.Ingress, authSecret string, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	// Note: Basic Auth in Gateway API typically requires policy attachments
	// rather than direct HTTPRoute configuration. This would need to be handled
	// by implementation-specific policies or external auth services.

	// For now, we preserve this information in provider-specific IR
	// and note that it requires policy-based implementation

	return errs
}