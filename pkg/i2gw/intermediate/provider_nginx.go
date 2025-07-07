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

package intermediate

// NginxServiceIR holds nginx-specific service configurations
// from annotations that need provider-specific tracking
type NginxServiceIR struct {
	// Application protocol for backend services (e.g., "https", "grpc")
	AppProtocol string
}

// VirtualServer and other CRD-specific structures removed to reduce PR size.
// This file now only contains annotation-specific IR extensions.