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
	"fmt"
	"net"
	"strings"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	KindHTTPRoute = "HTTPRoute"
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

	// Gateway must be defined for resources to be processed.
	if p.source.gateway == nil {
		p.Error("Gateway is not defined!")
		return
	}

	var matchingRoutes []*gatewayapi_v1alpha1.HTTPRoute

	for _, route := range p.source.httproutes {

		// Filter the HTTPRoutes that match the gateway which Contour is configured to watch.
		// RouteBindingSelector defines a schema for associating routes with the Gateway.
		// If Namespaces and Selector are defined, only routes matching both selectors are associated with the Gateway.

		// ## RouteBindingSelector ##
		//
		// Selector specifies a set of route labels used for selecting routes to associate
		// with the Gateway. If this Selector is defined, only routes matching the Selector
		// are associated with the Gateway. An empty Selector matches all routes.
		for _, listener := range p.source.gateway.Spec.Listeners {

			// Validate the Group on the selector is a supported type.
			if listener.Routes.Group != "" && listener.Routes.Group != "networking.x-k8s.io" {
				// TODO: Set the “ResolvedRefs” condition to false for this listener with the “InvalidRoutesRef” reason.
				p.Errorf("Listener.Routes.Group %q is not supported.", listener.Routes.Kind)
				continue
			}

			// Validate the Kind on the selector is a supported type.
			if listener.Routes.Kind != KindHTTPRoute {
				// TODO: Set the “ResolvedRefs” condition to false for this listener with the “InvalidRoutesRef” reason.
				p.Errorf("Listener.Routes.Kind %q is not supported.", listener.Routes.Kind)
			}

			nsMatches, err := p.namespaceMatches(listener.Routes.Namespaces, route)
			if err != nil {
				p.Errorf("error validating namespaces against Listener.Routes.Namespaces: %s", err)
			}

			selMatches, err := selectorMatches(listener.Routes.Selector, route.Labels)
			if err != nil {
				p.Errorf("error validating routes against Listener.Routes.Selector: %s", err)
			}

			if selMatches && nsMatches {
				// Empty Selector matches all routes.
				matchingRoutes = append(matchingRoutes, route)
				break
			}
		}
	}

	// Process all the routes that match this Gateway.
	for _, matchingRoute := range matchingRoutes {
		p.computeHTTPRoute(matchingRoute)
	}

}

// namespaceMatches returns true if the namespaces selector matches
// the HTTPRoute that is being processed.
func (p *GatewayAPIProcessor) namespaceMatches(namespaces gatewayapi_v1alpha1.RouteNamespaces, route *gatewayapi_v1alpha1.HTTPRoute) (bool, error) {
	// From indicates where Routes will be selected for this Gateway.
	// Possible values are:
	//   * All: Routes in all namespaces may be used by this Gateway.
	//   * Selector: Routes in namespaces selected by the selector may be used by
	//     this Gateway.
	//   * Same: Only Routes in the same namespace may be used by this Gateway.

	switch namespaces.From {
	case gatewayapi_v1alpha1.RouteSelectAll:
		return true, nil
	case gatewayapi_v1alpha1.RouteSelectSame:
		return p.source.ConfiguredGateway.Namespace == route.Namespace, nil
	case gatewayapi_v1alpha1.RouteSelectSelector:
		if len(namespaces.Selector.MatchLabels) == 0 || len(namespaces.Selector.MatchExpressions) == 0 {
			return false, fmt.Errorf("RouteNamespaces selector must be specified when `RouteSelectType=Selector`")
		}

		// Look up the HTTPRoute's namespace in the list of cached namespaces.
		if ns := p.source.namespaces[route.Namespace]; ns != nil {

			// Check that the route's namespace is included in the Gateway's
			// namespace selector/expression.
			l, err := metav1.LabelSelectorAsSelector(&namespaces.Selector)
			if err != nil {
				return false, err
			}

			// Look for matching labels on Selector.
			return l.Matches(labels.Set(ns.Labels)), nil
		}
	}
	return true, nil
}

// selectorMatches returns true if the selector matches the labels on the object or is not defined.
func selectorMatches(selector metav1.LabelSelector, objLabels map[string]string) (bool, error) {

	// If a selector is defined then check that it matches the labels on the object.
	if len(selector.MatchLabels) > 0 || len(selector.MatchExpressions) > 0 {
		l, err := metav1.LabelSelectorAsSelector(&selector)
		if err != nil {
			return false, err
		}

		// Look for matching labels on Selector.
		return l.Matches(labels.Set(objLabels)), nil
	}
	// If no selector is defined then it matches by default.
	return true, nil
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

			hostname := string(host)
			if isIP := net.ParseIP(hostname) != nil; isIP {
				p.Errorf("hostname %q must be a DNS name, not an IP address", hostname)
				continue
			}
			if strings.Contains(hostname, "*") {

				if errs := validation.IsWildcardDNS1123Subdomain(hostname); errs != nil {
					p.Errorf("invalid hostname %q: %v", hostname, errs)
					continue
				}
			} else {
				if errs := validation.IsDNS1123Subdomain(hostname); errs != nil {
					p.Errorf("invalid listener hostname %q", hostname, errs)
					continue
				}
			}
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
