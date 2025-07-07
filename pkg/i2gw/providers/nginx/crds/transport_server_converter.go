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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/common"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

// TransportServerConverter converts TransportServer resources to Gateway API TLS/TCP/UDP routes
type TransportServerConverter struct {
	transportServer  nginxv1.TransportServer
	notificationList *[]notifications.Notification
	listenerMap      map[string]gatewayv1.Listener
}

// NewTransportServerConverter creates a new TransportServer converter
func NewTransportServerConverter(
	ts nginxv1.TransportServer,
	notifs *[]notifications.Notification,
	listenerMap map[string]gatewayv1.Listener,
) *TransportServerConverter {
	return &TransportServerConverter{
		transportServer:  ts,
		notificationList: notifs,
		listenerMap:      listenerMap,
	}
}

// ConvertToRoutes converts TransportServer to appropriate Gateway API route resources
func (c *TransportServerConverter) ConvertToRoutes() (
	map[types.NamespacedName]gatewayv1alpha2.TCPRoute,
	map[types.NamespacedName]gatewayv1alpha2.TLSRoute,
	map[types.NamespacedName]gatewayv1alpha2.UDPRoute,
) {
	tcpRoutes := make(map[types.NamespacedName]gatewayv1alpha2.TCPRoute)
	tlsRoutes := make(map[types.NamespacedName]gatewayv1alpha2.TLSRoute)
	udpRoutes := make(map[types.NamespacedName]gatewayv1alpha2.UDPRoute)

	protocol := c.getProtocolType()
	switch protocol {
	case "TCP":
		if c.transportServer.Spec.TLS != nil {
			// TCP with TLS termination - still creates TCPRoute but with TLS listener
			tcpRoute, routeKey := c.createTCPRoute()
			tcpRoutes[routeKey] = tcpRoute
		} else {
			// Plain TCP
			tcpRoute, routeKey := c.createTCPRoute()
			tcpRoutes[routeKey] = tcpRoute
		}
	case "TLS_PASSTHROUGH":
		// TLS passthrough - creates TLSRoute
		tlsRoute, routeKey := c.createTLSRoute()
		tlsRoutes[routeKey] = tlsRoute
	case "UDP":
		// UDP
		udpRoute, routeKey := c.createUDPRoute()
		udpRoutes[routeKey] = udpRoute
	default:
		c.addNotification(notifications.ErrorNotification,
			fmt.Sprintf("Unsupported protocol '%s' in TransportServer '%s'", protocol, c.transportServer.Name))
	}

	return tcpRoutes, tlsRoutes, udpRoutes
}

// getProtocolType determines the protocol type from TransportServer configuration
func (c *TransportServerConverter) getProtocolType() string {
	if c.transportServer.Spec.Listener.Protocol == "" {
		c.addNotification(notifications.ErrorNotification,
			fmt.Sprintf("TransportServer '%s' has no listener protocol configured", c.transportServer.Name))
		return ""
	}
	return c.transportServer.Spec.Listener.Protocol
}

// createTCPRoute creates a TCPRoute resource
func (c *TransportServerConverter) createTCPRoute() (gatewayv1alpha2.TCPRoute, types.NamespacedName) {
	routeName := c.transportServer.Name + "-tcproute"
	routeKey := types.NamespacedName{
		Namespace: c.transportServer.Namespace,
		Name:      routeName,
	}

	tcpRoute := gatewayv1alpha2.TCPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1alpha2.GroupVersion.String(),
			Kind:       "TCPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: c.transportServer.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ingress2gateway",
				"ingress2gateway.io/source":    "nginx-transportserver",
				"ingress2gateway.io/ts-name":   c.transportServer.Name,
			},
		},
		Spec: gatewayv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: c.createParentRefs(),
			},
			Rules: []gatewayv1alpha2.TCPRouteRule{
				{
					BackendRefs: c.createBackendRefs(),
				},
			},
		},
	}

	c.addNotification(notifications.InfoNotification,
		fmt.Sprintf("Created TCPRoute '%s' for TransportServer '%s'", routeName, c.transportServer.Name))

	return tcpRoute, routeKey
}

// createTLSRoute creates a TLSRoute resource for TLS passthrough
func (c *TransportServerConverter) createTLSRoute() (gatewayv1alpha2.TLSRoute, types.NamespacedName) {
	routeName := c.transportServer.Name + "-tlsroute"
	routeKey := types.NamespacedName{
		Namespace: c.transportServer.Namespace,
		Name:      routeName,
	}

	tlsRoute := gatewayv1alpha2.TLSRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1alpha2.GroupVersion.String(),
			Kind:       "TLSRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: c.transportServer.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ingress2gateway",
				"ingress2gateway.io/source":    "nginx-transportserver",
				"ingress2gateway.io/ts-name":   c.transportServer.Name,
			},
		},
		Spec: gatewayv1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: c.createParentRefs(),
			},
			Rules: []gatewayv1alpha2.TLSRouteRule{
				{
					BackendRefs: c.createBackendRefs(),
				},
			},
		},
	}

	// Add hostname for SNI matching if specified
	if c.transportServer.Spec.Host != "" {
		tlsRoute.Spec.Hostnames = []gatewayv1alpha2.Hostname{
			gatewayv1alpha2.Hostname(c.transportServer.Spec.Host),
		}
	}

	c.addNotification(notifications.InfoNotification,
		fmt.Sprintf("Created TLSRoute '%s' for TransportServer '%s'", routeName, c.transportServer.Name))

	return tlsRoute, routeKey
}

