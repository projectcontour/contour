// Copyright © 2018 Heptio
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

// Package dag provides a data model, in the form of a directed acyclic graph,
// of the relationship between Kubernetes Ingress, Service, and Secret objects.
package dag

import (
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// A DAG represents a directed acylic graph of objects representing the relationship
// between Kubernetes Ingress objects, the backend Services, and Secret objects.
// The DAG models these relationships as Roots and Vertices.
//
// A DAG is mutable and not thread safe.
type DAG struct {
	rwm sync.RWMutex

	roots    map[string]*VirtualHost
	secrets  map[meta]*Secret
	services map[meta]*Service
}

// meta holds the name and namespace of a Kubernetes object.
type meta struct {
	name, namespace string
}

// Visit calls f for every root of this DAG.
func (d *DAG) Visit(f func(Vertex)) {
	d.rwm.RLock()
	defer d.rwm.RUnlock()
	d.visit(f)
}

func (d *DAG) visit(f func(Vertex)) {
	for _, r := range d.roots {
		f(r)
	}
}

// VisitAll calls the function f for each Vertex registered with this DAG.
// This includes Vertices which are not reachable from a Root, ie, those that
// are orphaned.
func (d *DAG) VisitAll(f func(Vertex)) {
	d.rwm.RLock()
	defer d.rwm.RUnlock()
	for _, v := range d.roots {
		f(v)
	}
	for _, v := range d.secrets {
		f(v)
	}
	for _, v := range d.services {
		f(v)
	}
}

// Insert inserts obj into the DAG. If an object with a matching type, name, and
// namespace exists, it will be overwritten.
func (d *DAG) Insert(obj interface{}) {
	d.rwm.Lock()
	defer d.rwm.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		d.insertSecret(obj)
	case *v1.Service:
		d.insertService(obj)
	case *v1beta1.Ingress:
		d.insertIngress(obj)
	}
}

