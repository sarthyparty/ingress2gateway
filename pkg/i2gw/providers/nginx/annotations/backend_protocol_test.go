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
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

func TestSSLServicesAnnotation(t *testing.T) {
	tests := []struct {
		name             string
		annotation       string
		expectedPolicies int
		expectedServices []string
	}{
		{
			name:             "single service",
			annotation:       "secure-api",
			expectedPolicies: 1,
			expectedServices: []string{"secure-api"},
		},
		{
			name:             "multiple services",
			annotation:       "secure-api,auth-service",
			expectedPolicies: 2,
			expectedServices: []string{"secure-api", "auth-service"},
		},
		{
			name:             "spaces in annotation",
			annotation:       " secure-api , auth-service , payment-api ",
			expectedPolicies: 3,
			expectedServices: []string{"secure-api", "auth-service", "payment-api"},
		},
		{
			name:             "empty annotation",
			annotation:       "",
			expectedPolicies: 0,
			expectedServices: []string{},
		},
		{
			name:             "empty values in annotation",
			annotation:       "secure-api,,auth-service,",
			expectedPolicies: 2,
			expectedServices: []string{"secure-api", "auth-service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						nginxSSLServicesAnnotation: tt.annotation,
					},
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: ptr.To("nginx"),
					Rules: []networkingv1.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/",
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
			}

			ir := intermediate.IR{
				BackendTLSPolicies: make(map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy),
			}

			errs := processSSLServicesAnnotation(ingress, tt.annotation, &ir)
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			if len(ir.BackendTLSPolicies) != tt.expectedPolicies {
				t.Errorf("Expected %d BackendTLSPolicies, got %d", tt.expectedPolicies, len(ir.BackendTLSPolicies))
			}

			serviceNames := make(map[string]struct{})
			for _, policy := range ir.BackendTLSPolicies {
				if len(policy.Spec.TargetRefs) > 0 {
					serviceName := string(policy.Spec.TargetRefs[0].Name)
					serviceNames[serviceName] = struct{}{}

					if policy.Spec.TargetRefs[0].Kind != "Service" {
						t.Errorf("Expected TargetRef Kind 'Service', got '%s'", policy.Spec.TargetRefs[0].Kind)
					}
					if policy.Spec.TargetRefs[0].Group != gatewayv1.GroupName {
						t.Errorf("Expected TargetRef Group '%s', got '%s'", gatewayv1.GroupName, policy.Spec.TargetRefs[0].Group)
					}

					if policy.Labels["app.kubernetes.io/managed-by"] != "ingress2gateway" {
						t.Errorf("Expected managed-by label 'ingress2gateway', got '%s'", policy.Labels["app.kubernetes.io/managed-by"])
					}
					if policy.Labels["ingress2gateway.io/source"] != "nginx-ssl-services" {
						t.Errorf("Expected source label 'nginx-ssl-services', got '%s'", policy.Labels["ingress2gateway.io/source"])
					}
				}
			}

			// Verify all expected services are present
			for _, expectedService := range tt.expectedServices {
				if _, exists := serviceNames[expectedService]; !exists {
					t.Errorf("Expected BackendTLSPolicy for service '%s' not found", expectedService)
				}
			}
		})
	}
}

func TestBackendProtocolFeature(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    int
	}{
		{
			name: "ssl services",
			annotations: map[string]string{
				nginxSSLServicesAnnotation: "secure-api,auth-service",
			},
			expected: 2,
		},
		{
			name: "no backend annotations",
			annotations: map[string]string{
				"nginx.org/rewrite-target": "/",
			},
			expected: 0,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expected:    0,
		},
		{
			name: "grpc services",
			annotations: map[string]string{
				nginxGRPCServicesAnnotation: "grpc-service",
			},
			expected: 0, // No BackendTLSPolicies expected for gRPC
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: ptr.To("nginx"),
					Rules: []networkingv1.IngressRule{
						{
							Host: "example.com",
						},
					},
				},
			}

			// Setup IR
			ir := intermediate.IR{
				BackendTLSPolicies: make(map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy),
			}

			// Execute
			errs := BackendProtocolFeature([]networkingv1.Ingress{ingress}, nil, &ir)
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			if len(ir.BackendTLSPolicies) != tt.expected {
				t.Errorf("Expected %d BackendTLSPolicies, got %d", tt.expected, len(ir.BackendTLSPolicies))
			}
		})
	}
}

