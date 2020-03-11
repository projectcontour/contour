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
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/google/go-cmp/cmp"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
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
		switch {
		case int(p.Port) == port.IntValue():
			return b.addService(svc, p)
		case port.String() == p.Name:
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
		MaxConnections:     maxConnections(svc),
		MaxPendingRequests: maxPendingRequests(svc),
		MaxRequests:        maxRequests(svc),
		MaxRetries:         maxRetries(svc),
		ExternalName:       externalName(svc),
	}
	b.services[s.toMeta()] = s
	return s
}

func upstreamProtocol(svc *v1.Service, port *v1.ServicePort) string {
	up := parseUpstreamProtocols(svc.Annotations)
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
					version := compatAnnotation(ing, "tls-minimum-protocol-version")
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

	for _, d := range b.Source.httpproxydelegations {
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
			b.computeIngressRule(ing, rule)
		}
	}
}

func (b *Builder) computeIngressRule(ing *v1beta1.Ingress, rule v1beta1.IngressRule) {
	host := rule.Host
	if strings.Contains(host, "*") {
		// reject hosts with wildcard characters.
		return
	}
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
		sw.SetInvalid("Spec.VirtualHost.Fqdn %q cannot use wildcards", host)
		return
	}

	var enforceTLS, passthrough bool
	if tls := ir.Spec.VirtualHost.TLS; tls != nil {
		m := splitSecret(tls.SecretName, ir.Namespace)
		sec := b.lookupSecret(m, validSecret)
		if sec != nil {
			if !b.delegationPermitted(m, ir.Namespace) {
				sw.SetInvalid("%s: certificate delegation not permitted", tls.SecretName)
				return
			}
			svhost := b.lookupSecureVirtualHost(ir.Spec.VirtualHost.Fqdn)
			svhost.Secret = sec
			svhost.MinProtoVersion = MinProtoVersion(ir.Spec.VirtualHost.TLS.MinimumProtocolVersion)
			enforceTLS = true
		}
		// passthrough is true if tls.secretName is not present, and
		// tls.passthrough is set to true.
		passthrough = tls.SecretName == "" && tls.Passthrough

		// If not passthrough and secret is invalid, then set status
		if sec == nil && !passthrough {
			sw.SetInvalid("TLS Secret [%s] not found or is malformed", tls.SecretName)
			return
		}
	}

	if ir.Spec.TCPProxy != nil && (passthrough || enforceTLS) {
		b.processIngressRouteTCPProxy(sw, ir, nil, host)
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
		sw.SetInvalid("Spec.VirtualHost.Fqdn %q cannot use wildcards", host)
		return
	}

	var tlsValid bool
	if tls := proxy.Spec.VirtualHost.TLS; tls != nil {

		// tls is valid if passthrough == true XOR secretName != ""
		tlsValid = tls.Passthrough != !isBlank(tls.SecretName)

		// attach secrets to TLS enabled vhosts
		m := splitSecret(tls.SecretName, proxy.Namespace)
		sec := b.lookupSecret(m, validSecret)
		if sec != nil {
			if !b.delegationPermitted(m, proxy.Namespace) {
				sw.SetInvalid("%s: certificate delegation not permitted", tls.SecretName)
				return
			}
			svhost := b.lookupSecureVirtualHost(host)
			svhost.Secret = sec
			svhost.MinProtoVersion = MinProtoVersion(proxy.Spec.VirtualHost.TLS.MinimumProtocolVersion)
		}

		if sec == nil && !tls.Passthrough {
			sw.SetInvalid("TLS Secret [%s] not found or is malformed", tls.SecretName)
			return
		}
	}

	if proxy.Spec.TCPProxy != nil {
		if !tlsValid {
			sw.SetInvalid("tcpproxy: missing tls.passthrough or tls.secretName")
			return
		}
		if !b.processHTTPProxyTCPProxy(sw, proxy, nil, host) {
			return
		}
	}

	routes := b.computeRoutes(sw, proxy, nil, nil, tlsValid)
	insecure := b.lookupVirtualHost(host)
	addRoutes(insecure, routes)

	// if TLS is enabled for this virtual host and there is no tcp proxy defined,
	// then add routes to the secure virtualhost definition.
	if tlsValid && proxy.Spec.TCPProxy == nil {
		secure := b.lookupSecureVirtualHost(host)
		addRoutes(secure, routes)
	}
}

