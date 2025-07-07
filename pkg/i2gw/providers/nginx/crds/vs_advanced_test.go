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

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

func TestCreateAdvancedMatches(t *testing.T) {
	tests := []struct {
		name     string
		vs       nginxv1.VirtualServer
		route    nginxv1.Route
		expected int // number of matches expected
	}{
		{
			name: "basic path match only",
			vs: nginxv1.VirtualServer{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			route: nginxv1.Route{
				Path: "/api",
			},
			expected: 1,
		},
		{
			name: "header and query param matching",
			vs: nginxv1.VirtualServer{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			route: nginxv1.Route{
				Path: "/api",
				Matches: []nginxv1.Match{
					{
						Conditions: []nginxv1.Condition{
							{Header: "Authorization", Value: "Bearer .*"},
							{Argument: "version", Value: "v1"},
						},
					},
				},
			},
			expected: 1,
		},
		{
			name: "cookie matching",
			vs: nginxv1.VirtualServer{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			route: nginxv1.Route{
				Path: "/api",
				Matches: []nginxv1.Match{
					{
						Conditions: []nginxv1.Condition{
							{Cookie: "session-id", Value: "abc123"},
						},
					},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notifs []notifications.Notification
			matches := createAdvancedMatches(tt.vs, tt.route, &notifs)

			if len(matches) != tt.expected {
				t.Errorf("Expected %d matches, got %d", tt.expected, len(matches))
			}

			// Verify path match is always present
			if len(matches) > 0 && matches[0].Path == nil {
				t.Error("Expected path match to be present")
			}
		})
	}
}

func TestCreatePathRewriteFilter(t *testing.T) {
	tests := []struct {
		name        string
		rewritePath string
		expectType  gatewayv1.HTTPPathModifierType
	}{
		{
			name:        "absolute path replacement",
			rewritePath: "/new/path",
			expectType:  gatewayv1.FullPathHTTPPathModifier,
		},
		{
			name:        "prefix replacement",
			rewritePath: "api/v2",
			expectType:  gatewayv1.PrefixMatchHTTPPathModifier,
		},
		{
			name:        "empty path removal",
			rewritePath: "",
			expectType:  gatewayv1.PrefixMatchHTTPPathModifier,
		},
	}

	vs := nginxv1.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notifs []notifications.Notification
			filter := createPathRewriteFilter(tt.rewritePath, vs, &notifs)

			if filter == nil {
				t.Fatal("Expected filter to be created")
			}

			if filter.Type != gatewayv1.HTTPRouteFilterURLRewrite {
				t.Errorf("Expected filter type URLRewrite, got %v", filter.Type)
			}

			if filter.URLRewrite == nil || filter.URLRewrite.Path == nil {
				t.Fatal("Expected URLRewrite.Path to be set")
			}

			if filter.URLRewrite.Path.Type != tt.expectType {
				t.Errorf("Expected path modifier type %v, got %v", tt.expectType, filter.URLRewrite.Path.Type)
			}
		})
	}
}

func TestCreateRequestHeaderFilter(t *testing.T) {
	requestHeaders := &nginxv1.ProxyRequestHeaders{
		Set: []nginxv1.Header{
			{Name: "X-Forwarded-Proto", Value: "https"},
			{Name: "X-Custom", Value: "test"},
		},
	}

	vs := nginxv1.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	var notifs []notifications.Notification
	filter := createRequestHeaderFilter(requestHeaders, vs, &notifs)

	if filter == nil {
		t.Fatal("Expected filter to be created")
	}

	if filter.Type != gatewayv1.HTTPRouteFilterRequestHeaderModifier {
		t.Errorf("Expected filter type RequestHeaderModifier, got %v", filter.Type)
	}

	if filter.RequestHeaderModifier == nil {
		t.Fatal("Expected RequestHeaderModifier to be set")
	}

	if len(filter.RequestHeaderModifier.Set) != 2 {
		t.Errorf("Expected 2 headers to be set, got %d", len(filter.RequestHeaderModifier.Set))
	}
}

func TestCreateResponseHeaderFilter(t *testing.T) {
	responseHeaders := &nginxv1.ProxyResponseHeaders{
		Add: []nginxv1.AddHeader{
			{Header: nginxv1.Header{Name: "X-Response-Time", Value: "100ms"}, Always: true},
		},
		Hide: []string{"X-Powered-By", "Server"},
	}

	vs := nginxv1.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	var notifs []notifications.Notification
	filter := createResponseHeaderFilter(responseHeaders, vs, &notifs)

	if filter == nil {
		t.Fatal("Expected filter to be created")
	}

	if filter.Type != gatewayv1.HTTPRouteFilterResponseHeaderModifier {
		t.Errorf("Expected filter type ResponseHeaderModifier, got %v", filter.Type)
	}

	if filter.ResponseHeaderModifier == nil {
		t.Fatal("Expected ResponseHeaderModifier to be set")
	}

	if len(filter.ResponseHeaderModifier.Set) != 1 {
		t.Errorf("Expected 1 header to be set, got %d", len(filter.ResponseHeaderModifier.Set))
	}

	if len(filter.ResponseHeaderModifier.Remove) != 2 {
		t.Errorf("Expected 2 headers to be removed, got %d", len(filter.ResponseHeaderModifier.Remove))
	}
}

func TestValidateUpstream(t *testing.T) {
	vs := nginxv1.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	tests := []struct {
		name     string
		upstream nginxv1.Upstream
		expected bool
	}{
		{
			name: "valid upstream",
			upstream: nginxv1.Upstream{
				Name:    "webapp",
				Service: "webapp-service",
				Port:    80,
			},
			expected: true,
		},
		{
			name: "missing service",
			upstream: nginxv1.Upstream{
				Name: "webapp",
				Port: 80,
			},
			expected: false,
		},
		{
			name: "missing port",
			upstream: nginxv1.Upstream{
				Name:    "webapp",
				Service: "webapp-service",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notifs []notifications.Notification
			result := validateUpstream(&tt.upstream, &vs, &notifs)

			if result != tt.expected {
				t.Errorf("Expected validation result %v, got %v", tt.expected, result)
			}
		})
	}
}