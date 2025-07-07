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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// serverAliasFeature converts nginx.org/server-alias annotation to additional Gateway listeners
func ServerAliasFeature(ingresses []networkingv1.Ingress, servicePorts map[types.NamespacedName]map[string]int32, ir *intermediate.IR) field.ErrorList {
	var errs field.ErrorList

	for _, ingress := range ingresses {
		aliasValue, exists := ingress.Annotations[nginxServerAliasAnnotation]
		if !exists || aliasValue == "" {
			continue
		}

		// Parse comma-separated server aliases
		aliases := strings.Split(aliasValue, ",")

		for _ = range ingress.Spec.Rules {
			gatewayName := ingress.Spec.IngressClassName
			if gatewayName == nil {
				gatewayName = ptr.To(ingress.Name)
			}
			gatewayKey := types.NamespacedName{Namespace: ingress.Namespace, Name: *gatewayName}

			gatewayContext, exists := ir.Gateways[gatewayKey]
			if !exists {
				continue
			}

			// Add listeners for each alias
			for _, alias := range aliases {
				alias = strings.TrimSpace(alias)
				if alias == "" {
					continue
				}

				hostname := gatewayv1.Hostname(alias)

				// Check if listener already exists
				listenerExists := false
				for _, listener := range gatewayContext.Gateway.Spec.Listeners {
					if listener.Hostname != nil && *listener.Hostname == hostname {
						listenerExists = true
						break
					}
				}

				if !listenerExists {
					// Add HTTP listener for alias
					aliasListener := gatewayv1.Listener{
						Name:     gatewayv1.SectionName(fmt.Sprintf("http-%s", strings.ReplaceAll(alias, ".", "-"))),
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &hostname,
					}

					gatewayContext.Gateway.Spec.Listeners = append(gatewayContext.Gateway.Spec.Listeners, aliasListener)

					// Note: Added listener for server alias
				}
			}

			ir.Gateways[gatewayKey] = gatewayContext
		}
	}

	return errs
}
