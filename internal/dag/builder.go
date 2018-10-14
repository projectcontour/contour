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
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
)

// A KubernetesCache holds Kubernetes objects and associated configuration and produces
// DAG values.
type KubernetesCache struct {
	// IngressRouteRootNamespaces specifies the namespaces where root
	// IngressRoutes can be defined. If empty, roots can be defined in any
	// namespace.
	IngressRouteRootNamespaces []string

	mu sync.RWMutex

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

// Insert inserts obj into the KubernetesCache.
// If an object with a matching type, name, and namespace exists, it will be overwritten.
func (kc *KubernetesCache) Insert(obj interface{}) {
	kc.mu.Lock()
	defer kc.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if kc.secrets == nil {
			kc.secrets = make(map[meta]*v1.Secret)
		}
		kc.secrets[m] = obj
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if kc.services == nil {
			kc.services = make(map[meta]*v1.Service)
		}
		kc.services[m] = obj
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if kc.ingresses == nil {
			kc.ingresses = make(map[meta]*v1beta1.Ingress)
		}
		kc.ingresses[m] = obj
	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if kc.ingressroutes == nil {
			kc.ingressroutes = make(map[meta]*ingressroutev1.IngressRoute)
		}
		kc.ingressroutes[m] = obj
	default:
		// not an interesting object
	}
}

// Remove removes obj from the KubernetesCache.
// If no object with a matching type, name, and namespace exists in the DAG, no action is taken.
func (kc *KubernetesCache) Remove(obj interface{}) {
	switch obj := obj.(type) {
	default:
		kc.remove(obj)
	case cache.DeletedFinalStateUnknown:
		kc.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (kc *KubernetesCache) remove(obj interface{}) {
	kc.mu.Lock()
	defer kc.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.secrets, m)
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.services, m)
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.ingresses, m)
	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(kc.ingressroutes, m)
	default:
		// not interesting
	}
}

// A Builder builds a *DAGs
type Builder struct {
	KubernetesCache
}

// Build builds a new *DAG.
func (b *Builder) Build() *DAG {
	builder := &builder{source: b}
	return builder.compute()
}

// A builder holds the state of one invocation of Builder.Build.
// Once used, the builder should be discarded.
type builder struct {
	source *Builder

	services map[servicemeta]*Service
	secrets  map[meta]*Secret
	vhosts   map[hostport]*VirtualHost
	svhosts  map[hostport]*SecureVirtualHost

	orphaned map[meta]bool

	statuses []Status
}

// lookupService returns a Service that matches the meta and port supplied.
// If no matching Service is found lookup returns nil.
func (b *builder) lookupService(m meta, port intstr.IntOrString, weight int, strategy string, hc *ingressroutev1.HealthCheck) *Service {
	if port.Type == intstr.Int {
		m := servicemeta{
			name:        m.name,
			namespace:   m.namespace,
			port:        int32(port.IntValue()),
			weight:      weight,
			strategy:    strategy,
			healthcheck: healthcheckToString(hc),
		}
		if s, ok := b.services[m]; ok {
			return s
		}
	}
	svc, ok := b.source.services[m]
	if !ok {
		return nil
	}
	for i := range svc.Spec.Ports {
		p := &svc.Spec.Ports[i]
		if int(p.Port) == port.IntValue() {
			return b.addService(svc, p, weight, strategy, hc)
		}
		if port.String() == p.Name {
			return b.addService(svc, p, weight, strategy, hc)
		}
	}
	return nil
}

func healthcheckToString(hc *ingressroutev1.HealthCheck) string {
	return fmt.Sprintf("%#v", hc)
}

func (b *builder) addService(svc *v1.Service, port *v1.ServicePort, weight int, strategy string, hc *ingressroutev1.HealthCheck) *Service {
	if b.services == nil {
		b.services = make(map[servicemeta]*Service)
	}
	up := parseUpstreamProtocols(svc.Annotations, annotationUpstreamProtocol, "h2", "h2c")
	protocol := up[port.Name]
	if protocol == "" {
		protocol = up[strconv.Itoa(int(port.Port))]
	}

	s := &Service{
		Object:               svc,
		ServicePort:          port,
		Protocol:             protocol,
		Weight:               weight,
		LoadBalancerStrategy: strategy,
		HealthCheck:          hc,

		MaxConnections:     parseAnnotation(svc.Annotations, annotationMaxConnections),
		MaxPendingRequests: parseAnnotation(svc.Annotations, annotationMaxPendingRequests),
		MaxRequests:        parseAnnotation(svc.Annotations, annotationMaxRequests),
		MaxRetries:         parseAnnotation(svc.Annotations, annotationMaxRetries),
	}
	b.services[s.toMeta()] = s
	return s
}