func TestGRPCServicesRemoveHTTPRoute(t *testing.T) {
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grpc-ingress",
			Namespace: "default",
			Annotations: map[string]string{
				nginxGRPCServicesAnnotation: "grpc-service",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ptr.To("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: "grpc.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/grpc.service/Method",
									PathType: ptr.To(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "grpc-service",
											Port: networkingv1.ServiceBackendPort{Number: 50051},
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

	// Setup IR with an existing HTTPRoute that should be removed
	routeName := common.RouteName(ingress.Name, ingress.Spec.Rules[0].Host)
	routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}

	ir := intermediate.IR{
		HTTPRoutes: map[types.NamespacedName]intermediate.HTTPRouteContext{
			routeKey: {
				HTTPRoute: gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: ingress.Namespace,
					},
				},
			},
		},
		GRPCRoutes:         make(map[types.NamespacedName]gatewayv1.GRPCRoute),
		BackendTLSPolicies: make(map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy),
	}

	// Verify HTTPRoute exists before
	if _, exists := ir.HTTPRoutes[routeKey]; !exists {
		t.Fatal("HTTPRoute should exist before calling BackendProtocolFeature")
	}

	// Execute
	errs := BackendProtocolFeature([]networkingv1.Ingress{ingress}, nil, &ir)
	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	// Debug output
	t.Logf("HTTPRoutes after: %d", len(ir.HTTPRoutes))
	t.Logf("GRPCRoutes after: %d", len(ir.GRPCRoutes))
	t.Logf("Expected routeKey: %s", routeKey)
	for k := range ir.HTTPRoutes {
		t.Logf("HTTPRoute key: %s", k)
	}
	for k := range ir.GRPCRoutes {
		t.Logf("GRPCRoute key: %s", k)
	}

	// Verify HTTPRoute was removed
	if _, exists := ir.HTTPRoutes[routeKey]; exists {
		t.Error("HTTPRoute should be removed for gRPC services")
	}

	// Verify GRPCRoute was created
	if _, exists := ir.GRPCRoutes[routeKey]; !exists {
		t.Error("GRPCRoute should be created for gRPC services")
		return // Don't continue testing structure if route doesn't exist
	}

	// Verify GRPCRoute structure
	grpcRoute := ir.GRPCRoutes[routeKey]
	if len(grpcRoute.Spec.Rules) == 0 {
		t.Error("GRPCRoute should have rules")
		return
	}

	if len(grpcRoute.Spec.Rules[0].BackendRefs) == 0 {
		t.Error("GRPCRoute should have backend refs")
		return
	}

	backendRef := grpcRoute.Spec.Rules[0].BackendRefs[0]
	if string(backendRef.BackendRef.BackendObjectReference.Name) != "grpc-service" {
		t.Errorf("Expected backend service 'grpc-service', got '%s'", backendRef.BackendRef.BackendObjectReference.Name)
	}
}

