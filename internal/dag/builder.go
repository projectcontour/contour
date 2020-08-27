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

package dag

import (
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
)

type RouteServiceName struct {
	Name      string
	Namespace string
	Port      int32
}

// Processor uses a DAG builder to construct part
// of a DAG.
type Processor interface {
	// Run executes the processor with the given Builder.
	Run(builder *Builder)
}

// Builder builds a DAG.
type Builder struct {
	// Source is the source of Kubernetes objects
	// from which to build a DAG.
	Source KubernetesCache

	// Processors is the ordered list of Processors to
	// use to build the DAG.
	Processors []Processor

	services           map[RouteServiceName]*Service
	virtualhosts       map[string]*VirtualHost
	securevirtualhosts map[string]*SecureVirtualHost
	listeners          []*Listener

	StatusWriter
	logrus.FieldLogger
}

// Build builds and returns a new DAG by running the
// configured DAG processors, in order.
func (b *Builder) Build() *DAG {
	b.reset()

	for _, p := range b.Processors {
		p.Run(b)
	}

	var dag DAG

	for i := range b.listeners {
		dag.roots = append(dag.roots, b.listeners[i])
	}

	dag.statuses = b.statuses
	return &dag
}

// reset (re)inialises the internal state of the builder.
func (b *Builder) reset() {
	b.services = make(map[RouteServiceName]*Service, len(b.services))
	b.virtualhosts = make(map[string]*VirtualHost)
	b.securevirtualhosts = make(map[string]*SecureVirtualHost)
	b.listeners = []*Listener{}

	b.statuses = make(map[types.NamespacedName]Status, len(b.statuses))
}

// lookupService returns a Service that matches the Meta and Port of the Kubernetes' Service,
// or an error if the service or port can't be located.
func (b *Builder) lookupService(m types.NamespacedName, port intstr.IntOrString) (*Service, error) {
	lookup := func() *Service {
		if port.Type != intstr.Int {
			// can't handle, give up
			return nil
		}
		return b.services[RouteServiceName{
			Name:      m.Name,
			Namespace: m.Namespace,
			Port:      int32(port.IntValue()),
		}]
	}

	s := lookup()
	if s != nil {
		return s, nil
	}
	svc, ok := b.Source.services[m]
	if !ok {
		return nil, fmt.Errorf("service %q not found", m)
	}

	for i := range svc.Spec.Ports {
		p := svc.Spec.Ports[i]
		if int(p.Port) == port.IntValue() || port.String() == p.Name {
			switch p.Protocol {
			case "", v1.ProtocolTCP:
			default:
				return nil, fmt.Errorf("unsupported service protocol %q", p.Protocol)
			}

			return b.addService(svc, p), nil
		}
	}

	return nil, fmt.Errorf("port %q on service %q not matched", port.String(), m)
}

func (b *Builder) addService(svc *v1.Service, port v1.ServicePort) *Service {
	name := k8s.NamespacedNameOf(svc)
	s := &Service{
		Weighted: WeightedService{
			ServiceName:      name.Name,
			ServiceNamespace: name.Namespace,
			ServicePort:      port,
			Weight:           1,
		},
		Protocol:           upstreamProtocol(svc, port),
		MaxConnections:     annotation.MaxConnections(svc),
		MaxPendingRequests: annotation.MaxPendingRequests(svc),
		MaxRequests:        annotation.MaxRequests(svc),
		MaxRetries:         annotation.MaxRetries(svc),
		ExternalName:       externalName(svc),
	}

	b.services[RouteServiceName{
		Name:      name.Name,
		Namespace: name.Namespace,
		Port:      port.Port,
	}] = s

	return s
}

func upstreamProtocol(svc *v1.Service, port v1.ServicePort) string {
	up := annotation.ParseUpstreamProtocols(svc.Annotations)
	protocol := up[port.Name]
	if protocol == "" {
		protocol = up[strconv.Itoa(int(port.Port))]
	}
	return protocol
}

func (b *Builder) lookupVirtualHost(name string) *VirtualHost {
	vh, ok := b.virtualhosts[name]
	if !ok {
		vh := &VirtualHost{
			Name: name,
		}
		b.virtualhosts[vh.Name] = vh
		return vh
	}
	return vh
}

func (b *Builder) lookupSecureVirtualHost(name string) *SecureVirtualHost {
	svh, ok := b.securevirtualhosts[name]
	if !ok {
		svh := &SecureVirtualHost{
			VirtualHost: VirtualHost{
				Name: name,
			},
		}
		b.securevirtualhosts[svh.VirtualHost.Name] = svh
		return svh
	}
	return svh
}

func externalName(svc *v1.Service) string {
	if svc.Spec.Type != v1.ServiceTypeExternalName {
		return ""
	}
	return svc.Spec.ExternalName
}

// validSecret returns true if the Secret contains certificate and private key material.
func validSecret(s *v1.Secret) error {
	if s.Type != v1.SecretTypeTLS {
		return fmt.Errorf("Secret type is not %q", v1.SecretTypeTLS)
	}

	if len(s.Data[v1.TLSCertKey]) == 0 {
		return fmt.Errorf("empty %q key", v1.TLSCertKey)
	}

	if len(s.Data[v1.TLSPrivateKeyKey]) == 0 {
		return fmt.Errorf("empty %q key", v1.TLSPrivateKeyKey)
	}

	return nil
}
