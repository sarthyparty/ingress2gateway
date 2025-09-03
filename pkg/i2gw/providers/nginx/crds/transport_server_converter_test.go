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
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

func TestTransportServerConverter_ConvertToRoutes(t *testing.T) {
	tests := []struct {
		name              string
		transportServer   nginxv1.TransportServer
		listenerMap       map[string]gatewayv1.Listener
		expectedTCPRoutes int
		expectedTLSRoutes int
		expectedUDPRoutes int
	}{
		{
			name: "TCP TransportServer",
			transportServer: nginxv1.TransportServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mysql-proxy",
					Namespace: "default",
				},
				Spec: nginxv1.TransportServerSpec{
					Listener: nginxv1.TransportServerListener{
						Name:     "mysql-tcp",
						Protocol: "TCP",
					},
					Upstreams: []nginxv1.TransportServerUpstream{
						{
							Name:    "mysql-backend",
							Service: "mysql-service",
							Port:    3306,
						},
					},
					Action: &nginxv1.TransportServerAction{
						Pass: "mysql-backend",
					},
				},
			},
			listenerMap: map[string]gatewayv1.Listener{
				"mysql-tcp": {
					Name:     "mysql-tcp",
					Port:     3306,
					Protocol: gatewayv1.TCPProtocolType,
				},
			},
			expectedTCPRoutes: 1,
			expectedTLSRoutes: 0,
			expectedUDPRoutes: 0,
		},
		{
			name: "UDP TransportServer",
			transportServer: nginxv1.TransportServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dns-server",
					Namespace: "default",
				},
				Spec: nginxv1.TransportServerSpec{
					Listener: nginxv1.TransportServerListener{
						Name:     "dns-udp",
						Protocol: "UDP",
					},
					Upstreams: []nginxv1.TransportServerUpstream{
						{
							Name:    "dns-backend",
							Service: "coredns",
							Port:    53,
						},
					},
					Action: &nginxv1.TransportServerAction{
						Pass: "dns-backend",
					},
				},
			},
			listenerMap: map[string]gatewayv1.Listener{
				"dns-udp": {
					Name:     "dns-udp",
					Port:     5353,
					Protocol: gatewayv1.UDPProtocolType,
				},
			},
			expectedTCPRoutes: 0,
			expectedTLSRoutes: 0,
			expectedUDPRoutes: 1,
		},
		{
			name: "TLS Passthrough TransportServer",
			transportServer: nginxv1.TransportServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secure-app",
					Namespace: "default",
				},
				Spec: nginxv1.TransportServerSpec{
					Listener: nginxv1.TransportServerListener{
						Name:     "tls-passthrough",
						Protocol: "TLS_PASSTHROUGH",
					},
					Host: "secure.example.com",
					Upstreams: []nginxv1.TransportServerUpstream{
						{
							Name:    "secure-backend",
							Service: "secure-app-service",
							Port:    8443,
						},
					},
					Action: &nginxv1.TransportServerAction{
						Pass: "secure-backend",
					},
				},
			},
			listenerMap: map[string]gatewayv1.Listener{
				"tls-passthrough": {
					Name:     "tls-passthrough",
					Port:     443,
					Protocol: gatewayv1.TLSProtocolType,
				},
			},
			expectedTCPRoutes: 0,
			expectedTLSRoutes: 1,
			expectedUDPRoutes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notificationList []notifications.Notification
			converter := NewTransportServerConverter(tt.transportServer, &notificationList, tt.listenerMap)

			tcpRoutes, tlsRoutes, udpRoutes := converter.ConvertToRoutes()

			if len(tcpRoutes) != tt.expectedTCPRoutes {
				t.Errorf("Expected %d TCP routes, got %d", tt.expectedTCPRoutes, len(tcpRoutes))
			}

			if len(tlsRoutes) != tt.expectedTLSRoutes {
				t.Errorf("Expected %d TLS routes, got %d", tt.expectedTLSRoutes, len(tlsRoutes))
			}

			if len(udpRoutes) != tt.expectedUDPRoutes {
				t.Errorf("Expected %d UDP routes, got %d", tt.expectedUDPRoutes, len(udpRoutes))
			}
		})
	}
}

