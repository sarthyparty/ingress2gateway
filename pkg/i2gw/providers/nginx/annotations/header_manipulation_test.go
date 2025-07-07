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
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParseSetHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "single header name only",
			input: "X-Custom-Header",
			expected: map[string]string{
				"X-Custom-Header": "",
			},
		},
		{
			name:  "single header with value",
			input: "X-Custom-Header: custom-value",
			expected: map[string]string{
				"X-Custom-Header": "custom-value",
			},
		},
		{
			name:  "multiple headers names only",
			input: "X-Header1,X-Header2,X-Header3",
			expected: map[string]string{
				"X-Header1": "",
				"X-Header2": "",
				"X-Header3": "",
			},
		},
		{
			name:  "multiple headers with values",
			input: "X-Header1: value1,X-Header2: value2",
			expected: map[string]string{
				"X-Header1": "value1",
				"X-Header2": "value2",
			},
		},
		{
			name:  "mixed format",
			input: "X-Default-Header,X-Custom-Header: custom-value,X-Another-Header",
			expected: map[string]string{
				"X-Default-Header":  "",
				"X-Custom-Header":   "custom-value",
				"X-Another-Header":  "",
			},
		},
		{
			name:  "headers with spaces",
			input: " X-Header1 : value1 , X-Header2 : value2 ",
			expected: map[string]string{
				"X-Header1": "value1",
				"X-Header2": "value2",
			},
		},
		{
			name:  "complex header values",
			input: "X-Forwarded-For: $remote_addr,X-Real-IP: $remote_addr,X-Custom: hello-world",
			expected: map[string]string{
				"X-Forwarded-For": "$remote_addr",
				"X-Real-IP":       "$remote_addr",
				"X-Custom":        "hello-world",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSetHeaders(tt.input)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d headers, got %d", len(tt.expected), len(result))
			}
			
			for expectedName, expectedValue := range tt.expected {
				if actualValue, exists := result[expectedName]; !exists {
					t.Errorf("Expected header %s not found", expectedName)
				} else if actualValue != expectedValue {
					t.Errorf("Header %s: expected value %q, got %q", expectedName, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestHideHeaders(t *testing.T) {
	tests := []struct {
		name            string
		hideHeaders     string
		expectedHeaders []string
	}{
		{
			name:            "single header",
			hideHeaders:     "Server",
			expectedHeaders: []string{"Server"},
		},
		{
			name:            "multiple headers",
			hideHeaders:     "Server,X-Powered-By,X-Version",
			expectedHeaders: []string{"Server", "X-Powered-By", "X-Version"},
		},
		{
			name:            "headers with spaces",
			hideHeaders:     " Server , X-Powered-By , X-Version ",
			expectedHeaders: []string{"Server", "X-Powered-By", "X-Version"},
		},
		{
			name:            "empty headers filtered out",
			hideHeaders:     "Server,,X-Powered-By,",
			expectedHeaders: []string{"Server", "X-Powered-By"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						nginxProxyHideHeadersAnnotation: tt.hideHeaders,
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
				Gateways:   make(map[types.NamespacedName]intermediate.GatewayContext),
				HTTPRoutes: make(map[types.NamespacedName]intermediate.HTTPRouteContext),
			}

			routeName := common.RouteName(ingress.Name, ingress.Spec.Rules[0].Host)
			routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}
			ir.HTTPRoutes[routeKey] = intermediate.HTTPRouteContext{
				HTTPRoute: gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: ingress.Namespace,
					},
					Spec: gatewayv1.HTTPRouteSpec{
						Rules: []gatewayv1.HTTPRouteRule{
							{
								BackendRefs: []gatewayv1.HTTPBackendRef{
									{
										BackendRef: gatewayv1.BackendRef{
											BackendObjectReference: gatewayv1.BackendObjectReference{
												Name: gatewayv1.ObjectName("web-service"),
												Port: ptr.To(gatewayv1.PortNumber(80)),
											},
										},
									},
								},
							},
						},
					},
				},
			}

			filter := createResponseHeaderModifier(tt.hideHeaders)
			if filter == nil {
				t.Fatal("Expected filter to be created")
			}
			errs := addFilterToIngressRoutes(ingress, *filter, &ir)
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			updatedRoute := ir.HTTPRoutes[routeKey].HTTPRoute
			if len(updatedRoute.Spec.Rules) == 0 {
				t.Fatal("Expected at least one rule")
			}

			rule := updatedRoute.Spec.Rules[0]
			if len(rule.Filters) != 1 {
				t.Fatalf("Expected 1 filter, got %d", len(rule.Filters))
			}

			filter = &rule.Filters[0]
			if filter.Type != gatewayv1.HTTPRouteFilterResponseHeaderModifier {
				t.Fatalf("Expected ResponseHeaderModifier filter, got %s", filter.Type)
			}

			if filter.ResponseHeaderModifier == nil {
				t.Fatal("Expected ResponseHeaderModifier to be non-nil")
			}

			if len(filter.ResponseHeaderModifier.Remove) != len(tt.expectedHeaders) {
				t.Fatalf("Expected %d headers to remove, got %d", len(tt.expectedHeaders), len(filter.ResponseHeaderModifier.Remove))
			}

			for _, expectedHeader := range tt.expectedHeaders {
				found := false
				for _, actualHeader := range filter.ResponseHeaderModifier.Remove {
					if actualHeader == expectedHeader {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected header %s not found in remove list", expectedHeader)
				}
			}
		})
	}
}

func TestSetHeaders(t *testing.T) {
	tests := []struct {
		name            string
		setHeaders      string
		expectedHeaders []gatewayv1.HTTPHeader
	}{
		{
			name:       "single header with value",
			setHeaders: "X-Custom: hello-world",
			expectedHeaders: []gatewayv1.HTTPHeader{
				{Name: "X-Custom", Value: "hello-world"},
			},
		},
		{
			name:       "multiple headers with values",
			setHeaders: "X-Custom: hello-world,X-Version: 1.0.0",
			expectedHeaders: []gatewayv1.HTTPHeader{
				{Name: "X-Custom", Value: "hello-world"},
				{Name: "X-Version", Value: "1.0.0"},
			},
		},
		{
			name:            "nginx variables filtered out",
			setHeaders:      "X-Real-IP: $remote_addr,X-Custom: hello-world",
			expectedHeaders: []gatewayv1.HTTPHeader{
				{Name: "X-Custom", Value: "hello-world"},
			},
		},
		{
			name:            "empty values filtered out",
			setHeaders:      "X-Empty-Header,X-Custom: hello-world",
			expectedHeaders: []gatewayv1.HTTPHeader{
				{Name: "X-Custom", Value: "hello-world"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						nginxProxySetHeadersAnnotation: tt.setHeaders,
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
				Gateways:   make(map[types.NamespacedName]intermediate.GatewayContext),
				HTTPRoutes: make(map[types.NamespacedName]intermediate.HTTPRouteContext),
			}

			routeName := common.RouteName(ingress.Name, ingress.Spec.Rules[0].Host)
			routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}
			ir.HTTPRoutes[routeKey] = intermediate.HTTPRouteContext{
				HTTPRoute: gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: ingress.Namespace,
					},
					Spec: gatewayv1.HTTPRouteSpec{
						Rules: []gatewayv1.HTTPRouteRule{
							{
								BackendRefs: []gatewayv1.HTTPBackendRef{
									{
										BackendRef: gatewayv1.BackendRef{
											BackendObjectReference: gatewayv1.BackendObjectReference{
												Name: gatewayv1.ObjectName("web-service"),
												Port: ptr.To(gatewayv1.PortNumber(80)),
											},
										},
									},
								},
							},
						},
					},
				},
			}

			filter := createRequestHeaderModifier(tt.setHeaders)
			var errs field.ErrorList
			if filter != nil {
				errs = addFilterToIngressRoutes(ingress, *filter, &ir)
			}
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			updatedRoute := ir.HTTPRoutes[routeKey].HTTPRoute
			if len(updatedRoute.Spec.Rules) == 0 {
				t.Fatal("Expected at least one rule")
			}

			rule := updatedRoute.Spec.Rules[0]
			if len(tt.expectedHeaders) == 0 {
				if len(rule.Filters) > 0 {
					t.Fatalf("Expected no filters, got %d", len(rule.Filters))
				}
				return
			}

			if len(rule.Filters) != 1 {
				t.Fatalf("Expected 1 filter, got %d", len(rule.Filters))
			}

			filter = &rule.Filters[0]
			if filter.Type != gatewayv1.HTTPRouteFilterRequestHeaderModifier {
				t.Fatalf("Expected RequestHeaderModifier filter, got %s", filter.Type)
			}

			if filter.RequestHeaderModifier == nil {
				t.Fatal("Expected RequestHeaderModifier to be non-nil")
			}

			if len(filter.RequestHeaderModifier.Set) != len(tt.expectedHeaders) {
				t.Fatalf("Expected %d headers to set, got %d", len(tt.expectedHeaders), len(filter.RequestHeaderModifier.Set))
			}

			for _, expectedHeader := range tt.expectedHeaders {
				found := false
				for _, actualHeader := range filter.RequestHeaderModifier.Set {
					if actualHeader.Name == expectedHeader.Name && actualHeader.Value == expectedHeader.Value {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected header %s: %s not found in set list", expectedHeader.Name, expectedHeader.Value)
				}
			}
		})
	}
}

