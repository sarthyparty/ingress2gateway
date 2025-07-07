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
	"testing"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestRewriteTargetFeature(t *testing.T) {
	testCases := []struct {
		name           string
		ingress        networkingv1.Ingress
		expectedFilter *gatewayv1.HTTPRouteFilter
	}{
		{
			name: "single service rewrite - simple format",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						"nginx.org/rewrites": "web-service=/api/v1",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/app",
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "web-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedFilter: &gatewayv1.HTTPRouteFilter{
				Type: gatewayv1.HTTPRouteFilterURLRewrite,
				URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
					Path: &gatewayv1.HTTPPathModifier{
						Type:               gatewayv1.PrefixMatchHTTPPathModifier,
						ReplacePrefixMatch: ptr.To("/api/v1"),
					},
				},
			},
		},
		{
			name: "single service rewrite - NIC format",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-nic",
					Namespace: "default",
					Annotations: map[string]string{
						"nginx.org/rewrites": "serviceName=coffee rewrite=/coffee",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "coffee.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/app",
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "coffee",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedFilter: &gatewayv1.HTTPRouteFilter{
				Type: gatewayv1.HTTPRouteFilterURLRewrite,
				URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
					Path: &gatewayv1.HTTPPathModifier{
						Type:               gatewayv1.PrefixMatchHTTPPathModifier,
						ReplacePrefixMatch: ptr.To("/coffee"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup IR with initial HTTPRoute
			ir := intermediate.IR{
				HTTPRoutes: make(map[types.NamespacedName]intermediate.HTTPRouteContext),
			}

			routeName := common.RouteName(tc.ingress.Name, tc.ingress.Spec.Rules[0].Host)
			routeKey := types.NamespacedName{Namespace: tc.ingress.Namespace, Name: routeName}
			
			httpRoute := gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeName,
					Namespace: tc.ingress.Namespace,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Matches: []gatewayv1.HTTPRouteMatch{
								{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
										Value: ptr.To("/app"),
									},
								},
							},
						},
					},
				},
			}

			ir.HTTPRoutes[routeKey] = intermediate.HTTPRouteContext{
				HTTPRoute: httpRoute,
			}

			// Execute feature parser
			errs := RewriteTargetFeature([]networkingv1.Ingress{tc.ingress}, nil, &ir)
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			// Verify filter was added
			updatedRoute := ir.HTTPRoutes[routeKey]
			if len(updatedRoute.HTTPRoute.Spec.Rules) == 0 || len(updatedRoute.HTTPRoute.Spec.Rules[0].Filters) == 0 {
				t.Fatal("Expected filter to be added to HTTPRoute")
			}

			filter := updatedRoute.HTTPRoute.Spec.Rules[0].Filters[0]
			if filter.Type != tc.expectedFilter.Type {
				t.Errorf("Expected filter type %v, got %v", tc.expectedFilter.Type, filter.Type)
			}

			if filter.URLRewrite == nil || filter.URLRewrite.Path == nil {
				t.Fatal("Expected URLRewrite filter with Path modifier")
			}

			if *filter.URLRewrite.Path.ReplacePrefixMatch != *tc.expectedFilter.URLRewrite.Path.ReplacePrefixMatch {
				t.Errorf("Expected rewrite path %v, got %v", 
					*tc.expectedFilter.URLRewrite.Path.ReplacePrefixMatch,
					*filter.URLRewrite.Path.ReplacePrefixMatch)
			}
		})
	}
}

func TestParseRewriteRules(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedRules  map[string]string
	}{
		{
			name:  "single rule",
			input: "web-service=/api/v1",
			expectedRules: map[string]string{
				"web-service": "/api/v1",
			},
		},
		{
			name:  "multiple rules",
			input: "web-service=/api/v1,api-service=/api/v2,auth-service=/auth",
			expectedRules: map[string]string{
				"web-service":  "/api/v1",
				"api-service":  "/api/v2", 
				"auth-service": "/auth",
			},
		},
		{
			name:  "rules with spaces",
			input: "web-service=/api/v1, api-service=/api/v2 , auth-service=/auth",
			expectedRules: map[string]string{
				"web-service":  "/api/v1",
				"api-service":  "/api/v2",
				"auth-service": "/auth",
			},
		},
		{
			name:          "empty input",
			input:         "",
			expectedRules: map[string]string{},
		},
		{
			name:          "invalid format",
			input:         "invalid-rule-without-equals",
			expectedRules: map[string]string{},
		},
		{
			name:  "NIC format single rule",
			input: "serviceName=coffee rewrite=/coffee",
			expectedRules: map[string]string{
				"coffee": "/coffee",
			},
		},
		{
			name:  "NIC format multiple rules",
			input: "serviceName=coffee rewrite=/coffee,serviceName=tea rewrite=/tea",
			expectedRules: map[string]string{
				"coffee": "/coffee",
				"tea":    "/tea",
			},
		},
		{
			name:  "mixed format - simple and NIC",
			input: "web-service=/api/v1,serviceName=coffee rewrite=/coffee",
			expectedRules: map[string]string{
				"web-service": "/api/v1",
				"coffee":      "/coffee",
			},
		},
		{
			name:  "NIC format with spaces",
			input: "serviceName=coffee rewrite=/coffee , serviceName=tea rewrite=/tea",
			expectedRules: map[string]string{
				"coffee": "/coffee",
				"tea":    "/tea",
			},
		},
		{
			name:  "NIC format with complex paths",
			input: "serviceName=api-service rewrite=/api/v2/users",
			expectedRules: map[string]string{
				"api-service": "/api/v2/users",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseRewriteRules(tc.input)
			
			if len(result) != len(tc.expectedRules) {
				t.Errorf("Expected %d rules, got %d", len(tc.expectedRules), len(result))
			}

			for expectedService, expectedPath := range tc.expectedRules {
				if actualPath, exists := result[expectedService]; !exists {
					t.Errorf("Expected service %s not found in result", expectedService)
				} else if actualPath != expectedPath {
					t.Errorf("Expected path %s for service %s, got %s", expectedPath, expectedService, actualPath)
				}
			}
		})
	}
}