type vhost interface {
	addRoute(*Route)
}

// addRoutes adds all routes to the vhost supplied.
func addRoutes(vhost vhost, routes []*Route) {
	for _, route := range routes {
		vhost.addRoute(route)
	}
}

// expandPrefixMatches adds new Routes to account for the difference
// between prefix replacement when matching on '/foo' and '/foo/'.
//
// The table below shows the behavior of Envoy prefix rewrite. If we
// match on only `/foo` or `/foo/`, then the unwanted rewrites marked
// with X can result. This means that we need to generate separate
// prefix matches (and replacements) for these cases.
//
// | Matching Prefix | Replacement | Client Path | Rewritten Path |
// |-----------------|-------------|-------------|----------------|
// | `/foo`          | `/bar`      | `/foosball` |   `/barsball`  |
// | `/foo`          | `/`         | `/foo/v1`   | X `//v1`       |
// | `/foo/`         | `/bar`      | `/foo/type` | X `/bartype`   |
// | `/foo`          | `/bar/`     | `/foosball` | X `/bar/sball` |
// | `/foo/`         | `/bar/`     | `/foo/type` |   `/bar/type`  |
func expandPrefixMatches(routes []*Route) []*Route {
	prefixedRoutes := map[string][]*Route{}

	expandedRoutes := []*Route{}

	// First, we group the Routes by their slash-consistent prefix match condition.
	for _, r := range routes {
		// If there is no path prefix, we won't do any expansion, so skip it.
		if !r.HasPathPrefix() {
			expandedRoutes = append(expandedRoutes, r)
		}

		routingPrefix := r.PathCondition.(*PrefixCondition).Prefix

		if routingPrefix != "/" {
			routingPrefix = strings.TrimRight(routingPrefix, "/")
		}

		prefixedRoutes[routingPrefix] = append(prefixedRoutes[routingPrefix], r)
	}

	for prefix, routes := range prefixedRoutes {
		// Propagate the Routes into the expanded set. Since
		// we have a slice of pointers, we can propagate here
		// prior to any Route modifications.
		expandedRoutes = append(expandedRoutes, routes...)

		switch len(routes) {
		case 1:
			// Don't modify if we are not doing a replacement.
			if len(routes[0].PrefixRewrite) == 0 {
				continue
			}

			routingPrefix := routes[0].PathCondition.(*PrefixCondition).Prefix

			// There's no alternate forms for '/' :)
			if routingPrefix == "/" {
				continue
			}

			// Shallow copy the Route. TODO(jpeach) deep copying would be more robust.
			newRoute := *routes[0]

			// Now, make the original route handle '/foo' and the new route handle '/foo'.
			routes[0].PrefixRewrite = strings.TrimRight(routes[0].PrefixRewrite, "/")
			routes[0].PathCondition = &PrefixCondition{Prefix: prefix}

			newRoute.PrefixRewrite = routes[0].PrefixRewrite + "/"
			newRoute.PathCondition = &PrefixCondition{Prefix: prefix + "/"}

			// Since we trimmed trailing '/', it's possible that
			// we made the replacement empty. There's no such
			// thing as an empty rewrite; it's the same as
			// rewriting to '/'.
			if len(routes[0].PrefixRewrite) == 0 {
				routes[0].PrefixRewrite = "/"
			}

			expandedRoutes = append(expandedRoutes, &newRoute)
		case 2:
			// This group routes on both '/foo' and
			// '/foo/' so we can't add any implicit prefix
			// matches. This is why we didn't filter out
			// routes that don't have replacements earlier.
			continue
		default:
			// This can't happen unless there are routes
			// with duplicate prefix paths.
		}

	}

	return expandedRoutes
}

func getProtocol(service projcontour.Service, s *Service) (string, error) {
	// Determine the protocol to use to speak to this Cluster.
	var protocol string
	if service.Protocol != nil {
		protocol = *service.Protocol
		switch protocol {
		case "h2c", "h2", "tls":
		default:
			return "", fmt.Errorf("unsupported protocol: %v", protocol)
		}
	} else {
		protocol = s.Protocol
	}

	return protocol, nil
}

