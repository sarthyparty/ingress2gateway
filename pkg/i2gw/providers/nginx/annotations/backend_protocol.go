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

import (
	"fmt"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	"strings"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

// BackendProtocolFeature converts backend protocol annotations to appropriate route types
func BackendProtocolFeature(ingresses []networkingv1.Ingress, _ map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		if sslServices, exists := ingress.Annotations[nginxSSLServicesAnnotation]; exists && sslServices != "" {
			errs = append(errs, processSSLServicesAnnotation(ingress, sslServices, ir)...)
		}

		if grpcServices, exists := ingress.Annotations[nginxGRPCServicesAnnotation]; exists && grpcServices != "" {
			errs = append(errs, processGRPCServicesAnnotation(ingress, grpcServices, ir)...)
		}

		if webSocketServices, exists := ingress.Annotations[nginxWebSocketServicesAnnotation]; exists && webSocketServices != "" {
			message := "nginx.org/websocket-services: Please make sure the services are configured to support WebSocket connections. This annotation does not create any Gateway API resources."
			notify(notifications.InfoNotification, message, &ingress)
		}
	}

	return errs
}

// processSSLServicesAnnotation configures HTTPS backend protocol using BackendTLSPolicy
func processSSLServicesAnnotation(ingress networkingv1.Ingress, sslServices string, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	services := strings.Split(sslServices, ",")
	sslServiceSet := make(map[string]bool)
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service != "" {
			sslServiceSet[service] = true
		}
	}

	if ir.BackendTLSPolicies == nil {
		ir.BackendTLSPolicies = make(map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy)
	}
	for serviceName := range sslServiceSet {
		policyName := fmt.Sprintf("%s-%s-backend-tls", ingress.Name, serviceName)
		policyKey := types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      policyName,
		}

		policy := gatewayv1alpha3.BackendTLSPolicy{
			TypeMeta: metav1.TypeMeta{
				APIVersion: gatewayv1alpha3.GroupVersion.String(),
				Kind:       BackendTLSPolicyKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      policyName,
				Namespace: ingress.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "ingress2gateway",
					"ingress2gateway.io/source":    "nginx-ssl-services",
				},
			},
			Spec: gatewayv1alpha3.BackendTLSPolicySpec{
				TargetRefs: []gatewayv1alpha2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gatewayv1alpha2.LocalPolicyTargetReference{
							Group: gatewayv1.GroupName,
							Kind:  ServiceKind,
							Name:  gatewayv1.ObjectName(serviceName),
						},
					},
				},
				Validation: gatewayv1alpha3.BackendTLSPolicyValidation{
					// Note: WellKnownCACertificates and Hostname fields are intentionally left empty
					// These fields must be manually configured based on your backend service's TLS setup
				},
			},
		}

		ir.BackendTLSPolicies[policyKey] = policy
	}

	// Add warning about manual certificate configuration
	if len(sslServiceSet) > 0 {
		message := "nginx.org/ssl-services: " + BackendTLSPolicyKind + " created but requires manual configuration. You must set the 'validation.hostname' field to match your backend service's TLS certificate hostname, and configure appropriate CA certificates or certificateRefs for TLS verification."
		notify(notifications.WarningNotification, message, &ingress)
	}

	return errs
}

// parseGRPCServiceMethod parses gRPC service and method from HTTP path
func parseGRPCServiceMethod(path string) (service, method string) {
	path = strings.TrimPrefix(path, "/")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 && parts[0] != "" {
		service = parts[0]
	}
	if len(parts) >= 2 && parts[1] != "" {
		method = parts[1]
	}

	return service, method
}

