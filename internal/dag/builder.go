// Copyright © 2019 VMware
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
	"sort"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// Builder builds a DAG.
type Builder struct {

	// Source is the source of Kuberenetes objects
	// from which to build a DAG.
	Source KubernetesCache

	// DisablePermitInsecure disables the use of the
	// permitInsecure field in IngressRoute.
	DisablePermitInsecure bool

	services map[servicemeta]*Service
	secrets  map[Meta]*Secret

	virtualhosts       map[string]*VirtualHost
	securevirtualhosts map[string]*SecureVirtualHost

	orphaned map[Meta]bool

	StatusWriter
}

// Build builds a new DAG.
func (b *Builder) Build() *DAG {
	b.reset()

	// setup secure vhosts if there is a matching secret
	// we do this first so that the set of active secure vhosts is stable
	// during computeIngresses.
	b.computeSecureVirtualhosts()

	b.computeIngresses()

	b.computeIngressRoutes()

	b.computeHTTPProxies()

	return b.buildDAG()
}

// reset (re)inialises the internal state of the builder.
func (b *Builder) reset() {
	b.services = make(map[servicemeta]*Service, len(b.services))
	b.secrets = make(map[Meta]*Secret, len(b.secrets))
	b.orphaned = make(map[Meta]bool, len(b.orphaned))

	b.virtualhosts = make(map[string]*VirtualHost)
	b.securevirtualhosts = make(map[string]*SecureVirtualHost)

	b.statuses = make(map[Meta]Status, len(b.statuses))
}

// lookupService returns a Service that matches the Meta and Port of the Kubernetes' Service.
func (b *Builder) lookupService(m Meta, port intstr.IntOrString) *Service {
	lookup := func() *Service {
		if port.Type != intstr.Int {
			// can't handle, give up
			return nil
		}
		sm := servicemeta{
			name:      m.name,
			namespace: m.namespace,
			port:      int32(port.IntValue()),
		}
		return b.services[sm]
	}

	s := lookup()
	if s != nil {
		return s
	}
	svc, ok := b.Source.services[m]
	if !ok {
		return nil
	}
	for i := range svc.Spec.Ports {
		p := &svc.Spec.Ports[i]
		if int(p.Port) == port.IntValue() {
			return b.addService(svc, p)
		}
		if port.String() == p.Name {
			return b.addService(svc, p)
		}
	}
	return nil
}

func (b *Builder) addService(svc *v1.Service, port *v1.ServicePort) *Service {
	s := &Service{
		Name:        svc.Name,
		Namespace:   svc.Namespace,
		ServicePort: port,

		Protocol:           upstreamProtocol(svc, port),
		MaxConnections:     parseUInt32(svc.Annotations[annotationMaxConnections]),
		MaxPendingRequests: parseUInt32(svc.Annotations[annotationMaxPendingRequests]),
		MaxRequests:        parseUInt32(svc.Annotations[annotationMaxRequests]),
		MaxRetries:         parseUInt32(svc.Annotations[annotationMaxRetries]),
		ExternalName:       externalName(svc),
	}
	b.services[s.toMeta()] = s
	return s
}

func upstreamProtocol(svc *v1.Service, port *v1.ServicePort) string {
	up := parseUpstreamProtocols(svc.Annotations, annotationUpstreamProtocol, "h2", "h2c", "tls")
	protocol := up[port.Name]
	if protocol == "" {
		protocol = up[strconv.Itoa(int(port.Port))]
	}
	return protocol
}

