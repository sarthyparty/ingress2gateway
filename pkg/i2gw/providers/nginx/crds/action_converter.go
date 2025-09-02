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

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	ncommon "github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/common"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/common/filters"
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
	collector := ncommon.NewSliceNotificationCollector()

	if strings.Contains(rewritePath, "$") {
		collector.AddWarning("Path rewrite contains $ - not supported in Gateway API", &vs)
		return nil
	}

	filter := filters.NewHTTPRouteFilter(filters.URLRewriteFilter, filters.FilterOptions{
		URLRewrite: &filters.URLRewriteOptions{
			Path: rewritePath,
		},
		NotificationCollector: collector,
		SourceObject:          &vs,
	})

	*notifs = append(*notifs, collector.GetNotifications()...)

	return filter
}

// createRequestHeaderFilter creates a RequestHeaderModifier filter using the unified factory
func createRequestHeaderFilter(requestHeaders *nginxv1.ProxyRequestHeaders, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	if requestHeaders == nil {
		return nil
	}

	collector := ncommon.NewSliceNotificationCollector()

	var setHeaders []filters.Header
	for _, h := range requestHeaders.Set {
		setHeaders = append(setHeaders, filters.Header{
			Name:  h.Name,
			Value: h.Value,
		})
	}

	filter := filters.NewHTTPRouteFilter(filters.RequestHeaderModifierFilter, filters.FilterOptions{
		HeaderModifier: &filters.HeaderModifierOptions{
			SetHeaders: setHeaders,
		},
		NotificationCollector: collector,
		SourceObject:          &vs,
	})

	// Handle header removal (Pass: false means remove all the other headers) - this is NGINX-specific
	if requestHeaders.Pass != nil && !*requestHeaders.Pass {
		collector.AddWarning("Request header pass=false ignored - complex header filtering not fully supported", &vs)
	}

	*notifs = append(*notifs, collector.GetNotifications()...)

	return filter
}

// createResponseHeaderFilter creates a ResponseHeaderModifier filter using the unified factory
func createResponseHeaderFilter(responseHeaders *nginxv1.ProxyResponseHeaders, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPRouteFilter {
	if responseHeaders == nil {
		return nil
	}

	collector := ncommon.NewSliceNotificationCollector()

	var filtersHeaders []filters.Header
	for _, addHeader := range responseHeaders.Add {
		filtersHeaders = append(filtersHeaders, filters.Header{
			Name:  addHeader.Name,
			Value: addHeader.Value,
		})
		// Handle the Always flag - NGINX-specific feature
		if !addHeader.Always {
			collector.AddWarning("always flag is always true in gateway api", &vs)
		}
	}

	filter := filters.NewHTTPRouteFilter(filters.ResponseHeaderModifierFilter, filters.FilterOptions{
		HeaderModifier: &filters.HeaderModifierOptions{
			SetHeaders:    filtersHeaders,
			RemoveHeaders: responseHeaders.Hide,
		},
		NotificationCollector: collector,
		SourceObject:          &vs,
	})

	// Handle selective header passing/ignoring - NGINX-specific
	if len(responseHeaders.Pass) > 0 || len(responseHeaders.Ignore) > 0 {
		collector.AddWarning("Response header pass/ignore configuration is not supported in Gateway API", &vs)
	}

	*notifs = append(*notifs, collector.GetNotifications()...)

	return filter
}
