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
	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func endpoints(ns, name string, subsets ...core_v1.EndpointSubset) *core_v1.Endpoints {
	return &core_v1.Endpoints{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Subsets: subsets,
	}
}

func addresses(ips ...string) []core_v1.EndpointAddress {
	var addrs []core_v1.EndpointAddress
	for _, ip := range ips {
		addrs = append(addrs, core_v1.EndpointAddress{IP: ip})
	}
	return addrs
}

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
