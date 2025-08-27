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

package crds

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

func TestTLSRedirectFunctionality(t *testing.T) {
	tests := []struct {
		name               string
		virtualServer      nginxv1.VirtualServer
		expectedRedirect   bool
		expectedHTTPRoutes int
		redirectCode       int
	}{
		{
			name: "HTTPS redirect enabled with custom code",
			virtualServer: nginxv1.VirtualServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secure-app",
					Namespace: "production",
				},
				Spec: nginxv1.VirtualServerSpec{
					Host: "secure.example.com",
					TLS: &nginxv1.TLS{
						Secret: "tls-secret",
						Redirect: &nginxv1.TLSRedirect{
							Enable: true,
							Code:   &[]int{302}[0], // Temporary redirect
						},
					},
					Upstreams: []nginxv1.Upstream{
						{
							Name:    "app-backend",
							Service: "app-service",
							Port:    8080,
						},
					},
					Routes: []nginxv1.Route{
						{
							Path: "/",
							Action: &nginxv1.Action{
								Pass: "app-backend",
							},
						},
					},
				},
			},
			expectedRedirect:   true,
			expectedHTTPRoutes: 2, // Normal route + redirect route
			redirectCode:       302,
		},
		{
			name: "HTTPS redirect disabled",
			virtualServer: nginxv1.VirtualServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-app",
					Namespace: "default",
				},
				Spec: nginxv1.VirtualServerSpec{
					Host: "mixed.example.com",
					TLS: &nginxv1.TLS{
						Secret: "tls-secret",
						Redirect: &nginxv1.TLSRedirect{
							Enable: false,
						},
					},
					Upstreams: []nginxv1.Upstream{
						{
							Name:    "app-backend",
							Service: "app-service",
							Port:    8080,
						},
					},
					Routes: []nginxv1.Route{
						{
							Path: "/",
							Action: &nginxv1.Action{
								Pass: "app-backend",
							},
						},
					},
				},
			},
			expectedRedirect:   false,
			expectedHTTPRoutes: 1, // Only normal route
		},
		{
			name: "TLS without redirect configuration",
			virtualServer: nginxv1.VirtualServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-only-app",
					Namespace: "default",
				},
				Spec: nginxv1.VirtualServerSpec{
					Host: "tls.example.com",
					TLS: &nginxv1.TLS{
						Secret: "tls-secret",
						// No Redirect field - should default to no redirect
					},
					Upstreams: []nginxv1.Upstream{
						{
							Name:    "app-backend",
							Service: "app-service",
							Port:    8080,
						},
					},
					Routes: []nginxv1.Route{
						{
							Path: "/",
							Action: &nginxv1.Action{
								Pass: "app-backend",
							},
						},
					},
				},
			},
			expectedRedirect:   false,
			expectedHTTPRoutes: 1, // Only normal route
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, notifications, errs := CRDsToGatewayIR(
				[]nginxv1.VirtualServer{tt.virtualServer},
				nil,
				[]nginxv1.TransportServer{}, // Empty TransportServers for these tests
				nil,
			)

			if len(errs) > 0 {
				t.Errorf("Unexpected errors: %v", errs)
			}

			// Check HTTPRoute count
			if len(ir.HTTPRoutes) != tt.expectedHTTPRoutes {
				t.Errorf("Expected %d HTTPRoutes, got %d", tt.expectedHTTPRoutes, len(ir.HTTPRoutes))
			}

			// Check for redirect route existence
			hasRedirectRoute := false
			var redirectRoute *gatewayv1.HTTPRoute
			for routeName, routeCtx := range ir.HTTPRoutes {
				if routeName.Name == tt.virtualServer.Name+"-redirect" {
					hasRedirectRoute = true
					redirectRoute = &routeCtx.HTTPRoute
					break
				}
			}

			if tt.expectedRedirect {
				if !hasRedirectRoute {
					t.Error("Expected redirect route but didn't find one")
				} else {
					// Validate redirect route properties
					validateRedirectRoute(t, redirectRoute, tt.redirectCode)
				}
			} else {
				if hasRedirectRoute {
					t.Error("Found redirect route but didn't expect one")
				}
			}

			// Check Gateway configuration for TLS
			if len(ir.Gateways) > 0 {
				gateway := getFirstGateway(ir.Gateways)
				validateGatewayTLS(t, &gateway.Gateway, tt.virtualServer.Spec.TLS != nil)
			}

			// Log notifications for debugging
			for _, notif := range notifications {
				t.Logf("Notification [%s]: %s", notif.Type, notif.Message)
			}
		})
	}
}

