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
	"net/http"
	"strings"

	"github.com/projectcontour/contour/internal/status"
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

// matchConditions holds match rules.
type matchConditions struct {
	pathMatchConditions  []MatchCondition
	headerMatchCondition []HeaderMatchCondition
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

	for _, listener := range p.source.gateway.Spec.Listeners {

		var matchingRoutes []*gatewayapi_v1alpha1.HTTPRoute
		var listenerSecret *Secret

		// Check for TLS on the Gateway.
		if listener.TLS != nil {
			if listenerSecret = p.validGatewayTLS(listener); listenerSecret == nil {
				// If TLS was configured on the Listener, but it's invalid, don't allow any
				// routes to be bound to this listener since it can't serve TLS traffic.
				continue
			}
		}

		// Validate the Group on the selector is a supported type.
		if listener.Routes.Group != "" && listener.Routes.Group != gatewayapi_v1alpha1.GroupName {
			p.Errorf("Listener.Routes.Group %q is not supported.", listener.Routes.Group)
			continue
		}

		// Validate the Kind on the selector is a supported type.
		if listener.Routes.Kind != KindHTTPRoute {
			p.Errorf("Listener.Routes.Kind %q is not supported.", listener.Routes.Kind)
			continue
		}

		for _, route := range p.source.httproutes {

			// Filter the HTTPRoutes that match the gateway which Contour is configured to watch.
			// RouteBindingSelector defines a schema for associating routes with the Gateway.
			// If Namespaces and Selector are defined, only routes matching both selectors are associated with the Gateway.

			// ## RouteBindingSelector ##
			//
			// Selector specifies a set of route labels used for selecting routes to associate
			// with the Gateway. If this Selector is defined, only routes matching the Selector
			// are associated with the Gateway. An empty Selector matches all routes.

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
			}
		}

		// Process all the routes that match this Gateway.
		for _, matchingRoute := range matchingRoutes {
			p.computeHTTPRoute(matchingRoute, listenerSecret)
		}
	}
}

func (p *GatewayAPIProcessor) validGatewayTLS(listener gatewayapi_v1alpha1.Listener) *Secret {

	// Validate the CertificateRef is configured.
	if listener.TLS.CertificateRef == nil {
		p.Errorf("Spec.VirtualHost.TLS.CertificateRef is not configured.")
		return nil
	}

	// Validate the correct protocol is specified.
	if listener.Protocol != gatewayapi_v1alpha1.HTTPSProtocolType {
		p.Errorf("Spec.VirtualHost.Protocol %q is not valid.", listener.Protocol)
		return nil
	}

	// Validate a v1.Secret is referenced which can be kind: secret & group: core.
	// ref: https://github.com/kubernetes-sigs/gateway-api/pull/562
	if !isSecretRef(listener.TLS.CertificateRef) {
		p.Error("Spec.VirtualHost.TLS Secret must be type core.Secret")
		return nil
	}

	listenerSecret, err := p.source.LookupSecret(types.NamespacedName{Name: listener.TLS.CertificateRef.Name, Namespace: p.source.gateway.Namespace}, validSecret)
	if err != nil {
		p.Errorf("Spec.VirtualHost.TLS Secret %q is invalid: %s", listener.TLS.CertificateRef.Name, err)
		return nil
	}
	return listenerSecret
}

func isSecretRef(certificateRef *gatewayapi_v1alpha1.LocalObjectReference) bool {
	return strings.ToLower(certificateRef.Kind) == "secret" && strings.ToLower(certificateRef.Group) == "core"
}

