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
	"strconv"
	"strings"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenPortsFeature processes nginx.org/listen-ports and nginx.org/listen-ports-ssl annotations
// These annotations REPLACE the default ports (80/443) with custom ports, following NIC behavior
func ListenPortsFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		// Extract custom ports from annotations
		httpPorts := extractListenPorts(ingress.Annotations[nginxListenPortsAnnotation])
		sslPorts := extractListenPorts(ingress.Annotations[nginxListenPortsSSLAnnotation])

		// Process if ANY custom ports are specified (replaces defaults)
		if len(httpPorts) > 0 || len(sslPorts) > 0 {
			errs = append(errs, replaceGatewayPortsWithCustom(ingress, httpPorts, sslPorts, ir)...)
		}
	}

	return errs
}

// extractListenPorts parses comma-separated port numbers from annotation value
func extractListenPorts(portsAnnotation string) []int32 {
	if portsAnnotation == "" {
		return nil
	}

	var ports []int32
	portStrings := strings.Split(portsAnnotation, ",")

	for _, portStr := range portStrings {
		portStr = strings.TrimSpace(portStr)
		if portStr == "" {
			continue
		}

		if port, err := strconv.ParseInt(portStr, 10, 32); err == nil {
			// Validate port range (1-65535)
			if port > 0 && port <= 65535 {
				ports = append(ports, int32(port))
			}
		}
	}

	return ports
}

// replaceGatewayPortsWithCustom modifies the Gateway to use ONLY the specified custom ports
// This follows NIC behavior where listen-ports annotations REPLACE default ports, not add to them
func replaceGatewayPortsWithCustom(ingress networkingv1.Ingress, httpPorts, sslPorts []int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	// Get gateway class name from ingress
	gatewayClassName := getGatewayClassName(ingress)
	gatewayKey := types.NamespacedName{Namespace: ingress.Namespace, Name: gatewayClassName}

	// Get or create gateway context
	gatewayContext, exists := ir.Gateways[gatewayKey]
	if !exists {
		// Create new gateway if it doesn't exist
		gatewayContext = intermediate.GatewayContext{
			Gateway: gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayClassName,
					Namespace: ingress.Namespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
					Listeners:        []gatewayv1.Listener{},
				},
			},
		}
	}

	// Check if annotations are present AND have non-empty values
	// Empty string annotations should be treated same as missing annotations (use defaults)
	hasHTTPAnnotation := ingress.Annotations[nginxListenPortsAnnotation] != "" && len(httpPorts) > 0
	hasSSLAnnotation := ingress.Annotations[nginxListenPortsSSLAnnotation] != "" && len(sslPorts) > 0

	// Determine which ports to use (custom ports replace defaults)
	portsToUse := determinePortsToUse(httpPorts, sslPorts, hasHTTPAnnotation, hasSSLAnnotation)

	// For this ingress, we need to remove any default listeners and add custom ones
	// First, remove any existing default listeners for this ingress's hostnames
	var filteredListeners []gatewayv1.Listener
	for _, existingListener := range gatewayContext.Gateway.Spec.Listeners {
		shouldKeep := true
		for _, rule := range ingress.Spec.Rules {
			hostname := rule.Host
			// Remove default port listeners that match this ingress's hostnames
			if existingListener.Hostname != nil && string(*existingListener.Hostname) == hostname {
				if (existingListener.Port == 80 && existingListener.Protocol == gatewayv1.HTTPProtocolType) ||
					(existingListener.Port == 443 && existingListener.Protocol == gatewayv1.HTTPSProtocolType) {
					shouldKeep = false
					break
				}
			}
		}
		if shouldKeep {
			filteredListeners = append(filteredListeners, existingListener)
		}
	}

	// Add custom listeners for this ingress
	for _, rule := range ingress.Spec.Rules {
		hostname := rule.Host

		// Add HTTP listeners
		for _, port := range portsToUse.HTTP {
			listener := createListener(hostname, port, gatewayv1.HTTPProtocolType)
			filteredListeners = append(filteredListeners, listener)
		}

		// Add HTTPS listeners
		for _, port := range portsToUse.HTTPS {
			listener := createListener(hostname, port, gatewayv1.HTTPSProtocolType)
			filteredListeners = append(filteredListeners, listener)
		}
	}

	// Update the gateway with filtered listeners
	gatewayContext.Gateway.Spec.Listeners = filteredListeners

	// Update gateway in IR
	ir.Gateways[gatewayKey] = gatewayContext

	return errs
}

// portConfiguration holds the final ports to use for HTTP and HTTPS
type portConfiguration struct {
	HTTP  []int32
	HTTPS []int32
}

// determinePortsToUse implements NIC logic: custom ports REPLACE defaults
func determinePortsToUse(customHTTPPorts, customSSLPorts []int32, hasHTTPAnnotation, hasSSLAnnotation bool) portConfiguration {
	config := portConfiguration{}

	if hasHTTPAnnotation {
		config.HTTP = customHTTPPorts
	} else if !hasSSLAnnotation {
		config.HTTP = []int32{80}
	}
	if hasSSLAnnotation {
		config.HTTPS = customSSLPorts
	} else if !hasHTTPAnnotation {
		config.HTTPS = []int32{443}
	}
	return config
}

// createListener creates a Gateway listener for the given hostname, port, and protocol
func createListener(hostname string, port int32, protocol gatewayv1.ProtocolType) gatewayv1.Listener {
	// Create safe listener name
	listenerName := createListenerName(hostname, port, protocol)

	listener := gatewayv1.Listener{
		Name:     gatewayv1.SectionName(listenerName),
		Port:     gatewayv1.PortNumber(port),
		Protocol: protocol,
	}

	// Add hostname if specified
	if hostname != "" {
		listener.Hostname = (*gatewayv1.Hostname)(&hostname)
	}

	return listener
}

// createListenerName generates a safe listener name from hostname, port, and protocol
func createListenerName(hostname string, port int32, protocol gatewayv1.ProtocolType) string {
	// Sanitize hostname for use in listener name
	safeName := common.NameFromHost(hostname)
	protocolStr := strings.ToLower(string(protocol))

	return fmt.Sprintf("%s-%s-%d", safeName, protocolStr, port)
}

// getGatewayClassName extracts the gateway class name from ingress
func getGatewayClassName(ingress networkingv1.Ingress) string {
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName != "" {
		return *ingress.Spec.IngressClassName
	}
	// Default fallback
	return "nginx"
}
