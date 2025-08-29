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
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

// CRDsToGatewayIR converts nginx VirtualServer, VirtualServerRoute, and TransportServer CRDs to Gateway API resources
// This function creates one shared Gateway per namespace that handles both Layer 7 and Layer 4 traffic
func CRDsToGatewayIR(
	virtualServers []nginxv1.VirtualServer,
	virtualServerRoutes []nginxv1.VirtualServerRoute,
	transportServers []nginxv1.TransportServer,
	globalConfiguration *nginxv1.GlobalConfiguration) (
	partial intermediate.IR,
	notificationList []notifications.Notification,
	errs field.ErrorList,
) {
	resolver := NewRouteResolver(virtualServers, virtualServerRoutes)

	notificationList = make([]notifications.Notification, 0)

	var validVirtualServers []nginxv1.VirtualServer
	for _, vs := range virtualServers {
		if vs.Spec.Host == "" {
			addNotification(&notificationList, notifications.WarningNotification,
				"VirtualServer has no host specified, skipping", &vs)
			continue
		}
		validVirtualServers = append(validVirtualServers, vs)
	}

	// Check if we have any resources to process
	if len(validVirtualServers) == 0 && len(transportServers) == 0 {
		return intermediate.IR{}, notificationList, errs
	}

	// Group resources by namespace
	namespaceVSMap := make(map[string][]nginxv1.VirtualServer)
	for _, vs := range validVirtualServers {
		namespaceVSMap[vs.Namespace] = append(namespaceVSMap[vs.Namespace], vs)
	}

	namespaceTSMap := make(map[string][]nginxv1.TransportServer)
	for _, ts := range transportServers {
		namespaceTSMap[ts.Namespace] = append(namespaceTSMap[ts.Namespace], ts)
	}

	// Initialize result maps
	gatewayMap := make(map[types.NamespacedName]intermediate.GatewayContext)
	httpRouteMap := make(map[types.NamespacedName]intermediate.HTTPRouteContext)
	backendTLSPoliciesMap := make(map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy)
	grpcRouteMap := make(map[types.NamespacedName]gatewayv1.GRPCRoute)
	tcpRouteMap := make(map[types.NamespacedName]gatewayv1alpha2.TCPRoute)
	tlsRouteMap := make(map[types.NamespacedName]gatewayv1alpha2.TLSRoute)
	udpRouteMap := make(map[types.NamespacedName]gatewayv1alpha2.UDPRoute)

	// Build a listener map
	listenerMap := make(map[string]gatewayv1.Listener)
	if globalConfiguration != nil {
		for _, l := range globalConfiguration.Spec.Listeners {
			listenerMap[l.Name] = gatewayv1.Listener{
				Name:     gatewayv1.SectionName(l.Name),
				Port:     gatewayv1.PortNumber(l.Port),
				Protocol: gatewayv1.ProtocolType(l.Protocol),
			}
		}
	}

	// Get all namespaces that have either VirtualServers or TransportServers
	allNamespaces := make(map[string]bool)
	for namespace := range namespaceVSMap {
		allNamespaces[namespace] = true
	}
	for namespace := range namespaceTSMap {
		allNamespaces[namespace] = true
	}

	for namespace := range allNamespaces {
		vsListForNamespace := namespaceVSMap[namespace] // May be empty slice
		tsListForNamespace := namespaceTSMap[namespace] // May be empty slice

		// Create shared gateway for both VirtualServers and TransportServers
		gatewayFactory := NewNamespaceGatewayFactory(namespace, vsListForNamespace, tsListForNamespace, &notificationList, listenerMap)
		gateways, virtualServerMap := gatewayFactory.CreateNamespaceGateway()

		for gatewayKey, gateway := range gateways {
			gatewayMap[gatewayKey] = gateway
		}

		// Convert each VirtualServer to routes (HTTPRoute or GRPCRoute)
		for _, vs := range vsListForNamespace {
			// Check for unsupported VirtualServer fields
			checkUnsupportedVirtualServerFields(vs, &notificationList)

			if vs.Spec.TLS != nil && vs.Spec.TLS.Redirect != nil && vs.Spec.TLS.Redirect.Enable {
				httpRouteMap[types.NamespacedName{Namespace: vs.Namespace, Name: vs.Name + "-redirect"}] = *createRedirectHTTPRoute(vs, listenerMap)
			}

			// First, process all upstreams and create config structs
			upstreamConfigs := make(map[string]*UpstreamConfig)
			for _, upstream := range vs.Spec.Upstreams {
				if validateUpstream(&upstream, &vs, &notificationList) {
					config := populateUpstreamConfig(&upstream, &vs, &notificationList)
					upstreamConfigs[upstream.Name] = config
				}
			}

			// Create HTTPRoute/GRPCRoute converter with upstream configs
			converter := NewVirtualServerRouteConverter(vs, resolver, virtualServerMap, &notificationList, listenerMap, upstreamConfigs)
			httpRoutes, grpcRoutes := converter.ConvertToRoutes()

			// Add HTTPRoutes to map
			for httpRouteKey, httpRoute := range httpRoutes {
				httpRouteMap[httpRouteKey] = httpRoute
			}

			// Add GRPCRoutes from converter to map
			for routeKey, grpcRoute := range grpcRoutes {
				grpcRouteMap[routeKey] = grpcRoute
			}

			// Process upstream TLS policies only
			backendTLSPolicies := processUpstreamTLSPolicies(vs, &notificationList)
			for policyKey, policy := range backendTLSPolicies {
				backendTLSPoliciesMap[policyKey] = policy
			}
		}

		// Convert each TransportServer to routes (TCPRoute, TLSRoute, or UDPRoute)
		for _, ts := range tsListForNamespace {
			// Validate that listener field is present (required for TransportServer)
			if ts.Spec.Listener.Name == "" || ts.Spec.Listener.Protocol == "" {
				notificationList = append(notificationList, notifications.Notification{
					Type:    notifications.WarningNotification,
					Message: fmt.Sprintf("TransportServer '%s' skipped: listener field is required but not specified", ts.Name),
				})
				continue
			}

			if _, exists := listenerMap[ts.Spec.Listener.Name]; !exists {
				notificationList = append(notificationList, notifications.Notification{
					Type:    notifications.WarningNotification,
					Message: fmt.Sprintf("TransportServer '%s' skipped: listener '%s' not found in GlobalConfiguration", ts.Name, ts.Spec.Listener.Name),
				})
				continue
			}

			converter := NewTransportServerConverter(ts, &notificationList, listenerMap)
			tcpRoutes, tlsRoutes, udpRoutes := converter.ConvertToRoutes()

			// Add routes to maps
			maps.Copy(tcpRouteMap, tcpRoutes)
			maps.Copy(tlsRouteMap, tlsRoutes)
			maps.Copy(udpRouteMap, udpRoutes)
		}
	}

	return intermediate.IR{
		Gateways:           gatewayMap,
		HTTPRoutes:         httpRouteMap,
		BackendTLSPolicies: backendTLSPoliciesMap,
		GRPCRoutes:         grpcRouteMap,
		TCPRoutes:          tcpRouteMap,
		TLSRoutes:          tlsRouteMap,
		UDPRoutes:          udpRouteMap,
	}, notificationList, errs
}

