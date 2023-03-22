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
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/sirupsen/logrus"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

// IngressProcessor translates Ingresses into DAG
// objects and adds them to the DAG.
type IngressProcessor struct {
	logrus.FieldLogger

	dag    *DAG
	source *KubernetesCache

	// ClientCertificate is the optional identifier of the TLS secret containing client certificate and
	// private key to be used when establishing TLS connection to upstream cluster.
	ClientCertificate *types.NamespacedName

	// EnableExternalNameService allows processing of ExternalNameServices
	// This is normally disabled for security reasons.
	// See https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc for details.
	EnableExternalNameService bool

	// Request headers that will be set on all routes (optional).
	RequestHeadersPolicy *HeadersPolicy

	// Response headers that will be set on all routes (optional).
	ResponseHeadersPolicy *HeadersPolicy

	// ConnectTimeout defines how long the proxy should wait when establishing connection to upstream service.
	ConnectTimeout time.Duration
}

// Run translates Ingresses into DAG objects and
// adds them to the DAG.
func (p *IngressProcessor) Run(dag *DAG, source *KubernetesCache) {
	p.dag = dag
	p.source = source

	// reset the processor when we're done
	defer func() {
		p.dag = nil
		p.source = nil
	}()

	// setup secure vhosts if there is a matching secret
	// we do this first so that the set of active secure vhosts is stable
	// during computeIngresses.
	p.computeSecureVirtualhosts()
	p.computeIngresses()
}

// computeSecureVirtualhosts populates tls parameters of
// secure virtual hosts.
func (p *IngressProcessor) computeSecureVirtualhosts() {
	for _, ing := range p.source.ingresses {
		for _, tls := range ing.Spec.TLS {
			secretName := k8s.NamespacedNameFrom(tls.SecretName, k8s.TLSCertAnnotationNamespace(ing), k8s.DefaultNamespace(ing.GetNamespace()))
			sec, err := p.source.LookupTLSSecret(secretName)
			if err != nil {
				p.WithError(err).
					WithField("name", ing.GetName()).
					WithField("namespace", ing.GetNamespace()).
					WithField("secret", secretName).
					Error("unresolved secret reference")
				continue
			}

			if !p.source.DelegationPermitted(secretName, ing.GetNamespace()) {
				p.WithError(err).
					WithField("name", ing.GetName()).
					WithField("namespace", ing.GetNamespace()).
					WithField("secret", secretName).
					Error("certificate delegation not permitted")
				continue
			}

			// We have validated the TLS secrets, so we can go
			// ahead and create the SecureVirtualHost for this
			// Ingress.
			for _, host := range tls.Hosts {
				svhost := p.dag.EnsureSecureVirtualHost(HTTPS_LISTENER_NAME, host)
				svhost.Secret = sec
				// default to a minimum TLS version of 1.2 if it's not specified
				svhost.MinTLSVersion = annotation.MinTLSVersion(annotation.ContourAnnotation(ing, "tls-minimum-protocol-version"), "1.2")
			}
		}
	}
}

func (p *IngressProcessor) computeIngresses() {
	// deconstruct each ingress into routes and virtualhost entries
	for _, ing := range p.source.ingresses {

		// rewrite the default ingress to a stock ingress rule.
		rules := rulesFromSpec(ing.Spec)
		for _, rule := range rules {
			p.computeIngressRule(ing, rule)
		}
	}
}

func (p *IngressProcessor) computeIngressRule(ing *networking_v1.Ingress, rule networking_v1.IngressRule) {
	host := rule.Host

	// If host name is blank, rewrite to Envoy's * default host.
	if host == "" {
		host = "*"
	}

	var clientCertSecret *Secret
	var err error
	if p.ClientCertificate != nil {
		clientCertSecret, err = p.source.LookupTLSSecret(*p.ClientCertificate)
		if err != nil {
			p.WithError(err).
				WithField("name", ing.GetName()).
				WithField("namespace", ing.GetNamespace()).
				WithField("secret", p.ClientCertificate).
				Error("tls.envoy-client-certificate contains unresolved secret reference")
			return
		}
	}

	for _, httppath := range httppaths(rule) {
		path := stringOrDefault(httppath.Path, "/")
		// Default to implementation specific path matching if not set.
		pathType := ref.Val(httppath.PathType, networking_v1.PathTypeImplementationSpecific)
		be := httppath.Backend
		m := types.NamespacedName{Name: be.Service.Name, Namespace: ing.Namespace}

		port := int(be.Service.Port.Number)
		if len(be.Service.Port.Name) > 0 {
			_, svcPort, err2 := p.source.LookupService(m, intstr.FromString(be.Service.Port.Name))
			if err2 != nil {
				p.WithError(err2).
					WithField("name", ing.GetName()).
					WithField("namespace", ing.GetNamespace()).
					WithField("service", be.Service.Name).
					Error("service is not found")
				continue
			}

			port = int(svcPort.Port)
		}
		s, err := p.dag.EnsureService(m, port, port, p.source, p.EnableExternalNameService)
		if err != nil {
			p.WithError(err).
				WithField("name", ing.GetName()).
				WithField("namespace", ing.GetNamespace()).
				WithField("service", be.Service.Name).
				Error("unresolved service reference")
			continue
		}

		r, err := p.route(ing, rule.Host, path, pathType, s, clientCertSecret, be.Service.Name, be.Service.Port.Number, p.FieldLogger)
		if err != nil {
			p.WithError(err).
				WithField("name", ing.GetName()).
				WithField("namespace", ing.GetNamespace()).
				WithField("regex", path).
				Errorf("path regex is not valid")
			return
		}

		// should we create port 80 routes for this ingress
		if annotation.TLSRequired(ing) || annotation.HTTPAllowed(ing) {
			vhost := p.dag.EnsureVirtualHost(HTTP_LISTENER_NAME, host)
			vhost.AddRoute(r)
		}

		// computeSecureVirtualhosts will have populated b.securevirtualhosts
		// with the names of tls enabled ingress objects. If host exists then
		// it is correctly configured for TLS.
		if svh := p.dag.GetSecureVirtualHost(HTTPS_LISTENER_NAME, host); svh != nil && host != "*" {
			svh.AddRoute(r)
		}
	}
}

