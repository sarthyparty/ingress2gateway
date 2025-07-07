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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)


// VirtualServerToGatewayIR converts nginx VirtualServer CRDs to Gateway API resources
func VirtualServerToGatewayIR(crds []nginxv1.VirtualServer) (
	partial intermediate.IR,
	notificationList []notifications.Notification,
	errs field.ErrorList,
) {
	gatewayMap := make(map[types.NamespacedName]intermediate.GatewayContext)
	httpRouteMap := make(map[types.NamespacedName]intermediate.HTTPRouteContext)
	var notificationsAggregator []notifications.Notification

	for _, vs := range crds {
		// Skip VirtualServers without host
		if vs.Spec.Host == "" {
			notificationsAggregator = append(notificationsAggregator, notifications.NewNotification(
				notifications.WarningNotification,
				"VirtualServer has no host specified, skipping",
				&vs,
			))
			continue
		}

		// Convert VirtualServer to Gateway
		gateway, gatewayNotifications := convertVirtualServerToGateway(vs)
		gatewayKey := types.NamespacedName{
			Namespace: vs.Namespace,
			Name:      generateGatewayName(vs),
		}
		gatewayMap[gatewayKey] = gateway
		notificationsAggregator = append(notificationsAggregator, gatewayNotifications...)

		// Convert VirtualServer routes to HTTPRoutes
		for i, route := range vs.Spec.Routes {
			httpRoute, routeNotifications := convertRouteToHTTPRoute(vs, route, i, gatewayKey)
			routeKey := types.NamespacedName{
				Namespace: vs.Namespace,
				Name:      generateHTTPRouteName(vs, i),
			}
			httpRouteMap[routeKey] = httpRoute
			notificationsAggregator = append(notificationsAggregator, routeNotifications...)
		}
	}

	return intermediate.IR{
		Gateways:   gatewayMap,
		HTTPRoutes: httpRouteMap,
	}, notificationsAggregator, errs
}