func createRedirectHTTPRoute(vs nginxv1.VirtualServer, listenerMap map[string]gatewayv1.Listener) *intermediate.HTTPRouteContext {
	port := 80
	if vs.Spec.Listener != nil && vs.Spec.Listener.HTTP != "" {
		port = int(listenerMap[vs.Spec.Listener.HTTP].Port)
	}
	return &intermediate.HTTPRouteContext{
		HTTPRoute: gatewayv1.HTTPRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: gatewayv1.GroupVersion.String(),
				Kind:       "HTTPRoute",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vs.Name + "-redirect",
				Namespace: vs.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "ingress2gateway",
					"ingress2gateway.io/source":    "nginx-virtualserver",
					"ingress2gateway.io/vs-name":   vs.Name,
				},
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{
						{
							Name:        gatewayv1.ObjectName(vs.Namespace + "-gateway"),
							SectionName: (*gatewayv1.SectionName)(Ptr(fmt.Sprintf("http-%d-%s", port, sanitizeHostname(vs.Spec.Host)))),
						},
					},
				},
				Rules: []gatewayv1.HTTPRouteRule{
					{
						Matches: []gatewayv1.HTTPRouteMatch{
							{
								Path: &gatewayv1.HTTPPathMatch{
									Type:  Ptr(gatewayv1.PathMatchPathPrefix),
									Value: Ptr("/"),
								},
							},
						},
						Filters: []gatewayv1.HTTPRouteFilter{
							{
								Type: gatewayv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
									StatusCode: vs.Spec.TLS.Redirect.Code,
								},
							},
						},
					},
				},
			},
		},
	}
}

// checkUnsupportedVirtualServerFields checks for VirtualServer fields that are not supported in Gateway API conversion
func checkUnsupportedVirtualServerFields(vs nginxv1.VirtualServer, notifs *[]notifications.Notification) {
	// Check for Gunzip field
	if vs.Spec.Gunzip {
		addNotification(notifs, notifications.WarningNotification,
			"VirtualServer field 'gunzip' is not supported in Gateway API conversion", &vs)
	}

	// Check for ExternalDNS field
	if vs.Spec.ExternalDNS.Enable {
		addNotification(notifs, notifications.WarningNotification,
			"VirtualServer field 'externalDNS' is not supported in Gateway API conversion", &vs)
	}

	// Check for DOS field
	if vs.Spec.Dos != "" {
		addNotification(notifs, notifications.WarningNotification,
			"VirtualServer field 'dos' is not supported in Gateway API conversion", &vs)
	}

	// Check for Policies field
	if len(vs.Spec.Policies) > 0 {
		addNotification(notifs, notifications.WarningNotification,
			fmt.Sprintf("VirtualServer field 'policies' (%d policies) is not supported in Gateway API conversion", len(vs.Spec.Policies)), &vs)
	}

	// Check for InternalRoute field
	if vs.Spec.InternalRoute {
		addNotification(notifs, notifications.WarningNotification,
			"VirtualServer field 'internalRoute' is not supported in Gateway API conversion", &vs)
	}

	// Check for HTTPSnippets field
	if vs.Spec.HTTPSnippets != "" {
		addNotification(notifs, notifications.WarningNotification,
			"VirtualServer field 'http-snippets' is not supported in Gateway API conversion", &vs)
	}

	// Check for ServerSnippets field
	if vs.Spec.ServerSnippets != "" {
		addNotification(notifs, notifications.WarningNotification,
			"VirtualServer field 'server-snippets' is not supported in Gateway API conversion", &vs)
	}
}
