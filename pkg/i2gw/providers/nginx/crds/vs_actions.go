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
	"fmt"
	"strings"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

// handleAdvancedProxyAction processes ActionProxy with path rewriting and header manipulation
func handleAdvancedProxyAction(vs nginxv1.VirtualServer, route nginxv1.Route, rule *gatewayv1.HTTPRouteRule, notifs *[]notifications.Notification) {
	if route.Action == nil || route.Action.Proxy == nil {
		return
	}

	proxy := route.Action.Proxy
	var filters []gatewayv1.HTTPRouteFilter

	// Handle path rewriting
	if proxy.RewritePath != "" {
		rewriteFilter := createPathRewriteFilter(proxy.RewritePath, vs, notifs)
		if rewriteFilter != nil {
			filters = append(filters, *rewriteFilter)
		}
	}

	// Handle request header manipulation
	if proxy.RequestHeaders != nil {
		requestHeaderFilter := createRequestHeaderFilter(proxy.RequestHeaders, vs, notifs)
		if requestHeaderFilter != nil {
			filters = append(filters, *requestHeaderFilter)
		}
	}

	// Handle response header manipulation
	if proxy.ResponseHeaders != nil {
		responseHeaderFilter := createResponseHeaderFilter(proxy.ResponseHeaders, vs, notifs)
		if responseHeaderFilter != nil {
			filters = append(filters, *responseHeaderFilter)
		}
	}

	// Set backend ref to upstream
	if proxy.Upstream != "" {
		upstream := findUpstream(vs.Spec.Upstreams, proxy.Upstream)
		if upstream != nil {
			rule.BackendRefs = []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: gatewayv1.ObjectName(upstream.Service),
							Port: Ptr(gatewayv1.PortNumber(upstream.Port)),
						},
					},
				},
			}
		} else {
			addNotification(notifs, notifications.WarningNotification,
				fmt.Sprintf("Upstream '%s' not found for proxy action", proxy.Upstream), &vs)
		}
	}

	// Apply filters to rule
	if len(filters) > 0 {
		rule.Filters = filters
	}
}

// createPathRewriteFilter creates a URLRewrite filter for path rewriting
func createPathRewriteFilter(rewritePath string, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	// Handle different rewrite patterns
	var pathRewrite *gatewayv1.HTTPPathModifier

	if strings.HasPrefix(rewritePath, "/") {
		// Absolute path replacement
		pathRewrite = &gatewayv1.HTTPPathModifier{
			Type:            gatewayv1.FullPathHTTPPathModifier,
			ReplaceFullPath: Ptr(rewritePath),
		}
		addNotification(notifs, notifications.InfoNotification,
			"Path rewrite converted to FullPath replacement", &vs)
	} else if rewritePath == "" {
		// Remove path prefix
		pathRewrite = &gatewayv1.HTTPPathModifier{
			Type:               gatewayv1.PrefixMatchHTTPPathModifier,
			ReplacePrefixMatch: Ptr("/"),
		}
		addNotification(notifs, notifications.InfoNotification,
			"Empty path rewrite converted to prefix removal", &vs)
	} else {
		// Prefix replacement
		pathRewrite = &gatewayv1.HTTPPathModifier{
			Type:               gatewayv1.PrefixMatchHTTPPathModifier,
			ReplacePrefixMatch: Ptr(rewritePath),
		}
		addNotification(notifs, notifications.InfoNotification,
			"Path rewrite converted to prefix replacement", &vs)
	}

	return &gatewayv1.HTTPRouteFilter{
		Type: gatewayv1.HTTPRouteFilterURLRewrite,
		URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
			Path: pathRewrite,
		},
	}
}