func (b *builder) lookupSecret(m meta) *Secret {
	if s, ok := b.secrets[m]; ok {
		return s
	}
	sec, ok := b.source.secrets[m]
	if !ok {
		return nil
	}
	s := &Secret{
		object: sec,
	}
	if b.secrets == nil {
		b.secrets = make(map[meta]*Secret)
	}
	b.secrets[s.toMeta()] = s
	return s
}

func (b *builder) lookupVirtualHost(host string, port int) *VirtualHost {
	hp := hostport{host: host, port: port}
	vh, ok := b.vhosts[hp]
	if !ok {
		vh = &VirtualHost{
			Host: host,
			Port: port,
		}
		if b.vhosts == nil {
			b.vhosts = make(map[hostport]*VirtualHost)
		}
		b.vhosts[hp] = vh
	}
	return vh
}

func (b *builder) lookupSecureVirtualHost(host string, port int) *SecureVirtualHost {
	hp := hostport{host: host, port: port}
	svh, ok := b.svhosts[hp]
	if !ok {
		svh = &SecureVirtualHost{
			VirtualHost: VirtualHost{
				Host: host,
				Port: port,
			},
		}
		if b.svhosts == nil {
			b.svhosts = make(map[hostport]*SecureVirtualHost)
		}
		b.svhosts[hp] = svh
	}
	return svh
}

type hostport struct {
	host string
	port int
}

func (b *builder) compute() *DAG {
	b.source.KubernetesCache.mu.RLock() // blocks mutation of the underlying cache until compute is done.
	defer b.source.KubernetesCache.mu.RUnlock()

	// setup secure vhosts if there is a matching secret
	// we do this first so that the set of active secure vhosts is stable
	// during the second ingress pass
	for _, ing := range b.source.ingresses {
		for _, tls := range ing.Spec.TLS {
			m := meta{name: tls.SecretName, namespace: ing.Namespace}
			if sec := b.lookupSecret(m); sec != nil {
				for _, host := range tls.Hosts {
					svhost := b.lookupSecureVirtualHost(host, 443)
					svhost.secret = sec
					svhost.MinProtoVersion = minProtoVersion(ing)
				}
			}
		}
	}

	// deconstruct each ingress into routes and virtualhost entries
	for _, ing := range b.source.ingresses {
		// rewrite the default ingress to a stock ingress rule.
		rules := ing.Spec.Rules
		if backend := ing.Spec.Backend; backend != nil {
			rule := v1beta1.IngressRule{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: backend.ServiceName,
								ServicePort: backend.ServicePort,
							},
						}},
					},
				},
			}
			rules = append(rules, rule)
		}

		for _, rule := range rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			for _, httppath := range httppaths(rule) {
				prefix := httppath.Path
				if prefix == "" {
					prefix = "/"
				}

				r := prefixRoute(ing, prefix)
				m := meta{name: httppath.Backend.ServiceName, namespace: ing.Namespace}
				if s := b.lookupService(m, httppath.Backend.ServicePort, 0, "", nil); s != nil {
					r.addService(s)
				}

				// should we create port 80 routes for this ingress
				if httpAllowed(ing) {
					b.lookupVirtualHost(host, 80).addRoute(r)
				}
				if _, ok := b.svhosts[hostport{host: host, port: 443}]; ok && host != "*" {
					b.lookupSecureVirtualHost(host, 443).addRoute(r)
				}
			}
		}
	}

	// process ingressroute documents
	for _, ir := range b.validIngressRoutes() {
		if ir.Spec.VirtualHost == nil {
			// mark delegate ingressroute orphaned.
			b.setOrphaned(ir)
			continue
		}

		// ensure root ingressroute lives in allowed namespace
		if !b.rootAllowed(ir) {
			b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: "root IngressRoute cannot be defined in this namespace"})
			continue
		}

		host := ir.Spec.VirtualHost.Fqdn
		if isBlank(host) {
			b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: "Spec.VirtualHost.Fqdn must be specified"})
			continue
		}

		enforceTLS := false
		if tls := ir.Spec.VirtualHost.TLS; tls != nil {
			// attach secrets to TLS enabled vhosts
			m := meta{name: tls.SecretName, namespace: ir.Namespace}
			if sec := b.lookupSecret(m); sec != nil {
				svhost := b.lookupSecureVirtualHost(host, 443)
				svhost.secret = sec
				enforceTLS = true

				// process min protocol version
				switch ir.Spec.VirtualHost.TLS.MinimumProtocolVersion {
				case "1.3":
					svhost.MinProtoVersion = auth.TlsParameters_TLSv1_3
				case "1.2":
					svhost.MinProtoVersion = auth.TlsParameters_TLSv1_2
				default:
					// any other value is interpreted as TLS/1.1
					svhost.MinProtoVersion = auth.TlsParameters_TLSv1_1
				}
			}
		}

		b.processIngressRoute(ir, "", nil, host, enforceTLS)
	}

	return b.DAG()
}

