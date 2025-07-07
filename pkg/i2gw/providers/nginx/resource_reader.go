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

package nginx

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/nginx/annotations"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

// CommonNginxIngressClasses contains NGINX IngressClass names
var CommonNginxIngressClasses = sets.New(
	"nginx",                    // Default NGINX Inc class
	"nic",                      // Common abbreviation
	"nginx-controller",         // Descriptive name
	"nginx-ingress",            // Alternative naming
	"nginx-inc",                // Company-specific
	"nginx-ingress-controller", // Full descriptive name
)

type resourceReader struct {
	conf *i2gw.ProviderConf
}

func newResourceReader(conf *i2gw.ProviderConf) *resourceReader {
	return &resourceReader{
		conf: conf,
	}
}

func (r *resourceReader) readResourcesFromCluster(ctx context.Context) (*storage, error) {
	storage := newResourceStorage()

	// Read core Ingress resources
	ingresses, err := common.ReadIngressesFromCluster(ctx, r.conf.Client, CommonNginxIngressClasses)
	if err != nil {
		return nil, err
	}
	storage.Ingresses = ingresses

	// Read VirtualServer CRDs
	virtualServers, err := r.readVirtualServersFromCluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read VirtualServers: %w", err)
	}
	storage.VirtualServers = virtualServers

	// Read Services
	services, err := common.ReadServicesFromCluster(ctx, r.conf.Client)
	if err != nil {
		return nil, err
	}
	storage.ServicePorts = common.GroupServicePortsByPortName(services)

	return storage, nil
}

func (r *resourceReader) readResourcesFromFile(filename string) (*storage, error) {
	storage := newResourceStorage()

	// Read core Ingress resources
	ingresses, err := common.ReadIngressesFromFile(filename, r.conf.Namespace, CommonNginxIngressClasses)
	if err != nil {
		return nil, err
	}
	storage.Ingresses = ingresses

	// Read VirtualServer CRDs
	virtualServers, err := r.readVirtualServersFromFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read VirtualServers: %w", err)
	}
	storage.VirtualServers = virtualServers

	// Read Services
	services, err := common.ReadServicesFromFile(filename, r.conf.Namespace)
	if err != nil {
		return nil, err
	}
	storage.ServicePorts = common.GroupServicePortsByPortName(services)

	return storage, nil
}

func (r *resourceReader) readVirtualServersFromCluster(ctx context.Context) ([]nginxv1.VirtualServer, error) {
	virtualServerList := &unstructured.UnstructuredList{}
	virtualServerList.SetGroupVersionKind(annotations.VirtualServerGVK)

	err := r.conf.Client.List(ctx, virtualServerList)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", annotations.VirtualServerGVK.GroupKind().String(), err)
	}

	var virtualServers []nginxv1.VirtualServer
	for _, obj := range virtualServerList.Items {
		// Apply namespace filtering if configured
		if r.conf.Namespace != "" && obj.GetNamespace() != r.conf.Namespace {
			continue
		}

		var virtualServer nginxv1.VirtualServer
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &virtualServer); err != nil {
			return nil, fmt.Errorf("failed to parse NGINX VirtualServer object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}

		virtualServers = append(virtualServers, virtualServer)
	}

	return virtualServers, nil
}

func (r *resourceReader) readVirtualServersFromFile(filename string) ([]nginxv1.VirtualServer, error) {
	stream, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %v: %w", filename, err)
	}

	reader := bytes.NewReader(stream)
	objs, err := common.ExtractObjectsFromReader(reader, r.conf.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to extract objects: %w", err)
	}

	virtualServers := []nginxv1.VirtualServer{}
	for _, obj := range objs {
		if r.conf.Namespace != "" && obj.GetNamespace() != r.conf.Namespace {
			continue
		}
		// Use the standardized GVK constant instead of hardcoded values
		if !obj.GroupVersionKind().Empty() && obj.GroupVersionKind() == annotations.VirtualServerGVK {
			var vs nginxv1.VirtualServer
			err = runtime.DefaultUnstructuredConverter.
				FromUnstructured(obj.UnstructuredContent(), &vs)
			if err != nil {
				return nil, fmt.Errorf("failed to parse VirtualServer object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			virtualServers = append(virtualServers, vs)
		}
	}

	return virtualServers, nil
}