// lookupSecret returns a Secret if present or nil if the underlying kubernetes
// secret fails validation or is missing.
func (b *Builder) lookupSecret(m Meta, validate func(*v1.Secret) bool) *Secret {
	sec, ok := b.Source.secrets[m]
	if !ok {
		return nil
	}
	if !validate(sec) {
		return nil
	}
	s := &Secret{
		Object: sec,
	}
	b.secrets[toMeta(sec)] = s
	return s
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

// validIngressRoutes returns a slice of *ingressroutev1.IngressRoute objects.
// invalid IngressRoute objects are excluded from the slice and their status
// updated accordingly.
func (b *Builder) validIngressRoutes() []*ingressroutev1.IngressRoute {
	// ensure that a given fqdn is only referenced in a single ingressroute resource
	var valid []*ingressroutev1.IngressRoute
	fqdnIngressroutes := make(map[string][]*ingressroutev1.IngressRoute)
	for _, ir := range b.Source.ingressroutes {
		if ir.Spec.VirtualHost == nil {
			valid = append(valid, ir)
			continue
		}
		fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn] = append(fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn], ir)
	}

	for fqdn, irs := range fqdnIngressroutes {
		switch len(irs) {
		case 1:
			valid = append(valid, irs[0])
		default:
			// multiple irs use the same fqdn. mark them as invalid.
			var conflicting []string
			for _, ir := range irs {
				conflicting = append(conflicting, ir.Namespace+"/"+ir.Name)
			}
			sort.Strings(conflicting) // sort for test stability
			msg := fmt.Sprintf("fqdn %q is used in multiple IngressRoutes: %s", fqdn, strings.Join(conflicting, ", "))
			for _, ir := range irs {
				sw, commit := b.WithObject(ir)
				sw.WithValue("vhost", fqdn).SetInvalid(msg)
				commit()
			}
		}
	}
	return valid
}

// validHTTPProxies returns a slice of *projcontour.HTTPProxy objects.
// invalid HTTPProxy objects are excluded from the slice and their status
// updated accordingly.
func (b *Builder) validHTTPProxies() []*projcontour.HTTPProxy {
	// ensure that a given fqdn is only referenced in a single HTTPProxy resource
	var valid []*projcontour.HTTPProxy
	fqdnHTTPProxies := make(map[string][]*projcontour.HTTPProxy)
	for _, proxy := range b.Source.httpproxies {
		if proxy.Spec.VirtualHost == nil {
			valid = append(valid, proxy)
			continue
		}
		fqdnHTTPProxies[proxy.Spec.VirtualHost.Fqdn] = append(fqdnHTTPProxies[proxy.Spec.VirtualHost.Fqdn], proxy)
	}

	for fqdn, proxies := range fqdnHTTPProxies {
		switch len(proxies) {
		case 1:
			valid = append(valid, proxies[0])
		default:
			// multiple irs use the same fqdn. mark them as invalid.
			var conflicting []string
			for _, proxy := range proxies {
				conflicting = append(conflicting, proxy.Namespace+"/"+proxy.Name)
			}
			sort.Strings(conflicting) // sort for test stability
			msg := fmt.Sprintf("fqdn %q is used in multiple HTTPProxies: %s", fqdn, strings.Join(conflicting, ", "))
			for _, proxy := range proxies {
				sw, commit := b.WithObject(proxy)
				sw.WithValue("vhost", fqdn).SetInvalid(msg)
				commit()
			}
		}
	}
	return valid
}

// computeSecureVirtualhosts populates tls parameters of
// secure virtual hosts.
func (b *Builder) computeSecureVirtualhosts() {
	for _, ing := range b.Source.ingresses {
		for _, tls := range ing.Spec.TLS {
			m := splitSecret(tls.SecretName, ing.Namespace)
			sec := b.lookupSecret(m, validSecret)
			if sec != nil && b.delegationPermitted(m, ing.Namespace) {
				for _, host := range tls.Hosts {
					svhost := b.lookupSecureVirtualHost(host)
					svhost.Secret = sec
					version := ing.Annotations["contour.heptio.com/tls-minimum-protocol-version"]
					svhost.MinProtoVersion = MinProtoVersion(version)
				}
			}
		}
	}
}

func (b *Builder) delegationPermitted(secret Meta, to string) bool {
	contains := func(haystack []string, needle string) bool {
		if len(haystack) == 1 && haystack[0] == "*" {
			return true
		}
		for _, h := range haystack {
			if h == needle {
				return true
			}
		}
		return false
	}

	if secret.namespace == to {
		// secret is in the same namespace as target
		return true
	}
	for _, d := range b.Source.irdelegations {
		if d.Namespace != secret.namespace {
			continue
		}
		for _, d := range d.Spec.Delegations {
			if contains(d.TargetNamespaces, to) {
				if secret.name == d.SecretName {
					return true
				}
			}
		}
	}
	return false
}