// prefixRoute returns a new dag.Route for the (ingress,prefix) tuple.
func prefixRoute(ingress *v1beta1.Ingress, prefix string) *Route {
	// compute websocket enabled routes
	wr := websocketRoutes(ingress)

	// compute timeout for any routes on this ingress
	timeout := parseAnnotationTimeout(ingress.Annotations, annotationRequestTimeout)

	var perTryTimeout time.Duration
	if val, ok := ingress.Annotations[annotationPerTryTimeout]; ok {
		perTryTimeout, _ = time.ParseDuration(val)
	}

	return &Route{
		Prefix:        prefix,
		object:        ingress,
		HTTPSUpgrade:  tlsRequired(ingress),
		Websocket:     wr[prefix],
		Timeout:       timeout,
		RetryOn:       ingress.Annotations[annotationRetryOn],
		NumRetries:    parseAnnotation(ingress.Annotations, annotationNumRetries),
		PerTryTimeout: perTryTimeout,
	}
}

// isBlank indicates if a string contains nothing but blank characters.
func isBlank(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// minProtoVersion returns the TLS protocol version specified by an ingress annotation
// or default if non present.
func minProtoVersion(i *v1beta1.Ingress) auth.TlsParameters_TlsProtocol {
	switch i.Annotations["contour.heptio.com/tls-minimum-protocol-version"] {
	case "1.3":
		return auth.TlsParameters_TLSv1_3
	case "1.2":
		return auth.TlsParameters_TLSv1_2
	default:
		// any other value is interpreted as TLS/1.1
		return auth.TlsParameters_TLSv1_1
	}
}

// validIngressRoutes returns a slice of *ingressroutev1.IngressRoute objects.
// invalid IngressRoute objects are excluded from the slice and a corresponding entry
// added via setStatus.
func (b *builder) validIngressRoutes() []*ingressroutev1.IngressRoute {
	// ensure that a given fqdn is only referenced in a single ingressroute resource
	var valid []*ingressroutev1.IngressRoute
	fqdnIngressroutes := make(map[string][]*ingressroutev1.IngressRoute)
	for _, ir := range b.source.ingressroutes {
		if ir.Spec.VirtualHost == nil {
			valid = append(valid, ir)
			continue
		}
		fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn] = append(fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn], ir)
	}

	for fqdn, irs := range fqdnIngressroutes {
		if len(irs) == 1 {
			valid = append(valid, irs[0])
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
			b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: msg, Vhost: fqdn})
		}
	}
	return valid
}

