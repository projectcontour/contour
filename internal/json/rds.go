// Copyright © 2017 Heptio
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

package json

import (
	"strings"

	"github.com/heptio/contour/internal/envoy"
	"github.com/pkg/errors"
	"k8s.io/api/extensions/v1beta1"
)

// IngressToVirtualHosts translates an Ingress to a slice of *envoy.VirtualHost.
func IngressToVirtualHosts(i *v1beta1.Ingress) ([]*envoy.VirtualHost, error) {
	if err := validateIngress(i); err != nil {
		return nil, err
	}

	class, ok := i.Annotations["kubernetes.io/ingress.class"]
	if ok && class != "contour" {
		// if there is an ingress class set, but it is not set to "contour"
		// ignore this ingress.
		// TODO(dfc) we should also skip creating any cluster backends,
		// but this is hard to do at the moment because cds and rds are
		// independent.
		return nil, nil
	}

	// an Ingress may refer to a default backend, or a set of rules
	// validateIngress ensures this.
	if i.Spec.Backend != nil {
		v := envoy.VirtualHost{
			Name: hashname(60, i.Namespace, i.Name),
		}
		v.AddDomain("*")
		v.AddRoute(envoy.Route{
			Prefix:  "/", // match all
			Cluster: ingressBackendToClusterName(i, i.Spec.Backend),
		})
		return []*envoy.VirtualHost{&v}, nil
	}
	var vhosts []*envoy.VirtualHost
	for _, rule := range i.Spec.Rules {
		var v envoy.VirtualHost
		switch rule.Host {
		case "":
			// quothe the spec,
			// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
			v.Name = hashname(60, i.Namespace, i.Name)
			v.AddDomain("*")
		default:
			v.Name = hashname(60, i.Namespace, i.Name, rule.Host)
			v.AddDomain(rule.Host)
		}
		if rule.IngressRuleValue.HTTP == nil {
			return nil, errors.Errorf("ingress %s/%s: Ingress.Spec.Rules[0].IngressRuleValue.HTTP is nil", i.ObjectMeta.Namespace, i.ObjectMeta.Name)
		}
		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			r := pathToRoute(p)
			r.Cluster = ingressBackendToClusterName(i, &p.Backend)
			v.AddRoute(r)
		}
		vhosts = append(vhosts, &v)
	}
	return vhosts, nil
}

// validateIngress asserts that the required fields in e are present.
// Fields which are required for conversion must be present or an error is returned.
// For the fields that are converted, if Envoy places a limit on their contents or length,
// and error is returned if those fields are invalid.
// Many fields in *v1beta1.Ingress which are not needed for conversion and are ignored.
func validateIngress(i *v1beta1.Ingress) error {
	if i.ObjectMeta.Name == "" {
		return errors.New("Ingress.Meta.Name is blank")
	}
	if i.ObjectMeta.Namespace == "" {
		return errors.New("Ingress.Meta.Namespace is blank")
	}
	if i.Spec.Backend == nil && len(i.Spec.Rules) == 0 {
		return errors.New("Ingress.Spec.Backend and Ingress.Spec.Rules is blank")
	}
	if i.Spec.Backend != nil && len(i.Spec.Rules) > 0 {
		return errors.New("Only one of Ingress.Spec.Backend or Ingress.Spec.Rules permitted")
	}

	// TODO(dfc) need to validate Backend.ServiceName can be expressed as an envoy cluster; 60 chars or less
	// some restrictions on valid characters.
	return nil
}

// ingressBackendToClusterName renders a cluster name from an Ingress and an IngressBackend.
func ingressBackendToClusterName(i *v1beta1.Ingress, b *v1beta1.IngressBackend) string {
	return hashname(60, i.ObjectMeta.Namespace, b.ServiceName, b.ServicePort.String())
}

// pathToRoute converts a HTTPIngressPath to a partial envoy.Route.
func pathToRoute(p v1beta1.HTTPIngressPath) envoy.Route {
	if p.Path == "" {
		// If the Path is empty, the k8s spec says
		// "If unspecified, the path defaults to a catch all sending
		// traffic to the backend."
		// We map this it a catch all prefix route.A
		return envoy.Route{
			Prefix: "/",
		}
	}
	// TODO(dfc) handle the case where p.Path does not start with "/"
	if strings.IndexAny(p.Path, `[(*\`) == -1 {
		// Envoy requires that regex matches match completely, wheres the
		// HTTPIngressPath.Path regex only requires a partial match. eg,
		// "/foo" matches "/" according to k8s rules, but does not match
		// according to Envoy.
		// To deal with this we handle the simple case, a Path without regex
		// characters as a Envoy prefix route.
		return envoy.Route{
			Prefix: p.Path,
		}
	}
	// At this point the path is a regex, which we hope is the same between k8s
	// IEEE 1003.1 POSIX regex, and Envoys Javascript regex.
	return envoy.Route{
		Regex: p.Path,
	}
}
