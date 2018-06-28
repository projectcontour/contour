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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"

	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
)

// A DAG represents a directed acylic graph of objects representing the relationship
// between Kubernetes Ingress objects, the backend Services, and Secret objects.
// The DAG models these relationships as Roots and Vertices.
type DAG struct {
	mu sync.Mutex

	ingresses     map[meta]*v1beta1.Ingress
	ingressroutes map[meta]*ingressroutev1.IngressRoute
	secrets       map[meta]*v1.Secret
	services      map[meta]*v1.Service

	dag *dag
}

// dag represents
type dag struct {
	// roots are the roots of this dag
	roots []Vertex
}

// meta holds the name and namespace of a Kubernetes object.
type meta struct {
	name, namespace string
}

// Visit calls f for every root of this DAG.
func (d *DAG) Visit(f func(Vertex)) {
	d.mu.Lock()
	dag := d.dag
	d.mu.Unlock()
	for _, r := range dag.roots {
		f(r)
	}
}

// Insert inserts obj into the DAG. If an object with a matching type, name, and
// namespace exists, it will be overwritten.
func (d *DAG) Insert(obj interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.secrets == nil {
			d.secrets = make(map[meta]*v1.Secret)
		}
		d.secrets[m] = obj
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.services == nil {
			d.services = make(map[meta]*v1.Service)
		}
		d.services[m] = obj
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.ingresses == nil {
			d.ingresses = make(map[meta]*v1beta1.Ingress)
		}
		d.ingresses[m] = obj

	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.ingressroutes == nil {
			d.ingressroutes = make(map[meta]*ingressroutev1.IngressRoute)
		}
		d.ingressroutes[m] = obj
	default:
		// not an interesting object
	}
}

// Remove removes obj from the DAG. If no object with a matching type, name, and
// namespace exists in the DAG, no action is taken.
func (d *DAG) Remove(obj interface{}) {
	switch obj := obj.(type) {
	default:
		d.remove(obj)
	case cache.DeletedFinalStateUnknown:
		d.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (d *DAG) remove(obj interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.secrets, m)
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.services, m)
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.ingresses, m)
	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.ingressroutes, m)
	default:
		// not interesting
	}
}

// Recompute recomputes the DAG.
func (d *DAG) Recompute() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dag = d.recompute()
}

// serviceMap memoise access to a service map, built
// as needed from the list of services cached
// from k8s.
type serviceMap struct {

	// backing services from k8s api.
	services map[meta]*v1.Service

	// cached Services.
	_services map[portmeta]*Service
}

// lookup returns a Service that matches the meta and port supplied.
// If no matching Service is found lookup returns nil.
func (sm *serviceMap) lookup(m meta, port intstr.IntOrString) *Service {
	if port.Type == intstr.Int {
		if s, ok := sm._services[portmeta{name: m.name, namespace: m.namespace, port: int(port.IntValue())}]; ok {
			return s
		}
	}
	svc, ok := sm.services[m]
	if !ok {
		return nil
	}
	for _, p := range svc.Spec.Ports {
		if int(p.Port) == port.IntValue() {
			return sm.insert(svc, int(p.Port))
		}
		if port.String() == p.Name {
			return sm.insert(svc, int(p.Port))
		}
	}
	return nil
}

func (sm *serviceMap) insert(svc *v1.Service, port int) *Service {
	if sm._services == nil {
		sm._services = make(map[portmeta]*Service)
	}
	s := &Service{
		object: svc,
		Port:   port,
	}
	sm._services[s.toMeta()] = s
	return s
}