func (b *Builder) computeRoutes(sw *ObjectStatusWriter, proxy *projcontour.HTTPProxy, conditions []projcontour.Condition, visited []*projcontour.HTTPProxy, enforceTLS bool) []*Route {
	for _, v := range visited {
		// ensure we are not following an edge that produces a cycle
		var path []string
		for _, vir := range visited {
			path = append(path, fmt.Sprintf("%s/%s", vir.Namespace, vir.Name))
		}
		if v.Name == proxy.Name && v.Namespace == proxy.Namespace {
			path = append(path, fmt.Sprintf("%s/%s", proxy.Namespace, proxy.Name))
			sw.SetInvalid("include creates a delegation cycle: %s", strings.Join(path, " -> "))
			return nil
		}
	}

	visited = append(visited, proxy)
	var routes []*Route

	// Check for duplicate conditions on the includes
	if includeConditionsIdentical(proxy.Spec.Includes) {
		sw.SetInvalid("duplicate conditions defined on an include")
		return nil
	}

	// Loop over and process all includes
	for _, include := range proxy.Spec.Includes {
		namespace := include.Namespace
		if namespace == "" {
			namespace = proxy.Namespace
		}

		delegate, ok := b.Source.httpproxies[Meta{name: include.Name, namespace: namespace}]
		if !ok {
			sw.SetInvalid("include %s/%s not found", namespace, include.Name)
			return nil
		}
		if delegate.Spec.VirtualHost != nil {
			sw.SetInvalid("root httpproxy cannot delegate to another root httpproxy")
			return nil
		}

		if err := pathConditionsValid(include.Conditions); err != nil {
			sw.SetInvalid("include: %s", err)
			return nil
		}

		sw, commit := b.WithObject(delegate)
		routes = append(routes, b.computeRoutes(sw, delegate, append(conditions, include.Conditions...), visited, enforceTLS)...)
		commit()

		// dest is not an orphaned httpproxy, as there is an httpproxy that points to it
		delete(b.orphaned, Meta{name: delegate.Name, namespace: delegate.Namespace})
	}

	for _, route := range proxy.Spec.Routes {
		if err := pathConditionsValid(route.Conditions); err != nil {
			sw.SetInvalid("route: %s", err)
			return nil
		}

		conds := append(conditions, route.Conditions...)

		// Look for duplicate exact match headers on this route
		if !headerConditionsAreValid(conds) {
			sw.SetInvalid("cannot specify duplicate header 'exact match' conditions in the same route")
			return nil
		}

		reqHP, err := headersPolicy(route.RequestHeadersPolicy, true /* allow Host */)
		if err != nil {
			sw.SetInvalid(err.Error())
			return nil
		}

		respHP, err := headersPolicy(route.ResponseHeadersPolicy, false /* disallow Host */)
		if err != nil {
			sw.SetInvalid(err.Error())
			return nil
		}

		if len(route.Services) < 1 {
			sw.SetInvalid("route.services must have at least one entry")
			return nil
		}

		r := &Route{
			PathCondition:         mergePathConditions(conds),
			HeaderConditions:      mergeHeaderConditions(conds),
			Websocket:             route.EnableWebsockets,
			HTTPSUpgrade:          routeEnforceTLS(enforceTLS, route.PermitInsecure && !b.DisablePermitInsecure),
			TimeoutPolicy:         timeoutPolicy(route.TimeoutPolicy),
			RetryPolicy:           retryPolicy(route.RetryPolicy),
			RequestHeadersPolicy:  reqHP,
			ResponseHeadersPolicy: respHP,
		}

		if len(route.GetPrefixReplacements()) > 0 {
			if !r.HasPathPrefix() {
				sw.SetInvalid("cannot specify prefix replacements without a prefix condition")
				return nil
			}

			if err := prefixReplacementsAreValid(route.GetPrefixReplacements()); err != nil {
				sw.SetInvalid(err.Error())
				return nil
			}

			// Note that we are guaranteed to always have a prefix
			// condition. Even if the CRD user didn't specify a
			// prefix condition, mergePathConditions() guarantees
			// a prefix of '/'.
			routingPrefix := r.PathCondition.(*PrefixCondition).Prefix

			// First, try to apply an exact prefix match.
			for _, prefix := range route.GetPrefixReplacements() {
				if len(prefix.Prefix) > 0 && routingPrefix == prefix.Prefix {
					r.PrefixRewrite = prefix.Replacement
					break
				}
			}

			// If there wasn't a match, we can apply the default replacement.
			if len(r.PrefixRewrite) == 0 {
				for _, prefix := range route.GetPrefixReplacements() {
					if len(prefix.Prefix) == 0 {
						r.PrefixRewrite = prefix.Replacement
						break
					}
				}
			}

		}

		for _, service := range route.Services {
			if service.Port < 1 || service.Port > 65535 {
				sw.SetInvalid("service %q: port must be in the range 1-65535", service.Name)
				return nil
			}
			m := Meta{name: service.Name, namespace: proxy.Namespace}
			s := b.lookupService(m, intstr.FromInt(service.Port))

			if s == nil {
				sw.SetInvalid("Service [%s:%d] is invalid or missing", service.Name, service.Port)
				return nil
			}

			// Determine the protocol to use to speak to this Cluster.
			protocol, err := getProtocol(service, s)
			if err != nil {
				sw.SetInvalid(err.Error())
				return nil
			}

			var uv *UpstreamValidation
			if protocol == "tls" {
				// we can only validate TLS connections to services that talk TLS
				uv, err = b.lookupUpstreamValidation(service.UpstreamValidation, proxy.Namespace)
				if err != nil {
					sw.SetInvalid("Service [%s:%d] TLS upstream validation policy error: %s",
						service.Name, service.Port, err)
					return nil
				}
			}

			reqHP, err := headersPolicy(service.RequestHeadersPolicy, true /* allow Host */)
			if err != nil {
				sw.SetInvalid(err.Error())
				return nil
			}

			respHP, err := headersPolicy(service.ResponseHeadersPolicy, false /* disallow Host */)
			if err != nil {
				sw.SetInvalid(err.Error())
				return nil
			}

			c := &Cluster{
				Upstream:              s,
				LoadBalancerPolicy:    loadBalancerPolicy(route.LoadBalancerPolicy),
				Weight:                uint32(service.Weight),
				HTTPHealthCheckPolicy: httpHealthCheckPolicy(route.HealthCheckPolicy),
				UpstreamValidation:    uv,
				RequestHeadersPolicy:  reqHP,
				ResponseHeadersPolicy: respHP,
				Protocol:              protocol,
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

	routes = expandPrefixMatches(routes)

	sw.SetValid()
	return routes
}

func escapeHeaderValue(value string) string {
	// Envoy supports %-encoded variables, so literal %'s in the header's value must be escaped.  See:
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#custom-request-response-headers
	return strings.Replace(value, "%", "%%", -1)
}

func includeConditionsIdentical(includes []projcontour.Include) bool {
	j := 0
	for i := 1; i < len(includes); i++ {
		// Now compare each include's set of conditions
		for _, cA := range includes[i].Conditions {
			for _, cB := range includes[j].Conditions {
				if (cA.Prefix == cB.Prefix) && cmp.Equal(cA.Header, cB.Header) {
					return true
				}
			}
		}
		j++
	}
	return false
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
			sw.WithValue("status", k8s.StatusOrphaned).
				WithValue("description", "this IngressRoute is not part of a delegation chain from a root IngressRoute")
			commit()
		}
		proxy, ok := b.Source.httpproxies[meta]
		if ok {
			sw, commit := b.WithObject(proxy)
			sw.WithValue("status", k8s.StatusOrphaned).
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
			sw.SetInvalid("route %q: cannot specify services and delegate in the same route", route.Match)
			return
		}

		// base case: The route points to services, so we add them to the vhost
		if len(route.Services) > 0 {
			if !matchesPathPrefix(route.Match, prefixMatch) {
				sw.SetInvalid("the path prefix %q does not match the parent's path prefix %q", route.Match, prefixMatch)
				return
			}

			permitInsecure := route.PermitInsecure && !b.DisablePermitInsecure
			r := &Route{
				PathCondition: &PrefixCondition{Prefix: route.Match},
				Websocket:     route.EnableWebsockets,
				HTTPSUpgrade:  routeEnforceTLS(enforceTLS, permitInsecure),
				PrefixRewrite: route.PrefixRewrite,
				TimeoutPolicy: ingressrouteTimeoutPolicy(route.TimeoutPolicy),
				RetryPolicy:   retryPolicy(route.RetryPolicy),
			}
			for _, service := range route.Services {
				if service.Port < 1 || service.Port > 65535 {
					sw.SetInvalid("route %q: service %q: port must be in the range 1-65535", route.Match, service.Name)
					return
				}
				m := Meta{name: service.Name, namespace: ir.Namespace}

				s := b.lookupService(m, intstr.FromInt(service.Port))
				if s == nil {
					sw.SetInvalid("Service [%s:%d] is invalid or missing", service.Name, service.Port)
					return
				}

				var uv *UpstreamValidation
				var err error
				if s.Protocol == "tls" {
					// we can only validate TLS connections to services that talk TLS
					uv, err = b.lookupUpstreamValidation(service.UpstreamValidation, ir.Namespace)
					if err != nil {
						sw.SetInvalid("Service [%s:%d] TLS upstream validation policy error: %s",
							service.Name, service.Port, err)
						return
					}
				}

				r.Clusters = append(r.Clusters, &Cluster{
					Upstream:              s,
					LoadBalancerPolicy:    service.Strategy,
					Weight:                uint32(service.Weight),
					HTTPHealthCheckPolicy: ingressrouteHealthCheckPolicy(service.HealthCheck),
					UpstreamValidation:    uv,
					Protocol:              s.Protocol,
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
					sw.SetInvalid("route creates a delegation cycle: %s", strings.Join(path, " -> "))
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

func (b *Builder) lookupUpstreamValidation(uv *projcontour.UpstreamValidation, namespace string) (*UpstreamValidation, error) {
	if uv == nil {
		// no upstream validation requested, nothing to do
		return nil, nil
	}

	cacert := b.lookupSecret(Meta{name: uv.CACertificate, namespace: namespace}, validCA)
	if cacert == nil {
		// UpstreamValidation is requested, but cert is missing or not configured
		return nil, errors.New("secret not found or misconfigured")
	}

	if uv.SubjectName == "" {
		// UpstreamValidation is requested, but SAN is not provided
		return nil, errors.New("missing subject alternative name")
	}

	return &UpstreamValidation{
		CACertificate: cacert,
		SubjectName:   uv.SubjectName,
	}, nil
}

func (b *Builder) processIngressRouteTCPProxy(sw *ObjectStatusWriter, ir *ingressroutev1.IngressRoute, visited []*ingressroutev1.IngressRoute, host string) {
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
				sw.SetInvalid("tcpproxy: service %s/%s/%d: not found", ir.Namespace, service.Name, service.Port)
				return
			}
			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream:           s,
				LoadBalancerPolicy: service.Strategy,
				Protocol:           s.Protocol,
			})
		}
		b.lookupSecureVirtualHost(host).TCPProxy = &proxy
		sw.SetValid()
		return
	}

	if tcpproxy.Delegate == nil {
		// Not a delegate tcpproxy. Note that we allow a TCPProxy to be
		// empty (no services and no delegates) for IngressRoute backwards
		// compatibility. This is not allowed in HTTPProxy.
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
				sw.SetInvalid("tcpproxy creates a delegation cycle: %s", strings.Join(path, " -> "))
				return
			}
		}

		// follow the link and process the target ingress route
		sw, commit := sw.WithObject(dest)
		b.processIngressRouteTCPProxy(sw, dest, visited, host)
		commit()
	}
}