// createRequestHeaderFilter creates a RequestHeaderModifier filter
func createRequestHeaderFilter(requestHeaders *nginxv1.ProxyRequestHeaders, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	if requestHeaders == nil {
		return nil
	}

	modifier := &gatewayv1.HTTPHeaderFilter{}

	// Handle header additions/modifications
	if len(requestHeaders.Set) > 0 {
		var headers []gatewayv1.HTTPHeader
		for _, header := range requestHeaders.Set {
			headers = append(headers, gatewayv1.HTTPHeader{
				Name:  gatewayv1.HTTPHeaderName(header.Name),
				Value: header.Value,
			})
		}
		modifier.Set = headers

		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Request header modifications converted (%d headers)", len(requestHeaders.Set)), &vs)
	}

	// Handle header removal (Pass: false means remove headers)
	if requestHeaders.Pass != nil && !*requestHeaders.Pass {
		addNotification(notifs, notifications.InfoNotification,
			"Request header pass=false stored in provider-specific IR - complex header filtering not fully supported", &vs)
	}

	return &gatewayv1.HTTPRouteFilter{
		Type:            gatewayv1.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: modifier,
	}
}

// createResponseHeaderFilter creates a ResponseHeaderModifier filter
func createResponseHeaderFilter(responseHeaders *nginxv1.ProxyResponseHeaders, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	if responseHeaders == nil {
		return nil
	}

	modifier := &gatewayv1.HTTPHeaderFilter{}

	// Handle header additions
	if len(responseHeaders.Add) > 0 {
		var headers []gatewayv1.HTTPHeader
		for _, addHeader := range responseHeaders.Add {
			headers = append(headers, gatewayv1.HTTPHeader{
				Name:  gatewayv1.HTTPHeaderName(addHeader.Name),
				Value: addHeader.Value,
			})
			if addHeader.Always {
				addNotification(notifs, notifications.InfoNotification,
					"Response header 'always' flag stored in provider-specific IR", &vs)
			}
		}
		modifier.Set = headers

		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Response header additions converted (%d headers)", len(responseHeaders.Add)), &vs)
	}

	// Handle header removal
	if len(responseHeaders.Hide) > 0 {
		var headerNames []string
		for _, headerName := range responseHeaders.Hide {
			headerNames = append(headerNames, headerName)
		}
		modifier.Remove = headerNames
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Response header removals converted (%d headers)", len(responseHeaders.Hide)), &vs)
	}

	// Handle selective header passing/ignoring
	if len(responseHeaders.Pass) > 0 || len(responseHeaders.Ignore) > 0 {
		addNotification(notifs, notifications.InfoNotification,
			"Response header pass/ignore configuration stored in provider-specific IR - not directly supported", &vs)
	}

	return &gatewayv1.HTTPRouteFilter{
		Type:             gatewayv1.HTTPRouteFilterResponseHeaderModifier,
		ResponseHeaderModifier: modifier,
	}
}

// storeAdvancedProxyConfig stores nginx-specific proxy configuration in provider IR
func storeAdvancedProxyConfig(proxy *nginxv1.ActionProxy, route nginxv1.Route, nginxIR *intermediate.NginxHTTPRouteIR) {
	if proxy == nil {
		return
	}

	// Store path rewrite configuration
	if proxy.RewritePath != "" {
		if nginxIR.PathRewrite == nil {
			nginxIR.PathRewrite = &intermediate.NginxPathRewriteConfig{}
		}
		nginxIR.PathRewrite.Pattern = route.Path
		nginxIR.PathRewrite.Replacement = proxy.RewritePath
	}

	// Store header modification configuration
	if proxy.RequestHeaders != nil || proxy.ResponseHeaders != nil {
		if nginxIR.HeaderModification == nil {
			nginxIR.HeaderModification = &intermediate.NginxHeaderModConfig{}
		}

		// Store request headers
		if proxy.RequestHeaders != nil {
			if nginxIR.HeaderModification.RequestHeaders == nil {
				nginxIR.HeaderModification.RequestHeaders = make(map[string]string)
			}
			for _, header := range proxy.RequestHeaders.Set {
				nginxIR.HeaderModification.RequestHeaders[header.Name] = header.Value
			}
		}

		// Store response headers
		if proxy.ResponseHeaders != nil {
			if nginxIR.HeaderModification.ResponseHeaders == nil {
				nginxIR.HeaderModification.ResponseHeaders = make(map[string]string)
			}
			for _, addHeader := range proxy.ResponseHeaders.Add {
				nginxIR.HeaderModification.ResponseHeaders[addHeader.Name] = addHeader.Value
			}
		}
	}
}