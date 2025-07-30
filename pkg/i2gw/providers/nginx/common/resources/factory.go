package resources

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/common"
)

const (
	BackendTLSPolicyKind = "BackendTLSPolicy"
	GRPCRouteKind        = "GRPCRoute"
	ServiceKind          = "Service"
)

// ResourceType represents the type of resource to create
type ResourceType string

const (
	BackendTLSPolicyType ResourceType = "BackendTLSPolicy"
	GRPCRouteType        ResourceType = "GRPCRoute"
)

// BackendTLSPolicyOptions contains options for BackendTLSPolicy creation
type BackendTLSPolicyOptions struct {
	// Name of the policy
	Name string
	// Namespace of the policy
	Namespace string
	// Target service name
	ServiceName string
	// Source label for tracking the origin (e.g., "nginx-ssl-services")
	SourceLabel string
	// Additional labels to apply
	Labels map[string]string
}

// GRPCRouteOptions contains options for GRPCRoute creation
type GRPCRouteOptions struct {
	// Name of the GRPCRoute
	Name string
	// Namespace of the GRPCRoute
	Namespace string
	// Hostnames for the route
	Hostnames []string
	// Parent gateway references
	ParentRefs []gatewayv1.ParentReference
	// GRPC route rules
	Rules []gatewayv1.GRPCRouteRule
	// Source label for tracking the origin (e.g., "nginx-grpc-services")
	SourceLabel string
	// Additional labels to apply
	Labels map[string]string
}

// PolicyOptions contains all policy configuration options
type PolicyOptions struct {
	BackendTLS *BackendTLSPolicyOptions
	GRPCRoute  *GRPCRouteOptions
	// NotificationCollector for gathering notifications during policy creation
	NotificationCollector common.NotificationCollector
	// Source object for notifications (e.g., VirtualServer, Ingress)
	SourceObject client.Object
}

// CreateBackendTLSPolicy creates a BackendTLSPolicy using the provided options
func CreateBackendTLSPolicy(opts PolicyOptions) *gatewayv1alpha3.BackendTLSPolicy {
	if opts.BackendTLS == nil {
		return nil
	}

	btlsOpts := opts.BackendTLS

	// Build labels
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "ingress2gateway",
	}
	if btlsOpts.SourceLabel != "" {
		labels["ingress2gateway.io/source"] = btlsOpts.SourceLabel
	}
	for k, v := range btlsOpts.Labels {
		labels[k] = v
	}

	policy := &gatewayv1alpha3.BackendTLSPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1alpha3.GroupVersion.String(),
			Kind:       BackendTLSPolicyKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      btlsOpts.Name,
			Namespace: btlsOpts.Namespace,
			Labels:    labels,
		},
		Spec: gatewayv1alpha3.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1alpha2.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1alpha2.LocalPolicyTargetReference{
						Group: gatewayv1.GroupName,
						Kind:  ServiceKind,
						Name:  gatewayv1.ObjectName(btlsOpts.ServiceName),
					},
				},
			},
			Validation: gatewayv1alpha3.BackendTLSPolicyValidation{
				// Note: WellKnownCACertificates and Hostname fields are intentionally left empty
				// These fields must be manually configured based on your backend service's TLS setup
			},
		},
	}

	// Add notification about manual configuration required
	if opts.NotificationCollector != nil {
		message := fmt.Sprintf("BackendTLSPolicy '%s' created but requires manual configuration. You must set the 'validation.hostname' field to match your backend service's TLS certificate hostname, and configure appropriate CA certificates or certificateRefs for TLS verification.", btlsOpts.Name)
		opts.NotificationCollector.AddWarning(message, opts.SourceObject)
	}

	return policy
}

// CreateGRPCRoute creates a GRPCRoute using the provided options
func CreateGRPCRoute(opts PolicyOptions) *gatewayv1.GRPCRoute {
	if opts.GRPCRoute == nil {
		return nil
	}

	grpcOpts := opts.GRPCRoute

	// Build labels
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "ingress2gateway",
	}
	if grpcOpts.SourceLabel != "" {
		labels["ingress2gateway.io/source"] = grpcOpts.SourceLabel
	}
	for k, v := range grpcOpts.Labels {
		labels[k] = v
	}

	// Convert string hostnames to Gateway API Hostname type
	var hostnames []gatewayv1.Hostname
	for _, hostname := range grpcOpts.Hostnames {
		if hostname != "" {
			hostnames = append(hostnames, gatewayv1.Hostname(hostname))
		}
	}

	route := &gatewayv1.GRPCRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1.GroupVersion.String(),
			Kind:       GRPCRouteKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      grpcOpts.Name,
			Namespace: grpcOpts.Namespace,
			Labels:    labels,
		},
		Spec: gatewayv1.GRPCRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: grpcOpts.ParentRefs,
			},
			Hostnames: hostnames,
			Rules:     grpcOpts.Rules,
		},
	}

	// Add notification about GRPC route creation
	if opts.NotificationCollector != nil {
		message := fmt.Sprintf("GRPCRoute '%s' created with %d rules. Ensure your backend services support gRPC protocol.", grpcOpts.Name, len(grpcOpts.Rules))
		opts.NotificationCollector.AddInfo(message, opts.SourceObject)
	}

	return route
}

// Helper functions for building policy options

// NewBackendTLSPolicyOptions creates BackendTLSPolicyOptions with common defaults
func NewBackendTLSPolicyOptions(name, namespace, serviceName, sourceLabel string) *BackendTLSPolicyOptions {
	return &BackendTLSPolicyOptions{
		Name:        name,
		Namespace:   namespace,
		ServiceName: serviceName,
		SourceLabel: sourceLabel,
		Labels:      make(map[string]string),
	}
}

// NewGRPCRouteOptions creates GRPCRouteOptions with common defaults
func NewGRPCRouteOptions(name, namespace, sourceLabel string) *GRPCRouteOptions {
	return &GRPCRouteOptions{
		Name:        name,
		Namespace:   namespace,
		SourceLabel: sourceLabel,
		Labels:      make(map[string]string),
		ParentRefs:  make([]gatewayv1.ParentReference, 0),
		Rules:       make([]gatewayv1.GRPCRouteRule, 0),
	}
}

// GenerateBackendTLSPolicyName generates a consistent policy name
func GenerateBackendTLSPolicyName(serviceName, suffix string) string {
	if suffix != "" {
		return fmt.Sprintf("%s-%s-backend-tls", serviceName, suffix)
	}
	return fmt.Sprintf("%s-backend-tls", serviceName)
}

// GenerateGRPCRouteName generates a consistent GRPC route name
func GenerateGRPCRouteName(baseName, suffix string) string {
	if suffix != "" {
		return fmt.Sprintf("%s-%s-grpc", baseName, suffix)
	}
	return fmt.Sprintf("%s-grpc", baseName)
}

// GeneratePolicyKey generates a NamespacedName key for policy storage
func GeneratePolicyKey(namespace, name string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
}