// processHTTPProxyTCPProxy processes the spec.tcpproxy stanza in a HTTPProxy document
// following the chain of spec.tcpproxy.include references. It returns true if processing
// was successful, otherwise false if an error was encountered. The details of the error
// will be recorded on the status of the relevant HTTPProxy object,
func (b *Builder) processHTTPProxyTCPProxy(sw *ObjectStatusWriter, httpproxy *projcontour.HTTPProxy, visited []*projcontour.HTTPProxy, host string) bool {
	tcpproxy := httpproxy.Spec.TCPProxy
	if tcpproxy == nil {
		// nothing to do
		return true
	}

	visited = append(visited, httpproxy)

	// #2218 Allow support for both plural and singular "Include" for TCPProxy for the v1 API Spec
	// Prefer configurations for singular over the plural version
	tcpProxyInclude := tcpproxy.Include
	if tcpproxy.Include == nil {
		tcpProxyInclude = tcpproxy.IncludesDeprecated
	}

	if len(tcpproxy.Services) > 0 && tcpProxyInclude != nil {
		sw.SetInvalid("tcpproxy: cannot specify services and include in the same httpproxy")
		return false
	}

	if len(tcpproxy.Services) > 0 {
		var proxy TCPProxy
		for _, service := range httpproxy.Spec.TCPProxy.Services {
			m := Meta{name: service.Name, namespace: httpproxy.Namespace}
			s := b.lookupService(m, intstr.FromInt(service.Port))
			if s == nil {
				sw.SetInvalid("tcpproxy: service %s/%s/%d: not found", httpproxy.Namespace, service.Name, service.Port)
				return false
			}
			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream:             s,
				Protocol:             s.Protocol,
				LoadBalancerPolicy:   loadBalancerPolicy(tcpproxy.LoadBalancerPolicy),
				TCPHealthCheckPolicy: tcpHealthCheckPolicy(tcpproxy.HealthCheckPolicy),
			})
		}
		b.lookupSecureVirtualHost(host).TCPProxy = &proxy
		return true
	}

	if tcpProxyInclude == nil {
		// We don't allow an empty TCPProxy object.
		sw.SetInvalid("tcpproxy: either services or inclusion must be specified")
		return false
	}

	namespace := tcpProxyInclude.Namespace
	if namespace == "" {
		// we are delegating to another HTTPProxy in the same namespace
		namespace = httpproxy.Namespace
	}

	m := Meta{name: tcpProxyInclude.Name, namespace: namespace}
	dest, ok := b.Source.httpproxies[m]
	if !ok {
		sw.SetInvalid("tcpproxy: include %s/%s not found", m.namespace, m.name)
		return false
	}

	if dest.Spec.VirtualHost != nil {
		sw.SetInvalid("root httpproxy cannot delegate to another root httpproxy")
		return false
	}

	// dest is no longer an orphan
	delete(b.orphaned, toMeta(dest))

	// ensure we are not following an edge that produces a cycle
	var path []string
	for _, hp := range visited {
		path = append(path, fmt.Sprintf("%s/%s", hp.Namespace, hp.Name))
	}
	for _, hp := range visited {
		if dest.Name == hp.Name && dest.Namespace == hp.Namespace {
			path = append(path, fmt.Sprintf("%s/%s", dest.Namespace, dest.Name))
			sw.SetInvalid("tcpproxy include creates a cycle: %s", strings.Join(path, " -> "))
			return false
		}
	}

	// follow the link and process the target tcpproxy
	sw, commit := sw.WithObject(dest)
	defer commit()
	ok = b.processHTTPProxyTCPProxy(sw, dest, visited, host)
	if ok {
		sw.SetValid()
	}
	return ok
}

func externalName(svc *v1.Service) string {
	if svc.Spec.Type != v1.ServiceTypeExternalName {
		return ""
	}
	return svc.Spec.ExternalName
}

// route builds a dag.Route for the supplied Ingress.
func route(ingress *v1beta1.Ingress, path string, service *Service) *Route {
	wr := websocketRoutes(ingress)
	r := &Route{
		HTTPSUpgrade:  tlsRequired(ingress),
		Websocket:     wr[path],
		TimeoutPolicy: ingressTimeoutPolicy(ingress),
		RetryPolicy:   ingressRetryPolicy(ingress),
		Clusters: []*Cluster{{
			Upstream: service,
			Protocol: service.Protocol,
		}},
	}

	if strings.ContainsAny(path, "^+*[]%") {
		// path smells like a regex
		r.PathCondition = &RegexCondition{Regex: path}
		return r
	}

	r.PathCondition = &PrefixCondition{Prefix: path}
	return r
}

// isBlank indicates if a string contains nothing but blank characters.
func isBlank(s string) bool {
	return len(strings.TrimSpace(s)) == 0
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
	return len(s.Data[CACertificateKey]) > 0
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
