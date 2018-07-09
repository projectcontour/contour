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
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
)

// A DAG represents a directed acylic graph of objects representing the relationship
// between Kubernetes Ingress objects, the backend Services, and Secret objects.
// The DAG models these relationships as Roots and Vertices.
type DAG struct {
	// IngressRouteRootNamespaces specifies the namespaces where root
	// IngressRoutes can be defined. If empty, roots can be defined in any
	// namespace.
	IngressRouteRootNamespaces []string

	mu sync.Mutex

	ingresses     map[meta]*v1beta1.Ingress
	ingressroutes map[meta]*ingressroutev1.IngressRoute
	secrets       map[meta]*v1.Secret
	services      map[meta]*v1.Service

	dag
}

// dag represents
type dag struct {
	// roots are the roots of this dag
	roots []Vertex

	version int
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
func (d *DAG) Recompute() IngressrouteStatus {
	var statuses IngressrouteStatus
	d.mu.Lock()
	defer d.mu.Unlock()
	version := d.dag.version
	// TODO(abrand): Handle status returned by recompute
	d.dag, statuses = d.recompute()
	d.dag.version = version + 1
	return statuses
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
		if s, ok := sm._services[portmeta{name: m.name, namespace: m.namespace, port: int32(port.IntValue())}]; ok {
			return s
		}
	}
	svc, ok := sm.services[m]
	if !ok {
		return nil
	}
	for i := range svc.Spec.Ports {
		p := &svc.Spec.Ports[i]
		if int(p.Port) == port.IntValue() {
			return sm.insert(svc, p)
		}
		if port.String() == p.Name {
			return sm.insert(svc, p)
		}
	}
	return nil
}

func (sm *serviceMap) insert(svc *v1.Service, port *v1.ServicePort) *Service {
	if sm._services == nil {
		sm._services = make(map[portmeta]*Service)
	}
	up := parseUpstreamProtocols(svc.Annotations, annotationUpstreamProtocol, "h2", "h2c")
	protocol := up[port.Name]
	if protocol == "" {
		protocol = up[strconv.Itoa(int(port.Port))]
	}

	s := &Service{
		object:      svc,
		ServicePort: port,
		Protocol:    protocol,

		MaxConnections:     parseAnnotation(svc.Annotations, annotationMaxConnections),
		MaxPendingRequests: parseAnnotation(svc.Annotations, annotationMaxPendingRequests),
		MaxRequests:        parseAnnotation(svc.Annotations, annotationMaxRequests),
		MaxRetries:         parseAnnotation(svc.Annotations, annotationMaxRetries),
	}
	sm._services[s.toMeta()] = s
	return s
}

