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
	"strings"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// SSLRedirectFeature converts SSL redirect annotations to Gateway API filters,
// handling the distinction between conditional and unconditional redirects.
func SSLRedirectFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		modernRedirect, modernExists := ingress.Annotations[nginxRedirectToHTTPSAnnotation]
		legacyRedirect, legacyExists := ingress.Annotations[legacySSLRedirectAnnotation]

		var redirectType string
		if modernExists && modernRedirect == "true" {
			redirectType = "conditional"
		} else if legacyExists && legacyRedirect == "true" {
			redirectType = "unconditional"
		} else {
			continue
		}

		for _, rule := range ingress.Spec.Rules {
			ensureHTTPSListener(ingress, rule, ir)

			routeName := common.RouteName(ingress.Name, rule.Host)
			routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}
			httpRouteContext, routeExists := ir.HTTPRoutes[routeKey]
			if !routeExists {
				continue
			}

			switch redirectType {
			case "conditional":
				redirectRule := gatewayv1.HTTPRouteRule{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Headers: []gatewayv1.HTTPHeaderMatch{
								{
									Type:  ptr.To(gatewayv1.HeaderMatchExact),
									Name:  "X-Forwarded-Proto",
									Value: "http",
								},
							},
						},
					},
					Filters: []gatewayv1.HTTPRouteFilter{
						{
							Type: gatewayv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
								Scheme:     ptr.To("https"),
								StatusCode: ptr.To(301),
							},
						},
					},
				}
				httpRouteContext.HTTPRoute.Spec.Rules = append([]gatewayv1.HTTPRouteRule{redirectRule}, httpRouteContext.HTTPRoute.Spec.Rules...)

			case "unconditional":
				for i := range httpRouteContext.HTTPRoute.Spec.Rules {
					httpRouteContext.HTTPRoute.Spec.Rules[i].Filters = []gatewayv1.HTTPRouteFilter{
						{
							Type: gatewayv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
								Scheme:     ptr.To("https"),
								StatusCode: ptr.To(301),
							},
						},
					}
					httpRouteContext.HTTPRoute.Spec.Rules[i].BackendRefs = nil
				}
			}

			ir.HTTPRoutes[routeKey] = httpRouteContext
		}
	}

	return errs
}

// ensureHTTPSListener ensures that a Gateway resource has an HTTPS listener configured
// for the specified Ingress rule. If it doesn't, one is created.
func ensureHTTPSListener(ingress networkingv1.Ingress, rule networkingv1.IngressRule, ir *intermediate.IR) {
	gatewayName := ingress.Spec.IngressClassName
	if gatewayName == nil {
		gatewayName = ptr.To(ingress.Name)
	}
	gatewayKey := types.NamespacedName{Namespace: ingress.Namespace, Name: *gatewayName}
	gatewayContext, exists := ir.Gateways[gatewayKey]
	if !exists {
		return
	}

	hostname := gatewayv1.Hostname(rule.Host)
	for _, listener := range gatewayContext.Gateway.Spec.Listeners {
		if listener.Protocol == gatewayv1.HTTPSProtocolType && (listener.Hostname == nil || *listener.Hostname == hostname) {
			return
		}
	}

	httpsListener := gatewayv1.Listener{
		Name:     gatewayv1.SectionName(fmt.Sprintf("https-%s", strings.ReplaceAll(rule.Host, ".", "-"))),
		Protocol: gatewayv1.HTTPSProtocolType,
		Port:     443,
		Hostname: &hostname,
		TLS: &gatewayv1.GatewayTLSConfig{
			Mode: ptr.To(gatewayv1.TLSModeTerminate),
			CertificateRefs: []gatewayv1.SecretObjectReference{
				{Name: gatewayv1.ObjectName(fmt.Sprintf("%s-tls", strings.ReplaceAll(rule.Host, ".", "-")))},
			},
		},
	}
	gatewayContext.Gateway.Spec.Listeners = append(gatewayContext.Gateway.Spec.Listeners, httpsListener)
	ir.Gateways[gatewayKey] = gatewayContext
}