func TestTransportServerValidation(t *testing.T) {
	tests := []struct {
		name                string
		transportServers    []nginxv1.TransportServer
		globalConfiguration *nginxv1.GlobalConfiguration
		expectedTCPRoutes   int
		expectedWarnings    int
	}{
		{
			name: "Valid TransportServer with GlobalConfiguration",
			transportServers: []nginxv1.TransportServer{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysql-proxy",
						Namespace: "default",
					},
					Spec: nginxv1.TransportServerSpec{
						Listener: nginxv1.TransportServerListener{
							Name:     "db-tcp",
							Protocol: "TCP",
						},
						Upstreams: []nginxv1.TransportServerUpstream{
							{
								Name:    "mysql-backend",
								Service: "mysql-service",
								Port:    3306,
							},
						},
						Action: &nginxv1.TransportServerAction{
							Pass: "mysql-backend",
						},
					},
				},
			},
			globalConfiguration: &nginxv1.GlobalConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-configuration",
					Namespace: "nginx-ingress",
				},
				Spec: nginxv1.GlobalConfigurationSpec{
					Listeners: []nginxv1.Listener{
						{
							Name:     "db-tcp",
							Port:     3306,
							Protocol: "TCP",
						},
					},
				},
			},
			expectedTCPRoutes: 1,
			expectedWarnings:  0,
		},
		{
			name: "TransportServer with missing listener",
			transportServers: []nginxv1.TransportServer{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysql-proxy",
						Namespace: "default",
					},
					Spec: nginxv1.TransportServerSpec{
						Listener: nginxv1.TransportServerListener{
							Name:     "missing-listener",
							Protocol: "TCP",
						},
						Upstreams: []nginxv1.TransportServerUpstream{
							{
								Name:    "mysql-backend",
								Service: "mysql-service",
								Port:    3306,
							},
						},
						Action: &nginxv1.TransportServerAction{
							Pass: "mysql-backend",
						},
					},
				},
			},
			globalConfiguration: &nginxv1.GlobalConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-configuration",
					Namespace: "nginx-ingress",
				},
				Spec: nginxv1.GlobalConfigurationSpec{
					Listeners: []nginxv1.Listener{
						{
							Name:     "db-tcp",
							Port:     3306,
							Protocol: "TCP",
						},
					},
				},
			},
			expectedTCPRoutes: 0,
			expectedWarnings:  1,
		},
		{
			name: "TransportServer with no GlobalConfiguration",
			transportServers: []nginxv1.TransportServer{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysql-proxy",
						Namespace: "default",
					},
					Spec: nginxv1.TransportServerSpec{
						Listener: nginxv1.TransportServerListener{
							Name:     "db-tcp",
							Protocol: "TCP",
						},
						Upstreams: []nginxv1.TransportServerUpstream{
							{
								Name:    "mysql-backend",
								Service: "mysql-service",
								Port:    3306,
							},
						},
						Action: &nginxv1.TransportServerAction{
							Pass: "mysql-backend",
						},
					},
				},
			},
			globalConfiguration: nil,
			expectedTCPRoutes:   0,
			expectedWarnings:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, notifications, errs := CRDsToGatewayIR(
				[]nginxv1.VirtualServer{},
				[]nginxv1.VirtualServerRoute{},
				tt.transportServers,
				tt.globalConfiguration,
			)

			if len(errs) > 0 {
				t.Errorf("Unexpected errors: %v", errs)
			}

			if len(ir.TCPRoutes) != tt.expectedTCPRoutes {
				t.Errorf("Expected %d TCP routes, got %d", tt.expectedTCPRoutes, len(ir.TCPRoutes))
			}

			// Count warnings
			warningCount := 0
			for _, notif := range notifications {
				if notif.Type == "WARNING" {
					warningCount++
				}
			}

			if warningCount != tt.expectedWarnings {
				t.Errorf("Expected %d warnings, got %d", tt.expectedWarnings, warningCount)
			}
		})
	}
}
