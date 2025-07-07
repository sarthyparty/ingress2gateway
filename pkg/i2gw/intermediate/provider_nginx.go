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

package intermediate

import (
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

// NginxGatewayIR holds nginx-specific gateway configurations
// from VirtualServer and other nginx CRDs that don't map directly to Gateway API
type NginxGatewayIR struct {
	// TLS configuration from VirtualServer
	TLSTermination *NginxTLSConfig
	// Global rate limiting configuration
	RateLimit *NginxRateLimitConfig
	// SSL policies
	SSLPolicy *NginxSSLPolicyConfig
}

// NginxHTTPRouteIR holds nginx-specific HTTP route configurations
// from VirtualServer paths and subroutes
type NginxHTTPRouteIR struct {
	// Path rewriting rules
	PathRewrite *NginxPathRewriteConfig
	// Header modification rules
	HeaderModification *NginxHeaderModConfig
	// Rate limiting per route
	RateLimit *NginxRateLimitConfig
	// Traffic splitting configuration
	TrafficSplit *NginxTrafficSplitConfig
}

// NginxServiceIR holds nginx-specific service configurations
// from VirtualServer upstreams
type NginxServiceIR struct {
	// Health check configuration
	HealthCheck *NginxHealthCheckConfig
	// Load balancing method
	LoadBalancing *NginxLoadBalancingConfig
	// Session persistence
	SessionPersistence *NginxSessionPersistenceConfig
	// Application protocol for backend services (e.g., "https", "grpc")
	AppProtocol string
}

// Supporting configuration structures for VirtualServer features

type NginxTLSConfig struct {
	SecretName      string
	TerminationMode string
}

type NginxRateLimitConfig struct {
	Rate    string
	Burst   *int
	Delay   *int
	NoDelay *bool
}

type NginxSSLPolicyConfig struct {
	Protocols []string
	Ciphers   []string
}

type NginxPathRewriteConfig struct {
	Pattern     string
	Replacement string
}

type NginxHeaderModConfig struct {
	RequestHeaders  map[string]string
	ResponseHeaders map[string]string
}

type NginxTrafficSplitConfig struct {
	Splits []NginxTrafficSplit
}

type NginxTrafficSplit struct {
	Weight  int
	Service string
}

type NginxHealthCheckConfig struct {
	Path       string
	Interval   string
	Timeout    string
	Retries    *int
	StatusCode *int
}

type NginxLoadBalancingConfig struct {
	Method string // round_robin, least_conn, ip_hash, etc.
}

type NginxSessionPersistenceConfig struct {
	Method string
	Cookie *NginxCookieConfig
}

type NginxCookieConfig struct {
	Name     string
	Domain   string
	Path     string
	Expires  string
	Secure   bool
	HTTPOnly bool
}

// NginxUpstreamConfig holds nginx-specific upstream configurations
// from VirtualServer upstreams that don't map directly to Gateway API
type NginxUpstreamConfig struct {
	Name            string
	Service         string
	Port            uint16
	LBMethod        string
	Keepalive       *int
	HealthCheck     *nginxv1.HealthCheck
	TLS             *nginxv1.UpstreamTLS
	MaxConns        *int
	ConnectTimeout  string
	ReadTimeout     string
	SendTimeout     string
	FailTimeout     string
	MaxFails        *int
	ProxyBuffering  *bool
	SlowStart       string
	Queue           *nginxv1.UpstreamQueue
	SessionCookie   *nginxv1.SessionCookie
}