const singleDNSLabelWildcardRegex = "^[a-z0-9]([-a-z0-9]*[a-z0-9])?"

var _ = regexp.MustCompile(singleDNSLabelWildcardRegex)

// route builds a dag.Route for the supplied Ingress.
func (p *IngressProcessor) route(ingress *networking_v1.Ingress, host string, path string, pathType networking_v1.PathType, service *Service, clientCertSecret *Secret, serviceName string, servicePort int32, log logrus.FieldLogger) (*Route, error) {
	log = log.WithFields(logrus.Fields{
		"name":      ingress.Name,
		"namespace": ingress.Namespace,
	})

	dynamicHeaders := map[string]string{
		"CONTOUR_NAMESPACE": ingress.Namespace,
	}
	dynamicHeaders["CONTOUR_SERVICE_NAME"] = serviceName
	dynamicHeaders["CONTOUR_SERVICE_PORT"] = strconv.Itoa(int(servicePort))

	// Get default headersPolicies
	reqHP, err := headersPolicyService(p.RequestHeadersPolicy, nil, true, dynamicHeaders)
	if err != nil {
		return nil, err
	}
	respHP, err := headersPolicyService(p.ResponseHeadersPolicy, nil, false, dynamicHeaders)
	if err != nil {
		return nil, err
	}

	r := &Route{
		HTTPSUpgrade:  annotation.TLSRequired(ingress),
		Websocket:     annotation.WebsocketRoutes(ingress)[path],
		TimeoutPolicy: ingressTimeoutPolicy(ingress, log),
		RetryPolicy:   ingressRetryPolicy(ingress, log),
		Clusters: []*Cluster{{
			Upstream:              service,
			Protocol:              service.Protocol,
			ClientCertificate:     clientCertSecret,
			RequestHeadersPolicy:  reqHP,
			ResponseHeadersPolicy: respHP,
			TimeoutPolicy:         ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
		}},
	}

	switch pathType {
	case networking_v1.PathTypePrefix:
		prefixMatchType := PrefixMatchSegment
		// An "all paths" prefix should be treated as a generic string prefix
		// match.
		if path == "/" {
			prefixMatchType = PrefixMatchString
		} else {
			// Strip trailing slashes. Ensures /foo matches prefix /foo/
			path = strings.TrimRight(path, "/")
		}
		r.PathMatchCondition = &PrefixMatchCondition{Prefix: path, PrefixMatchType: prefixMatchType}
	case networking_v1.PathTypeExact:
		r.PathMatchCondition = &ExactMatchCondition{Path: path}
	case networking_v1.PathTypeImplementationSpecific:
		// If a path "looks like" a regex we give a regex path match.
		// Otherwise you get a string prefix match.
		if strings.ContainsAny(path, "^+*[]%") {
			// validate the regex
			if err := ValidateRegex(path); err != nil {
				return nil, err
			}
			r.PathMatchCondition = &RegexMatchCondition{Regex: path}
		} else {
			r.PathMatchCondition = &PrefixMatchCondition{Prefix: path, PrefixMatchType: PrefixMatchString}
		}
	}

	// If we have a wildcard match, add a header match regex rule to match the
	// hostname so we can be sure to only match one DNS label. This is required
	// as Envoy's virtualhost hostname wildcard matching can match multiple
	// labels. This match ignores a port in the hostname in case it is present.
	if strings.HasPrefix(host, "*.") {
		r.HeaderMatchConditions = append(r.HeaderMatchConditions, wildcardDomainHeaderMatch(host))
	}

	return r, nil
}

// rulesFromSpec merges the IngressSpec's Rules with a synthetic
// rule representing the default backend.
// Prepend the default backend so it can be overridden by later rules.
func rulesFromSpec(spec networking_v1.IngressSpec) []networking_v1.IngressRule {
	rules := spec.Rules
	if backend := spec.DefaultBackend; backend != nil {
		rule := defaultBackendRule(backend)
		rules = append([]networking_v1.IngressRule{rule}, rules...)
	}
	return rules
}

// defaultBackendRule returns an IngressRule that represents the IngressBackend.
func defaultBackendRule(be *networking_v1.IngressBackend) networking_v1.IngressRule {
	return networking_v1.IngressRule{
		IngressRuleValue: networking_v1.IngressRuleValue{
			HTTP: &networking_v1.HTTPIngressRuleValue{
				Paths: []networking_v1.HTTPIngressPath{{
					Backend: networking_v1.IngressBackend{
						Service: &networking_v1.IngressServiceBackend{
							Name: be.Service.Name,
							Port: be.Service.Port,
						},
					},
				}},
			},
		},
	}
}

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// httppaths returns a slice of HTTPIngressPath values for a given IngressRule.
// In the case that the IngressRule contains no valid HTTPIngressPaths, a
// nil slice is returned.
func httppaths(rule networking_v1.IngressRule) []networking_v1.HTTPIngressPath {
	if rule.IngressRuleValue.HTTP == nil {
		// rule.IngressRuleValue.HTTP value is optional.
		return nil
	}
	return rule.IngressRuleValue.HTTP.Paths
}
