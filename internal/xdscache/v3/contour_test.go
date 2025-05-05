// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v3

import (
	discovery_v1 "k8s.io/api/discovery/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func endpointSlice(ns, name, service string, addressType discovery_v1.AddressType, endpoints []discovery_v1.Endpoint, ports []discovery_v1.EndpointPort) *discovery_v1.EndpointSlice {
	return &discovery_v1.EndpointSlice{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				discovery_v1.LabelServiceName: service,
			},
		},

		AddressType: addressType,
		Endpoints:   endpoints,
		Ports:       ports,
	}
}
