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
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RewriteTargetFeature converts nginx.org/rewrites annotation to URLRewrite filter
func RewriteTargetFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		rewriteValue, exists := ingress.Annotations[nginxRewritesAnnotation]
		if !exists || rewriteValue == "" {
			continue
		}

		rewriteRules := parseRewriteRules(rewriteValue)
		if len(rewriteRules) == 0 {
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
				for _, path := range rule.HTTP.Paths {
					serviceName := path.Backend.Service.Name
					if rewritePath, hasRewrite := rewriteRules[serviceName]; hasRewrite {
						filter := gatewayv1.HTTPRouteFilter{
							Type: gatewayv1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
								Path: &gatewayv1.HTTPPathModifier{
									Type:               gatewayv1.PrefixMatchHTTPPathModifier,
									ReplacePrefixMatch: ptr.To(rewritePath),
								},
							},
						}

						if httpRouteContext.HTTPRoute.Spec.Rules[i].Filters == nil {
							httpRouteContext.HTTPRoute.Spec.Rules[i].Filters = []gatewayv1.HTTPRouteFilter{}
						}
						httpRouteContext.HTTPRoute.Spec.Rules[i].Filters = append(httpRouteContext.HTTPRoute.Spec.Rules[i].Filters, filter)
					}
				}
			}

			ir.HTTPRoutes[routeKey] = httpRouteContext
		}
	}

	return errs
}

// parseRewriteRules parses nginx.org/rewrites annotation format
// Supports both formats:
// 1. Simple format: "serviceName=rewritePath[,serviceName2=rewritePath2]"
// 2. NIC format: "serviceName=service rewrite=path[,serviceName2=service2 rewrite=path2]"
func parseRewriteRules(rewriteValue string) map[string]string {
	rules := make(map[string]string)

	if rewriteValue == "" {
		return rules
	}

	parts := strings.Split(rewriteValue, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, " rewrite=") {
			mainParts := strings.SplitN(part, "=", 2)
			if len(mainParts) != 2 {
				continue
			}

			remaining := strings.TrimSpace(mainParts[1])

			rewriteIndex := strings.Index(remaining, " rewrite=")
			if rewriteIndex == -1 {
				continue
			}

			serviceName := strings.TrimSpace(remaining[:rewriteIndex])
			rewritePath := strings.TrimSpace(remaining[rewriteIndex+9:])

			if serviceName != "" && rewritePath != "" {
				rules[serviceName] = rewritePath
			}
		} else {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}

			serviceName := strings.TrimSpace(kv[0])
			rewritePath := strings.TrimSpace(kv[1])

			if serviceName != "" && rewritePath != "" {
				rules[serviceName] = rewritePath
			}
		}
	}

	return rules
}