func (p *GatewayAPIProcessor) computeHosts(route *gatewayapi_v1alpha1.HTTPRoute) ([]string, []error) {
	// Determine the hosts on the route, if no hosts
	// are defined, then set to "*".
	var hosts []string
	var errors []error
	if len(route.Spec.Hostnames) == 0 {
		hosts = append(hosts, "*")
		return hosts, nil
	}

	for _, host := range route.Spec.Hostnames {

		hostname := string(host)
		if isIP := net.ParseIP(hostname) != nil; isIP {
			errors = append(errors, fmt.Errorf("hostname %q must be a DNS name, not an IP address", hostname))
			continue
		}
		if strings.Contains(hostname, "*") {
			if errs := validation.IsWildcardDNS1123Subdomain(hostname); errs != nil {
				errors = append(errors, fmt.Errorf("invalid hostname %q: %v", hostname, errs))
				continue
			}
		} else {
			if errs := validation.IsDNS1123Subdomain(hostname); errs != nil {
				errors = append(errors, fmt.Errorf("invalid listener hostname %q: %v", hostname, errs))
				continue
			}
		}
		hosts = append(hosts, string(host))
	}
	return hosts, errors
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

func (p *GatewayAPIProcessor) computeHTTPRoute(route *gatewayapi_v1alpha1.HTTPRoute, listenerSecret *Secret) {
	routeAccessor, commit := p.dag.StatusCache.HTTPRouteAccessor(route)
	defer commit()

	hosts, errs := p.computeHosts(route)
	for _, err := range errs {
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
	}

	// Check if all the hostnames are invalid.
	if len(hosts) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
		return
	}

	// Validate TLS Configuration
	if route.Spec.TLS != nil {
		routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonNotImplemented, "HTTPRoute.Spec.TLS: Not yet implemented.")
	}

	for _, rule := range route.Spec.Rules {

		matchconditions := []*matchConditions{}

		for _, match := range rule.Matches {
			mc := &matchConditions{}
			// TODO: Replace this with nil check when gateway-api includes this fix -
			// https://github.com/kubernetes-sigs/gateway-api/commit/9d63656df8cc9e67da60d4fe3f6289aad22e72d3
			if match.Path.Value == "" || match.Path.Type == "" {
				match.Path = gatewayapi_v1alpha1.HTTPPathMatch{Type: gatewayapi_v1alpha1.PathMatchPrefix, Value: "/"}
			}
			switch match.Path.Type {
			case gatewayapi_v1alpha1.PathMatchPrefix:
				mc.pathMatchConditions = append(mc.pathMatchConditions, &PrefixMatchCondition{Prefix: stringOrDefault(match.Path.Value, "/")})
			case gatewayapi_v1alpha1.PathMatchExact:
				mc.pathMatchConditions = append(mc.pathMatchConditions, &ExactMatchCondition{Path: stringOrDefault(match.Path.Value, "/")})
			default:
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonPathMatchType, "HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported.")
			}

			if match.Headers != nil {
				switch match.Headers.Type {
				case gatewayapi_v1alpha1.HeaderMatchExact:
					for k, v := range match.Headers.Values {
						mc.headerMatchCondition = append(mc.headerMatchCondition, HeaderMatchCondition{MatchType: HeaderMatchTypeExact, Name: k, Value: v})
					}
				default:
					routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHeaderMatchType, "HTTPRoute.Spec.Rules.HeaderMatch: Only Exact match type is supported.")
				}
			}
			matchconditions = append(matchconditions, mc)
		}

		if len(rule.ForwardTo) == 0 {
			routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.ForwardTo must be specified.")
			continue
		}

		var clusters []*Cluster

		// Validate the ForwardTos.
		for _, forward := range rule.ForwardTo {

			// Verify the service is valid
			if forward.ServiceName == nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "Spec.Rules.ForwardTo.ServiceName must be specified.")
				continue
			}

			// TODO: Do not require port to be present (#3352).
			if forward.Port == nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "Spec.Rules.ForwardTo.ServicePort must be specified.")
				continue
			}

			meta := types.NamespacedName{Name: *forward.ServiceName, Namespace: route.Namespace}

			// TODO: Refactor EnsureService to take an int32 so conversion to intstr is not needed.
			service, err := p.dag.EnsureService(meta, intstr.FromInt(int(*forward.Port)), p.source)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("Service %q does not exist in namespace %q", meta.Name, meta.Namespace))
				continue
			}

			var headerPolicy *HeadersPolicy
			for _, filter := range forward.Filters {
				switch filter.Type {
				case gatewayapi_v1alpha1.HTTPRouteFilterRequestHeaderModifier:
					var err error
					headerPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier)
					if err != nil {
						routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
					}
				default:
					routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHTTPRouteFilterType, "HTTPRoute.Spec.Rules.ForwardTo.Filters: Only RequestHeaderModifier type is supported.")
				}
			}

			cluster := p.cluster(headerPolicy, service)
			clusters = append(clusters, cluster)
		}

		var headerPolicy *HeadersPolicy
		for _, filter := range rule.Filters {
			switch filter.Type {
			case gatewayapi_v1alpha1.HTTPRouteFilterRequestHeaderModifier:
				var err error
				headerPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier)
				if err != nil {
					routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			default:
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHTTPRouteFilterType, "HTTPRoute.Spec.Rules.Filters: Only RequestHeaderModifier type is supported.")
			}
		}

		routes := p.routes(matchconditions, headerPolicy, clusters)
		for _, host := range hosts {
			for _, route := range routes {

				if len(clusters) == 0 {
					// Configure a direct response HTTP status code of 503 so the
					// route still matches the configured conditions since the
					// service is missing or invalid.
					route.DirectResponse = &DirectResponse{
						StatusCode: http.StatusServiceUnavailable,
					}
				}

				if listenerSecret != nil {
					svhost := p.dag.EnsureSecureVirtualHost(ListenerName{Name: host, ListenerName: "ingress_https"})
					svhost.Secret = listenerSecret
					svhost.addRoute(route)
				} else {
					vhost := p.dag.EnsureVirtualHost(ListenerName{Name: host, ListenerName: "ingress_http"})
					vhost.addRoute(route)
				}
			}
		}
	}

	// Determine if any errors exist in conditions and set the "Admitted"
	// condition accordingly.
	switch len(routeAccessor.Conditions) {
	case 0:
		routeAccessor.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionTrue, status.ReasonValid, "Valid HTTPRoute")
	default:
		routeAccessor.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
	}
}

// routes builds a []*dag.Route for the supplied set of matchConditions, headerPolicy and clusters.
func (p *GatewayAPIProcessor) routes(matchConditions []*matchConditions, headerPolicy *HeadersPolicy, clusters []*Cluster) []*Route {
	var routes []*Route

	for _, mc := range matchConditions {
		for _, pathMatch := range mc.pathMatchConditions {
			r := &Route{
				Clusters: clusters,
			}
			r.PathMatchCondition = pathMatch
			r.HeaderMatchConditions = mc.headerMatchCondition
			r.RequestHeadersPolicy = headerPolicy
			routes = append(routes, r)
		}
	}

	return routes
}

// cluster builds a *dag.Cluster for the supplied set of headerPolicy and service.
func (p *GatewayAPIProcessor) cluster(headerPolicy *HeadersPolicy, service *Service) *Cluster {
	return &Cluster{
		Upstream:             service,
		Protocol:             service.Protocol,
		RequestHeadersPolicy: headerPolicy,
	}
}