func TestHeaderManipulationFeature(t *testing.T) {
	tests := []struct {
		name                   string
		annotations            map[string]string
		expectedHideHeaders    []string
		expectedSetHeaders     []gatewayv1.HTTPHeader
	}{
		{
			name: "both hide and set headers",
			annotations: map[string]string{
				nginxProxyHideHeadersAnnotation: "Server,X-Powered-By",
				nginxProxySetHeadersAnnotation:  "X-Custom: hello-world",
			},
			expectedHideHeaders: []string{"Server", "X-Powered-By"},
			expectedSetHeaders: []gatewayv1.HTTPHeader{
				{Name: "X-Custom", Value: "hello-world"},
			},
		},
		{
			name: "only hide headers",
			annotations: map[string]string{
				nginxProxyHideHeadersAnnotation: "Server",
			},
			expectedHideHeaders: []string{"Server"},
			expectedSetHeaders:  []gatewayv1.HTTPHeader{},
		},
		{
			name: "only set headers",
			annotations: map[string]string{
				nginxProxySetHeadersAnnotation: "X-Custom: hello-world",
			},
			expectedHideHeaders: []string{},
			expectedSetHeaders: []gatewayv1.HTTPHeader{
				{Name: "X-Custom", Value: "hello-world"},
			},
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
				Gateways:   make(map[types.NamespacedName]intermediate.GatewayContext),
				HTTPRoutes: make(map[types.NamespacedName]intermediate.HTTPRouteContext),
			}

			routeName := common.RouteName(ingress.Name, ingress.Spec.Rules[0].Host)
			routeKey := types.NamespacedName{Namespace: ingress.Namespace, Name: routeName}
			ir.HTTPRoutes[routeKey] = intermediate.HTTPRouteContext{
				HTTPRoute: gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: ingress.Namespace,
					},
					Spec: gatewayv1.HTTPRouteSpec{
						Rules: []gatewayv1.HTTPRouteRule{
							{
								BackendRefs: []gatewayv1.HTTPBackendRef{
									{
										BackendRef: gatewayv1.BackendRef{
											BackendObjectReference: gatewayv1.BackendObjectReference{
												Name: gatewayv1.ObjectName("web-service"),
												Port: ptr.To(gatewayv1.PortNumber(80)),
											},
										},
									},
								},
							},
						},
					},
				},
			}

			errs := HeaderManipulationFeature([]networkingv1.Ingress{ingress}, nil, &ir)
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			updatedRoute := ir.HTTPRoutes[routeKey].HTTPRoute
			if len(updatedRoute.Spec.Rules) == 0 {
				t.Fatal("Expected at least one rule")
			}

			rule := updatedRoute.Spec.Rules[0]

			expectedFilterCount := 0
			if len(tt.expectedHideHeaders) > 0 {
				expectedFilterCount++
			}
			if len(tt.expectedSetHeaders) > 0 {
				expectedFilterCount++
			}

			if len(rule.Filters) != expectedFilterCount {
				t.Fatalf("Expected %d filters, got %d", expectedFilterCount, len(rule.Filters))
			}

			var responseHeaderFilter *gatewayv1.HTTPRouteFilter
			var requestHeaderFilter *gatewayv1.HTTPRouteFilter

			for i := range rule.Filters {
				filter := &rule.Filters[i]
				if filter.Type == gatewayv1.HTTPRouteFilterResponseHeaderModifier {
					responseHeaderFilter = filter
				} else if filter.Type == gatewayv1.HTTPRouteFilterRequestHeaderModifier {
					requestHeaderFilter = filter
				}
			}

			if len(tt.expectedHideHeaders) > 0 {
				if responseHeaderFilter == nil {
					t.Fatal("Expected ResponseHeaderModifier filter")
				}
				if len(responseHeaderFilter.ResponseHeaderModifier.Remove) != len(tt.expectedHideHeaders) {
					t.Fatalf("Expected %d headers to remove, got %d", len(tt.expectedHideHeaders), len(responseHeaderFilter.ResponseHeaderModifier.Remove))
				}
			}

			if len(tt.expectedSetHeaders) > 0 {
				if requestHeaderFilter == nil {
					t.Fatal("Expected RequestHeaderModifier filter")
				}
				if len(requestHeaderFilter.RequestHeaderModifier.Set) != len(tt.expectedSetHeaders) {
					t.Fatalf("Expected %d headers to set, got %d", len(tt.expectedSetHeaders), len(requestHeaderFilter.RequestHeaderModifier.Set))
				}
			}
		})
	}
}