// processGRPCServicesAnnotation handles gRPC backend services
func processGRPCServicesAnnotation(ingress networkingv1.Ingress, grpcServices string, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	// Parse comma-separated service names that should use gRPC
	services := strings.Split(grpcServices, ",")
	grpcServiceSet := make(map[string]struct{})
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service != "" {
			grpcServiceSet[service] = struct{}{}
		}
	}

	// Initialize GRPCRoutes map if needed
	if ir.GRPCRoutes == nil {
		ir.GRPCRoutes = make(map[types.NamespacedName]gatewayv1.GRPCRoute)
	}

	// Mark services as gRPC in provider-specific IR
	if ir.Services == nil {
		ir.Services = make(map[types.NamespacedName]intermediate.ProviderSpecificServiceIR)
	}

	// Update each gRPC service with AppProtocol
	for serviceName := range grpcServiceSet {
		serviceKey := types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      serviceName,
		}

		// Get or create a provider-specific service IR
		serviceIR := ir.Services[serviceKey]
		if serviceIR.Nginx == nil {
			serviceIR.Nginx = &intermediate.NginxServiceIR{}
		}
		serviceIR.Nginx.AppProtocol = "grpc"
		ir.Services[serviceKey] = serviceIR
	}

	// Create GRPCRoute for ingress rules that use gRPC services
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}

		var grpcRouteRules []gatewayv1.GRPCRouteRule

		// Check each path to see if it uses a gRPC service
		for _, path := range rule.HTTP.Paths {
			serviceName := path.Backend.Service.Name
			if _, exists := grpcServiceSet[serviceName]; exists {
				// Create a GRPCRoute rule for this path
				grpcMatch := gatewayv1.GRPCRouteMatch{}

				// Convert an HTTP path to gRPC service/method match
				if path.Path != "" {
					// Parse gRPC service and method from path
					// Expected format: /service.name/Method or /service.name
					service, method := parseGRPCServiceMethod(path.Path)
					if service != "" {
						grpcMatch.Method = &gatewayv1.GRPCMethodMatch{
							Service: &service,
						}
						if method != "" {
							grpcMatch.Method.Method = &method
						}
					}
				}

				// Create a backend reference
				var port *gatewayv1.PortNumber
				if path.Backend.Service.Port.Number != 0 {
					portNum := gatewayv1.PortNumber(path.Backend.Service.Port.Number)
					port = &portNum
				}

				backendRef := gatewayv1.GRPCBackendRef{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: gatewayv1.ObjectName(serviceName),
							Port: port,
						},
					},
				}

				grpcRule := gatewayv1.GRPCRouteRule{
					Matches:     []gatewayv1.GRPCRouteMatch{grpcMatch},
					BackendRefs: []gatewayv1.GRPCBackendRef{backendRef},
				}

				grpcRouteRules = append(grpcRouteRules, grpcRule)
			}
		}

		// Create GRPCRoute if we have any gRPC rules
		if len(grpcRouteRules) > 0 {
			// Use the same route name as HTTPRoute to replace it
			routeName := common.RouteName(ingress.Name, rule.Host)
			routeKey := types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      routeName,
			}

			// Create a hostname list
			var hostnames []gatewayv1.Hostname
			if rule.Host != "" {
				hostnames = []gatewayv1.Hostname{gatewayv1.Hostname(rule.Host)}
			}

			grpcRoute := gatewayv1.GRPCRoute{
				TypeMeta: metav1.TypeMeta{
					APIVersion: gatewayv1.GroupVersion.String(),
					Kind:       GRPCRouteKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeName,
					Namespace: ingress.Namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "ingress2gateway",
						"ingress2gateway.io/source":    "nginx-grpc-services",
					},
				},
				Spec: gatewayv1.GRPCRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: func() gatewayv1.ObjectName {
									if ingress.Spec.IngressClassName != nil {
										return gatewayv1.ObjectName(*ingress.Spec.IngressClassName)
									}
									return NginxIngressClass
								}(),
							},
						},
					},
					Hostnames: hostnames,
					Rules:     grpcRouteRules,
				},
			}

			ir.GRPCRoutes[routeKey] = grpcRoute

			// Remove the corresponding HTTPRoute since gRPC services should only have GRPCRoutes
			if _, exists := ir.HTTPRoutes[routeKey]; exists {
				delete(ir.HTTPRoutes, routeKey)
			}
		}
	}

	return errs
}