func (b *Builder) computeIngresses() {
	// deconstruct each ingress into routes and virtualhost entries
	for _, ing := range b.Source.ingresses {

		// rewrite the default ingress to a stock ingress rule.
		rules := rulesFromSpec(ing.Spec)

		for _, rule := range rules {
			if strings.Contains(rule.Host, "*") {
				// reject hosts with wildcard characters.
				continue
			}
			host := rule.Host
			if host == "" {
				// if host name is blank, rewrite to Envoy's * default host.
				host = "*"
			}
			for _, httppath := range httppaths(rule) {
				path := stringOrDefault(httppath.Path, "/")
				be := httppath.Backend
				m := Meta{name: be.ServiceName, namespace: ing.Namespace}
				s := b.lookupService(m, be.ServicePort)
				if s == nil {
					continue
				}

				r := route(ing, path, s)

				// should we create port 80 routes for this ingress
				if tlsRequired(ing) || httpAllowed(ing) {
					b.lookupVirtualHost(host).addRoute(r)
				}

				// computeSecureVirtualhosts will have populated b.securevirtualhosts
				// with the names of tls enabled ingress objects. If host exists then
				// it is correctly configured for TLS.
				svh, ok := b.securevirtualhosts[host]
				if ok && host != "*" {
					svh.addRoute(r)
				}
			}
		}
	}
}

func (b *Builder) computeIngressRoutes() {
	for _, ir := range b.validIngressRoutes() {
		b.computeIngressRoute(ir)
	}
}

func (b *Builder) computeIngressRoute(ir *ingressroutev1.IngressRoute) {
	sw, commit := b.WithObject(ir)
	defer commit()

	if ir.Spec.VirtualHost == nil {
		// mark delegate ingressroute orphaned.
		b.setOrphaned(ir)
		return
	}

	// ensure root ingressroute lives in allowed namespace
	if !b.rootAllowed(ir.Namespace) {
		sw.SetInvalid("root IngressRoute cannot be defined in this namespace")
		return
	}

	host := ir.Spec.VirtualHost.Fqdn
	if isBlank(host) {
		sw.SetInvalid("Spec.VirtualHost.Fqdn must be specified")
		return
	}
	sw.WithValue("vhost", host)

	if strings.Contains(host, "*") {
		sw.SetInvalid(fmt.Sprintf("Spec.VirtualHost.Fqdn %q cannot use wildcards", host))
		return
	}

	var enforceTLS, passthrough bool
	if tls := ir.Spec.VirtualHost.TLS; tls != nil {
		m := splitSecret(tls.SecretName, ir.Namespace)
		sec := b.lookupSecret(m, validSecret)
		if sec != nil {
			if !b.delegationPermitted(m, ir.Namespace) {
				sw.SetInvalid(fmt.Sprintf("%s: certificate delegation not permitted", tls.SecretName))
				return
			}
			svhost := b.lookupSecureVirtualHost(ir.Spec.VirtualHost.Fqdn)
			svhost.Secret = sec
			svhost.MinProtoVersion = MinProtoVersion(ir.Spec.VirtualHost.TLS.MinimumProtocolVersion)
			enforceTLS = true
		}
		// passthrough is true if tls.secretName is not present, and
		// tls.passthrough is set to true.
		passthrough = isBlank(tls.SecretName) && tls.Passthrough

		// If not passthrough and secret is invalid, then set status
		if sec == nil && !passthrough {
			sw.SetInvalid(fmt.Sprintf("TLS Secret [%s] not found or is malformed", tls.SecretName))
			return
		}
	}

	if ir.Spec.TCPProxy != nil && (passthrough || enforceTLS) {
		b.processTCPProxy(sw, ir, nil, host)
	}
	b.processIngressRoutes(sw, ir, "", nil, host, ir.Spec.TCPProxy == nil && enforceTLS)
}

func (b *Builder) computeHTTPProxies() {
	for _, proxy := range b.validHTTPProxies() {
		b.computeHTTPProxy(proxy)
	}
}

