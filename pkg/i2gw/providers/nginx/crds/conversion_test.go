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
	
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

func TestVirtualServerToGatewayIR_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name                 string
		virtualServers       []nginxv1.VirtualServer
		virtualServerRoutes  []nginxv1.VirtualServerRoute
		globalConfiguration  *nginxv1.GlobalConfiguration
		expectedGateways     int
		expectedHTTPRoutes   int
		expectedWarnings     int
		expectedInfos        int
	}{
		{
			name: "e-commerce application with API and web traffic",
			virtualServers: []nginxv1.VirtualServer{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ecommerce-web",
						Namespace: "production",
					},
					Spec: nginxv1.VirtualServerSpec{
						Host: "shop.example.com",
						TLS: &nginxv1.TLS{
							Secret: "shop-tls",
							Redirect: &nginxv1.TLSRedirect{
								Enable: true,
								Code:   &[]int{301}[0],
							},
						},
						Upstreams: []nginxv1.Upstream{
							{
								Name:    "web-frontend",
								Service: "web-frontend-svc",
								Port:    3000,
							},
							{
								Name:    "api-backend",
								Service: "api-backend-svc",
								Port:    8080,
								TLS: nginxv1.UpstreamTLS{
									Enable: true,
								},
							},
						},
						Routes: []nginxv1.Route{
							{
								Path: "/",
								Action: &nginxv1.Action{
									Pass: "web-frontend",
								},
							},
							{
								Path: "/api",
								Matches: []nginxv1.Match{
									{
										Conditions: []nginxv1.Condition{
											{
												Header: "X-API-Version",
												Value:  "v2",
											},
										},
										Action: &nginxv1.Action{
											Proxy: &nginxv1.ActionProxy{
												Upstream:    "api-backend",
												RewritePath: "/v2/api",
												RequestHeaders: &nginxv1.ProxyRequestHeaders{
													Set: []nginxv1.Header{
														{Name: "X-Forwarded-Proto", Value: "https"},
													},
												},
											},
										},
									},
								},
								Action: &nginxv1.Action{
									Pass: "api-backend",
								},
							},
						},
					},
				},
			},
			expectedGateways:   1,
			expectedHTTPRoutes: 2, // Normal route + redirect route
			expectedWarnings:   1, // BackendTLSPolicy warning
			expectedInfos:      2, // Path rewrite and header modification notifications
		},
		{
			name: "microservices with shared authentication service",
			virtualServers: []nginxv1.VirtualServer{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auth-service",
						Namespace: "auth",
					},
					Spec: nginxv1.VirtualServerSpec{
						Host: "auth.internal.example.com",
						Upstreams: []nginxv1.Upstream{
							{
								Name:    "auth-backend",
								Service: "auth-service",
								Port:    8080,
							},
						},
						Routes: []nginxv1.Route{
							{
								Path: "/oauth",
								Action: &nginxv1.Action{
									Pass: "auth-backend",
								},
							},
							{
								Path: "/health",
								Action: &nginxv1.Action{
									Return: &nginxv1.ActionReturn{
										Code: 200,
										Body: "OK",
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "user-service",
						Namespace: "users",
					},
					Spec: nginxv1.VirtualServerSpec{
						Host: "users.api.example.com",
						TLS: &nginxv1.TLS{
							Secret: "api-tls",
						},
						Upstreams: []nginxv1.Upstream{
							{
								Name:    "user-api",
								Service: "user-service",
								Port:    9000,
							},
						},
						Routes: []nginxv1.Route{
							{
								Path: "/users",
								Matches: []nginxv1.Match{
									{
										Conditions: []nginxv1.Condition{
											{
												Header: "Authorization",
												Value:  "Bearer *",
											},
										},
										Action: &nginxv1.Action{
											Pass: "user-api",
										},
									},
								},
								Action: &nginxv1.Action{
									Redirect: &nginxv1.ActionRedirect{
										URL:  "https://auth.internal.example.com/oauth/login",
										Code: 302,
									},
								},
							},
						},
					},
				},
			},
			expectedGateways:   2, // Different namespaces
			expectedHTTPRoutes: 2,
			expectedWarnings:   1, // Return action warning
			expectedInfos:      0, // No unsupported features in these basic upstreams
		},
		{
			name: "legacy application with unsupported features",
			virtualServers: []nginxv1.VirtualServer{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-app",
						Namespace: "legacy",
					},
					Spec: nginxv1.VirtualServerSpec{
						Host:   "legacy.example.com",
						Gunzip: true, // Unsupported
						ExternalDNS: nginxv1.ExternalDNS{
							Enable: true, // Unsupported
						},
						Dos:            "dos-policy", // Unsupported
						InternalRoute:  true,         // Unsupported
						HTTPSnippets:   "proxy_cache_bypass $http_secret_header;", // Unsupported
						ServerSnippets: "location /health { return 200; }",         // Unsupported
						Policies: []nginxv1.PolicyReference{ // Unsupported
							{Name: "rate-limit", Namespace: "legacy"},
						},
						Upstreams: []nginxv1.Upstream{
							{
								Name:    "legacy-backend",
								Service: "legacy-app-svc",
								Port:    8080,
							},
						},
						Routes: []nginxv1.Route{
							{
								Path: "/",
								Action: &nginxv1.Action{
									Pass: "legacy-backend",
								},
							},
						},
					},
				},
			},
			expectedGateways:   1,
			expectedHTTPRoutes: 1,
			expectedWarnings:   7, // All unsupported fields
			expectedInfos:      0, // No unsupported features in these basic upstreams
		},
		{
			name: "VirtualServer without host (should be skipped)",
			virtualServers: []nginxv1.VirtualServer{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-vs",
						Namespace: "test",
					},
					Spec: nginxv1.VirtualServerSpec{
						// Missing Host field
						Upstreams: []nginxv1.Upstream{
							{
								Name:    "backend",
								Service: "backend-svc",
								Port:    8080,
							},
						},
						Routes: []nginxv1.Route{
							{
								Path: "/",
								Action: &nginxv1.Action{
									Pass: "backend",
								},
							},
						},
					},
				},
			},
			expectedGateways:   0,
			expectedHTTPRoutes: 0,
			expectedWarnings:   1, // Host missing warning
			expectedInfos:      0, // No processing happens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, notifications, errs := CRDsToGatewayIR(
				tt.virtualServers,
				tt.virtualServerRoutes,
				[]nginxv1.TransportServer{}, // Empty TransportServers for these tests
				tt.globalConfiguration,
			)

			// Check for unexpected errors
			if len(errs) > 0 {
				t.Errorf("Unexpected errors: %v", errs)
			}

			// Validate Gateway count
			if len(ir.Gateways) != tt.expectedGateways {
				t.Errorf("Expected %d gateways, got %d", tt.expectedGateways, len(ir.Gateways))
			}

			// Validate HTTPRoute count
			if len(ir.HTTPRoutes) != tt.expectedHTTPRoutes {
				t.Errorf("Expected %d HTTPRoutes, got %d", tt.expectedHTTPRoutes, len(ir.HTTPRoutes))
			}

			// Count notification types
			warningCount := 0
			infoCount := 0
			for _, notif := range notifications {
				switch notif.Type {
				case "WARNING":
					warningCount++
				case "INFO":
					infoCount++
				}
			}

			// Validate notification counts
			if warningCount != tt.expectedWarnings {
				t.Errorf("Expected %d warnings, got %d", tt.expectedWarnings, warningCount)
				for _, notif := range notifications {
					if notif.Type == "WARNING" {
						t.Logf("Warning: %s", notif.Message)
					}
				}
			}

			if infoCount != tt.expectedInfos {
				t.Errorf("Expected %d info notifications, got %d", tt.expectedInfos, infoCount)
			}

			// Validate real-world specific aspects
			validateRealWorldAspects(t, tt.name, ir, notifications)
		})
	}
}

