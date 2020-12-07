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
	"strings"

	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	"k8s.io/api/networking/v1beta1"
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
			secretName := k8s.NamespacedNameFrom(tls.SecretName, k8s.DefaultNamespace(ing.GetNamespace()))
			sec, err := p.source.LookupSecret(secretName, validSecret)
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
				svhost := p.dag.EnsureSecureVirtualHost(host)
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

func (p *IngressProcessor) computeIngressRule(ing *v1beta1.Ingress, rule v1beta1.IngressRule) {
	host := rule.Host
	if strings.Contains(host, "*") {
		// reject hosts with wildcard characters.
		return
	}
	if host == "" {
		// if host name is blank, rewrite to Envoy's * default host.
		host = "*"
	}

	var clientCertSecret *Secret
	var err error
	if p.ClientCertificate != nil {
		clientCertSecret, err = p.source.LookupSecret(*p.ClientCertificate, validSecret)
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
		be := httppath.Backend
		m := types.NamespacedName{Name: be.ServiceName, Namespace: ing.Namespace}
		s, err := p.dag.EnsureService(m, be.ServicePort, p.source)
		if err != nil {
			continue
		}

		r, err := route(ing, path, s, clientCertSecret, p.FieldLogger)
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
			vhost := p.dag.EnsureVirtualHost(host)
			vhost.addRoute(r)
		}

		// computeSecureVirtualhosts will have populated b.securevirtualhosts
		// with the names of tls enabled ingress objects. If host exists then
		// it is correctly configured for TLS.
		if svh := p.dag.GetSecureVirtualHost(host); svh != nil && host != "*" {
			svh.addRoute(r)
		}
	}
}

// route builds a dag.Route for the supplied Ingress.
func route(ingress *v1beta1.Ingress, path string, service *Service, clientCertSecret *Secret, log logrus.FieldLogger) (*Route, error) {
	log = log.WithFields(logrus.Fields{
		"name":      ingress.Name,
		"namespace": ingress.Namespace,
	})

	r := &Route{
		HTTPSUpgrade:  annotation.TLSRequired(ingress),
		Websocket:     annotation.WebsocketRoutes(ingress)[path],
		TimeoutPolicy: ingressTimeoutPolicy(ingress, log),
		RetryPolicy:   ingressRetryPolicy(ingress, log),
		Clusters: []*Cluster{{
			Upstream:          service,
			Protocol:          service.Protocol,
			ClientCertificate: clientCertSecret,
		}},
	}

	if strings.ContainsAny(path, "^+*[]%") {
		// validate the regex
		if err := ValidateRegex(path); err != nil {
			return nil, err
		}

		r.PathMatchCondition = &RegexMatchCondition{Regex: path}
		return r, nil
	}

	r.PathMatchCondition = &PrefixMatchCondition{Prefix: path}
	return r, nil
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

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
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