func (b *Builder) computeHTTPProxy(proxy *projcontour.HTTPProxy) {
	sw, commit := b.WithObject(proxy)
	defer commit()

	if proxy.Spec.VirtualHost == nil {
		// mark HTTPProxy as orphaned.
		b.setOrphaned(proxy)
		return
	}

	// ensure root httpproxy lives in allowed namespace
	if !b.rootAllowed(proxy.Namespace) {
		sw.SetInvalid("root HTTPProxy cannot be defined in this namespace")
		return
	}

	host := proxy.Spec.VirtualHost.Fqdn
	if isBlank(host) {
		sw.SetInvalid("Spec.VirtualHost.Fqdn must be specified")
		return
	}
	sw = sw.WithValue("vhost", host)
	if strings.Contains(host, "*") {
		sw.SetInvalid(fmt.Sprintf("Spec.VirtualHost.Fqdn %q cannot use wildcards", host))
		return
	}

	var enforceTLS, passthrough bool
	if tls := proxy.Spec.VirtualHost.TLS; tls != nil {
		// attach secrets to TLS enabled vhosts
		m := splitSecret(tls.SecretName, proxy.Namespace)
		sec := b.lookupSecret(m, validSecret)
		if sec != nil && b.delegationPermitted(m, proxy.Namespace) {
			svhost := b.lookupSecureVirtualHost(host)
			svhost.Secret = sec
			svhost.MinProtoVersion = MinProtoVersion(proxy.Spec.VirtualHost.TLS.MinimumProtocolVersion)
			enforceTLS = true
		}
		// passthrough is true if tls.secretName is not present, and
		// tls.passthrough is set to true.
		passthrough = isBlank(tls.SecretName) && tls.Passthrough

		// If not passthrough and secret is invalid, then set status
		if sec == nil && !passthrough {
			sw.SetInvalid(fmt.Sprintf("TLS Secret [%s] not found or is malformed", tls.SecretName))
			return
		}
	}

	insecure := b.lookupVirtualHost(host)
	secure := b.lookupSecureVirtualHost(host)
	for _, route := range b.computeRoutes(sw, proxy, nil, nil, enforceTLS) {
		insecure.addRoute(route)
		secure.addRoute(route)
	}
	sw.SetValid()
}

// httpProxyConditions converts a list of HTTPProxy conditions to dag Conditions.
func httpProxyConditions(conds []projcontour.Condition) []Condition {
	var c []Condition
	for _, cond := range conds {
		if cond.Prefix != "" {
			c = append(c, &PrefixCondition{
				Prefix: cond.Prefix,
			})
		}
		if cond.Header != nil {
			// Header present only
			if cond.Header.Present {
				c = append(c, &HeaderCondition{
					Name:      cond.Header.Name,
					MatchType: "present",
					Invert:    false,
				})
			} else {
				// Header contains
				if cond.Header.Contains != "" {
					c = append(c, &HeaderCondition{
						Name:      cond.Header.Name,
						Value:     cond.Header.Contains,
						MatchType: "contains",
						Invert:    false,
					})
				}
			}
		}
	}
	return c
}

func (b *Builder) computeRoutes(sw *ObjectStatusWriter, proxy *projcontour.HTTPProxy, conditions []projcontour.Condition, visited []*projcontour.HTTPProxy, enforceTLS bool) []*Route {
	for _, v := range visited {
		if v.Name == proxy.Name && v.Namespace == proxy.Namespace {
			sw.SetInvalid("loop")
			return nil
		}
	}

	visited = append(visited, proxy)
	var routes []*Route
	// Loop over and process all includes
	for _, include := range proxy.Spec.Includes {
		if delegate, ok := b.Source.httpproxies[Meta{name: include.Name, namespace: include.Namespace}]; ok {
			if delegate.Spec.VirtualHost != nil {
				sw.SetInvalid("root httpproxy cannot delegate to another root httpproxy")
				return nil
			}

			sw, commit := b.WithObject(delegate)
			routes = append(routes, b.computeRoutes(sw, delegate, append(conditions, include.Conditions...), visited, enforceTLS)...)
			commit()

			// dest is not an orphaned httpproxy, as there is an httpproxy that points to it
			delete(b.orphaned, Meta{name: delegate.Name, namespace: delegate.Namespace})
		}
	}
	for _, route := range proxy.Spec.Routes {
		if len(route.Services) > 1 && route.EnableWebsockets {
			sw.SetInvalid("route: cannot specify multiple services and enable websockets")
			continue
		}

		r := &Route{
			Conditions:    httpProxyConditions(append(conditions, route.Conditions...)),
			Websocket:     route.EnableWebsockets,
			HTTPSUpgrade:  routeEnforceTLS(enforceTLS, route.PermitInsecure && !b.DisablePermitInsecure),
			PrefixRewrite: route.PrefixRewrite,
			TimeoutPolicy: timeoutPolicy(route.TimeoutPolicy),
			RetryPolicy:   retryPolicy(route.RetryPolicy),
		}

		for _, service := range route.Services {
			if service.Port < 1 || service.Port > 65535 {
				sw.SetInvalid(fmt.Sprintf("route %q: service %q: port must be in the range 1-65535", "??", service.Name))
				return nil
			}
			m := Meta{name: service.Name, namespace: proxy.Namespace}
			s := b.lookupService(m, intstr.FromInt(service.Port))

			if s == nil {
				msg := fmt.Sprintf("Service [%s:%d] is invalid or missing", service.Name, service.Port)
				sw.SetInvalid(msg)
				return nil
			}

			var uv *UpstreamValidation
			var err error
			if s.Protocol == "tls" {
				// we can only validate TLS connections to services that talk TLS
				uv, err = b.lookupUpstreamValidation("??", service.Name, service.UpstreamValidation, proxy.Namespace)
				if err != nil {
					sw.SetInvalid(err.Error())
					return nil
				}
			}

			c := &Cluster{
				Upstream:             s,
				LoadBalancerStrategy: service.Strategy,
				Weight:               service.Weight,
				HealthCheckPolicy:    healthCheckPolicy(service.HealthCheck),
				UpstreamValidation:   uv,
			}
			if service.Mirror && r.MirrorPolicy != nil {
				sw.SetInvalid("only one service per route may be nominated as mirror")
				return nil
			}
			if service.Mirror {
				r.MirrorPolicy = &MirrorPolicy{
					Cluster: c,
				}
			} else {
				r.Clusters = append(r.Clusters, c)
			}
		}
		routes = append(routes, r)
	}
	return routes
}

