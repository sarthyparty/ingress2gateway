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

	"k8s.io/apimachinery/pkg/types"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	ncommon "github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/common"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/common/resources"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

// UpstreamConfig represents supported upstream configuration for conversion
type UpstreamConfig struct {
	Name    string // The name of the upstream
	Service string // The name of a service
	Port    uint16 // The port of the service
	Type    string // The type of the upstream (http or grpc)
	TLS     *nginxv1.UpstreamTLS // The TLS configuration for the Upstream
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

// populateUpstreamConfig fills the UpstreamConfig struct with essential fields needed for conversion
func populateUpstreamConfig(upstream *nginxv1.Upstream, vs *nginxv1.VirtualServer, notifs *[]notifications.Notification) *UpstreamConfig {
	// Generate warnings for unsupported fields during population
	checkUnsupportedUpstreamFields(upstream, vs, notifs)

	return &UpstreamConfig{
		Name:    upstream.Name,
		Service: upstream.Service,
		Port:    upstream.Port,
		Type:    upstream.Type,
		TLS:     &upstream.TLS,
	}
}

// checkUnsupportedUpstreamFields creates notifications for upstream fields that are not currently converted to Gateway API
func checkUnsupportedUpstreamFields(upstream *nginxv1.Upstream, vs *nginxv1.VirtualServer, notifs *[]notifications.Notification) {
	upstreamName := upstream.Name

	// Check load balancing method - only notify if explicitly specified and not least_conn
	if upstream.LBMethod != "" && upstream.LBMethod != "least_conn" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': LBMethod '%s' will be set to random two least_conn by default", upstreamName, upstream.LBMethod), vs)
	}

	// Check subselector
	if len(upstream.Subselector) > 0 {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Subselector field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check use-cluster-ip
	if upstream.UseClusterIP {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': UseClusterIP field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check fail timeout
	if upstream.FailTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': FailTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check max fails
	if upstream.MaxFails != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': MaxFails field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check max connections
	if upstream.MaxConns != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': MaxConns field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check keepalive
	if upstream.Keepalive != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Keepalive field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check connection timeout
	if upstream.ProxyConnectTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': ConnectTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check read timeout
	if upstream.ProxyReadTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': ReadTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check send timeout
	if upstream.ProxySendTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': SendTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check next upstream
	if upstream.ProxyNextUpstream != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NextUpstream field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check next upstream timeout
	if upstream.ProxyNextUpstreamTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NextUpstreamTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check next upstream tries
	if upstream.ProxyNextUpstreamTries != 0 {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NextUpstreamTries field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check client max body size
	if upstream.ClientMaxBodySize != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': ClientMaxBodySize field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check health check
	if upstream.HealthCheck != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': HealthCheck field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check slow start
	if upstream.SlowStart != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': SlowStart field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check queue
	if upstream.Queue != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Queue field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check buffering
	if upstream.ProxyBuffering != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Buffering field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check buffer size
	if upstream.ProxyBufferSize != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': BufferSize field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check NTLM
	if upstream.NTLM {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NTLM field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check backup
	if upstream.Backup != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Backup field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check backup port
	if upstream.BackupPort != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': BackupPort field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check session cookie
	if upstream.SessionCookie != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': SessionCookie field is not currently converted to Gateway API", upstreamName), vs)
	}
}

// processUpstreamTLSPolicies processes upstreams and creates only BackendTLSPolicy resources (GRPCRoute creation moved to route converter)
func processUpstreamTLSPolicies(vs nginxv1.VirtualServer, notifs *[]notifications.Notification) map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy {
	backendTLSPolicies := make(map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy)

	// Create notification collector for resource creation
	collector := ncommon.NewSliceNotificationCollector()

	for _, upstream := range vs.Spec.Upstreams {
		if !validateUpstream(&upstream, &vs, notifs) {
			continue
		}

		// Create BackendTLSPolicy if TLS is enabled
		if upstream.TLS.Enable {
			policyName := resources.GenerateBackendTLSPolicyName(upstream.Service, upstream.Name)
			policyKey := resources.GeneratePolicyKey(vs.Namespace, policyName)

			// Create BackendTLSPolicy using unified factory
			policy := resources.CreateBackendTLSPolicy(resources.PolicyOptions{
				BackendTLS: resources.NewBackendTLSPolicyOptions(
					policyName,
					vs.Namespace,
					upstream.Service,
					"nginx-virtualserver-tls",
				),
				NotificationCollector: collector,
				SourceObject:          &vs,
			})

			if policy != nil {
				backendTLSPolicies[policyKey] = *policy
			}
		}
	}

	// Merge notifications from factory into the main notification list
	*notifs = append(*notifs, collector.GetNotifications()...)

	return backendTLSPolicies
}