// recompute builds a new *dag.dag.
func (d *DAG) recompute() (dag, IngressrouteStatus) {
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

	// setup secure vhosts if there is a matching secret
	// we do this first so that the set of active secure vhosts is stable
	// during the second ingress pass
	for _, ing := range d.ingresses {
		for _, tls := range ing.Spec.TLS {
			m := meta{name: tls.SecretName, namespace: ing.Namespace}
			if sec := secret(m); sec != nil {
				for _, host := range tls.Hosts {
					svhost(host, 443).secret = sec
					// process annotations
					switch ing.ObjectMeta.Annotations["contour.heptio.com/tls-minimum-protocol-version"] {
					case "1.3":
						svhost(host, 443).MinProtoVersion = auth.TlsParameters_TLSv1_3
					case "1.2":
						svhost(host, 443).MinProtoVersion = auth.TlsParameters_TLSv1_2
					default:
						// any other value is interpreted as TLS/1.1
						svhost(host, 443).MinProtoVersion = auth.TlsParameters_TLSv1_1
					}
				}
			}
		}
	}

	// deconstruct each ingress into routes and virtualhost entries
	for _, ing := range d.ingresses {
		// should we create port 80 routes for this ingress
		httpAllowed := httpAllowed(ing)

		// compute websocket enabled routes
		wr := websocketRoutes(ing)

		// compute timeout for any routes on this ingress
		timeout := parseAnnotationTimeout(ing.Annotations, annotationRequestTimeout)

		if ing.Spec.Backend != nil {
			// handle the annoying default ingress
			r := &Route{
				path:         "/",
				object:       ing,
				HTTPSUpgrade: tlsRequired(ing),
				Websocket:    wr["/"],
				Timeout:      timeout,
			}
			m := meta{name: ing.Spec.Backend.ServiceName, namespace: ing.Namespace}
			if s := service(m, ing.Spec.Backend.ServicePort); s != nil {
				r.addService(s, nil, "", 0)
			}
			if httpAllowed {
				vhost("*", 80).routes[r.path] = r
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
					path:         path,
					object:       ing,
					HTTPSUpgrade: tlsRequired(ing),
					Websocket:    wr[path],
					Timeout:      timeout,
				}

				m := meta{name: rule.IngressRuleValue.HTTP.Paths[n].Backend.ServiceName, namespace: ing.Namespace}
				if s := service(m, rule.IngressRuleValue.HTTP.Paths[n].Backend.ServicePort); s != nil {
					r.addService(s, nil, "", s.Weight)
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
	var status []Status
	orphaned := make(map[meta]bool)
	for _, ir := range d.ingressroutes {
		if ir.Spec.VirtualHost == nil {
			// delegate ingress route. mark as orphaned if we haven't reached it before.
			if _, ok := orphaned[meta{name: ir.Name, namespace: ir.Namespace}]; !ok {
				orphaned[meta{name: ir.Name, namespace: ir.Namespace}] = true
			}
			continue
		}

		// ensure root ingressroute lives in allowed namespace
		if !d.rootAllowed(ir) {
			status = append(status, Status{object: ir, status: "invalid", msg: "root IngressRoute cannot be defined in this namespace"})
			continue
		}

		host := ir.Spec.VirtualHost.Fqdn
		if len(strings.TrimSpace(host)) == 0 {
			status = append(status, Status{object: ir, status: "invalid", msg: "Spec.VirtualHost.Fqdn must be specified"})
			continue
		}

		if tls := ir.Spec.VirtualHost.TLS; tls != nil {
			// attach secrets to TLS enabled vhosts
			m := meta{name: tls.SecretName, namespace: ir.Namespace}
			if sec := secret(m); sec != nil {
				svhost(host, 443).secret = sec
				svhost(host, 443).MinProtoVersion = auth.TlsParameters_TLSv1_1 // TODO(dfc) issue 467
			}
		}

		prefixMatch := ""
		ve := d.processIngressRoute(ir, prefixMatch, nil, host, service, vhost, orphaned)
		status = append(status, ve...)
	}

	var _d dag
	for _, vh := range _vhosts {
		_d.roots = append(_d.roots, vh)
	}
	for _, svh := range _svhosts {
		_d.roots = append(_d.roots, svh)
	}

	for meta, orph := range orphaned {
		if orph {
			ir, ok := d.ingressroutes[meta]
			if ok {
				status = append(status, Status{object: ir, status: "orphaned", msg: "this IngressRoute is not part of a delegation chain from a root IngressRoute"})
			}
		}
	}
	return _d, IngressrouteStatus{statuses: status, version: d.version}
}

func (d *DAG) processIngressRoute(ir *ingressroutev1.IngressRoute, prefixMatch string, visited []*ingressroutev1.IngressRoute, host string, service func(m meta, port intstr.IntOrString) *Service, vhost func(host string, port int) *VirtualHost, orphaned map[meta]bool) []Status {
	visited = append(visited, ir)

	var status []Status
	for _, route := range ir.Spec.Routes {
		// route cannot both delegate and point to services
		if len(route.Services) > 0 && route.Delegate.Name != "" {
			return []Status{{object: ir, status: "invalid", msg: fmt.Sprintf("route %q: cannot specify services and delegate in the same route", route.Match)}}
		}
		// base case: The route points to services, so we add them to the vhost
		if len(route.Services) > 0 {
			if !matchesPathPrefix(route.Match, prefixMatch) {
				return []Status{{object: ir, status: "invalid", msg: fmt.Sprintf("the path prefix %q does not match the parent's path prefix %q", route.Match, prefixMatch)}}
			}
			r := &Route{
				path:   route.Match,
				object: ir,
			}
			for _, s := range route.Services {
				if s.Port < 1 || s.Port > 65535 {
					return []Status{{object: ir, status: "invalid", msg: fmt.Sprintf("route %q: service %q: port must be in the range 1-65535", route.Match, s.Name)}}
				}
				if s.Weight < 0 {
					return []Status{{object: ir, status: "invalid", msg: fmt.Sprintf("route %q: service %q: weight must be greater than or equal to zero", route.Match, s.Name)}}
				}
				m := meta{name: s.Name, namespace: ir.Namespace}
				if svc := service(m, intstr.FromInt(s.Port)); svc != nil {
					r.addService(svc, s.HealthCheck, s.Strategy, s.Weight)
				}
			}
			vhost(host, 80).routes[r.path] = r
			continue
		}

		// otherwise, if the route is delegating to another ingressroute, find it and process it.
		if route.Delegate.Name != "" {
			namespace := route.Delegate.Namespace
			if namespace == "" {
				// we are delegating to another IngressRoute in the same namespace
				namespace = ir.Namespace
			}
			dest, ok := d.ingressroutes[meta{name: route.Delegate.Name, namespace: namespace}]
			if ok {
				// dest is not an orphaned route, as there is an IR that points to it
				orphaned[meta{name: dest.Name, namespace: dest.Namespace}] = false

				// ensure we are not following an edge that produces a cycle
				var path []string
				for _, vir := range visited {
					path = append(path, fmt.Sprintf("%s/%s", vir.Namespace, vir.Name))
				}
				for _, vir := range visited {
					if dest.Name == vir.Name && dest.Namespace == vir.Namespace {
						path = append(path, fmt.Sprintf("%s/%s", dest.Namespace, dest.Name))
						msg := fmt.Sprintf("route creates a delegation cycle: %s", strings.Join(path, " -> "))
						return []Status{{object: ir, status: "invalid", msg: msg}}
					}
				}

				// follow the link and process the target ingress route
				status = append(status, d.processIngressRoute(dest, route.Match, visited, host, service, vhost, orphaned)...)
			}
		}
	}
	return append(status, Status{object: ir, status: "valid", msg: "valid IngressRoute"})
}

// matchesPathPrefix checks whether the given path matches the given prefix
func matchesPathPrefix(path, prefix string) bool {
	if len(prefix) == 0 {
		return true
	}
	// an empty string cannot have a prefix
	if len(path) == 0 {
		return false
	}
	if prefix[len(prefix)-1] != '/' {
		prefix = prefix + "/"
	}
	if path[len(path)-1] != '/' {
		path = path + "/"
	}
	return strings.HasPrefix(path, prefix)
}

// returns true if the root ingressroute lives in a root namespace
func (d *DAG) rootAllowed(ir *ingressroutev1.IngressRoute) bool {
	if len(d.IngressRouteRootNamespaces) == 0 {
		return true
	}
	for _, ns := range d.IngressRouteRootNamespaces {
		if ns == ir.Namespace {
			return true
		}
	}
	return false
}

type Root interface {
	Vertex
}

type Route struct {
	path     string
	object   interface{} // one of Ingress or IngressRoute
	services map[portmeta]*Service

	// Should this route generate a 301 upgrade if accessed
	// over HTTP?
	HTTPSUpgrade bool

	// Is this a websocket route?
	// TODO(dfc) this should go on the service
	Websocket bool

	// A timeout applied to requests on this route.
	// A timeout of zero implies "use envoy's default"
	// A timeout of -1 represents "infinity"
	// TODO(dfc) should this move to service?
	Timeout time.Duration
}

func (r *Route) Prefix() string { return r.path }

func (r *Route) addService(s *Service, hc *ingressroutev1.HealthCheck, lbStrat string, weight int) {
	if r.services == nil {
		r.services = make(map[portmeta]*Service)
	}
	s.HealthCheck = hc
	s.LoadBalancerStrategy = lbStrat
	s.Weight = weight
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

	// TLS minimum protocol version. Defaults to auth.TlsParameters_TLS_AUTO
	MinProtoVersion auth.TlsParameters_TlsProtocol

	host   string
	routes map[string]*Route
	secret *Secret
}

func (s *SecureVirtualHost) Data() map[string][]byte {
	if s.secret == nil {
		return nil
	}
	return s.secret.Data()
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

// Service represents a K8s Sevice as a DAG vertex. A Service is
// a leaf in the DAG.
type Service struct {
	object *v1.Service

	*v1.ServicePort
	Weight int

	// Protocol is the layer 7 protocol of this service
	Protocol string

	HealthCheck          *ingressroutev1.HealthCheck
	LoadBalancerStrategy string

	// Curcuit breaking limits

	// Max connections is maximum number of connections
	// that Envoy will make to the upstream cluster.
	MaxConnections int

	// MaxPendingRequests is maximum number of pending
	// requests that Envoy will allow to the upstream cluster.
	MaxPendingRequests int

	// MaxRequests is the maximum number of parallel requests that
	// Envoy will make to the upstream cluster.
	MaxRequests int

	// MaxRetries is the maximum number of parallel retries that
	// Envoy will allow to the upstream cluster.
	MaxRetries int
}

func (s *Service) Name() string       { return s.object.Name }
func (s *Service) Namespace() string  { return s.object.Namespace }
func (s *Service) Visit(func(Vertex)) {}

type portmeta struct {
	name      string
	namespace string
	port      int32
}

func (s *Service) toMeta() portmeta {
	return portmeta{
		name:      s.object.Name,
		namespace: s.object.Namespace,
		port:      s.Port,
	}
}

// Secret represents a K8s Secret for TLS usage as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	object *v1.Secret
}

func (s *Secret) Name() string       { return s.object.Name }
func (s *Secret) Namespace() string  { return s.object.Namespace }
func (s *Secret) Visit(func(Vertex)) {}

// Data returns the contents of the backing secret's map.
func (s *Secret) Data() map[string][]byte {
	return s.object.Data
}

func (s *Secret) toMeta() meta {
	return meta{
		name:      s.object.Name,
		namespace: s.object.Namespace,
	}
}

// IngressrouteStatus contains the status for
// an IngressRoute (valid / invalid / orphan, etc)
type IngressrouteStatus struct {
	statuses []Status
	version  int
}

type Status struct {
	object *ingressroutev1.IngressRoute
	status string
	msg    string
}

func (irs *IngressrouteStatus) GetStatuses() []Status {
	return irs.statuses
}

func (irs *IngressrouteStatus) GetVersion() int {
	return irs.version
}

func (s *Status) GetStatus() string {
	return s.status
}

func (s *Status) GetMsg() string {
	return s.msg
}

func (s *Status) GetIngressRouteName() string {
	return s.object.GetName()
}

func (s *Status) GetIngressRouteNamespace() string {
	return s.object.GetNamespace()
}
