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

func TestPathRegexFeature(t *testing.T) {
	testCases := []struct {
		name                string
		annotations         map[string]string
		expectedPathType    gatewayv1.PathMatchType
		shouldModifyMatches bool
	}{
		{
			name: "nginx.org/path-regex=true enables regex path matching",
			annotations: map[string]string{
				"nginx.org/path-regex": "true",
			},
			expectedPathType:    gatewayv1.PathMatchRegularExpression,
			shouldModifyMatches: true,
		},
		{
			name: "nginx.org/path-regex=case_sensitive enables regex path matching",
			annotations: map[string]string{
				"nginx.org/path-regex": "case_sensitive",
			},
			expectedPathType:    gatewayv1.PathMatchRegularExpression,
			shouldModifyMatches: true,
		},
		{
			name: "nginx.org/path-regex=case_insensitive enables regex path matching",
			annotations: map[string]string{
				"nginx.org/path-regex": "case_insensitive",
			},
			expectedPathType:    gatewayv1.PathMatchRegularExpression,
			shouldModifyMatches: true,
		},
		{
			name: "nginx.org/path-regex=exact enables regex path matching",
			annotations: map[string]string{
				"nginx.org/path-regex": "exact",
			},
			expectedPathType:    gatewayv1.PathMatchRegularExpression,
			shouldModifyMatches: true,
		},
		{
			name: "nginx.org/path-regex=false does not enable regex matching",
			annotations: map[string]string{
				"nginx.org/path-regex": "false",
			},
			expectedPathType:    gatewayv1.PathMatchPathPrefix,
			shouldModifyMatches: false,
		},
		{
			name: "missing nginx.org/path-regex annotation does not enable regex matching",
			annotations: map[string]string{
				"nginx.org/rewrites": "service=/api",
			},
			expectedPathType:    gatewayv1.PathMatchPathPrefix,
			shouldModifyMatches: false,
		},
		{
			name:                "no annotations does not enable regex matching",
			annotations:         map[string]string{},
			expectedPathType:    gatewayv1.PathMatchPathPrefix,
			shouldModifyMatches: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup Ingress
			ingress := networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/api/.*",
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "api-service",
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
			}

			// Setup IR with initial HTTPRoute
			ir := intermediate.IR{
				HTTPRoutes: make(map[types.NamespacedName]intermediate.HTTPRouteContext),
			}

			routeName := common.RouteName(ingress.Name, ingress.Spec.Rules[0].Host)
			routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}

			httpRoute := gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeName,
					Namespace: ingress.Namespace,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Matches: []gatewayv1.HTTPRouteMatch{
								{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
										Value: ptr.To("/api/.*"),
									},
								},
							},
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name:  gatewayv1.ObjectName("api-service"),
											Kind:  ptr.To(gatewayv1.Kind("Service")),
											Group: ptr.To(gatewayv1.Group("")),
											Port:  ptr.To(gatewayv1.PortNumber(80)),
										},
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

			// Execute pathRegexFeature
			errs := PathRegexFeature([]networkingv1.Ingress{ingress}, nil, &ir)
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			// Verify results
			updatedRoute := ir.HTTPRoutes[routeKey]
			if len(updatedRoute.HTTPRoute.Spec.Rules) == 0 || len(updatedRoute.HTTPRoute.Spec.Rules[0].Matches) == 0 {
				t.Fatal("Expected HTTPRoute to have rules and matches")
			}

			match := updatedRoute.HTTPRoute.Spec.Rules[0].Matches[0]
			if match.Path == nil {
				t.Fatal("Expected path match to exist")
			}

			actualPathType := *match.Path.Type
			if actualPathType != tc.expectedPathType {
				t.Errorf("Expected path type %v, got %v", tc.expectedPathType, actualPathType)
			}

			// Verify that the path value remains unchanged
			expectedPath := "/api/.*"
			if *match.Path.Value != expectedPath {
				t.Errorf("Expected path value %v, got %v", expectedPath, *match.Path.Value)
			}
		})
	}
}

func TestPathRegexFeatureMultipleMatches(t *testing.T) {
	// Test with multiple path matches to ensure all are converted
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-multi-paths",
			Namespace: "default",
			Annotations: map[string]string{
				"nginx.org/path-regex": "true",
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
									Path: "/api/v1/.*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-v1-service",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
								{
									Path: "/api/v2/.*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-v2-service",
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
	}

	// Setup IR with HTTPRoute containing multiple matches
	ir := intermediate.IR{
		HTTPRoutes: make(map[types.NamespacedName]intermediate.HTTPRouteContext),
	}

	routeName := common.RouteName(ingress.Name, ingress.Spec.Rules[0].Host)
	routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}

	httpRoute := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: ingress.Namespace,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
								Value: ptr.To("/api/v1/.*"),
							},
						},
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
								Value: ptr.To("/api/v2/.*"),
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

	// Execute pathRegexFeature
	errs := PathRegexFeature([]networkingv1.Ingress{ingress}, nil, &ir)
	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	// Verify all matches were converted to regex
	updatedRoute := ir.HTTPRoutes[routeKey]
	matches := updatedRoute.HTTPRoute.Spec.Rules[0].Matches

	if len(matches) != 2 {
		t.Fatalf("Expected 2 matches, got %d", len(matches))
	}

	for i, match := range matches {
		if match.Path == nil {
			t.Fatalf("Expected path match %d to exist", i)
		}

		if *match.Path.Type != gatewayv1.PathMatchRegularExpression {
			t.Errorf("Expected match %d to have RegularExpression type, got %v", i, *match.Path.Type)
		}
	}
}