// recompute builds a new *dag.dag.
func (d *DAG) recompute() *dag {
	sm := serviceMap{
		services: d.services,
	}
	service := sm.lookup

	// memoise access to a secrets map, built
	// as needed from the list of secrets cached
	// from k8s.
	_secrets := make(map[meta]*Secret)
	secret := func(m meta) *Secret {
		if s, ok := _secrets[m]; ok {
			return s
		}
		sec, ok := d.secrets[m]
		if !ok {
			return nil
		}
		s := &Secret{
			object: sec,
		}
		_secrets[s.toMeta()] = s
		return s
	}

	type hostport struct {
		host string
		port int
	}

	// memoise the production of vhost entries as needed.
	_vhosts := make(map[hostport]*VirtualHost)
	vhost := func(host string, port int) *VirtualHost {
		hp := hostport{host: host, port: port}
		vh, ok := _vhosts[hp]
		if !ok {
			vh = &VirtualHost{
				Port:   port,
				host:   host,
				routes: make(map[string]*Route),
			}
			_vhosts[hp] = vh
		}
		return vh
	}

	_svhosts := make(map[hostport]*SecureVirtualHost)
	svhost := func(host string, port int) *SecureVirtualHost {
		hp := hostport{host: host, port: port}
		svh, ok := _svhosts[hp]
		if !ok {
			svh = &SecureVirtualHost{
				Port:   port,
				host:   host,
				routes: make(map[string]*Route),
			}
			_svhosts[hp] = svh
		}
		return svh
	}

	// deconstruct each ingress into routes and virtualhost entries
	for _, ing := range d.ingresses {
		// should we create port 80 routes for this ingress
		httpAllowed := httpAllowed(ing)

		if ing.Spec.Backend != nil {
			// handle the annoying default ingress
			r := &Route{
				path:   "/",
				object: ing,
			}
			m := meta{name: ing.Spec.Backend.ServiceName, namespace: ing.Namespace}
			if s := service(m, ing.Spec.Backend.ServicePort); s != nil {
				r.addService(s)
			}
			if httpAllowed {
				vhost("*", 80).routes[r.path] = r
			}
		}

		// attach secrets from ingress to vhosts
		for _, tls := range ing.Spec.TLS {
			m := meta{name: tls.SecretName, namespace: ing.Namespace}
			if sec := secret(m); sec != nil {
				for _, host := range tls.Hosts {
					svhost(host, 443).secret = sec
				}
			}
		}

		for _, rule := range ing.Spec.Rules {
			// handle Spec.Rule declarations
			host := rule.Host
			if host == "" {
				host = "*"
			}
			for n := range rule.IngressRuleValue.HTTP.Paths {
				path := rule.IngressRuleValue.HTTP.Paths[n].Path
				if path == "" {
					path = "/"
				}
				r := &Route{
					path:   path,
					object: ing,
				}

				m := meta{name: rule.IngressRuleValue.HTTP.Paths[n].Backend.ServiceName, namespace: ing.Namespace}
				if s := service(m, rule.IngressRuleValue.HTTP.Paths[n].Backend.ServicePort); s != nil {
					r.addService(s)
				}
				if httpAllowed {
					vhost(host, 80).routes[r.path] = r
				}
				if _, ok := _svhosts[hostport{host: host, port: 443}]; ok && host != "*" {
					svhost(host, 443).routes[r.path] = r
				}
			}
		}
	}

	// process ingressroute documents
	for _, ir := range d.ingressroutes {
		if ir.Spec.VirtualHost == nil {
			// delegate ingressroute, skip it
			continue
		}

		host := ir.Spec.VirtualHost.Fqdn

		if tls := ir.Spec.VirtualHost.TLS; tls != nil {
			// attach secrets to TLS enabled vhosts
			m := meta{name: tls.SecretName, namespace: ir.Namespace}
			if sec := secret(m); sec != nil {
				svhost(host, 443).secret = sec
			}
		}

		// attach routes to vhost
		for _, route := range ir.Spec.Routes {
			r := &Route{
				path:   route.Match,
				object: ir,
			}
			for _, s := range route.Services {
				m := meta{name: s.Name, namespace: ir.Namespace}
				if svc := service(m, intstr.FromInt(s.Port)); svc != nil {
					r.addService(svc)
				}
			}
			vhost(host, 80).routes[r.path] = r
		}
	}

	_d := new(dag)
	for _, vh := range _vhosts {
		_d.roots = append(_d.roots, vh)
	}
	for _, svh := range _svhosts {
		_d.roots = append(_d.roots, svh)
	}
	return _d
}

type Root interface {
	Vertex
}

type Route struct {
	path     string
	object   interface{} // one of Ingress or IngressRoute
	services map[portmeta]*Service
}

func (r *Route) Prefix() string { return r.path }

func (r *Route) addService(s *Service) {
	if r.services == nil {
		r.services = make(map[portmeta]*Service)
	}
	r.services[s.toMeta()] = s
}

func (r *Route) Visit(f func(Vertex)) {
	for _, c := range r.services {
		f(c)
	}
}

// A VirtualHost represents an insecure HTTP host.
type VirtualHost struct {
	// Port is the port that the VirtualHost will listen on.
	// Expected values are 80 and 443, but others are possible
	// if the VirtualHost is generated inside Contour.
	Port int

	host   string
	routes map[string]*Route
}

func (v *VirtualHost) FQDN() string { return v.host }

func (v *VirtualHost) Visit(f func(Vertex)) {
	for _, r := range v.routes {
		f(r)
	}
}

// A SecureVirtualHost represents a HTTP host protected by TLS.
type SecureVirtualHost struct {
	// Port is the port that the VirtualHost will listen on.
	// Expected values are 80 and 443, but others are possible
	// if the VirtualHost is generated inside Contour.
	Port int

	host   string
	routes map[string]*Route
	secret *Secret
}

func (s *SecureVirtualHost) FQDN() string { return s.host }
func (s *SecureVirtualHost) Visit(f func(Vertex)) {
	for _, r := range s.routes {
		f(r)
	}
	f(s.secret)
}

type Vertex interface {
	Visit(func(Vertex))
}

// Secret represents a K8s Sevice as a DAG vertex. A Serivce is
// a leaf in the DAG.
type Service struct {
	object *v1.Service

	// Port is the port of this service
	Port int
}

func (s *Service) Name() string       { return s.object.Name }
func (s *Service) Namespace() string  { return s.object.Namespace }
func (s *Service) Visit(func(Vertex)) {}

type portmeta struct {
	name      string
	namespace string
	port      int
}

func (s *Service) toMeta() portmeta {
	return portmeta{
		name:      s.object.Name,
		namespace: s.object.Namespace,
		port:      s.Port,
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