// Remove removes obj from the DAG. If no object with a matching type, name, and
// namespace exists in the DAG, no action is taken.
func (d *DAG) Remove(obj interface{}) {
	d.rwm.Lock()
	defer d.rwm.Unlock()
	switch obj := obj.(type) {
	case cache.DeletedFinalStateUnknown:
		d.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

// insertSecret inserts a Secret into the DAG. If there is an existing Service with
// the same name and namespace, it will be replaced.
func (d *DAG) insertSecret(s *v1.Secret) {

	m := meta{name: s.Name, namespace: s.Namespace}

	// lookup vertex in secrets map
	v, ok := d.secrets[m]
	if !ok {
		v = new(Secret)
		if d.secrets == nil {
			d.secrets = make(map[meta]*Secret)
		}
		d.secrets[m] = v
	}
	v.object = s

	// now visit each root and if there is an ingress object matching this secret,
	// add this secret as a child of that root.
	d.visit(func(v Vertex) {
		vh := v.(*VirtualHost)
		vh.Visit(func(v Vertex) {
			r, ok := v.(*Route)
			if !ok {
				// not a route, skip it
				return
			}
			if r.object.Namespace != s.Namespace {
				// this secret does not match the namespace the ingress
				// that geneated this route belogs to, skip it.
				return
			}
			for _, tls := range r.object.Spec.TLS {
				d.addSecret(&tls, vh, tls.SecretName, s.Namespace)
			}
		})
	})
}

// addSecret looks up the named secret and if it exists
// adds it to vh.
func (d *DAG) addSecret(tls *v1beta1.IngressTLS, vh *VirtualHost, name, namespace string) {
	m := meta{name: name, namespace: namespace}
	if s, ok := d.secrets[m]; ok {
		for _, host := range tls.Hosts {
			if host == vh.FQDN() {
				vh.addSecret(s)
			}
		}
	}
}

// insertService inserts a Servce into the DAG. If there is an existing Service with
// the same name and namespace, it will be replaced.
func (d *DAG) insertService(svc *v1.Service) {
	s := d.service(svc)

	// foreach root, foreach route, attach this vertex as a child if the
	// name and namespace match.
	d.visit(func(virtualhost Vertex) {
		virtualhost.Visit(func(v Vertex) {
			r, ok := v.(*Route)
			if !ok {
				// not a route, skip it
				return
			}
			if r.object.Namespace != s.object.Namespace {
				// route's ingress object doesn't match service
				return
			}
			if r.backend.ServiceName != s.object.Name {
				// services's name doesn't match ingress's backend
				return
			}
			// iterate through the ports on the service object, if we
			// find a match against the port's name or number, we add
			// the service as a child of the route.
			for _, p := range s.object.Spec.Ports {
				if r.backend.ServicePort.IntValue() == int(p.Port) || r.backend.ServicePort.String() == p.Name {
					r.addService(s)
				}
			}
		})
	})
}

func (d *DAG) insertIngress(i *v1beta1.Ingress) {
	if i.Spec.Backend != nil {
		vh := d.virtualhost("*")
		r := &Route{
			path:    "/",
			object:  i,
			backend: i.Spec.Backend,
		}
		vh.routes[r.path] = r

		m := meta{name: r.backend.ServiceName, namespace: r.object.Namespace}
		if s, ok := d.services[m]; ok {
			// iterate through the ports on the service object, if we
			// find a match against the backends port's name or number, we add
			// the service as a child of the route.
			for _, p := range s.object.Spec.Ports {
				if r.backend.ServicePort.IntValue() == int(p.Port) || r.backend.ServicePort.String() == p.Name {
					r.addService(s)
				}
			}
		}
	}

	for _, rule := range i.Spec.Rules {
		host := rule.Host
		if host == "" {
			host = "*"
		}
		vh := d.virtualhost(host)

		for _, tls := range i.Spec.TLS {
			d.addSecret(&tls, vh, tls.SecretName, i.Namespace)
		}

		for n := range rule.IngressRuleValue.HTTP.Paths {
			path := rule.IngressRuleValue.HTTP.Paths[n].Path
			if path == "" {
				path = "/"
			}
			r := &Route{
				path:    path,
				object:  i,
				backend: &rule.IngressRuleValue.HTTP.Paths[n].Backend,
			}
			vh.routes[r.path] = r

			m := meta{name: r.backend.ServiceName, namespace: r.object.Namespace}
			if s, ok := d.services[m]; ok {
				// iterate through the ports on the service object, if we
				// find a match against the backends port's name or number, we add
				// the service as a child of the route.
				for _, p := range s.object.Spec.Ports {
					if r.backend.ServicePort.IntValue() == int(p.Port) || r.backend.ServicePort.String() == p.Name {
						r.addService(s)
					}
				}
			}
		}
	}
}

// virtualhost returns the *VirtualHost record
// for this host. If none exists, it is created.
func (d *DAG) virtualhost(host string) *VirtualHost {
	vh, ok := d.roots[host]
	if ok {
		return vh
	}
	vh = &VirtualHost{
		host:   host,
		routes: make(map[string]*Route),
	}
	if d.roots == nil {
		d.roots = make(map[string]*VirtualHost)
	}
	d.roots[vh.host] = vh
	return vh
}

// service returns the *Service record for the *v1.Service.
// If none exists, it is created.
func (d *DAG) service(svc *v1.Service) *Service {
	m := meta{name: svc.Name, namespace: svc.Namespace}
	s, ok := d.services[m]
	if !ok {
		s = new(Service)
		if d.services == nil {
			d.services = make(map[meta]*Service)
		}
		d.services[m] = s
	}
	s.object = svc
	return s
}

type Root interface {
	Vertex
}

type Route struct {
	path     string
	object   *v1beta1.Ingress // the ingress which mentioned this route
	services map[meta]*Service
	backend  *v1beta1.IngressBackend
}

func (r *Route) Prefix() string      { return r.path }
func (r *Route) ServicePort() string { return r.backend.ServicePort.String() }

func (r *Route) addService(s *Service) {
	if r.services == nil {
		r.services = make(map[meta]*Service)
	}
	r.services[s.toMeta()] = s
}

func (r *Route) Visit(f func(Vertex)) {
	for _, c := range r.services {
		f(c)
	}
}

type VirtualHost struct {
	host    string
	routes  map[string]*Route
	secrets map[meta]*Secret
}

func (v *VirtualHost) FQDN() string { return v.host }

func (v *VirtualHost) Visit(f func(Vertex)) {
	for _, r := range v.routes {
		f(r)
	}
	for _, s := range v.secrets {
		f(s)
	}
}

func (v *VirtualHost) addSecret(s *Secret) {
	if v.secrets == nil {
		v.secrets = make(map[meta]*Secret)
	}
	v.secrets[s.toMeta()] = s
}

type Vertex interface {
	Visit(func(Vertex))
}

// Secret represents a K8s Sevice as a DAG vertex. A Serivce is
// a leaf in the DAG.
type Service struct {
	object *v1.Service
}

func (s *Service) Name() string       { return s.object.Name }
func (s *Service) Namespace() string  { return s.object.Namespace }
func (s *Service) Visit(func(Vertex)) {}

func (s *Service) toMeta() meta {
	return meta{
		name:      s.object.Name,
		namespace: s.object.Namespace,
	}
}

// Secret represents a K8s Secret as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	object *v1.Secret
}

func (s *Secret) Name() string       { return s.object.Name }
func (s *Secret) Namespace() string  { return s.object.Namespace }
func (s *Secret) Visit(func(Vertex)) {}

func (s *Secret) toMeta() meta {
	return meta{
		name:      s.object.Name,
		namespace: s.object.Namespace,
	}
}
