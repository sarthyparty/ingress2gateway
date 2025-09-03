/*
Copyright 2025 The Kubernetes Authors.

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
	"fmt"
	"strings"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	ncommon "github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/common"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

// handleAdvancedProxyAction processes ActionProxy with path rewriting and header manipulation
func handleAdvancedProxyAction(vs nginxv1.VirtualServer, action *nginxv1.Action, notifs *[]notifications.Notification) (*gatewayv1.HTTPBackendRef, []gatewayv1.HTTPRouteFilter) {
	if action.Proxy == nil {
		return nil, nil
	}

	proxy := action.Proxy

	if proxy.Upstream == "" {
		addNotification(notifs, notifications.WarningNotification,
			"Proxy action missing upstream reference", &vs)
		return nil, nil
	}
	upstream := findUpstream(vs.Spec.Upstreams, proxy.Upstream)
	if upstream == nil {
		addNotification(notifs, notifications.WarningNotification,
			fmt.Sprintf("Upstream '%s' not found for proxy action", proxy.Upstream), &vs)
		return nil, nil
	}

	var filters []gatewayv1.HTTPRouteFilter

	if proxy.RewritePath != "" {
		if f := createPathRewriteFilter(proxy.RewritePath, vs, notifs); f != nil {
			filters = append(filters, *f)
		}
	}
	if f := createRequestHeaderFilter(proxy.RequestHeaders, vs, notifs); f != nil {
		filters = append(filters, *f)
	}

	if f := createResponseHeaderFilter(proxy.ResponseHeaders, vs, notifs); f != nil {
		filters = append(filters, *f)
	}

	// Create backend ref for validated upstream
	backendRef := &gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(proxy.Upstream), // Use proxy.Upstream for upstream name
				Port: Ptr(gatewayv1.PortNumber(upstream.Port)),
			},
		},
	}

	return backendRef, filters
}

// createPathRewriteFilter creates a URLRewrite filter for path rewriting using the unified factory
func createPathRewriteFilter(rewritePath string, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	if strings.Contains(rewritePath, "$") {
		addNotification(notifs, notifications.WarningNotification, "Path rewrite contains $ - not supported in Gateway API", &vs)
		return nil
	}

	return ncommon.CreateURLRewriteFilter(rewritePath)
}

// createRequestHeaderFilter creates a RequestHeaderModifier filter using the unified factory
func createRequestHeaderFilter(requestHeaders *nginxv1.ProxyRequestHeaders, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	if requestHeaders == nil {
		return nil
	}

	headersToSet := make(map[string]string)
	for _, h := range requestHeaders.Set {
		headersToSet[h.Name] = h.Value
	}

	// Handle header removal (Pass: false means remove all the other headers) - this is NGINX-specific
	if requestHeaders.Pass != nil && !*requestHeaders.Pass {
		addNotification(notifs, notifications.WarningNotification, "Request header pass=false ignored - complex header filtering not fully supported", &vs)
	}

	return ncommon.CreateRequestHeaderModifier(headersToSet)
}

// createResponseHeaderFilter creates a ResponseHeaderModifier filter using the unified factory
func createResponseHeaderFilter(responseHeaders *nginxv1.ProxyResponseHeaders, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	if responseHeaders == nil {
		return nil
	}

	// Handle add headers with warnings for unsupported features
	headersToSet := make(map[string]string)
	for _, addHeader := range responseHeaders.Add {
		headersToSet[addHeader.Name] = addHeader.Value
		// Handle the Always flag - NGINX-specific feature
		if !addHeader.Always {
			addNotification(notifs, notifications.WarningNotification, "always flag is always true in gateway api", &vs)
		}
	}

	// Handle selective header passing/ignoring - NGINX-specific
	if len(responseHeaders.Pass) > 0 || len(responseHeaders.Ignore) > 0 {
		addNotification(notifs, notifications.WarningNotification, "Response header pass/ignore configuration is not supported in Gateway API", &vs)
	}

	// Create filter with both set and remove operations
	if len(headersToSet) > 0 && len(responseHeaders.Hide) > 0 {
		// For now, prioritize hide (remove) headers since it's more commonly needed
		addNotification(notifs, notifications.InfoNotification, "Response header add operation ignored when hide is also specified", &vs)
		return ncommon.CreateResponseHeaderModifier(responseHeaders.Hide)
	}

	if len(responseHeaders.Hide) > 0 {
		return ncommon.CreateResponseHeaderModifier(responseHeaders.Hide)
	}

	// Handle add headers (not directly supported by our common function, so create manually)
	if len(headersToSet) > 0 {
		var headers []gatewayv1.HTTPHeader
		for name, value := range headersToSet {
			headers = append(headers, gatewayv1.HTTPHeader{
				Name:  gatewayv1.HTTPHeaderName(name),
				Value: value,
			})
		}
		return &gatewayv1.HTTPRouteFilter{
			Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
			ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
				Set: headers,
			},
		}
	}

	return nil
}
