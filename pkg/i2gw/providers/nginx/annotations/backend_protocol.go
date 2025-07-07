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
	"strings"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

// backendProtocolFeature converts backend protocol annotations to appropriate route types
func BackendProtocolFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		// Process ssl-services annotation for HTTPS backends
		if sslServices, exists := ingress.Annotations[nginxSSLServicesAnnotation]; exists && sslServices != "" {
			errs = append(errs, processSSLServicesAnnotation(ingress, sslServices, ir)...)
		}

		// Process grpc-services annotation for gRPC backends
		if grpcServices, exists := ingress.Annotations[nginxGRPCServicesAnnotation]; exists && grpcServices != "" {
			errs = append(errs, processGRPCServicesAnnotation(ingress, grpcServices, ir)...)
		}
	}

	return errs
}

// processSSLServicesAnnotation configures HTTPS backend protocol using BackendTLSPolicy
func processSSLServicesAnnotation(ingress networkingv1.Ingress, sslServices string, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	// Parse comma-separated service names that should use HTTPS
	services := strings.Split(sslServices, ",")
	sslServiceSet := make(map[string]bool)
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service != "" {
			sslServiceSet[service] = true
		}
	}

	// Initialize BackendTLSPolicies map if needed
	if ir.BackendTLSPolicies == nil {
		ir.BackendTLSPolicies = make(map[types.NamespacedName]gatewayv1alpha3.BackendTLSPolicy)
	}

	// Create BackendTLSPolicy for each SSL service
	for serviceName := range sslServiceSet {
		policyName := fmt.Sprintf("%s-%s-backend-tls", ingress.Name, serviceName)
		policyKey := types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      policyName,
		}

		// Use system CA certificates for TLS validation
		wellKnownCACerts := gatewayv1alpha3.WellKnownCACertificatesSystem

		// Create BackendTLSPolicy
		policy := gatewayv1alpha3.BackendTLSPolicy{
			TypeMeta: metav1.TypeMeta{
				APIVersion: gatewayv1alpha3.GroupVersion.String(),
				Kind:       "BackendTLSPolicy",
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
							Kind:  "Service",
							Name:  gatewayv1.ObjectName(serviceName),
						},
					},
				},
				Validation: gatewayv1alpha3.BackendTLSPolicyValidation{
					WellKnownCACertificates: &wellKnownCACerts,
					Hostname:                getIngressHostname(ingress),
				},
			},
		}

		ir.BackendTLSPolicies[policyKey] = policy
	}

	return errs
}

// parseGRPCServiceMethod parses gRPC service and method from HTTP path
// Expected formats:
//   - /helloworld.Greeter/SayHello -> service: "helloworld.Greeter", method: "SayHello"
//   - /helloworld.Greeter -> service: "helloworld.Greeter", method: ""
func parseGRPCServiceMethod(path string) (service, method string) {
	// Remove leading slash
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	
	// Split by slash to separate service and method
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 && parts[0] != "" {
		service = parts[0]
	}
	if len(parts) >= 2 && parts[1] != "" {
		method = parts[1]
	}
	
	return service, method
}

// getIngressHostname extracts the hostname from the ingress rules
func getIngressHostname(ingress networkingv1.Ingress) gatewayv1.PreciseHostname {
	// Use the first rule's hostname if available
	if len(ingress.Spec.Rules) > 0 && ingress.Spec.Rules[0].Host != "" {
		return gatewayv1.PreciseHostname(ingress.Spec.Rules[0].Host)
	}
	
	// Fallback to a default hostname if no rules or host specified
	return "backend.local"
}

// processGRPCServicesAnnotation handles gRPC backend services
func processGRPCServicesAnnotation(ingress networkingv1.Ingress, grpcServices string, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	// Parse comma-separated service names that should use gRPC
	services := strings.Split(grpcServices, ",")
	grpcServiceSet := make(map[string]bool)
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service != "" {
			grpcServiceSet[service] = true
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

		// Get or create provider-specific service IR
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
			if grpcServiceSet[serviceName] {
				// Create a GRPCRoute rule for this path
				grpcMatch := gatewayv1.GRPCRouteMatch{}
				
				// Convert HTTP path to gRPC service/method match
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

				// Create backend reference
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
			routeName := fmt.Sprintf("%s-grpc", ingress.Name)
			if rule.Host != "" {
				routeName = fmt.Sprintf("%s-%s-grpc", ingress.Name, rule.Host)
			}

			routeKey := types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      routeName,
			}

			// Create hostnames list
			var hostnames []gatewayv1.Hostname
			if rule.Host != "" {
				hostnames = []gatewayv1.Hostname{gatewayv1.Hostname(rule.Host)}
			}

			grpcRoute := gatewayv1.GRPCRoute{
				TypeMeta: metav1.TypeMeta{
					APIVersion: gatewayv1.GroupVersion.String(),
					Kind:       "GRPCRoute",
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
									return gatewayv1.ObjectName("nginx")
								}(),
							},
						},
					},
					Hostnames: hostnames,
					Rules:     grpcRouteRules,
				},
			}

			ir.GRPCRoutes[routeKey] = grpcRoute
		}
	}

	return errs
}