// buildDAG returns a *DAG representing the current state of this builder.
func (b *Builder) buildDAG() *DAG {
	var dag DAG

	http := b.buildHTTPListener()
	if len(http.VirtualHosts) > 0 {
		dag.roots = append(dag.roots, http)
	}

	https := b.buildHTTPSListener()
	if len(https.VirtualHosts) > 0 {
		dag.roots = append(dag.roots, https)
	}

	for meta := range b.orphaned {
		ir, ok := b.Source.ingressroutes[meta]
		if ok {
			sw, commit := b.WithObject(ir)
			sw.WithValue("status", StatusOrphaned).
				WithValue("description", "this IngressRoute is not part of a delegation chain from a root IngressRoute")
			commit()
		}
		proxy, ok := b.Source.httpproxies[meta]
		if ok {
			sw, commit := b.WithObject(proxy)
			sw.WithValue("status", StatusOrphaned).
				WithValue("description", "this HTTPProxy is not part of a delegation chain from a root HTTPProxy")
			commit()
		}
	}
	dag.statuses = b.statuses
	return &dag
}

// buildHTTPListener builds a *dag.Listener for the vhosts bound to port 80.
// The list of virtual hosts will attached to the listener will be sorted
// by hostname.
func (b *Builder) buildHTTPListener() *Listener {
	var virtualhosts = make([]Vertex, 0, len(b.virtualhosts))

	for _, vh := range b.virtualhosts {
		if vh.Valid() {
			virtualhosts = append(virtualhosts, vh)
		}
	}
	sort.SliceStable(virtualhosts, func(i, j int) bool {
		return virtualhosts[i].(*VirtualHost).Name < virtualhosts[j].(*VirtualHost).Name
	})
	return &Listener{
		Port:         80,
		VirtualHosts: virtualhosts,
	}
}

// buildHTTPSListener builds a *dag.Listener for the vhosts bound to port 443.
// The list of virtual hosts will attached to the listener will be sorted
// by hostname.
func (b *Builder) buildHTTPSListener() *Listener {
	var virtualhosts = make([]Vertex, 0, len(b.securevirtualhosts))
	for _, svh := range b.securevirtualhosts {
		if svh.Valid() {
			virtualhosts = append(virtualhosts, svh)
		}
	}
	sort.SliceStable(virtualhosts, func(i, j int) bool {
		return virtualhosts[i].(*SecureVirtualHost).Name < virtualhosts[j].(*SecureVirtualHost).Name
	})
	return &Listener{
		Port:         443,
		VirtualHosts: virtualhosts,
	}
}

// setOrphaned records an IngressRoute/HTTPProxy resource as orphaned.
func (b *Builder) setOrphaned(obj Object) {
	m := Meta{
		name:      obj.GetObjectMeta().GetName(),
		namespace: obj.GetObjectMeta().GetNamespace(),
	}
	b.orphaned[m] = true
}

