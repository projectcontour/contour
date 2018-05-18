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
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
)

// A DAG represents a directed acylic graph of objects representing the relationship
// between Kubernetes Ingress objects, the backend Services, and Secret objects.
// The DAG models these relationships as Roots and Vertices.
//
// A DAG is mutable and not thread safe.
type DAG struct {
	roots    map[string]*VirtualHost
	secrets  map[meta]*Secret
	services map[meta]*Service
}

// meta holds the name and namespace of a Kubernetes object.
type meta struct {
	name, namespace string
}

// Roots calls the function f for each Root registered from this DAG.
func (d *DAG) Roots(f func(Vertex)) {
	for _, r := range d.roots {
		f(r)
	}
}

// Vertices calls the function f for each Vertex registered with this DAG.
// This includes Vertices which are not reachable from a Root, ie, those that
// are orphaned.
func (d *DAG) Vertices(f func(Vertex)) {
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

// InsertSecret inserts a Secret into the DAG. If there is an existing Service with
// the same name and namespace, it will be replaced.
func (d *DAG) InsertSecret(s *v1.Secret) {

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

// InsertService inserts a Servce into the DAG. If there is an existing Service with
// the same name and namespace, it will be replaced.
func (d *DAG) InsertService(s *v1.Service) {

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

	// foreach root, foreach prefixVertex, attach this vertex as a child if the
	// name and namespace match.
}

func (d *DAG) InsertIngress(i *v1beta1.Ingress) {
	for _, rule := range i.Spec.Rules {
		if rule.Host == "" {
			fmt.Println("skipping blank host", i.Name, i.Namespace)
			continue
		}
		r := &VirtualHost{
			object: i,
			host:   rule.Host,
		}

		for _, tls := range i.Spec.TLS {
			if tls.SecretName == "" {
				continue
			}
			m := meta{name: tls.SecretName, namespace: i.Namespace}
			s, ok := d.secrets[m]
			if !ok {
				continue
			}
			// add this secret as a child of the virtualhost so that
			// the ingress_https vistor can find it.
			r.children = append(r.children, s)
		}

		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			path := p.Path
			if path == "" {
				path = "/"
			}
			rr := &Route{
				path: path,
			}
			m := meta{name: p.Backend.ServiceName, namespace: i.Namespace}
			s, ok := d.services[m]
			if !ok {
				continue
			}
			// add this service as a child of the route
			rr.children = append(rr.children, s)
			// add this route as a child of the vhost
			r.children = append(r.children, rr)
		}

		if d.roots == nil {
			d.roots = make(map[string]*VirtualHost)
		}
		d.roots[rule.Host] = r
	}
}

type Root interface {
	Vertex
}

type Route struct {
	vertices
	path string
}

func (r *Route) Prefix() string { return r.path }

type VirtualHost struct {
	vertices
	host   string
	object *v1beta1.Ingress
}

func (v *VirtualHost) FQDN() string { return v.host }

type Vertex interface {
	ChildVertices(func(Vertex))
}

// Secret represents a K8s Sevice as a DAG vertex. A Serivce is
// a leaf in the DAG.
type Service struct {
	leaf
	object *v1.Service
}

func (s *Service) Name() string      { return s.object.Name }
func (s *Service) Namespace() string { return s.object.Namespace }

// Secret represents a K8s Secret as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	leaf
	object *v1.Secret
}

func (s *Secret) Name() string      { return s.object.Name }
func (s *Secret) Namespace() string { return s.object.Namespace }

// leaf is a helper type for vertices which hold no children.
type leaf struct{}

func (l *leaf) ChildVertices(func(Vertex)) {}

func (l *leaf) HasChildren() bool { return false }

type vertices struct {
	children []Vertex
}

func (v *vertices) ChildVertices(f func(Vertex)) {
	for _, c := range v.children {
		f(c)
	}
}

func (v *vertices) HasChildren() bool { return len(v.children) > 0 }