// createUDPRoute creates a UDPRoute resource
func (c *TransportServerConverter) createUDPRoute() (gatewayv1alpha2.UDPRoute, types.NamespacedName) {
	routeName := c.transportServer.Name + "-udproute"
	routeKey := types.NamespacedName{
		Namespace: c.transportServer.Namespace,
		Name:      routeName,
	}

	udpRoute := gatewayv1alpha2.UDPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1alpha2.GroupVersion.String(),
			Kind:       "UDPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: c.transportServer.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ingress2gateway",
				"ingress2gateway.io/source":    "nginx-transportserver",
				"ingress2gateway.io/ts-name":   c.transportServer.Name,
			},
		},
		Spec: gatewayv1alpha2.UDPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: c.createParentRefs(),
			},
			Rules: []gatewayv1alpha2.UDPRouteRule{
				{
					BackendRefs: c.createBackendRefs(),
				},
			},
		},
	}

	c.addNotification(notifications.InfoNotification,
		fmt.Sprintf("Created UDPRoute '%s' for TransportServer '%s'", routeName, c.transportServer.Name))

	return udpRoute, routeKey
}

// createParentRefs creates parent references to Gateway listeners
func (c *TransportServerConverter) createParentRefs() []gatewayv1.ParentReference {
	// For now, use a simple naming convention for the gateway and listener
	gatewayName := c.transportServer.Namespace + "-gateway"

	// Determine the listener name based on protocol and port
	listenerName := c.generateListenerName()

	return []gatewayv1.ParentReference{
		{
			Name:        gatewayv1.ObjectName(gatewayName),
			SectionName: (*gatewayv1.SectionName)(&listenerName),
		},
	}
}

// generateListenerName creates a listener name based on protocol, port, and hostname
func (c *TransportServerConverter) generateListenerName() string {
	protocol := strings.ToLower(c.getProtocolType())
	port := c.getListenerPort()

	// Handle different protocol naming
	switch protocol {
	case "tls_passthrough":
		protocol = "tls"
	}

	if c.transportServer.Spec.Host != "" {
		hostname := sanitizeHostname(c.transportServer.Spec.Host)
		return fmt.Sprintf("%s-%d-%s", protocol, port, hostname)
	}

	return fmt.Sprintf("%s-%d", protocol, port)
}

// getListenerPort determines the port from GlobalConfiguration listener or defaults
func (c *TransportServerConverter) getListenerPort() int {
	if c.transportServer.Spec.Listener.Name == "" {
		c.addNotification(notifications.ErrorNotification,
			fmt.Sprintf("TransportServer '%s' has no listener name configured", c.transportServer.Name))
		return 80 // fallback port
	}
	
	listenerName := c.transportServer.Spec.Listener.Name
	if listener, exists := c.listenerMap[listenerName]; exists {
		return int(listener.Port)
	}

	// Handle built-in listeners
	if listenerName == "tls-passthrough" {
		return 443 // Default TLS passthrough port
	}

	// Default ports by protocol
	switch c.getProtocolType() {
	case "TCP":
		return 80
	case "UDP":
		return 53 // Common UDP port for DNS
	default:
		return 80
	}
}

// createBackendRefs converts TransportServer upstreams to Gateway API backend references
func (c *TransportServerConverter) createBackendRefs() []gatewayv1.BackendRef {
	if c.transportServer.Spec.Action == nil || c.transportServer.Spec.Action.Pass == "" {
		c.addNotification(notifications.WarningNotification,
			fmt.Sprintf("TransportServer '%s' has no action.pass configured", c.transportServer.Name))
		return []gatewayv1.BackendRef{}
	}

	// Find the upstream referenced by action.pass
	upstreamName := c.transportServer.Spec.Action.Pass
	var targetUpstream *nginxv1.TransportServerUpstream

	for _, upstream := range c.transportServer.Spec.Upstreams {
		if upstream.Name == upstreamName {
			targetUpstream = &upstream
			break
		}
	}

	if targetUpstream == nil {
		c.addNotification(notifications.ErrorNotification,
			fmt.Sprintf("Upstream '%s' not found in TransportServer '%s'", upstreamName, c.transportServer.Name))
		return []gatewayv1.BackendRef{}
	}

	return []gatewayv1.BackendRef{
		{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name:      gatewayv1.ObjectName(targetUpstream.Service),
				Namespace: (*gatewayv1.Namespace)(&c.transportServer.Namespace),
				Port:      Ptr(gatewayv1.PortNumber(targetUpstream.Port)),
				Kind:      Ptr(gatewayv1.Kind(common.ServiceKind)),
				Group:     Ptr(gatewayv1.Group(common.CoreGroup)),
			},
		},
	}
}

// addNotification adds a notification to the notification list
func (c *TransportServerConverter) addNotification(messageType notifications.MessageType, message string) {
	*c.notificationList = append(*c.notificationList, notifications.Notification{
		Type:    messageType,
		Message: message,
	})
}
