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
	"sort"
	"strconv"
	"strings"
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
)

// A Builder holds Kubernetes objects and associated configuration and produces
// DAG values.
type Builder struct {
	// IngressRouteRootNamespaces specifies the namespaces where root
	// IngressRoutes can be defined. If empty, roots can be defined in any
	// namespace.
	IngressRouteRootNamespaces []string

	mu sync.Mutex

	ingresses     map[meta]*v1beta1.Ingress
	ingressroutes map[meta]*ingressroutev1.IngressRoute
	secrets       map[meta]*v1.Secret
	services      map[meta]*v1.Service
}

// meta holds the name and namespace of a Kubernetes object.
type meta struct {
	name, namespace string
}

const (
	StatusValid    = "valid"
	StatusInvalid  = "invalid"
	StatusOrphaned = "orphaned"
)

// Insert inserts obj into the Builder.
// If an object with a matching type, name, and namespace exists, it will be overwritten.
func (b *Builder) Insert(obj interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if b.secrets == nil {
			b.secrets = make(map[meta]*v1.Secret)
		}
		b.secrets[m] = obj
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if b.services == nil {
			b.services = make(map[meta]*v1.Service)
		}
		b.services[m] = obj
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if b.ingresses == nil {
			b.ingresses = make(map[meta]*v1beta1.Ingress)
		}
		b.ingresses[m] = obj
	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if b.ingressroutes == nil {
			b.ingressroutes = make(map[meta]*ingressroutev1.IngressRoute)
		}
		b.ingressroutes[m] = obj
	default:
		// not an interesting object
	}
}

