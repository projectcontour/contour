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
	if ok {
		// found, that means v is already a child of any ingressVertex
		// that requires it, just update the attached object and we're done
		v.object = s
		return
	}

	// the vertex was not present in the secret map which means it is not
	// a attached to any ingressVertices.
	v = &Secret{
		object: s,
	}
	if d.secrets == nil {
		d.secrets = make(map[meta]*Secret)
	}
	d.secrets[m] = v

	// foreach root, if r.object references this secret, attach secret as child.
}

// insertService inserts a Servce into the DAG. If there is an existing Service with
// the same name and namespace, it will be replaced.
func (d *DAG) insertService(s *v1.Service) {

	m := meta{name: s.Name, namespace: s.Namespace}

	// lookup vertex in services map
	v, ok := d.services[m]
	if ok {
		// found, that means v is already a child of any prefixVertex
		// that requires it, just update the attached object and we're done
		v.object = s
		return
	}

	// the vertex was not present in the services map which means it is not
	// a attached to any prefixVertex.
	v = &Service{
		object: s,
	}
	if d.services == nil {
		d.services = make(map[meta]*Service)
	}
	d.services[m] = v

	// foreach root, foreach route, attach this vertex as a child if the
	// name and namespace match.
	d.visit(func(virtualhost Vertex) {
		virtualhost.Visit(func(v Vertex) {
			r, ok := v.(*Route)
			if !ok {
				// not a route, skip it
				return
			}
			if r.object.Namespace != s.Namespace {
				// route's ingress object doesn't match service
				return
			}
			if r.object.Spec.Backend != nil && r.object.Spec.Backend.ServiceName == s.Name {
				r.insertIfNotPresent(v)
			}
		})
	})
}

func (d *DAG) insertIngress(i *v1beta1.Ingress) {
	if i.Spec.Backend != nil {
		vh := d.virtualhost("*")
		r := &Route{
			path:   "/",
			object: i,
		}
		vh.routes[r.path] = r

		m := meta{name: i.Spec.Backend.ServiceName, namespace: i.Namespace}
		s, ok := d.services[m]
		if ok {
			r.children = append(r.children, s)
		}
	}

	for _, rule := range i.Spec.Rules {
		host := rule.Host
		if host == "" {
			host = "*"
		}
		vh := d.virtualhost(host)

		for _, tls := range i.Spec.TLS {
			if tls.SecretName == "" {
				continue
			}
			m := meta{name: tls.SecretName, namespace: i.Namespace}
			if s, ok := d.secrets[m]; ok {
				vh.secrets[m] = s
			}
		}

		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			path := p.Path
			if path == "" {
				path = "/"
			}
			r := &Route{
				path:   path,
				object: i,
			}
			vh.routes[r.path] = r

			m := meta{name: p.Backend.ServiceName, namespace: i.Namespace}
			s, ok := d.services[m]
			if !ok {
				continue
			}
			// add this service as a child of the route
			r.children = append(r.children, s)
		}
	}
}

func (d *DAG) virtualhost(host string) *VirtualHost {
	vh, ok := d.roots[host]
	if ok {
		return vh
	}
	vh = &VirtualHost{
		host:    host,
		routes:  make(map[string]*Route),
		secrets: make(map[meta]*Secret),
	}
	if d.roots == nil {
		d.roots = make(map[string]*VirtualHost)
	}
	d.roots[vh.host] = vh
	return vh
}

type Root interface {
	Vertex
}

type Route struct {
	path     string
	object   *v1beta1.Ingress // the ingress which mentioned this route
	children []Vertex
}

func (r *Route) insertIfNotPresent(v Vertex) {
	// TODO(dfc) hack
	r.children = append(r.children, v)
}

func (r *Route) Prefix() string { return r.path }

func (r *Route) Visit(f func(Vertex)) {
	for _, c := range r.children {
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

// Secret represents a K8s Secret as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	object *v1.Secret
}

func (s *Secret) Name() string       { return s.object.Name }
func (s *Secret) Namespace() string  { return s.object.Namespace }
func (s *Secret) Visit(func(Vertex)) {}