// DAG returns a *DAG representing the current state of this builder.
func (b *builder) DAG() *DAG {
	var dag DAG
	for _, vh := range b.vhosts {
		// suppress virtual hosts without routes.
		if len(vh.routes) > 0 {
			dag.roots = append(dag.roots, vh)
		}
	}
	for _, svh := range b.svhosts {
		if svh.secret != nil {
			dag.roots = append(dag.roots, svh)
		}
	}
	for meta := range b.orphaned {
		ir, ok := b.source.ingressroutes[meta]
		if ok {
			b.setStatus(Status{Object: ir, Status: StatusOrphaned, Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"})
		}
	}
	dag.statuses = b.statuses
	return &dag
}

// setStatus assigns a status to an object.
func (b *builder) setStatus(st Status) {
	b.statuses = append(b.statuses, st)
}

// setOrphaned records an ingressroute as orphaned.
func (b *builder) setOrphaned(ir *ingressroutev1.IngressRoute) {
	if b.orphaned == nil {
		b.orphaned = make(map[meta]bool)
	}
	m := meta{name: ir.Name, namespace: ir.Namespace}
	b.orphaned[m] = true
}

// rootAllowed returns true if the ingressroute lives in a permitted root namespace.
func (b *builder) rootAllowed(ir *ingressroutev1.IngressRoute) bool {
	if len(b.source.IngressRouteRootNamespaces) == 0 {
		return true
	}
	for _, ns := range b.source.IngressRouteRootNamespaces {
		if ns == ir.Namespace {
			return true
		}
	}
	return false
}

func (b *builder) processIngressRoute(ir *ingressroutev1.IngressRoute, prefixMatch string, visited []*ingressroutev1.IngressRoute, host string, enforceTLS bool) {
	visited = append(visited, ir)

	for _, route := range ir.Spec.Routes {
		// route cannot both delegate and point to services
		if len(route.Services) > 0 && route.Delegate != nil {
			b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("route %q: cannot specify services and delegate in the same route", route.Match), Vhost: host})
			return
		}
		// base case: The route points to services, so we add them to the vhost
		if len(route.Services) > 0 {
			if !matchesPathPrefix(route.Match, prefixMatch) {
				b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("the path prefix %q does not match the parent's path prefix %q", route.Match, prefixMatch), Vhost: host})
				return
			}

			// Determine if the route should enforce TLS
			enforceTLSRoute := routeEnforceTLS(enforceTLS, route.PermitInsecure)

			r := &Route{
				Prefix:        route.Match,
				object:        ir,
				Websocket:     route.EnableWebsockets,
				HTTPSUpgrade:  enforceTLSRoute,
				PrefixRewrite: route.PrefixRewrite,
			}
			for _, s := range route.Services {
				if s.Port < 1 || s.Port > 65535 {
					b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("route %q: service %q: port must be in the range 1-65535", route.Match, s.Name), Vhost: host})
					return
				}
				if s.Weight < 0 {
					b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: fmt.Sprintf("route %q: service %q: weight must be greater than or equal to zero", route.Match, s.Name), Vhost: host})
					return
				}
				m := meta{name: s.Name, namespace: ir.Namespace}
				if svc := b.lookupService(m, intstr.FromInt(s.Port), s.Weight, s.Strategy, s.HealthCheck); svc != nil {
					r.addService(svc)
				}
			}

			b.lookupVirtualHost(host, 80).addRoute(r)
			b.lookupSecureVirtualHost(host, 443).addRoute(r)
			continue
		}

		if route.Delegate == nil {
			// not a delegate route
			continue
		}

		// otherwise, if the route is delegating to another ingressroute, find it and process it.
		namespace := route.Delegate.Namespace
		if namespace == "" {
			// we are delegating to another IngressRoute in the same namespace
			namespace = ir.Namespace
		}

		if dest, ok := b.source.ingressroutes[meta{name: route.Delegate.Name, namespace: namespace}]; ok {
			// dest is not an orphaned route, as there is an IR that points to it
			delete(b.orphaned, meta{name: dest.Name, namespace: dest.Namespace})

			// ensure we are not following an edge that produces a cycle
			var path []string
			for _, vir := range visited {
				path = append(path, fmt.Sprintf("%s/%s", vir.Namespace, vir.Name))
			}
			for _, vir := range visited {
				if dest.Name == vir.Name && dest.Namespace == vir.Namespace {
					path = append(path, fmt.Sprintf("%s/%s", dest.Namespace, dest.Name))
					description := fmt.Sprintf("route creates a delegation cycle: %s", strings.Join(path, " -> "))
					b.setStatus(Status{Object: ir, Status: StatusInvalid, Description: description, Vhost: host})
					return
				}
			}

			// follow the link and process the target ingress route
			b.processIngressRoute(dest, route.Match, visited, host, enforceTLS)
		}
	}
	b.setStatus(Status{Object: ir, Status: StatusValid, Description: "valid IngressRoute", Vhost: host})
}

// routeEnforceTLS determines if the route should redirect the user to a secure TLS listener
func routeEnforceTLS(enforceTLS, permitInsecure bool) bool {
	return enforceTLS && !permitInsecure
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
	Vhost       string
}