func validateRealWorldAspects(t *testing.T, testName string, ir intermediate.IR, notifications []notifications.Notification) {
	switch testName {
	case "e-commerce application with API and web traffic":
		// Validate TLS redirect is created
		hasRedirectRoute := false
		for routeName := range ir.HTTPRoutes {
			if routeName.Name == "ecommerce-web-redirect" {
				hasRedirectRoute = true
				break
			}
		}
		if !hasRedirectRoute {
			t.Error("Expected TLS redirect route for e-commerce application")
		}

		// Validate TLS policies for secure backend
		if len(ir.BackendTLSPolicies) == 0 {
			t.Error("Expected BackendTLS policy for secure API backend")
		}

	case "microservices with shared authentication service":
		// Validate separate gateways for different namespaces
		namespaces := make(map[string]bool)
		for gatewayKey := range ir.Gateways {
			namespaces[gatewayKey.Namespace] = true
		}
		if len(namespaces) != 2 {
			t.Errorf("Expected gateways in 2 namespaces, got %d", len(namespaces))
		}

	case "legacy application with unsupported features":
		// Validate all expected unsupported field warnings
		expectedWarnings := []string{"gunzip", "externalDNS", "dos", "policies", "internalRoute", "http-snippets", "server-snippets"}
		for _, expectedWarning := range expectedWarnings {
			found := false
			for _, notif := range notifications {
				if notif.Type == "WARNING" && containsString(notif.Message, expectedWarning) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected warning about '%s' but didn't find it", expectedWarning)
			}
		}
	}
}

func containsString(text, substr string) bool {
	return len(text) >= len(substr) && (text == substr || 
		(len(text) > len(substr) && 
			(text[:len(substr)] == substr || 
				text[len(text)-len(substr):] == substr ||
				findSubstring(text, substr))))
}

func findSubstring(text, substr string) bool {
	for i := 0; i <= len(text)-len(substr); i++ {
		if text[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
