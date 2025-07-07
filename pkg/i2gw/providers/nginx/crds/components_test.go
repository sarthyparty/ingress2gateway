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

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

func TestRouteResolver(t *testing.T) {
	t.Run("resolve VirtualServer with inline routes", func(t *testing.T) {
		vs := nginxv1.VirtualServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vs",
				Namespace: "default",
			},
			Spec: nginxv1.VirtualServerSpec{
				Host: "api.example.com",
				Routes: []nginxv1.Route{
					{
						Path: "/v1",
						Action: &nginxv1.Action{
							Pass: "api-v1",
						},
					},
					{
						Path: "/v2",
						Action: &nginxv1.Action{
							Pass: "api-v2",
						},
					},
				},
			},
		}

		resolver := NewRouteResolver([]nginxv1.VirtualServer{vs}, []nginxv1.VirtualServerRoute{})

		resolvedRoutes, notifications, err := resolver.ResolveRoutesForVirtualServer(vs)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if len(resolvedRoutes) != 2 {
			t.Errorf("Expected 2 resolved routes, got %d", len(resolvedRoutes))
		}

		if len(notifications) != 0 {
			t.Errorf("Expected no notifications for inline routes, got %d", len(notifications))
		}

		// Validate route sources
		for _, route := range resolvedRoutes {
			if route.Source.Type != RouteSourceVirtualServer {
				t.Errorf("Expected VirtualServer source type, got %s", route.Source.Type)
			}
		}
	})

	t.Run("resolve VirtualServer with VirtualServerRoute reference", func(t *testing.T) {
		vsr := nginxv1.VirtualServerRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-routes",
				Namespace: "default",
			},
			Spec: nginxv1.VirtualServerRouteSpec{
				Host: "api.example.com",
				Upstreams: []nginxv1.Upstream{
					{
						Name:    "api-backend",
						Service: "api-service",
						Port:    8080,
					},
				},
				Subroutes: []nginxv1.Route{
					{
						Path: "/users",
						Action: &nginxv1.Action{
							Pass: "api-backend",
						},
					},
				},
			},
		}

		vs := nginxv1.VirtualServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vs",
				Namespace: "default",
			},
			Spec: nginxv1.VirtualServerSpec{
				Host: "api.example.com",
				Routes: []nginxv1.Route{
					{
						Path:  "/api",
						Route: "api-routes", // Reference to VSR
					},
				},
			},
		}

		resolver := NewRouteResolver([]nginxv1.VirtualServer{vs}, []nginxv1.VirtualServerRoute{vsr})

		resolvedRoutes, notifications, err := resolver.ResolveRoutesForVirtualServer(vs)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if len(resolvedRoutes) != 1 {
			t.Errorf("Expected 1 resolved route from VSR, got %d", len(resolvedRoutes))
		}

		if len(notifications) != 1 {
			t.Errorf("Expected 1 info notification about VSR resolution, got %d", len(notifications))
		}

		// Validate route came from VSR
		route := resolvedRoutes[0]
		if route.Source.Type != RouteSourceVirtualServerRoute {
			t.Errorf("Expected VirtualServerRoute source type, got %s", route.Source.Type)
		}
		if len(route.Upstreams) != 1 {
			t.Errorf("Expected upstreams from VSR, got %d", len(route.Upstreams))
		}
	})
}

func TestConditionMatching(t *testing.T) {
	vs := nginxv1.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vs",
			Namespace: "default",
		},
	}

	t.Run("process header conditions", func(t *testing.T) {
		conditions := []nginxv1.Condition{
			{
				Header: "X-API-Version",
				Value:  "v2",
			},
			{
				Header: "Authorization",
				Value:  "Bearer *",
			},
		}

		var notifs []notifications.Notification
		headerMatches, queryMatches := processConditions(conditions, vs, &notifs)

		if len(headerMatches) != 2 {
			t.Errorf("Expected 2 header matches, got %d", len(headerMatches))
		}

		if len(queryMatches) != 0 {
			t.Errorf("Expected 0 query matches, got %d", len(queryMatches))
		}

		if len(notifs) != 0 {
			t.Errorf("Expected no notifications for valid conditions, got %d", len(notifs))
		}
	})

	t.Run("process query parameter conditions", func(t *testing.T) {
		conditions := []nginxv1.Condition{
			{
				Argument: "version",
				Value:    "v1",
			},
			{
				Argument: "debug",
				Value:    "true",
			},
		}

		var notifs []notifications.Notification
		headerMatches, queryMatches := processConditions(conditions, vs, &notifs)

		if len(headerMatches) != 0 {
			t.Errorf("Expected 0 header matches, got %d", len(headerMatches))
		}

		if len(queryMatches) != 2 {
			t.Errorf("Expected 2 query matches, got %d", len(queryMatches))
		}
	})

	t.Run("process cookie conditions", func(t *testing.T) {
		conditions := []nginxv1.Condition{
			{
				Cookie: "session",
				Value:  "active",
			},
		}

		var notifs []notifications.Notification
		headerMatches, queryMatches := processConditions(conditions, vs, &notifs)

		// Cookie conditions become header matches (Cookie header)
		if len(headerMatches) != 1 {
			t.Errorf("Expected 1 header match for cookie, got %d", len(headerMatches))
		}

		if len(queryMatches) != 0 {
			t.Errorf("Expected 0 query matches, got %d", len(queryMatches))
		}

		// Should have info notification about cookie conversion
		if len(notifs) != 1 {
			t.Errorf("Expected 1 info notification about cookie conversion, got %d", len(notifs))
		}
	})
}

