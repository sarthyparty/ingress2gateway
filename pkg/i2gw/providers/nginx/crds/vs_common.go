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

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants for VirtualServer conversion
const (
	defaultGatewayClassName = "nginx"
	defaultHTTPPort         = 80
	defaultHTTPSPort        = 443
)

// Name generation utilities
func generateGatewayName(vs nginxv1.VirtualServer) string {
	return fmt.Sprintf("%s-gateway", vs.Name)
}

func generateHTTPRouteName(vs nginxv1.VirtualServer, routeIndex int) string {
	return fmt.Sprintf("%s-route-%d", vs.Name, routeIndex)
}

// Gateway class configuration
func getGatewayClassName(vs nginxv1.VirtualServer) string {
	if vs.Spec.IngressClass != "" {
		return vs.Spec.IngressClass
	}
	return defaultGatewayClassName
}

// Upstream utilities
func findUpstream(upstreams []nginxv1.Upstream, name string) *nginxv1.Upstream {
	for _, upstream := range upstreams {
		if upstream.Name == name {
			return &upstream
		}
	}
	return nil
}

// Ptr is Generic pointer conversion utility
func Ptr[T any](t T) *T {
	return &t
}

// Helper function to add notifications
func addNotification(notificationList *[]notifications.Notification, messageType notifications.MessageType, message string, obj client.Object) {
	n := notifications.NewNotification(messageType, message, obj)
	*notificationList = append(*notificationList, n)
}
