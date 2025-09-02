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
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

// RouteResolver handles VirtualServerRoute reference resolution and dependency management
type RouteResolver struct {
	virtualServers      []nginxv1.VirtualServer
	virtualServerRoutes []nginxv1.VirtualServerRoute
	routeIndex          map[types.NamespacedName]nginxv1.VirtualServerRoute
}

// ResolvedRoute represents a resolved VirtualServerRoute with its associated VirtualServer context
type ResolvedRoute struct {
	Route         nginxv1.Route
	Source        RouteSource
	VirtualServer nginxv1.VirtualServer // Context for namespace, gateway class, etc.
	Upstreams     []nginxv1.Upstream    // Upstreams from VirtualServerRoute (if applicable)
	Redirect      bool
}

// RouteSource indicates where the route comes from
type RouteSource struct {
	Type      RouteSourceType
	Namespace string
	Name      string
}

type RouteSourceType string

const (
	RouteSourceVirtualServer      RouteSourceType = "VirtualServer"
	RouteSourceVirtualServerRoute RouteSourceType = "VirtualServerRoute"
)

// NewRouteResolver creates a new RouteResolver instance
func NewRouteResolver(virtualServers []nginxv1.VirtualServer, virtualServerRoutes []nginxv1.VirtualServerRoute) *RouteResolver {
	resolver := &RouteResolver{
		virtualServers:      virtualServers,
		virtualServerRoutes: virtualServerRoutes,
		routeIndex:          make(map[types.NamespacedName]nginxv1.VirtualServerRoute),
	}

	// Build index of VirtualServerRoutes for efficient lookup
	for _, vsr := range virtualServerRoutes {
		key := types.NamespacedName{
			Namespace: vsr.Namespace,
			Name:      vsr.Name,
		}
		resolver.routeIndex[key] = vsr
	}

	return resolver
}

// ResolveRoutesForVirtualServer resolves all routes for a given VirtualServer,
// including both inline routes and VirtualServerRoute references
func (r *RouteResolver) ResolveRoutesForVirtualServer(vs nginxv1.VirtualServer) ([]ResolvedRoute, []notifications.Notification, error) {
	var resolvedRoutes []ResolvedRoute
	var notifs []notifications.Notification

	for _, route := range vs.Spec.Routes {
		resolved, routeNotifs, err := r.resolveRoute(route, vs)
		if err != nil {
			return nil, notifs, err
		}

		resolvedRoutes = append(resolvedRoutes, resolved...)
		notifs = append(notifs, routeNotifs...)
	}

	return resolvedRoutes, notifs, nil
}

// resolveRoute resolves a single route, handling both inline routes and VSR references
func (r *RouteResolver) resolveRoute(route nginxv1.Route, vs nginxv1.VirtualServer) ([]ResolvedRoute, []notifications.Notification, error) {
	var notifs []notifications.Notification

	// If this is an inline route (no reference to VirtualServerRoute)
	if route.Route == "" {
		return []ResolvedRoute{{
			Route: route,
			Source: RouteSource{
				Type:      RouteSourceVirtualServer,
				Namespace: vs.Namespace,
				Name:      vs.Name,
			},
			VirtualServer: vs,
		}}, notifs, nil
	}

	// This is a VirtualServerRoute reference
	vsrKey := r.parseVSRReference(route.Route, vs.Namespace)

	// Find the referenced VirtualServerRoute
	vsr, exists := r.routeIndex[vsrKey]
	if !exists {
		// Create a notification for missing VSR instead of failing
		notif := notifications.NewNotification(
			notifications.WarningNotification,
			fmt.Sprintf("VirtualServerRoute %s not found, skipping route", vsrKey),
			&vs,
		)
		notifs = append(notifs, notif)
		return []ResolvedRoute{}, notifs, nil
	}

	// Resolve all routes in the VirtualServerRoute
	var resolvedRoutes []ResolvedRoute
	for _, vsrRoute := range vsr.Spec.Subroutes {
		resolvedRoute := ResolvedRoute{
			Route: vsrRoute,
			Source: RouteSource{
				Type:      RouteSourceVirtualServerRoute,
				Namespace: vsr.Namespace,
				Name:      vsr.Name,
			},
			VirtualServer: vs,
			Upstreams:     vsr.Spec.Upstreams,
		}

		resolvedRoutes = append(resolvedRoutes, resolvedRoute)
	}

	if len(resolvedRoutes) > 0 {
		notif := notifications.NewNotification(
			notifications.InfoNotification,
			fmt.Sprintf("Resolved %d routes from VirtualServerRoute %s", len(resolvedRoutes), vsrKey),
			&vs,
		)
		notifs = append(notifs, notif)
	}

	return resolvedRoutes, notifs, nil
}

// parseVSRReference parses a VirtualServerRoute reference string
// Format can be "vsr-name" (same namespace) or "namespace/vsr-name"
func (r *RouteResolver) parseVSRReference(ref string, defaultNamespace string) types.NamespacedName {
	parts := strings.Split(ref, "/")
	if len(parts) == 1 {
		// Same namespace reference
		return types.NamespacedName{
			Namespace: defaultNamespace,
			Name:      parts[0],
		}
	} else if len(parts) == 2 {
		// Cross-namespace reference
		return types.NamespacedName{
			Namespace: parts[0],
			Name:      parts[1],
		}
	}

	return types.NamespacedName{
		Namespace: defaultNamespace,
		Name:      ref,
	}
}

// GetVirtualServerRouteByKey retrieves a VirtualServerRoute by its namespaced name
func (r *RouteResolver) GetVirtualServerRouteByKey(key types.NamespacedName) (nginxv1.VirtualServerRoute, bool) {
	vsr, exists := r.routeIndex[key]
	return vsr, exists
}

// addNotification is implemented in utils.go