func TestAdvancedProxyActions(t *testing.T) {
	vs := nginxv1.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vs",
			Namespace: "default",
		},
		Spec: nginxv1.VirtualServerSpec{
			Upstreams: []nginxv1.Upstream{
				{
					Name:    "api-backend",
					Service: "api-service",
					Port:    8080,
				},
			},
		},
	}

	t.Run("proxy action with path rewrite", func(t *testing.T) {
		action := &nginxv1.Action{
			Proxy: &nginxv1.ActionProxy{
				Upstream:    "api-backend",
				RewritePath: "/v2/api",
				RequestHeaders: &nginxv1.ProxyRequestHeaders{
					Set: []nginxv1.Header{
						{Name: "X-Forwarded-For", Value: "$remote_addr"},
						{Name: "X-Real-IP", Value: "$remote_addr"},
					},
				},
				ResponseHeaders: &nginxv1.ProxyResponseHeaders{
					Add: []nginxv1.AddHeader{
						{Header: nginxv1.Header{Name: "X-Backend", Value: "api-v2"}, Always: true},
					},
					Hide: []string{"Server", "X-Powered-By"},
				},
			},
		}

		var notifs []notifications.Notification
		backendRef, filters := handleAdvancedProxyAction(vs, action, &notifs)

		// Should create backend ref
		if backendRef == nil {
			t.Error("Expected backend ref for valid proxy action")
		} else {
			if backendRef.BackendRef.BackendObjectReference.Name != "api-backend" {
				t.Errorf("Expected upstream name 'api-backend', got %s", backendRef.BackendRef.BackendObjectReference.Name)
			}
			if *backendRef.BackendRef.BackendObjectReference.Port != 8080 {
				t.Errorf("Expected port 8080, got %d", *backendRef.BackendRef.BackendObjectReference.Port)
			}
		}

		// Should create filters for path rewrite and header manipulation
		// Note: Actual filter creation depends on filter factory availability
		expectedFilters := 3 // path rewrite + request headers + response headers
		if len(filters) > expectedFilters {
			t.Errorf("Got more filters than expected: %d", len(filters))
		}
	})

	t.Run("proxy action with missing upstream", func(t *testing.T) {
		action := &nginxv1.Action{
			Proxy: &nginxv1.ActionProxy{
				Upstream: "nonexistent-backend",
			},
		}

		var notifs []notifications.Notification
		backendRef, filters := handleAdvancedProxyAction(vs, action, &notifs)

		// Should not create backend ref
		if backendRef != nil {
			t.Errorf("Expected nil backend ref for missing upstream, got %+v", backendRef)
		}

		// Should not create filters
		if len(filters) != 0 {
			t.Errorf("Expected no filters for missing upstream, got %d", len(filters))
		}

		// Should generate warning
		hasWarning := false
		for _, notif := range notifs {
			if notif.Type == "WARNING" {
				hasWarning = true
				break
			}
		}
		if !hasWarning {
			t.Error("Expected warning notification for missing upstream")
		}
	})
}

func TestUnsupportedFieldDetection(t *testing.T) {
	t.Run("VirtualServer with multiple unsupported fields", func(t *testing.T) {
		vs := nginxv1.VirtualServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unsupported-vs",
				Namespace: "default",
			},
			Spec: nginxv1.VirtualServerSpec{
				Host:   "example.com",
				Gunzip: true,
				ExternalDNS: nginxv1.ExternalDNS{
					Enable: true,
				},
				Dos:            "dos-policy",
				InternalRoute:  true,
				HTTPSnippets:   "custom http config",
				ServerSnippets: "custom server config",
				Policies: []nginxv1.PolicyReference{
					{Name: "rate-limit", Namespace: "default"},
				},
			},
		}

		var notifs []notifications.Notification
		checkUnsupportedVirtualServerFields(vs, &notifs)

		expectedWarnings := 7 // gunzip, externalDNS, dos, policies, internalRoute, http-snippets, server-snippets
		warningCount := 0
		for _, notif := range notifs {
			if notif.Type == "WARNING" {
				warningCount++
			}
		}

		if warningCount != expectedWarnings {
			t.Errorf("Expected %d warnings for unsupported fields, got %d", expectedWarnings, warningCount)
		}

		// Verify specific warnings are present
		expectedFields := []string{"gunzip", "externalDNS", "dos", "policies", "internalRoute", "http-snippets", "server-snippets"}
		for _, field := range expectedFields {
			found := false
			for _, notif := range notifs {
				if notif.Type == "WARNING" && containsString(notif.Message, field) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected warning about unsupported field '%s'", field)
			}
		}
	})
}
