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

package annotations

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	// NGINX Ingress Controller annotation prefixes
	nginxOrgPrefix = "nginx.org/"
	nginxComPrefix = "nginx.com/"

	// Standard annotations that map directly to Gateway API
	nginxRewritesAnnotation        = nginxOrgPrefix + "rewrites"
	nginxRedirectToHTTPSAnnotation = nginxOrgPrefix + "redirect-to-https"
	nginxLBMethodAnnotation        = nginxOrgPrefix + "lb-method"
	nginxServerAliasAnnotation     = nginxOrgPrefix + "server-alias"

	// Header manipulation annotations
	nginxProxyHideHeadersAnnotation = nginxOrgPrefix + "proxy-hide-headers"
	nginxProxyPassHeadersAnnotation = nginxOrgPrefix + "proxy-pass-headers"
	nginxProxySetHeadersAnnotation  = nginxOrgPrefix + "proxy-set-headers"

	// Port configuration annotations
	nginxListenPortsAnnotation    = nginxOrgPrefix + "listen-ports"
	nginxListenPortsSSLAnnotation = nginxOrgPrefix + "listen-ports-ssl"

	// Backend service annotations
	nginxSSLServicesAnnotation  = nginxOrgPrefix + "ssl-services"
	nginxGRPCServicesAnnotation = nginxOrgPrefix + "grpc-services"

	// Path matching annotations
	nginxPathRegexAnnotation = nginxOrgPrefix + "path-regex"

	// Security annotations
	nginxHSTSAnnotation                  = nginxOrgPrefix + "hsts"
	nginxHSTSMaxAgeAnnotation            = nginxOrgPrefix + "hsts-max-age"
	nginxHSTSIncludeSubdomainsAnnotation = nginxOrgPrefix + "hsts-include-subdomains"
	nginxBasicAuthSecretAnnotation       = nginxOrgPrefix + "basic-auth-secret"
	nginxBasicAuthRealmAnnotation        = nginxOrgPrefix + "basic-auth-realm"

	// Legacy SSL redirect annotation
	legacySSLRedirectAnnotation = "ingress.kubernetes.io/ssl-redirect"

	v1Version = "v1"

	nginxResourcesGroup = "k8s.nginx.org"

	virtualServerKind = "VirtualServer"
)

var (
	VirtualServerGVK = schema.GroupVersionKind{
		Group:   nginxResourcesGroup,
		Version: v1Version,
		Kind:    virtualServerKind,
	}
)
