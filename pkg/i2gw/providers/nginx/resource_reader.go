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
	"context"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
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

	// VirtualServer support removed to reduce PR size

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

	// VirtualServer support removed to reduce PR size

	// Read Services
	services, err := common.ReadServicesFromFile(filename, r.conf.Namespace)
	if err != nil {
		return nil, err
	}
	storage.ServicePorts = common.GroupServicePortsByPortName(services)

	return storage, nil
}

// VirtualServer support removed to reduce PR size

// VirtualServer support removed to reduce PR size
