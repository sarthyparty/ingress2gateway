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
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// pathRegexFeature converts nginx.org/path-regex annotation to regex path matching
func PathRegexFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		pathRegex, exists := ingress.Annotations[nginxPathRegexAnnotation]
		if !exists || pathRegex == "" {
			continue
		}
		
		validValues := map[string]bool{
			"true":            true,
			"case_sensitive":  true,
			"case_insensitive": true,
			"exact":           true,
		}
		
		if !validValues[pathRegex] {
			continue
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

			for i := range httpRouteContext.HTTPRoute.Spec.Rules {
				for j := range httpRouteContext.HTTPRoute.Spec.Rules[i].Matches {
					match := &httpRouteContext.HTTPRoute.Spec.Rules[i].Matches[j]
					if match.Path != nil {
						match.Path.Type = ptr.To(gatewayv1.PathMatchRegularExpression)
					}
				}
			}

			ir.HTTPRoutes[routeKey] = httpRouteContext
		}
	}

	return errs
}