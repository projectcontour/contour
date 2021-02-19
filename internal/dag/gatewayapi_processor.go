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
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// GatewayAPIProcessor translates Gateway API types into DAG
// objects and adds them to the DAG.
type GatewayAPIProcessor struct {
	logrus.FieldLogger

	dag    *DAG
	source *KubernetesCache
}

// Run translates Service APIs into DAG objects and
// adds them to the DAG.
func (p *GatewayAPIProcessor) Run(dag *DAG, source *KubernetesCache) {
	p.dag = dag
	p.source = source

	// reset the processor when we're done
	defer func() {
		p.dag = nil
		p.source = nil
	}()

	for _, route := range p.source.httproutes {
		p.computeHTTPRoute(route)
	}
}

func (p *GatewayAPIProcessor) computeHTTPRoute(route *gatewayapi_v1alpha1.HTTPRoute) {

	// Validate TLS Configuration
	if route.Spec.TLS != nil {
		p.Error("NOT IMPLEMENTED: The 'RouteTLSConfig' is not yet implemented.")
	}

	// Determine the hosts on the route, if no hosts
	// are defined, then set to "*".
	var hosts []string
	if len(route.Spec.Hostnames) == 0 {
		hosts = append(hosts, "*")
	} else {
		for _, host := range route.Spec.Hostnames {
			hosts = append(hosts, string(host))
		}
	}

	for _, rule := range route.Spec.Rules {

		var pathPrefixes []string
		var services []*Service

		for _, match := range rule.Matches {
			switch match.Path.Type {
			case gatewayapi_v1alpha1.PathMatchPrefix:
				pathPrefixes = append(pathPrefixes, stringOrDefault(match.Path.Value, "/"))
			default:
				p.Error("NOT IMPLEMENTED: Only PathMatchPrefix is currently implemented.")
			}
		}

		// Validate the ForwardTos.
		var forwardTos []gatewayapi_v1alpha1.HTTPRouteForwardTo
		for _, forward := range rule.ForwardTo {
			// Verify the service is valid
			if forward.ServiceName == nil {
				p.Error("ServiceName must be specified and is currently only type implemented!")
				continue
			}

			// TODO: Do not require port to be present (#3352).
			if forward.Port == nil {
				p.Error("ServicePort must be specified.")
				continue
			}
			forwardTos = append(forwardTos, forward)
		}

		// Process any valid forwardTo.
		for _, forward := range forwardTos {

			meta := types.NamespacedName{Name: *forward.ServiceName, Namespace: route.Namespace}

			// TODO: Refactor EnsureService to take an int32 so conversion to intstr is not needed.
			service, err := p.dag.EnsureService(meta, intstr.FromInt(int(*forward.Port)), p.source)
			if err != nil {
				// TODO: Raise `ResolvedRefs` condition on Gateway with `DegradedRoutes` reason.
				p.Errorf("Service %q does not exist in namespace %q", meta.Name, meta.Namespace)
				return
			}
			services = append(services, service)
		}

		if len(services) == 0 {
			p.Errorf("Route %q rule invalid due to invalid forwardTo configuration.", route.Name)
			continue
		}

		routes := p.routes(pathPrefixes, services)
		for _, vhost := range hosts {
			vhost := p.dag.EnsureVirtualHost(vhost)
			for _, route := range routes {
				vhost.addRoute(route)
			}
		}
	}
}

// routes builds a []*dag.Route for the supplied set of pathPrefixes & services.
func (p *GatewayAPIProcessor) routes(pathPrefixes []string, services []*Service) []*Route {
	var clusters []*Cluster
	var routes []*Route

	for _, service := range services {
		clusters = append(clusters, &Cluster{
			Upstream: service,
			Protocol: service.Protocol,
		})
	}

	for _, prefix := range pathPrefixes {
		r := &Route{
			Clusters: clusters,
		}
		r.PathMatchCondition = &PrefixMatchCondition{Prefix: prefix}
		routes = append(routes, r)
	}

	return routes
}