// rootAllowed returns true if the IngressRoute or HTTPProxy lives in a permitted root namespace.
func (b *Builder) rootAllowed(namespace string) bool {
	if len(b.Source.RootNamespaces) == 0 {
		return true
	}
	for _, ns := range b.Source.RootNamespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func (b *Builder) processIngressRoutes(sw *ObjectStatusWriter, ir *ingressroutev1.IngressRoute, prefixMatch string, visited []*ingressroutev1.IngressRoute, host string, enforceTLS bool) {
	visited = append(visited, ir)

	for _, route := range ir.Spec.Routes {
		// route cannot both delegate and point to services
		if len(route.Services) > 0 && route.Delegate != nil {
			sw.SetInvalid(fmt.Sprintf("route %q: cannot specify services and delegate in the same route", route.Match))
			return
		}

		// Cannot support multiple services with websockets (See: https://github.com/projectcontour/contour/issues/732)
		if len(route.Services) > 1 && route.EnableWebsockets {
			sw.SetInvalid(fmt.Sprintf("route %q: cannot specify multiple services and enable websockets", route.Match))
			return
		}

		// base case: The route points to services, so we add them to the vhost
		if len(route.Services) > 0 {
			if !matchesPathPrefix(route.Match, prefixMatch) {
				sw.SetInvalid(fmt.Sprintf("the path prefix %q does not match the parent's path prefix %q", route.Match, prefixMatch))
				return
			}

			permitInsecure := route.PermitInsecure && !b.DisablePermitInsecure
			r := &Route{
				Websocket:     route.EnableWebsockets,
				HTTPSUpgrade:  routeEnforceTLS(enforceTLS, permitInsecure),
				PrefixRewrite: route.PrefixRewrite,
				TimeoutPolicy: ingressrouteTimeoutPolicy(route.TimeoutPolicy),
				RetryPolicy:   retryPolicy(route.RetryPolicy),
				Conditions: []Condition{
					&PrefixCondition{Prefix: route.Match},
				},
			}
			for _, service := range route.Services {
				if service.Port < 1 || service.Port > 65535 {
					sw.SetInvalid(fmt.Sprintf("route %q: service %q: port must be in the range 1-65535", route.Match, service.Name))
					return
				}
				m := Meta{name: service.Name, namespace: ir.Namespace}
				s := b.lookupService(m, intstr.FromInt(service.Port))

				if s == nil {
					sw.SetInvalid(fmt.Sprintf("Service [%s:%d] is invalid or missing", service.Name, service.Port))
					return
				}

				var uv *UpstreamValidation
				var err error
				if s.Protocol == "tls" {
					// we can only validate TLS connections to services that talk TLS
					uv, err = b.lookupUpstreamValidation(route.Match, service.Name, service.UpstreamValidation, ir.Namespace)
					if err != nil {
						sw.SetInvalid(err.Error())
					}
				}
				r.Clusters = append(r.Clusters, &Cluster{
					Upstream:             s,
					LoadBalancerStrategy: service.Strategy,
					Weight:               service.Weight,
					HealthCheckPolicy:    healthCheckPolicy(service.HealthCheck),
					UpstreamValidation:   uv,
				})
			}

			b.lookupVirtualHost(host).addRoute(r)
			if enforceTLS {
				b.lookupSecureVirtualHost(host).addRoute(r)
			}
			continue
		}

		if route.Delegate == nil {
			// not a delegate route
			continue
		}

		namespace := route.Delegate.Namespace
		if namespace == "" {
			// we are delegating to another IngressRoute in the same namespace
			namespace = ir.Namespace
		}

		if dest, ok := b.Source.ingressroutes[Meta{name: route.Delegate.Name, namespace: namespace}]; ok {
			if dest.Spec.VirtualHost != nil {
				sw.SetInvalid("root ingressroute cannot delegate to another root ingressroute")
				return
			}

			// dest is not an orphaned ingress route, as there is an IR that points to it
			delete(b.orphaned, Meta{name: dest.Name, namespace: dest.Namespace})

			// ensure we are not following an edge that produces a cycle
			var path []string
			for _, vir := range visited {
				path = append(path, fmt.Sprintf("%s/%s", vir.Namespace, vir.Name))
			}
			for _, vir := range visited {
				if dest.Name == vir.Name && dest.Namespace == vir.Namespace {
					path = append(path, fmt.Sprintf("%s/%s", dest.Namespace, dest.Name))
					sw.SetInvalid(fmt.Sprintf("route creates a delegation cycle: %s", strings.Join(path, " -> ")))
					return
				}
			}

			// follow the link and process the target ingress route
			sw, commit := b.WithObject(dest)
			b.processIngressRoutes(sw, dest, route.Match, visited, host, enforceTLS)
			commit()
		}
	}
	sw.SetValid()
}

func (b *Builder) lookupUpstreamValidation(match string, serviceName string, uv *projcontour.UpstreamValidation, namespace string) (*UpstreamValidation, error) {
	if uv == nil {
		// no upstream validation requested, nothing to do
		return nil, nil
	}

	cacert := b.lookupSecret(Meta{name: uv.CACertificate, namespace: namespace}, validCA)
	if cacert == nil {
		// UpstreamValidation is requested, but cert is missing or not configured
		return nil, fmt.Errorf("route %q: service %q: upstreamValidation requested but secret not found or misconfigured", match, serviceName)
	}

	if uv.SubjectName == "" {
		// UpstreamValidation is requested, but SAN is not provided
		return nil, fmt.Errorf("route %q: service %q: upstreamValidation requested but subject alt name not found or misconfigured", match, serviceName)
	}

	return &UpstreamValidation{
		CACertificate: cacert,
		SubjectName:   uv.SubjectName,
	}, nil
}

func (b *Builder) processTCPProxy(sw *ObjectStatusWriter, ir *ingressroutev1.IngressRoute, visited []*ingressroutev1.IngressRoute, host string) {
	visited = append(visited, ir)

	// tcpproxy cannot both delegate and point to services
	tcpproxy := ir.Spec.TCPProxy
	if len(tcpproxy.Services) > 0 && tcpproxy.Delegate != nil {
		sw.SetInvalid("tcpproxy: cannot specify services and delegate in the same tcpproxy")
		return
	}

	if len(tcpproxy.Services) > 0 {
		var proxy TCPProxy
		for _, service := range tcpproxy.Services {
			m := Meta{name: service.Name, namespace: ir.Namespace}
			s := b.lookupService(m, intstr.FromInt(service.Port))
			if s == nil {
				sw.SetInvalid(fmt.Sprintf("tcpproxy: service %s/%s/%d: not found", ir.Namespace, service.Name, service.Port))
				return
			}
			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream:             s,
				LoadBalancerStrategy: service.Strategy,
			})
		}
		b.lookupSecureVirtualHost(host).TCPProxy = &proxy
		sw.SetValid()
		return
	}

	if tcpproxy.Delegate == nil {
		// not a delegate tcpproxy
		return
	}

	namespace := tcpproxy.Delegate.Namespace
	if namespace == "" {
		// we are delegating to another IngressRoute in the same namespace
		namespace = ir.Namespace
	}

	if dest, ok := b.Source.ingressroutes[Meta{name: tcpproxy.Delegate.Name, namespace: namespace}]; ok {
		// dest is not an orphaned ingress route, as there is an IR that points to it
		delete(b.orphaned, Meta{name: dest.Name, namespace: dest.Namespace})

		// ensure we are not following an edge that produces a cycle
		var path []string
		for _, vir := range visited {
			path = append(path, fmt.Sprintf("%s/%s", vir.Namespace, vir.Name))
		}
		for _, vir := range visited {
			if dest.Name == vir.Name && dest.Namespace == vir.Namespace {
				path = append(path, fmt.Sprintf("%s/%s", dest.Namespace, dest.Name))
				sw.SetInvalid(fmt.Sprintf("tcpproxy creates a delegation cycle: %s", strings.Join(path, " -> ")))
				return
			}
		}

		// follow the link and process the target ingress route
		sw, commit := sw.WithObject(dest)
		b.processTCPProxy(sw, dest, visited, host)
		commit()
	}

	sw.SetValid()
}

