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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

// convertVirtualServerToGateway creates a Gateway from VirtualServer
func convertVirtualServerToGateway(vs nginxv1.VirtualServer) (intermediate.GatewayContext, []notifications.Notification) {
	var notificationList []notifications.Notification

	// Create listener for the VirtualServer
	listener := createListener(vs, &notificationList)

	// Create Gateway
	gateway := gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1.GroupVersion.String(),
			Kind:       "Gateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateGatewayName(vs),
			Namespace: vs.Namespace,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(getGatewayClassName(vs)),
			Listeners:        []gatewayv1.Listener{listener},
		},
	}

	// Create nginx-specific Gateway IR
	nginxGatewayIR := createNginxGatewayIR(vs, &notificationList)

	return intermediate.GatewayContext{
		Gateway: gateway,
		ProviderSpecificIR: intermediate.ProviderSpecificGatewayIR{
			Nginx: nginxGatewayIR,
		},
	}, notificationList
}

// createListener creates a Gateway listener from VirtualServer configuration
func createListener(vs nginxv1.VirtualServer, notifs *[]notifications.Notification) gatewayv1.Listener {
	// Default HTTP listener
	listener := gatewayv1.Listener{
		Name:     gatewayv1.SectionName("http"),
		Port:     gatewayv1.PortNumber(defaultHTTPPort),
		Protocol: gatewayv1.HTTPProtocolType,
		Hostname: Ptr(gatewayv1.Hostname(vs.Spec.Host)),
	}

	// Configure TLS if specified
	if vs.Spec.TLS != nil {
		configureTLS(&listener, vs, notifs)
	}

	return listener
}

// configureTLS configures TLS settings for the listener
func configureTLS(listener *gatewayv1.Listener, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) {
	listener.Name = gatewayv1.SectionName("https")
	listener.Port = gatewayv1.PortNumber(defaultHTTPSPort)
	listener.Protocol = gatewayv1.HTTPSProtocolType

	// Configure TLS certificate
	if secret := vs.Spec.TLS.Secret; secret != "" {
		listener.TLS = &gatewayv1.GatewayTLSConfig{
			Mode:            Ptr(gatewayv1.TLSModeTerminate),
			CertificateRefs: []gatewayv1.SecretObjectReference{{Name: gatewayv1.ObjectName(secret)}},
		}
	}

	// Handle TLS redirect configuration
	if vs.Spec.TLS.Redirect != nil && vs.Spec.TLS.Redirect.Enable {
		addNotification(notifs, notifications.InfoNotification,
			"TLS redirect configuration found, consider implementing via HTTPRoute redirect filter", &vs)
	}
}

// createNginxGatewayIR creates nginx-specific Gateway IR
func createNginxGatewayIR(vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *intermediate.NginxGatewayIR {
	nginxGatewayIR := &intermediate.NginxGatewayIR{}

	// Store TLS configuration
	if vs.Spec.TLS != nil {
		nginxGatewayIR.TLSTermination = &intermediate.NginxTLSConfig{
			SecretName:      vs.Spec.TLS.Secret,
			TerminationMode: "terminate",
		}
	}

	return nginxGatewayIR
}