// Remove removes obj from the Builder.
// If no object with a matching type, name, and namespace exists in the DAG, no action is taken.
func (b *Builder) Remove(obj interface{}) {
	switch obj := obj.(type) {
	default:
		b.remove(obj)
	case cache.DeletedFinalStateUnknown:
		b.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (b *Builder) remove(obj interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(b.secrets, m)
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(b.services, m)
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(b.ingresses, m)
	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(b.ingressroutes, m)
	default:
		// not interesting
	}
}

// Compute computes a new DAG value.
func (b *Builder) Compute() *DAG {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.compute()
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
		Object:      svc,
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

// compute builds a new *DAG
func (b *Builder) compute() *DAG {
	sm := serviceMap{
		services: b.services,
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
		sec, ok := b.secrets[m]
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
	vhost := func(host string, aliases []string, port int) *VirtualHost {
		hp := hostport{host: host, port: port}
		vh, ok := _vhosts[hp]
		if !ok {
			vh = &VirtualHost{
				Port:    port,
				host:    host,
				aliases: aliases,
				routes:  make(map[string]*Route),
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
	for _, ing := range b.ingresses {
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
	for _, ing := range b.ingresses {
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
				Object:       ing,
				HTTPSUpgrade: tlsRequired(ing),
				Websocket:    wr["/"],
				Timeout:      timeout,
			}
			m := meta{name: ing.Spec.Backend.ServiceName, namespace: ing.Namespace}
			if s := service(m, ing.Spec.Backend.ServicePort); s != nil {
				r.addService(s, nil, "", 0)
			}
			if httpAllowed {
				vhost("*", []string{}, 80).routes[r.path] = r
			}
		}

		for _, rule := range ing.Spec.Rules {
			// handle Spec.Rule declarations
			host := rule.Host
			if host == "" {
				host = "*"
			}
			for _, httppath := range httppaths(rule) {
				path := httppath.Path
				if path == "" {
					path = "/"
				}
				r := &Route{
					path:         path,
					Object:       ing,
					HTTPSUpgrade: tlsRequired(ing),
					Websocket:    wr[path],
					Timeout:      timeout,
				}

				m := meta{name: httppath.Backend.ServiceName, namespace: ing.Namespace}
				if s := service(m, httppath.Backend.ServicePort); s != nil {
					r.addService(s, nil, "", s.Weight)
				}
				if httpAllowed {
					vhost(host, []string{}, 80).routes[r.path] = r
				}
				if _, ok := _svhosts[hostport{host: host, port: 443}]; ok && host != "*" {
					svhost(host, 443).routes[r.path] = r
				}
			}
		}
	}

	// ensure that a given fqdn is only referenced in a single ingressroute resource
	var validirs []*ingressroutev1.IngressRoute
	fqdnIngressroutes := make(map[string][]*ingressroutev1.IngressRoute)
	for _, ir := range b.ingressroutes {
		if ir.Spec.VirtualHost == nil {
			validirs = append(validirs, ir)
			continue
		}
		fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn] = append(fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn], ir)
	}

	var status []Status
	for fqdn, irs := range fqdnIngressroutes {
		if len(irs) == 1 {
			validirs = append(validirs, irs[0])
			continue
		}

		// multiple irs use the same fqdn. mark them as invalid.
		var conflicting []string
		for _, ir := range irs {
			conflicting = append(conflicting, fmt.Sprintf("%s/%s", ir.Namespace, ir.Name))
		}
		sort.Strings(conflicting) // sort for test stability
		msg := fmt.Sprintf("fqdn %q is used in multiple IngressRoutes: %s", fqdn, strings.Join(conflicting, ", "))
		for _, ir := range irs {
			status = append(status, Status{Object: ir, Status: StatusInvalid, Description: msg, Vhost: fqdn})
		}
	}

	// process ingressroute documents
	orphaned := make(map[meta]bool)
	for _, ir := range validirs {
		if ir.Spec.VirtualHost == nil {
			// delegate ingress route. mark as orphaned if we haven't reached it before.
			if _, ok := orphaned[meta{name: ir.Name, namespace: ir.Namespace}]; !ok {
				orphaned[meta{name: ir.Name, namespace: ir.Namespace}] = true
			}
			continue
		}

		// ensure root ingressroute lives in allowed namespace
		if !b.rootAllowed(ir) {
			status = append(status, Status{Object: ir, Status: StatusInvalid, Description: "root IngressRoute cannot be defined in this namespace"})
			continue
		}

		host := ir.Spec.VirtualHost.Fqdn
		if len(strings.TrimSpace(host)) == 0 {
			status = append(status, Status{Object: ir, Status: StatusInvalid, Description: "Spec.VirtualHost.Fqdn must be specified"})
			continue
		}

		if tls := ir.Spec.VirtualHost.TLS; tls != nil {
			// attach secrets to TLS enabled vhosts
			m := meta{name: tls.SecretName, namespace: ir.Namespace}
			if sec := secret(m); sec != nil {
				svhost(host, 443).secret = sec

				// process min protocol version
				switch ir.Spec.VirtualHost.TLS.MinimumProtocolVersion {
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

		prefixMatch := ""
		irp := ingressRouteProcessor{
			host:          host,
			aliases:       ir.Spec.VirtualHost.Aliases,
			service:       service,
			vhost:         vhost,
			svhost:        svhost,
			ingressroutes: b.ingressroutes,
			orphaned:      orphaned,
		}
		sts := irp.process(ir, prefixMatch, nil, host)
		status = append(status, sts...)
	}

	var dag DAG
	for _, vh := range _vhosts {
		dag.roots = append(dag.roots, vh)
	}
	for _, svh := range _svhosts {
		if svh.secret != nil {
			dag.roots = append(dag.roots, svh)
		}
	}

	for meta, orph := range orphaned {
		if orph {
			ir, ok := b.ingressroutes[meta]
			if ok {
				status = append(status, Status{Object: ir, Status: StatusOrphaned, Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"})
			}
		}
	}
	dag.statuses = status
	return &dag
}

// returns true if the root ingressroute lives in a root namespace
func (b *Builder) rootAllowed(ir *ingressroutev1.IngressRoute) bool {
	if len(b.IngressRouteRootNamespaces) == 0 {
		return true
	}
	for _, ns := range b.IngressRouteRootNamespaces {
		if ns == ir.Namespace {
			return true
		}
	}
	return false
}

type ingressRouteProcessor struct {
	host          string
	aliases       []string
	service       func(m meta, port intstr.IntOrString) *Service
	svhost        func(host string, port int) *SecureVirtualHost
	vhost         func(host string, aliases []string, port int) *VirtualHost
	ingressroutes map[meta]*ingressroutev1.IngressRoute
	orphaned      map[meta]bool
}

func (irp *ingressRouteProcessor) process(ir *ingressroutev1.IngressRoute, prefixMatch string, visited []*ingressroutev1.IngressRoute, host string) []Status {
	visited = append(visited, ir)

	var status []Status
	for _, route := range ir.Spec.Routes {
		// route cannot both delegate and point to services
		if len(route.Services) > 0 && route.Delegate.Name != "" {
			return []Status{{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("route %q: cannot specify services and delegate in the same route", route.Match), Vhost: host}}
		}
		// base case: The route points to services, so we add them to the vhost
		if len(route.Services) > 0 {
			if !matchesPathPrefix(route.Match, prefixMatch) {
				return []Status{{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("the path prefix %q does not match the parent's path prefix %q", route.Match, prefixMatch), Vhost: host}}
			}
			r := &Route{
				path:      route.Match,
				Object:    ir,
				Websocket: route.EnableWebsockets,
			}
			for _, s := range route.Services {
				if s.Port < 1 || s.Port > 65535 {
					return []Status{{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("route %q: service %q: port must be in the range 1-65535", route.Match, s.Name), Vhost: host}}
				}
				if s.Weight < 0 {
					return []Status{{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("route %q: service %q: weight must be greater than or equal to zero", route.Match, s.Name), Vhost: host}}
				}
				m := meta{name: s.Name, namespace: ir.Namespace}
				if svc := irp.service(m, intstr.FromInt(s.Port)); svc != nil {
					r.addService(svc, s.HealthCheck, s.Strategy, s.Weight)
				}
			}
			irp.vhost(irp.host, irp.aliases, 80).routes[r.path] = r

			if hst := irp.svhost(irp.host, 443); hst != nil {
				if hst.secret != nil {
					irp.svhost(irp.host, 443).routes[r.path] = r
				}
			}
			continue
		}

		// otherwise, if the route is delegating to another ingressroute, find it and process it.
		if route.Delegate.Name != "" {
			namespace := route.Delegate.Namespace
			if namespace == "" {
				// we are delegating to another IngressRoute in the same namespace
				namespace = ir.Namespace
			}
			dest, ok := irp.ingressroutes[meta{name: route.Delegate.Name, namespace: namespace}]
			if ok {
				// dest is not an orphaned route, as there is an IR that points to it
				irp.orphaned[meta{name: dest.Name, namespace: dest.Namespace}] = false

				// ensure we are not following an edge that produces a cycle
				var path []string
				for _, vir := range visited {
					path = append(path, fmt.Sprintf("%s/%s", vir.Namespace, vir.Name))
				}
				for _, vir := range visited {
					if dest.Name == vir.Name && dest.Namespace == vir.Namespace {
						path = append(path, fmt.Sprintf("%s/%s", dest.Namespace, dest.Name))
						description := fmt.Sprintf("route creates a delegation cycle: %s", strings.Join(path, " -> "))
						return []Status{{Object: ir, Status: StatusInvalid, Description: description, Vhost: host}}
					}
				}

				// follow the link and process the target ingress route
				status = append(status, irp.process(dest, route.Match, visited, host)...)
			}
		}
	}
	return append(status, Status{Object: ir, Status: StatusValid, Description: "valid IngressRoute", Vhost: host})
}

// httppaths returns a slice of HTTPIngressPath values for a given IngressRule.
// In the case that the IngressRule contains no valid HTTPIngressPaths, a
// nil slice is returned.
func httppaths(rule v1beta1.IngressRule) []v1beta1.HTTPIngressPath {
	if rule.IngressRuleValue.HTTP == nil {
		// rule.IngressRuleValue.HTTP value is optional.
		return nil
	}
	return rule.IngressRuleValue.HTTP.Paths
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

// Status contains the status for an IngressRoute (valid / invalid / orphan, etc)
type Status struct {
	Object      *ingressroutev1.IngressRoute
	Status      string
	Description string
	Vhost       string // SAS: Support `aliases` once merged
}