func externalName(svc *v1.Service) string {
	if svc.Spec.Type != v1.ServiceTypeExternalName {
		return ""
	}
	return svc.Spec.ExternalName
}

// route returns a dag.Route for the supplied Ingress.
func route(ingress *v1beta1.Ingress, path string, service *Service) *Route {
	var retry *RetryPolicy
	if retryOn, ok := ingress.Annotations[annotationRetryOn]; ok && len(retryOn) > 0 {
		// if there is a non empty retry-on annotation, build a RetryPolicy manually.
		retry = &RetryPolicy{
			RetryOn: retryOn,
			// TODO(dfc) NumRetries may parse as 0, which is inconsistent with
			// retryPolicyIngressRoute()'s default value of 1.
			NumRetries: parseUInt32(ingress.Annotations[annotationNumRetries]),
			// TODO(dfc) PerTryTimeout will parse to -1, infinite, in the case of
			// invalid data, this is inconsistent with retryPolicyIngressRoute()'s default value
			// of 0 duration.
			PerTryTimeout: parseTimeout(ingress.Annotations[annotationPerTryTimeout]),
		}
	}

	var timeout *TimeoutPolicy
	if response, ok := ingress.Annotations[annotationRequestTimeout]; ok {
		// if the request timeout annotation is present on this ingress
		// construct and use the ingressroute timeout policy logic.
		// Note: due to a misunderstanding the name of the annotation is
		// request timeout, but it is actually applied as a timeout on
		// the response body.
		timeout = timeoutPolicy(&projcontour.TimeoutPolicy{
			Response: response,
		})
	}

	wr := websocketRoutes(ingress)
	r := &Route{
		HTTPSUpgrade:  tlsRequired(ingress),
		Websocket:     wr[path],
		TimeoutPolicy: timeout,
		RetryPolicy:   retry,
		Clusters: []*Cluster{{
			Upstream: service,
		}},
	}

	if strings.ContainsAny(path, "^+*[]%") {
		// path smells like a regex
		r.Conditions = append(r.Conditions, &RegexCondition{Regex: path})
		return r
	}

	r.Conditions = append(r.Conditions, &PrefixCondition{Prefix: path})
	return r
}

