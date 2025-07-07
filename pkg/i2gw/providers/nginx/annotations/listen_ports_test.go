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
	"reflect"
	"testing"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestExtractListenPorts(t *testing.T) {
	testCases := []struct {
		name        string
		annotation  string
		expected    []int32
	}{
		{
			name:        "empty annotation",
			annotation:  "",
			expected:    nil,
		},
		{
			name:        "single port",
			annotation:  "8080",
			expected:    []int32{8080},
		},
		{
			name:        "multiple ports",
			annotation:  "8080,9090,3000",
			expected:    []int32{8080, 9090, 3000},
		},
		{
			name:        "ports with spaces",
			annotation:  " 8080 , 9090 , 3000 ",
			expected:    []int32{8080, 9090, 3000},
		},
		{
			name:        "invalid ports filtered out",
			annotation:  "8080,invalid,9090,0,65536",
			expected:    []int32{8080, 9090},
		},
		{
			name:        "empty parts filtered out",
			annotation:  "8080,,9090,",
			expected:    []int32{8080, 9090},
		},
		{
			name:        "edge case - port 1 and 65535",
			annotation:  "1,65535",
			expected:    []int32{1, 65535},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractListenPorts(tc.annotation)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestCreateListenerName(t *testing.T) {
	testCases := []struct {
		name        string
		hostname    string
		port        int32
		protocol    gatewayv1.ProtocolType
		expected    string
	}{
		{
			name:        "HTTP listener",
			hostname:    "example.com",
			port:        8080,
			protocol:    gatewayv1.HTTPProtocolType,
			expected:    "example-com-http-8080",
		},
		{
			name:        "HTTPS listener",
			hostname:    "api.example.com",
			port:        8443,
			protocol:    gatewayv1.HTTPSProtocolType,
			expected:    "api-example-com-https-8443",
		},
		{
			name:        "empty hostname",
			hostname:    "",
			port:        9090,
			protocol:    gatewayv1.HTTPProtocolType,
			expected:    "all-hosts-http-9090",
		},
		{
			name:        "wildcard hostname",
			hostname:    "*",
			port:        8080,
			protocol:    gatewayv1.HTTPProtocolType,
			expected:    "all-hosts-http-8080",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := createListenerName(tc.hostname, tc.port, tc.protocol)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestCreateListener(t *testing.T) {
	testCases := []struct {
		name        string
		hostname    string
		port        int32
		protocol    gatewayv1.ProtocolType
		expected    gatewayv1.Listener
	}{
		{
			name:        "HTTP listener with hostname",
			hostname:    "example.com",
			port:        8080,
			protocol:    gatewayv1.HTTPProtocolType,
			expected: gatewayv1.Listener{
				Name:     "example-com-http-8080",
				Port:     8080,
				Protocol: gatewayv1.HTTPProtocolType,
				Hostname: (*gatewayv1.Hostname)(ptr.To("example.com")),
			},
		},
		{
			name:        "HTTPS listener with hostname",
			hostname:    "secure.example.com",
			port:        8443,
			protocol:    gatewayv1.HTTPSProtocolType,
			expected: gatewayv1.Listener{
				Name:     "secure-example-com-https-8443",
				Port:     8443,
				Protocol: gatewayv1.HTTPSProtocolType,
				Hostname: (*gatewayv1.Hostname)(ptr.To("secure.example.com")),
			},
		},
		{
			name:        "listener without hostname",
			hostname:    "",
			port:        9090,
			protocol:    gatewayv1.HTTPProtocolType,
			expected: gatewayv1.Listener{
				Name:     "all-hosts-http-9090",
				Port:     9090,
				Protocol: gatewayv1.HTTPProtocolType,
				Hostname: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := createListener(tc.hostname, tc.port, tc.protocol)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %+v, got %+v", tc.expected, result)
			}
		})
	}
}

func TestListenPortsFeature(t *testing.T) {
	testCases := []struct {
		name              string
		annotations       map[string]string
		expectedListeners int
		expectedHTTPPorts []int32
		expectedSSLPorts  []int32
	}{
		{
			name:              "no custom ports - should not modify gateway",
			annotations:       map[string]string{},
			expectedListeners: 0,
			expectedHTTPPorts: nil,
			expectedSSLPorts:  nil,
		},
		{
			name: "custom HTTP ports only - replaces default HTTP, no HTTPS",
			annotations: map[string]string{
				nginxListenPortsAnnotation: "8080,9090",
			},
			expectedListeners: 2,
			expectedHTTPPorts: []int32{8080, 9090},
			expectedSSLPorts:  nil, // No HTTPS listeners when only HTTP annotation present
		},
		{
			name: "custom SSL ports only - replaces default HTTPS, no HTTP",
			annotations: map[string]string{
				nginxListenPortsSSLAnnotation: "8443,9443",
			},
			expectedListeners: 2,
			expectedHTTPPorts: nil, // No HTTP listeners when only SSL annotation present
			expectedSSLPorts:  []int32{8443, 9443},
		},
		{
			name: "both HTTP and SSL custom ports - replaces both defaults",
			annotations: map[string]string{
				nginxListenPortsAnnotation:    "8080,9090",
				nginxListenPortsSSLAnnotation: "8443,9443",
			},
			expectedListeners: 4,
			expectedHTTPPorts: []int32{8080, 9090},
			expectedSSLPorts:  []int32{8443, 9443},
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

			// Setup IR
			ir := intermediate.IR{
				Gateways:   make(map[types.NamespacedName]intermediate.GatewayContext),
				HTTPRoutes: make(map[types.NamespacedName]intermediate.HTTPRouteContext),
			}

			// Execute feature parser
			errs := ListenPortsFeature([]networkingv1.Ingress{ingress}, nil, &ir)
			if len(errs) > 0 {
				t.Fatalf("Unexpected errors: %v", errs)
			}

			// Verify results
			if tc.expectedListeners == 0 {
				// Should not create any gateway if no custom ports
				if len(ir.Gateways) > 0 {
					t.Error("Expected no gateways to be created")
				}
				return
			}

			// Should create exactly one gateway
			if len(ir.Gateways) != 1 {
				t.Fatalf("Expected 1 gateway, got %d", len(ir.Gateways))
			}

			// Get the created gateway
			var gateway gatewayv1.Gateway
			for _, gwContext := range ir.Gateways {
				gateway = gwContext.Gateway
				break
			}

			// Verify listener count
			if len(gateway.Spec.Listeners) != tc.expectedListeners {
				t.Fatalf("Expected %d listeners, got %d", tc.expectedListeners, len(gateway.Spec.Listeners))
			}

			// Verify HTTP ports
			httpCount := 0
			for _, listener := range gateway.Spec.Listeners {
				if listener.Protocol == gatewayv1.HTTPProtocolType {
					httpCount++
					found := false
					for _, expectedPort := range tc.expectedHTTPPorts {
						if int32(listener.Port) == expectedPort {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Unexpected HTTP port %d", listener.Port)
					}
				}
			}

			if httpCount != len(tc.expectedHTTPPorts) {
				t.Errorf("Expected %d HTTP listeners, got %d", len(tc.expectedHTTPPorts), httpCount)
			}

			// Verify SSL ports
			sslCount := 0
			for _, listener := range gateway.Spec.Listeners {
				if listener.Protocol == gatewayv1.HTTPSProtocolType {
					sslCount++
					found := false
					for _, expectedPort := range tc.expectedSSLPorts {
						if int32(listener.Port) == expectedPort {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Unexpected HTTPS port %d", listener.Port)
					}
				}
			}

			if sslCount != len(tc.expectedSSLPorts) {
				t.Errorf("Expected %d HTTPS listeners, got %d", len(tc.expectedSSLPorts), sslCount)
			}

			// Verify all listeners have correct hostname
			for _, listener := range gateway.Spec.Listeners {
				if listener.Hostname == nil || string(*listener.Hostname) != "example.com" {
					t.Errorf("Expected hostname 'example.com', got %v", listener.Hostname)
				}
			}
		})
	}
}

func TestDeterminePortsToUse(t *testing.T) {
	testCases := []struct {
		name                 string
		customHTTPPorts      []int32
		customSSLPorts       []int32
		hasHTTPAnnotation    bool
		hasSSLAnnotation     bool
		expectedHTTPPorts    []int32
		expectedHTTPSPorts   []int32
	}{
		{
			name:                 "no annotations - use defaults",
			customHTTPPorts:      nil,
			customSSLPorts:       nil,
			hasHTTPAnnotation:    false,
			hasSSLAnnotation:     false,
			expectedHTTPPorts:    []int32{80},
			expectedHTTPSPorts:   []int32{443},
		},
		{
			name:                 "HTTP annotation only - replace default HTTP, no HTTPS",
			customHTTPPorts:      []int32{8080, 9090},
			customSSLPorts:       nil,
			hasHTTPAnnotation:    true,
			hasSSLAnnotation:     false,
			expectedHTTPPorts:    []int32{8080, 9090},
			expectedHTTPSPorts:   nil,
		},
		{
			name:                 "SSL annotation only - replace default HTTPS, no HTTP",
			customHTTPPorts:      nil,
			customSSLPorts:       []int32{8443, 9443},
			hasHTTPAnnotation:    false,
			hasSSLAnnotation:     true,
			expectedHTTPPorts:    nil,
			expectedHTTPSPorts:   []int32{8443, 9443},
		},
		{
			name:                 "both annotations - replace both defaults",
			customHTTPPorts:      []int32{8080},
			customSSLPorts:       []int32{8443},
			hasHTTPAnnotation:    true,
			hasSSLAnnotation:     true,
			expectedHTTPPorts:    []int32{8080},
			expectedHTTPSPorts:   []int32{8443},
		},
		{
			name:                 "empty HTTP ports with annotation - no HTTP listeners",
			customHTTPPorts:      []int32{},
			customSSLPorts:       nil,
			hasHTTPAnnotation:    true,
			hasSSLAnnotation:     false,
			expectedHTTPPorts:    []int32{},
			expectedHTTPSPorts:   nil,
		},
		{
			name:                 "empty SSL ports with annotation - no HTTPS listeners", 
			customHTTPPorts:      nil,
			customSSLPorts:       []int32{},
			hasHTTPAnnotation:    false,
			hasSSLAnnotation:     true,
			expectedHTTPPorts:    nil,
			expectedHTTPSPorts:   []int32{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := determinePortsToUse(tc.customHTTPPorts, tc.customSSLPorts, tc.hasHTTPAnnotation, tc.hasSSLAnnotation)

			if !reflect.DeepEqual(result.HTTP, tc.expectedHTTPPorts) {
				t.Errorf("Expected HTTP ports %v, got %v", tc.expectedHTTPPorts, result.HTTP)
			}

			if !reflect.DeepEqual(result.HTTPS, tc.expectedHTTPSPorts) {
				t.Errorf("Expected HTTPS ports %v, got %v", tc.expectedHTTPSPorts, result.HTTPS)
			}
		})
	}
}