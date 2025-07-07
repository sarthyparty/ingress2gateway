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

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

// UpstreamConfig represents upstream configuration for conversion
type UpstreamConfig struct {
	Name        string
	Service     string
	Port        uint16
	Weight      *int32
	LBMethod    string
	HealthCheck *nginxv1.HealthCheck
	TLS         *nginxv1.UpstreamTLS
	// Add more fields as needed for future enhancements
}

// convertUpstreamToBackendRef creates a Gateway API backend reference from upstream
func convertUpstreamToBackendRef(upstream *nginxv1.Upstream, weight *int32) gatewayv1.HTTPBackendRef {
	backendRef := gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(upstream.Service),
				Port: Ptr(gatewayv1.PortNumber(upstream.Port)),
			},
		},
	}

	if weight != nil {
		backendRef.BackendRef.Weight = weight
	}

	return backendRef
}

// validateUpstream performs basic validation on upstream configuration
func validateUpstream(upstream *nginxv1.Upstream, vs *nginxv1.VirtualServer, notifs *[]notifications.Notification) bool {
	if upstream.Service == "" {
		addNotification(notifs, notifications.WarningNotification,
			fmt.Sprintf("Upstream '%s' has no service specified", upstream.Name), vs)
		return false
	}

	if upstream.Port == 0 {
		addNotification(notifs, notifications.WarningNotification,
			fmt.Sprintf("Upstream '%s' has no port specified", upstream.Name), vs)
		return false
	}

	return true
}

// extractUpstreamFeatures extracts nginx-specific upstream features for provider IR
func extractUpstreamFeatures(upstream *nginxv1.Upstream, vs *nginxv1.VirtualServer, notifs *[]notifications.Notification) *intermediate.NginxUpstreamConfig {
	config := &intermediate.NginxUpstreamConfig{
		Name:     upstream.Name,
		Service:  upstream.Service,
		Port:     upstream.Port,
		LBMethod: upstream.LBMethod,
	}

	// Handle load balancing method
	if upstream.LBMethod != "" && upstream.LBMethod != "round_robin" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Load balancing method '%s' for upstream '%s' stored in provider-specific IR", upstream.LBMethod, upstream.Name), vs)
		config.LBMethod = upstream.LBMethod
	}

	// Handle connection settings (for future enhancement)
	if upstream.Keepalive != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Keepalive setting for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.Keepalive = upstream.Keepalive
	}

	// Handle health checks (for future enhancement)
	if upstream.HealthCheck != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Health check configuration for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.HealthCheck = upstream.HealthCheck
	}

	// Handle upstream TLS (for future enhancement)
	if upstream.TLS.Enable {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream TLS configuration for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.TLS = &upstream.TLS
	}

	// Handle connection limits (for future enhancement)
	if upstream.MaxConns != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Connection limits for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.MaxConns = upstream.MaxConns
	}

	// Handle timeouts (for future enhancement)
	if upstream.ProxyConnectTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Connection timeout for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.ConnectTimeout = upstream.ProxyConnectTimeout
	}

	if upstream.ProxyReadTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Read timeout for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.ReadTimeout = upstream.ProxyReadTimeout
	}

	if upstream.ProxySendTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Send timeout for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.SendTimeout = upstream.ProxySendTimeout
	}

	// Handle additional advanced features
	extractAdvancedUpstreamFeatures(upstream, vs, config, notifs)

	return config
}

// extractAdvancedUpstreamFeatures handles additional upstream configuration
func extractAdvancedUpstreamFeatures(upstream *nginxv1.Upstream, vs *nginxv1.VirtualServer, config *intermediate.NginxUpstreamConfig, notifs *[]notifications.Notification) {
	// Handle fail timeout and max fails
	if upstream.FailTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Fail timeout for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.FailTimeout = upstream.FailTimeout
	}

	if upstream.MaxFails != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Max fails setting for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.MaxFails = upstream.MaxFails
	}

	// Handle proxy next upstream settings
	if upstream.ProxyNextUpstream != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Proxy next upstream for '%s' stored in provider-specific IR", upstream.Name), vs)
	}

	// Handle buffering settings
	if upstream.ProxyBuffering != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Proxy buffering for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.ProxyBuffering = upstream.ProxyBuffering
	}

	// Handle client max body size
	if upstream.ClientMaxBodySize != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Client max body size for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
	}

	// Handle session persistence
	if upstream.SessionCookie != nil {
		handleSessionPersistence(upstream, vs, config, notifs)
	}

	// Handle advanced queue settings
	if upstream.Queue != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Queue settings for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.Queue = upstream.Queue
	}

	// Handle slow start
	if upstream.SlowStart != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Slow start for upstream '%s' stored in provider-specific IR", upstream.Name), vs)
		config.SlowStart = upstream.SlowStart
	}
}

// handleSessionPersistence processes session cookie configuration
func handleSessionPersistence(upstream *nginxv1.Upstream, vs *nginxv1.VirtualServer, config *intermediate.NginxUpstreamConfig, notifs *[]notifications.Notification) {
	if upstream.SessionCookie == nil {
		return
	}

	addNotification(notifs, notifications.InfoNotification,
		fmt.Sprintf("Session persistence configured for upstream '%s' - stored in provider-specific IR", upstream.Name), vs)

	config.SessionCookie = upstream.SessionCookie
}

// processUpstreams processes all upstreams in a VirtualServer for provider-specific features
func processUpstreams(vs nginxv1.VirtualServer, notifs *[]notifications.Notification) map[string]*intermediate.NginxUpstreamConfig {
	upstreamConfigs := make(map[string]*intermediate.NginxUpstreamConfig)

	for _, upstream := range vs.Spec.Upstreams {
		if validateUpstream(&upstream, &vs, notifs) {
			config := extractUpstreamFeatures(&upstream, &vs, notifs)
			upstreamConfigs[upstream.Name] = config
		}
	}

	return upstreamConfigs
}
