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
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// PathRegexFeature converts nginx.org/path-regex annotation to regex path matching
func PathRegexFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	// Valid values for path-regex annotation
	var validPathRegexValues = map[string]struct{}{
		"true":             {},
		"case_sensitive":   {},
		"case_insensitive": {},
		"exact":            {},
	}

	for _, ingress := range ingresses {
		pathRegex, exists := ingress.Annotations[nginxPathRegexAnnotation]
		if !exists || pathRegex == "" {
			continue
		}

		if _, valid := validPathRegexValues[pathRegex]; !valid {
			continue
		}

		// Determine the appropriate path match type based on the annotation value
		var pathMatchType gatewayv1.PathMatchType
		if pathRegex == "exact" {
			pathMatchType = gatewayv1.PathMatchExact
		} else {
			// "true", "case_sensitive", "case_insensitive" all use regex
			pathMatchType = gatewayv1.PathMatchRegularExpression

			// Add warning for case_insensitive since Gateway API doesn't support it
			if pathRegex == "case_insensitive" {
				message := "nginx.org/path-regex: case_insensitive behavior cannot be guaranteed with Gateway API PathMatchRegularExpression - case sensitivity depends on Gateway implementation"
				notify(notifications.WarningNotification, message, &ingress)
			}
		}

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

			for _, rule := range httpRouteContext.HTTPRoute.Spec.Rules {
				for _, match := range rule.Matches {
					if match.Path != nil {
						match.Path.Type = ptr.To(pathMatchType)
					}
				}
			}

			ir.HTTPRoutes[routeKey] = httpRouteContext
		}
	}

	return errs
}
