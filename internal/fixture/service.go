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

package fixture

import (
	core_v1 "k8s.io/api/core/v1"
)

type ServiceBuilder core_v1.Service

// NewService creates a new ServiceBuilder with the given resource name.
func NewService(name string) *ServiceBuilder {
	s := &ServiceBuilder{
		ObjectMeta: ObjectMeta(name),
		Spec:       core_v1.ServiceSpec{},
	}

	return s
}

// Annotate adds the given values as metadata annotations.
func (s *ServiceBuilder) Annotate(k, v string) *ServiceBuilder {
	s.ObjectMeta.Annotations[k] = v
	return s
}

// WithPorts specifies the ports for the .Spec.Ports field.
func (s *ServiceBuilder) WithPorts(ports ...core_v1.ServicePort) *core_v1.Service {
	s.Spec.Ports = make([]core_v1.ServicePort, len(ports))

	copy(s.Spec.Ports, ports)

	for _, p := range s.Spec.Ports {
		if p.Protocol == "" {
			p.Protocol = "TCP"
		}
	}

	return (*core_v1.Service)(s)
}

// WithSpec specifies the .Spec field.
func (s *ServiceBuilder) WithSpec(spec core_v1.ServiceSpec) *core_v1.Service {
	s.Spec = spec

	for _, p := range s.Spec.Ports {
		if p.Protocol == "" {
			p.Protocol = "TCP"
		}
	}

	return (*core_v1.Service)(s)
}
