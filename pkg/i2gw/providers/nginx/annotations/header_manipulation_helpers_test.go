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

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParseCommaSeparatedHeaders(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "single header",
			input:    "Server",
			expected: []string{"Server"},
		},
		{
			name:     "multiple headers",
			input:    "Server,X-Powered-By,X-Version",
			expected: []string{"Server", "X-Powered-By", "X-Version"},
		},
		{
			name:     "headers with spaces",
			input:    " Server , X-Powered-By , X-Version ",
			expected: []string{"Server", "X-Powered-By", "X-Version"},
		},
		{
			name:     "empty headers filtered out",
			input:    "Server,,X-Powered-By,",
			expected: []string{"Server", "X-Powered-By"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseCommaSeparatedHeaders(tc.input)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestCreateResponseHeaderModifier(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedFilter *gatewayv1.HTTPRouteFilter
	}{
		{
			name:           "empty input",
			input:          "",
			expectedFilter: nil,
		},
		{
			name:  "single header",
			input: "Server",
			expectedFilter: &gatewayv1.HTTPRouteFilter{
				Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
				ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
					Remove: []string{"Server"},
				},
			},
		},
		{
			name:  "multiple headers",
			input: "Server,X-Powered-By,X-Version",
			expectedFilter: &gatewayv1.HTTPRouteFilter{
				Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
				ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
					Remove: []string{"Server", "X-Powered-By", "X-Version"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := createResponseHeaderModifier(tc.input)
			if !reflect.DeepEqual(result, tc.expectedFilter) {
				t.Errorf("Expected %+v, got %+v", tc.expectedFilter, result)
			}
		})
	}
}

func TestCreateRequestHeaderModifier(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedFilter *gatewayv1.HTTPRouteFilter
	}{
		{
			name:           "empty input",
			input:          "",
			expectedFilter: nil,
		},
		{
			name:  "single header with value",
			input: "X-Custom: hello-world",
			expectedFilter: &gatewayv1.HTTPRouteFilter{
				Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
					Set: []gatewayv1.HTTPHeader{
						{Name: "X-Custom", Value: "hello-world"},
					},
				},
			},
		},
		{
			name:  "multiple headers with values",
			input: "X-Custom: hello-world,X-Version: 1.0.0",
			// Don't check exact filter here due to map iteration order
			expectedFilter: nil, // Will be verified manually in test
		},
		{
			name:           "headers with NGINX variables filtered out",
			input:          "X-Real-IP: $remote_addr",
			expectedFilter: nil,
		},
		{
			name:           "headers with empty values filtered out",
			input:          "X-Empty-Header",
			expectedFilter: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := createRequestHeaderModifier(tc.input)

			// Special handling for multiple headers test due to map iteration order
			if tc.name == "multiple headers with values" {
				if result == nil {
					t.Error("Expected non-nil filter for multiple headers")
					return
				}
				if result.Type != gatewayv1.HTTPRouteFilterRequestHeaderModifier {
					t.Errorf("Expected RequestHeaderModifier type, got %s", result.Type)
					return
				}
				if result.RequestHeaderModifier == nil {
					t.Error("Expected RequestHeaderModifier to be non-nil")
					return
				}
				if len(result.RequestHeaderModifier.Set) != 2 {
					t.Errorf("Expected 2 headers, got %d", len(result.RequestHeaderModifier.Set))
					return
				}
				// Check headers exist (order may vary due to map iteration)
				headers := make(map[string]string)
				for _, h := range result.RequestHeaderModifier.Set {
					headers[string(h.Name)] = h.Value
				}
				if headers["X-Custom"] != "hello-world" {
					t.Errorf("Expected X-Custom: hello-world, got %s", headers["X-Custom"])
				}
				if headers["X-Version"] != "1.0.0" {
					t.Errorf("Expected X-Version: 1.0.0, got %s", headers["X-Version"])
				}
				return
			}

			if !reflect.DeepEqual(result, tc.expectedFilter) {
				t.Errorf("Expected %+v, got %+v", tc.expectedFilter, result)
			}
		})
	}
}