// isBlank indicates if a string contains nothing but blank characters.
func isBlank(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// MinProtoVersion returns the TLS protocol version specified by an ingress annotation
// or default if non present.
func MinProtoVersion(version string) envoy_api_v2_auth.TlsParameters_TlsProtocol {
	switch version {
	case "1.3":
		return envoy_api_v2_auth.TlsParameters_TLSv1_3
	case "1.2":
		return envoy_api_v2_auth.TlsParameters_TLSv1_2
	default:
		// any other value is interpreted as TLS/1.1
		return envoy_api_v2_auth.TlsParameters_TLSv1_1
	}
}

// splitSecret splits a secretName into its namespace and name components.
// If there is no namespace prefix, the default namespace is returned.
func splitSecret(secret, defns string) Meta {
	v := strings.SplitN(secret, "/", 2)
	switch len(v) {
	case 1:
		// no prefix
		return Meta{
			name:      v[0],
			namespace: defns,
		}
	default:
		return Meta{
			name:      v[1],
			namespace: stringOrDefault(v[0], defns),
		}
	}
}

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// rulesFromSpec merges the IngressSpec's Rules with a synthetic
// rule representing the default backend.
func rulesFromSpec(spec v1beta1.IngressSpec) []v1beta1.IngressRule {
	rules := spec.Rules
	if backend := spec.Backend; backend != nil {
		rule := defaultBackendRule(backend)
		rules = append(rules, rule)
	}
	return rules
}

// defaultBackendRule returns an IngressRule that represents the IngressBackend.
func defaultBackendRule(be *v1beta1.IngressBackend) v1beta1.IngressRule {
	return v1beta1.IngressRule{
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: []v1beta1.HTTPIngressPath{{
					Backend: v1beta1.IngressBackend{
						ServiceName: be.ServiceName,
						ServicePort: be.ServicePort,
					},
				}},
			},
		},
	}
}

// validSecret returns true if the Secret contains certificate and private key material.
func validSecret(s *v1.Secret) bool {
	return s.Type == v1.SecretTypeTLS && len(s.Data[v1.TLSCertKey]) > 0 && len(s.Data[v1.TLSPrivateKeyKey]) > 0
}

func validCA(s *v1.Secret) bool {
	return len(s.Data["ca.crt"]) > 0
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
		prefix += "/"
	}
	if path[len(path)-1] != '/' {
		path += "/"
	}
	return strings.HasPrefix(path, prefix)
}