func TestGRPCServicesWithMixedServices(t *testing.T) {
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mixed-ingress",
			Namespace: "default",
			Annotations: map[string]string{
				nginxGRPCServicesAnnotation: "grpc-service",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ptr.To("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: "mixed.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/grpc.service/Method",
									PathType: ptr.To(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "grpc-service",
											Port: networkingv1.ServiceBackendPort{Number: 50051},
										},
									},
								},
								{
									Path:     "/api/v1",
									PathType: ptr.To(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "http-service",
											Port: networkingv1.ServiceBackendPort{Number: 8080},
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

	// Setup IR with existing HTTPRoute containing filters
	routeName := common.RouteName(ingress.Name, ingress.Spec.Rules[0].Host)
	routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}

	httpRouteRules := []gatewayv1.HTTPRouteRule{
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
						Value: ptr.To("/grpc.service/Method"),
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: "grpc-service",
							Port: ptr.To(gatewayv1.PortNumber(50051)),
						},
					},
				},
			},
			Filters: []gatewayv1.HTTPRouteFilter{
				{
					Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
						Set: []gatewayv1.HTTPHeader{
							{Name: "X-Custom-Header", Value: "test-value"},
						},
					},
				},
				{
					Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
					ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
						Remove: []string{"Server"},
					},
				},
			},
		},
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
						Value: ptr.To("/api/v1"),
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: "http-service",
							Port: ptr.To(gatewayv1.PortNumber(8080)),
						},
					},
				},
			},
			Filters: []gatewayv1.HTTPRouteFilter{
				{
					Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
						Add: []gatewayv1.HTTPHeader{
							{Name: "X-API-Version", Value: "v1"},
						},
					},
				},
			},
		},
	}

	ir := intermediate.IR{
		HTTPRoutes: map[types.NamespacedName]intermediate.HTTPRouteContext{
			routeKey: {
				HTTPRoute: gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: ingress.Namespace,
					},
					Spec: gatewayv1.HTTPRouteSpec{
						Rules: httpRouteRules,
					},
				},
			},
		},
		GRPCRoutes: make(map[types.NamespacedName]gatewayv1.GRPCRoute),
	}

	// Execute
	errs := BackendProtocolFeature([]networkingv1.Ingress{ingress}, nil, &ir)
	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	// Verify HTTPRoute still exists (but modified)
	httpRouteContext, httpExists := ir.HTTPRoutes[routeKey]
	if !httpExists {
		t.Error("HTTPRoute should still exist for mixed services")
		return
	}

	// Verify HTTPRoute only has non-gRPC rules
	if len(httpRouteContext.HTTPRoute.Spec.Rules) != 1 {
		t.Errorf("Expected 1 remaining HTTPRoute rule, got %d", len(httpRouteContext.HTTPRoute.Spec.Rules))
		return
	}

	remainingRule := httpRouteContext.HTTPRoute.Spec.Rules[0]
	if len(remainingRule.BackendRefs) != 1 {
		t.Errorf("Expected 1 backend ref in remaining rule, got %d", len(remainingRule.BackendRefs))
		return
	}

	if string(remainingRule.BackendRefs[0].BackendRef.BackendObjectReference.Name) != "http-service" {
		t.Errorf("Expected remaining backend to be 'http-service', got '%s'",
			remainingRule.BackendRefs[0].BackendRef.BackendObjectReference.Name)
	}

	// Verify GRPCRoute was created
	grpcRoute, grpcExists := ir.GRPCRoutes[routeKey]
	if !grpcExists {
		t.Error("GRPCRoute should be created for gRPC services")
		return
	}

	// Verify GRPCRoute structure
	if len(grpcRoute.Spec.Rules) != 1 {
		t.Errorf("Expected 1 GRPCRoute rule, got %d", len(grpcRoute.Spec.Rules))
		return
	}

	grpcRule := grpcRoute.Spec.Rules[0]
	if len(grpcRule.BackendRefs) != 1 {
		t.Errorf("Expected 1 gRPC backend ref, got %d", len(grpcRule.BackendRefs))
		return
	}

	if string(grpcRule.BackendRefs[0].BackendRef.BackendObjectReference.Name) != "grpc-service" {
		t.Errorf("Expected gRPC backend to be 'grpc-service', got '%s'",
			grpcRule.BackendRefs[0].BackendRef.BackendObjectReference.Name)
	}

	// Verify filters were copied to GRPCRoute
	if len(grpcRule.Filters) != 2 {
		t.Errorf("Expected 2 filters in GRPCRoute rule, got %d", len(grpcRule.Filters))
		return
	}

	// Check RequestHeaderModifier filter
	var hasRequestFilter, hasResponseFilter bool
	for _, filter := range grpcRule.Filters {
		if filter.Type == gatewayv1.GRPCRouteFilterRequestHeaderModifier {
			hasRequestFilter = true
			if filter.RequestHeaderModifier == nil {
				t.Error("RequestHeaderModifier should not be nil")
			} else if len(filter.RequestHeaderModifier.Set) != 1 ||
				string(filter.RequestHeaderModifier.Set[0].Name) != "X-Custom-Header" ||
				filter.RequestHeaderModifier.Set[0].Value != "test-value" {
				t.Error("RequestHeaderModifier not correctly copied")
			}
		}
		if filter.Type == gatewayv1.GRPCRouteFilterResponseHeaderModifier {
			hasResponseFilter = true
			if filter.ResponseHeaderModifier == nil {
				t.Error("ResponseHeaderModifier should not be nil")
			} else if len(filter.ResponseHeaderModifier.Remove) != 1 ||
				filter.ResponseHeaderModifier.Remove[0] != "Server" {
				t.Error("ResponseHeaderModifier not correctly copied")
			}
		}
	}

	if !hasRequestFilter {
		t.Error("GRPCRoute should have RequestHeaderModifier filter")
	}
	if !hasResponseFilter {
		t.Error("GRPCRoute should have ResponseHeaderModifier filter")
	}
}
