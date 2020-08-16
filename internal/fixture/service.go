package fixture

import (
	v1 "k8s.io/api/core/v1"
)

type ServiceBuilder v1.Service

// NewService creates a new ServiceBuilder with the given resource name.
func NewService(name string) *ServiceBuilder {
	s := &ServiceBuilder{
		ObjectMeta: ObjectMeta(name),
		Spec:       v1.ServiceSpec{},
	}

	return s
}

// Annotate adds the given values as metadata annotations.
func (s *ServiceBuilder) Annotate(k string, v string) *ServiceBuilder {
	s.ObjectMeta.Annotations[k] = v
	return s
}

// WithPorts specifies the ports for the .Spec.Ports field.
func (s *ServiceBuilder) WithPorts(ports ...v1.ServicePort) *v1.Service {
	s.Spec.Ports = make([]v1.ServicePort, len(ports))

	copy(s.Spec.Ports, ports)

	for _, p := range s.Spec.Ports {
		if p.Protocol == "" {
			p.Protocol = "TCP"
		}
	}

	return (*v1.Service)(s)
}

// WithSpec specifies the .Spec field.
func (s *ServiceBuilder) WithSpec(spec v1.ServiceSpec) *v1.Service {
	s.Spec = spec

	for _, p := range s.Spec.Ports {
		if p.Protocol == "" {
			p.Protocol = "TCP"
		}
	}

	return (*v1.Service)(s)
}
