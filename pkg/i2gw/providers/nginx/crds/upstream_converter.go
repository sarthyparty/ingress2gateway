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

// UpstreamConfig represents upstream configuration for conversion
type UpstreamConfig struct {
	Name                string                 // The name of the upstream. Must be a valid DNS label as defined in RFC 1035.
	Service             string                 // The name of a service. The service must belong to the same namespace as the resource.
	Subselector         map[string]string      // Selects the pods within the service using label keys and values.
	UseClusterIP        *bool                  // Enables using the Cluster IP and port of the service instead of the default behavior.
	Port                uint16                 // The port of the service. Must fall into the range 1..65535.
	LBMethod            string                 // The load balancing method. Default is specified in the lb-method ConfigMap key.
	FailTimeout         string                 // The time during which unsuccessful attempts should happen to consider server unavailable.
	MaxFails            *int                   // The number of unsuccessful attempts to consider server unavailable.
	MaxConns            *int                   // The maximum number of simultaneous active connections to an upstream server.
	Keepalive           *int                   // Configures the cache for connections to upstream servers.
	ConnectTimeout      string                 // The timeout for establishing a connection with an upstream server.
	ReadTimeout         string                 // The timeout for reading a response from an upstream server.
	SendTimeout         string                 // The timeout for transmitting a request to an upstream server.
	NextUpstream        string                 // Specifies in which cases a request should be passed to the next upstream server.
	NextUpstreamTimeout string                 // The time during which a request can be passed to the next upstream server.
	NextUpstreamTries   *int32                 // The number of possible tries for passing a request to the next upstream server.
	ClientMaxBodySize   string                 // Sets the maximum allowed size of the client request body.
	TLS                 *nginxv1.UpstreamTLS   // The TLS configuration for the Upstream.
	HealthCheck         *nginxv1.HealthCheck   // The health check configuration for the Upstream.
	SlowStart           string                 // The slow start allows an upstream server to gradually recover its weight.
	Queue               *nginxv1.UpstreamQueue // Configures a queue for an upstream.
	Buffering           *bool                  // Enables buffering of responses from the upstream server.
	BufferSize          string                 // Sets the size of the buffer used for reading the first part of a response.
	NTLM                bool                   // Allows proxying requests with NTLM Authentication.
	Type                string                 // The type of the upstream. Supported values are http and grpc.
	Backup              string                 // The name of the backup service of type ExternalName.
	BackupPort          *uint16                // The port of the backup service.
	Weight              *int32                 // Weight for load balancing (used in splits)
	SessionCookie       *nginxv1.SessionCookie // Configures session persistence for the upstream.
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

// populateUpstreamConfig fills the UpstreamConfig struct with all available information from nginxv1.Upstream
func populateUpstreamConfig(upstream *nginxv1.Upstream) *UpstreamConfig {
	config := &UpstreamConfig{
		Name:                upstream.Name,
		Service:             upstream.Service,
		Port:                upstream.Port,
		LBMethod:            upstream.LBMethod,
		FailTimeout:         upstream.FailTimeout,
		MaxFails:            upstream.MaxFails,
		MaxConns:            upstream.MaxConns,
		Keepalive:           upstream.Keepalive,
		ConnectTimeout:      upstream.ProxyConnectTimeout,
		ReadTimeout:         upstream.ProxyReadTimeout,
		SendTimeout:         upstream.ProxySendTimeout,
		NextUpstream:        upstream.ProxyNextUpstream,
		NextUpstreamTimeout: upstream.ProxyNextUpstreamTimeout,
		ClientMaxBodySize:   upstream.ClientMaxBodySize,
		TLS:                 &upstream.TLS,
		HealthCheck:         upstream.HealthCheck,
		SlowStart:           upstream.SlowStart,
		Queue:               upstream.Queue,
		Buffering:           upstream.ProxyBuffering,
		NTLM:                upstream.NTLM,
		Type:                upstream.Type,
		Backup:              upstream.Backup,
		BackupPort:          upstream.BackupPort,
		NextUpstreamTries:   Ptr(int32(upstream.ProxyNextUpstreamTries)),
		UseClusterIP:        Ptr(upstream.UseClusterIP),
		BufferSize:          upstream.ProxyBufferSize,
		Subselector:         upstream.Subselector,
		SessionCookie:       upstream.SessionCookie,
	}

	// Handle session cookie if it exists
	if upstream.SessionCookie != nil {
		// Note: SessionCookie is not part of UpstreamConfig struct, handled separately
	}

	return config
}

// checkUnsupportedUpstreamConversions creates notifications for upstream fields that are not currently converted to Gateway API
func checkUnsupportedUpstreamConversions(config *UpstreamConfig, vs *nginxv1.VirtualServer, notifs *[]notifications.Notification) {
	upstreamName := config.Name

	// Check load balancing method
	if config.LBMethod != "least_conn" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': LBMethod '%s' will be set to random two least_conn by default", upstreamName, config.LBMethod), vs)
	}

	// Check subselector
	if len(config.Subselector) > 0 {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Subselector field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check use-cluster-ip
	if config.UseClusterIP != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': UseClusterIP field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check fail timeout
	if config.FailTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': FailTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check max fails
	if config.MaxFails != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': MaxFails field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check max connections
	if config.MaxConns != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': MaxConns field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check keepalive
	if config.Keepalive != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Keepalive field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check connection timeout
	if config.ConnectTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': ConnectTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check read timeout
	if config.ReadTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': ReadTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check send timeout
	if config.SendTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': SendTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check next upstream
	if config.NextUpstream != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NextUpstream field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check next upstream timeout
	if config.NextUpstreamTimeout != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NextUpstreamTimeout field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check next upstream tries
	if config.NextUpstreamTries != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NextUpstreamTries field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check client max body size
	if config.ClientMaxBodySize != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': ClientMaxBodySize field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check health check
	if config.HealthCheck != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': HealthCheck field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check slow start
	if config.SlowStart != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': SlowStart field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check queue
	if config.Queue != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Queue field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check buffering
	if config.Buffering != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Buffering field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check buffer size
	if config.BufferSize != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': BufferSize field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check NTLM
	if config.NTLM {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': NTLM field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check backup
	if config.Backup != "" {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': Backup field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check backup port
	if config.BackupPort != nil {
		addNotification(notifs, notifications.InfoNotification,
			fmt.Sprintf("Upstream '%s': BackupPort field is not currently converted to Gateway API", upstreamName), vs)
	}

	// Check session cookie (handled separately since it's not in UpstreamConfig)
	// This would be checked in the original upstream object
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