func validateRedirectRoute(t *testing.T, route *gatewayv1.HTTPRoute, expectedCode int) {
	if route == nil {
		t.Fatal("Redirect route is nil")
	}

	// Check route has redirect filter
	if len(route.Spec.Rules) == 0 {
		t.Fatal("Redirect route has no rules")
	}

	rule := route.Spec.Rules[0]
	if len(rule.Filters) == 0 {
		t.Fatal("Redirect route has no filters")
	}

	// Find redirect filter
	var redirectFilter *gatewayv1.HTTPRouteFilter
	for _, filter := range rule.Filters {
		if filter.Type == gatewayv1.HTTPRouteFilterRequestRedirect {
			redirectFilter = &filter
			break
		}
	}

	if redirectFilter == nil {
		t.Fatal("No redirect filter found in redirect route")
	}

	if redirectFilter.RequestRedirect == nil {
		t.Fatal("RequestRedirect configuration is nil")
	}

	// Check redirect status code
	if redirectFilter.RequestRedirect.StatusCode == nil {
		t.Fatal("Redirect status code is nil")
	}

	if *redirectFilter.RequestRedirect.StatusCode != expectedCode {
		t.Errorf("Expected redirect code %d, got %d", expectedCode, *redirectFilter.RequestRedirect.StatusCode)
	}

	// Check route matches all paths (catch-all for HTTP traffic)
	if len(rule.Matches) == 0 {
		t.Fatal("Redirect route has no matches")
	}

	match := rule.Matches[0]
	if match.Path == nil {
		t.Fatal("Redirect route has no path match")
	}

	if *match.Path.Type != gatewayv1.PathMatchPathPrefix {
		t.Errorf("Expected PathPrefix match type, got %s", *match.Path.Type)
	}

	if *match.Path.Value != "/" {
		t.Errorf("Expected '/' path for redirect catch-all, got %s", *match.Path.Value)
	}
}

func validateGatewayTLS(t *testing.T, gateway *gatewayv1.Gateway, expectTLS bool) {
	httpListenerFound := false
	httpsListenerFound := false

	for _, listener := range gateway.Spec.Listeners {
		switch listener.Protocol {
		case gatewayv1.HTTPProtocolType:
			httpListenerFound = true
			if listener.TLS != nil {
				t.Error("HTTP listener should not have TLS configuration")
			}
		case gatewayv1.HTTPSProtocolType:
			httpsListenerFound = true
			if listener.TLS == nil {
				t.Error("HTTPS listener should have TLS configuration")
			}
		}
	}

	if !httpListenerFound {
		t.Error("Expected HTTP listener in gateway")
	}

	if expectTLS && !httpsListenerFound {
		t.Error("Expected HTTPS listener when TLS is configured")
	}
}

func TestCreateRedirectHTTPRoute(t *testing.T) {
	vs := nginxv1.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vs",
			Namespace: "default",
		},
		Spec: nginxv1.VirtualServerSpec{
			Host: "example.com",
			TLS: &nginxv1.TLS{
				Secret: "tls-secret",
				Redirect: &nginxv1.TLSRedirect{
					Enable: true,
					Code:   &[]int{301}[0],
				},
			},
		},
	}

	listenerMap := map[string]gatewayv1.Listener{
		"test-vs": {
			Name: "http-80",
			Port: 80,
		},
	}

	routeCtx := createRedirectHTTPRoute(vs, listenerMap)

	if routeCtx == nil {
		t.Fatal("Expected redirect route context but got nil")
	}

	route := &routeCtx.HTTPRoute

	// Check basic route properties
	if route.Name != "test-vs-redirect" {
		t.Errorf("Expected route name 'test-vs-redirect', got '%s'", route.Name)
	}

	if route.Namespace != "default" {
		t.Errorf("Expected route namespace 'default', got '%s'", route.Namespace)
	}

	// Check route has correct labels
	expectedLabels := map[string]string{
		"app.kubernetes.io/managed-by": "ingress2gateway",
		"ingress2gateway.io/source":    "nginx-virtualserver",
		"ingress2gateway.io/vs-name":   "test-vs",
	}

	for key, expectedValue := range expectedLabels {
		if actualValue, exists := route.Labels[key]; !exists || actualValue != expectedValue {
			t.Errorf("Expected label %s=%s, got %s=%s", key, expectedValue, key, actualValue)
		}
	}

	// Validate redirect configuration
	validateRedirectRoute(t, route, 301)
}
