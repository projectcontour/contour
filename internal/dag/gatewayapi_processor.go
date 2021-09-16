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
	"regexp"
	"strings"

	"github.com/projectcontour/contour/internal/errors"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/status"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	KindHTTPRoute = "HTTPRoute"
	KindTLSRoute  = "TLSRoute"
)

// GatewayAPIProcessor translates Gateway API types into DAG
// objects and adds them to the DAG.
type GatewayAPIProcessor struct {
	logrus.FieldLogger

	dag    *DAG
	source *KubernetesCache

	// EnableExternalNameService allows processing of ExternalNameServices
	// This is normally disabled for security reasons.
	// See https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc for details.
	EnableExternalNameService bool
}

// matchConditions holds match rules.
type matchConditions struct {
	pathMatchConditions  []MatchCondition
	headerMatchCondition []HeaderMatchCondition
}

// Run translates Service APIs into DAG objects and
// adds them to the DAG.
func (p *GatewayAPIProcessor) Run(dag *DAG, source *KubernetesCache) {
	var gatewayErrors field.ErrorList
	path := field.NewPath("spec")

	p.dag = dag
	p.source = source

	// reset the processor when we're done
	defer func() {
		p.dag = nil
		p.source = nil
	}()

	// Gateway and GatewayClass must be defined for resources to be processed.
	if p.source.gateway == nil {
		p.Info("Gateway not found in cache.")
		return
	}
	if p.source.gatewayclass == nil {
		p.Info("Gatewayclass not found in cache.")
		return
	}

	if len(p.source.gateway.Spec.Addresses) > 0 {
		gatewayErrors = append(gatewayErrors, &field.Error{Type: field.ErrorTypeNotSupported, Field: path.String(), BadValue: p.source.gateway.Spec.Addresses, Detail: "Spec.Addresses is not supported"})
	}

	for _, listener := range p.source.gateway.Spec.Listeners {

		var matchingHTTPRoutes []*gatewayapi_v1alpha2.HTTPRoute
		var matchingTLSRoutes []*gatewayapi_v1alpha2.TLSRoute
		var listenerSecret *Secret

		// Validate the listener protocol is a supported type.
		switch listener.Protocol {
		case gatewayapi_v1alpha2.HTTPSProtocolType:
			// Validate that if protocol is type HTTPS, that TLS is defined.
			if listener.TLS == nil {
				p.Errorf("Listener.TLS is required when protocol is %q.", listener.Protocol)
				continue
			}

			// Check for TLS on the Gateway.
			if listenerSecret = p.validGatewayTLS(listener); listenerSecret == nil {
				// If TLS was configured on the Listener, but it's invalid, don't allow any
				// routes to be bound to this listener since it can't serve TLS traffic.
				continue
			}
		case gatewayapi_v1alpha2.TLSProtocolType:

			// TLS is required for the type TLS.
			if listener.TLS == nil {
				p.Errorf("Listener.TLS is required when protocol is %q.", listener.Protocol)
				continue
			}

			if listener.TLS.Mode != nil {
				switch *listener.TLS.Mode {
				case gatewayapi_v1alpha2.TLSModeTerminate:
					// Check for TLS on the Gateway.
					if listenerSecret = p.validGatewayTLS(listener); listenerSecret == nil {
						// If TLS was configured on the Listener, but it's invalid, don't allow any
						// routes to be bound to this listener since it can't serve TLS traffic.
						continue
					}
				case gatewayapi_v1alpha2.TLSModePassthrough:
					if listener.TLS.CertificateRef != nil {
						p.Errorf("Listener.TLS.CertificateRef cannot be defined when TLS Mode is %q.", *listener.TLS.Mode)
						continue
					}
				}
			}
		case gatewayapi_v1alpha2.HTTPProtocolType:
			break
		default:
			p.Errorf("Listener.Protocol %q is not supported.", listener.Protocol)
			continue
		}

		// Get a list of the kinds of routes that the listener accepts.
		routeKinds, err := getListenerRouteKinds(listener)
		if err != nil {
			p.Errorf("error getting listener route kinds: %v", err)
			continue
		}

		for _, routeKind := range routeKinds {
			switch routeKind {
			case KindHTTPRoute:
				for _, route := range p.source.httproutes {
					// Check if the route is in a namespace that the listener allows.
					nsMatches, err := p.namespaceMatches(listener.AllowedRoutes.Namespaces, route.Namespace)
					if err != nil {
						p.Errorf("error validating namespaces against Listener.Routes.Namespaces: %s", err)
					}
					if !nsMatches {
						continue
					}

					// If the Gateway selects the HTTPRoute, check to see if the HTTPRoute selects
					// the Gateway/listener.
					if !routeSelectsGatewayListener(p.source.gateway, listener, route.Spec.ParentRefs, route.Namespace) {
						continue
					}

					matchingHTTPRoutes = append(matchingHTTPRoutes, route)
				}
			case KindTLSRoute:

				// Validate the listener protocol is type=TLS.
				if listener.Protocol != gatewayapi_v1alpha2.TLSProtocolType {
					p.Errorf("invalid listener protocol %q for Kind: TLSRoute", listener.Protocol)
					continue
				}

				for _, route := range p.source.tlsroutes {
					// Check if the route is in a namespace that the listener allows.
					nsMatches, err := p.namespaceMatches(listener.AllowedRoutes.Namespaces, route.Namespace)
					if err != nil {
						p.Errorf("error validating namespaces against Listener.Routes.Namespaces: %s", err)
					}
					if !nsMatches {
						continue
					}

					// If the Gateway selects the TLSRoute, check to see if the TLSRoute selects
					// the Gateway/listener.
					if !routeSelectsGatewayListener(p.source.gateway, listener, route.Spec.ParentRefs, route.Namespace) {
						continue
					}

					matchingTLSRoutes = append(matchingTLSRoutes, route)
				}
			}
		}

		validGateway := len(gatewayErrors) == 0

		// Process all the HTTPRoutes that match this Gateway.
		for _, matchingRoute := range matchingHTTPRoutes {
			p.computeHTTPRoute(matchingRoute, listenerSecret, listener.Hostname, validGateway)
		}

		// Process all the TLSRoutes that match this Gateway.
		for _, matchingRoute := range matchingTLSRoutes {
			p.computeTLSRoute(matchingRoute, validGateway, listenerSecret, listener.Hostname)
		}
	}

	p.computeGatewayConditions(p.source.gateway, gatewayErrors)
}

// getListenerRouteKinds gets a list of the kinds of routes that
// the listener accepts.
func getListenerRouteKinds(listener gatewayapi_v1alpha2.Listener) ([]string, error) {
	if len(listener.AllowedRoutes.Kinds) == 0 {
		switch listener.Protocol {
		case gatewayapi_v1alpha2.HTTPProtocolType:
			return []string{KindHTTPRoute}, nil
		case gatewayapi_v1alpha2.HTTPSProtocolType:
			return []string{KindHTTPRoute}, nil
		case gatewayapi_v1alpha2.TLSProtocolType:
			return []string{KindTLSRoute}, nil
		}
	}

	var routeKinds []string

	for _, routeKind := range listener.AllowedRoutes.Kinds {
		if routeKind.Group == nil {
			return nil, fmt.Errorf("Listener.AllowedRoutes.Group not specified")
		}
		if *routeKind.Group != gatewayapi_v1alpha2.GroupName {
			return nil, fmt.Errorf("Listener.AllowedRoutes.Group %q not supported", *routeKind.Group)
		}
		if routeKind.Kind != gatewayapi_v1alpha2.Kind(KindHTTPRoute) && routeKind.Kind != gatewayapi_v1alpha2.Kind(KindTLSRoute) {
			return nil, fmt.Errorf("Listener.AllowedRoutes.Kind %q not supported", routeKind.Kind)
		}

		routeKinds = append(routeKinds, string(routeKind.Kind))
	}

	return routeKinds, nil
}

func (p *GatewayAPIProcessor) validGatewayTLS(listener gatewayapi_v1alpha2.Listener) *Secret {

	// Validate the CertificateRef is configured.
	if listener.TLS == nil || listener.TLS.CertificateRef == nil {
		p.Errorf("Spec.VirtualHost.TLS.CertificateRef is not configured.")
		return nil
	}

	// Validate a v1.Secret is referenced which can be kind: secret & group: core.
	// ref: https://github.com/kubernetes-sigs/gateway-api/pull/562
	if !isSecretRef(*listener.TLS.CertificateRef) {
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

func isSecretRef(certificateRef gatewayapi_v1alpha2.SecretObjectReference) bool {
	return certificateRef.Group != nil && (*certificateRef.Group == "" || *certificateRef.Group == "core") &&
		certificateRef.Kind != nil && *certificateRef.Kind == "Secret"
}

// computeHosts validates the hostnames for a HTTPRoute as well as validating
// that the hostname on the HTTPRoute matches what is optionally defined on the
// listener.hostname.
func (p *GatewayAPIProcessor) computeHosts(hostnames []gatewayapi_v1alpha2.Hostname, listenerHostname *gatewayapi_v1alpha2.Hostname) (map[string]struct{}, []error) {

	hosts := make(map[string]struct{})
	var errors []error

	// Determine the hosts on the hostnames, if no hosts
	// are defined, then set to "*". If the listenerHostname is defined,
	// then the route must match the Gateway hostname.
	if len(hostnames) == 0 && listenerHostname == nil {
		hosts["*"] = struct{}{}
		return hosts, nil
	}

	if listenerHostname != nil {
		if string(*listenerHostname) != "*" {

			// Validate listener hostname.
			if err := validHostName(string(*listenerHostname)); err != nil {
				return hosts, []error{err}
			}

			if len(hostnames) == 0 {
				hosts[string(*listenerHostname)] = struct{}{}
				return hosts, nil
			}
		}
	}

	for _, host := range hostnames {

		hostname := string(host)

		// Validate the hostname.
		if err := validHostName(hostname); err != nil {
			errors = append(errors, err)
			continue
		}

		if listenerHostname != nil {
			lhn := string(*listenerHostname)

			// A "*" hostname matches anything.
			if lhn == "*" {
				hosts[hostname] = struct{}{}
				continue
			} else if lhn == hostname {
				// If the listener.hostname matches then no need to
				// do any other validation.
				hosts[hostname] = struct{}{}
				continue
			} else if strings.Contains(lhn, "*") {

				if removeFirstDNSLabel(lhn) != removeFirstDNSLabel(hostname) {
					errors = append(errors, fmt.Errorf("gateway hostname %q does not match route hostname %q", lhn, hostname))
					continue
				}
			} else {
				// Validate the gateway listener hostname matches the hostnames hostname.
				errors = append(errors, fmt.Errorf("gateway hostname %q does not match route hostname %q", lhn, hostname))
				continue
			}
		}
		hosts[hostname] = struct{}{}
	}
	return hosts, errors
}

func removeFirstDNSLabel(input string) string {
	if strings.Contains(input, ".") {
		return input[strings.IndexAny(input, "."):]
	}
	return input
}

func validHostName(hostname string) error {
	if isIP := net.ParseIP(hostname) != nil; isIP {
		return fmt.Errorf("hostname %q must be a DNS name, not an IP address", hostname)
	}
	if strings.Contains(hostname, "*") {
		if errs := validation.IsWildcardDNS1123Subdomain(hostname); errs != nil {
			return fmt.Errorf("invalid hostname %q: %v", hostname, errs)
		}
	} else {
		if errs := validation.IsDNS1123Subdomain(hostname); errs != nil {
			return fmt.Errorf("invalid hostname %q: %v", hostname, errs)
		}
	}
	return nil
}

// namespaceMatches returns true if the namespaces selector matches
// the route that is being processed.
func (p *GatewayAPIProcessor) namespaceMatches(namespaces *gatewayapi_v1alpha2.RouteNamespaces, routeNamespace string) (bool, error) {
	// From indicates where Routes will be selected for this Gateway.
	// Possible values are:
	//   * All: Routes in all namespaces may be used by this Gateway.
	//   * Selector: Routes in namespaces selected by the selector may be used by
	//     this Gateway.
	//   * Same: Only Routes in the same namespace may be used by this Gateway.

	if namespaces == nil {
		return true, nil
	}

	if namespaces.From == nil {
		return true, nil
	}

	switch *namespaces.From {
	case gatewayapi_v1alpha2.NamespacesFromAll:
		return true, nil
	case gatewayapi_v1alpha2.NamespacesFromSame:
		return p.source.gateway.Namespace == routeNamespace, nil
	case gatewayapi_v1alpha2.NamespacesFromSelector:
		if namespaces.Selector == nil ||
			(len(namespaces.Selector.MatchLabels) == 0 && len(namespaces.Selector.MatchExpressions) == 0) {
			return false, fmt.Errorf("RouteNamespaces selector must be specified when `RouteSelectType=Selector`")
		}

		// Look up the route's namespace in the list of cached namespaces.
		if ns := p.source.namespaces[routeNamespace]; ns != nil {

			// Check that the route's namespace is included in the Gateway's
			// namespace selector/expression.
			l, err := metav1.LabelSelectorAsSelector(namespaces.Selector)
			if err != nil {
				return false, err
			}

			// Look for matching labels on Selector.
			return l.Matches(labels.Set(ns.Labels)), nil
		}
	}
	return true, nil
}

// routeSelectsGatewayListener determines whether a route selects
// a given Gateway+Listener.
func routeSelectsGatewayListener(gateway *gatewayapi_v1alpha2.Gateway, listener gatewayapi_v1alpha2.Listener, routeParentRefs []gatewayapi_v1alpha2.ParentRef, routeNamespace string) bool {
	for _, ref := range routeParentRefs {
		if ref.Group == nil || ref.Kind == nil {
			continue
		}

		// If the ParentRef does not specify a namespace,
		// the Gateway must be in the same namespace as
		// the route itself. If the ParentRef specifies
		// a namespace then the Gateway must be in that
		// namespace.
		refNamespace := routeNamespace
		if ref.Namespace != nil {
			refNamespace = string(*ref.Namespace)
		}

		if *ref.Group == gatewayapi_v1alpha2.GroupName && *ref.Kind == "Gateway" && refNamespace == gateway.Namespace && ref.Name == gateway.Name {
			// no section name specified: it's a match
			if ref.SectionName == nil || *ref.SectionName == "" {
				return true
			}

			// section name specified: it must match the listener name
			return *ref.SectionName == listener.Name
		}
	}

	return false
}

func (p *GatewayAPIProcessor) computeGatewayConditions(gateway *gatewayapi_v1alpha2.Gateway, fieldErrs field.ErrorList) {

	gwAccessor, commit := p.dag.StatusCache.GatewayConditionsAccessor(k8s.NamespacedNameOf(gateway), gateway.Generation, status.ResourceGateway, &gateway.Status)
	defer commit()

	// Determine the gateway status based on fieldErrs.
	switch len(fieldErrs) {
	case 0:
		gwAccessor.AddCondition(gatewayapi_v1alpha2.GatewayConditionReady, metav1.ConditionTrue, status.ReasonValidGateway, "Valid Gateway")
	default:
		gwAccessor.AddCondition(gatewayapi_v1alpha2.GatewayConditionReady, metav1.ConditionFalse, status.ReasonInvalidGateway, errors.ParseFieldErrors(fieldErrs))
	}
}

func (p *GatewayAPIProcessor) computeTLSRoute(route *gatewayapi_v1alpha2.TLSRoute, validGateway bool, listenerSecret *Secret, listenerHostname *gatewayapi_v1alpha2.Hostname) {

	routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, status.ResourceTLSRoute, route.Status.Parents)
	defer commit()

	// If the Gateway is invalid, set status on the route.
	if !validGateway {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
		return
	}

	hosts, errs := p.computeHosts(route.Spec.Hostnames, listenerHostname)
	for _, err := range errs {
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
	}

	// Check if all the hostnames are invalid.
	if len(hosts) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
		return
	}

	for _, rule := range route.Spec.Rules {
		if len(rule.BackendRefs) == 0 {
			routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
			continue
		}

		var proxy TCPProxy
		var totalWeight uint32

		for _, backendRef := range rule.BackendRefs {

			service, err := p.validateBackendRef(backendRef, route.Namespace)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
				continue
			}

			// Route defaults to a weight of "1" unless otherwise specified.
			routeWeight := uint32(1)
			if backendRef.Weight != nil {
				routeWeight = uint32(*backendRef.Weight)
			}

			// Keep track of all the weights for this set of forwardTos. This will be
			// used later to understand if all the weights are set to zero.
			totalWeight += routeWeight

			// https://github.com/projectcontour/contour/issues/3593
			service.Weighted.Weight = routeWeight
			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream: service,
				SNI:      service.ExternalName,
				Weight:   routeWeight,
			})
		}

		// No valid clusters or all forwardTos have a weight of 0
		// so the route should get rejected.
		if len(proxy.Clusters) == 0 || totalWeight == 0 {
			continue
		}

		for host := range hosts {
			secure := p.dag.EnsureSecureVirtualHost(ListenerName{Name: host, ListenerName: "ingress_https"})

			if listenerSecret != nil {
				secure.Secret = listenerSecret
			}

			secure.TCPProxy = &proxy
		}

	}

	// Determine if any errors exist in conditions and set the "Admitted"
	// condition accordingly.
	switch len(routeAccessor.Conditions) {
	case 0:
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionTrue, status.ReasonValid, "Valid TLSRoute")
	default:
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
	}
}

func (p *GatewayAPIProcessor) computeHTTPRoute(route *gatewayapi_v1alpha2.HTTPRoute, listenerSecret *Secret, listenerHostname *gatewayapi_v1alpha2.Hostname, validGateway bool) {
	routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, status.ResourceHTTPRoute, route.Status.Parents)
	defer commit()

	// If the Gateway is invalid, set status on the route.
	if !validGateway {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
		return
	}

	hosts, errs := p.computeHosts(route.Spec.Hostnames, listenerHostname)
	for _, err := range errs {
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
	}

	// Check if all the hostnames are invalid.
	if len(hosts) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
		return
	}

	for _, rule := range route.Spec.Rules {

		var matchconditions []*matchConditions

		for _, match := range rule.Matches {
			mc := &matchConditions{}
			if err := pathMatchCondition(mc, match.Path); err != nil {
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonPathMatchType, "HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported.")
			}

			if err := headerMatchCondition(mc, match.Headers); err != nil {
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHeaderMatchType, "HTTPRoute.Spec.Rules.HeaderMatch: Only Exact match type is supported.")
			}
			matchconditions = append(matchconditions, mc)
		}

		if len(rule.BackendRefs) == 0 {
			routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
			continue
		}

		var clusters []*Cluster

		// Validate the ForwardTos.
		totalWeight := uint32(0)
		for _, backendRef := range rule.BackendRefs {

			service, err := p.validateBackendRef(backendRef.BackendRef, route.Namespace)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
				continue
			}

			var headerPolicy *HeadersPolicy
			for _, filter := range backendRef.Filters {
				switch filter.Type {
				case gatewayapi_v1alpha2.HTTPRouteFilterRequestHeaderModifier:
					var err error
					headerPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier)
					if err != nil {
						routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
					}
				default:
					routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHTTPRouteFilterType, "HTTPRoute.Spec.Rules.ForwardTo.Filters: Only RequestHeaderModifier type is supported.")
				}
			}

			// Route defaults to a weight of "1" unless otherwise specified.
			routeWeight := uint32(1)
			if backendRef.Weight != nil {
				routeWeight = uint32(*backendRef.Weight)
			}

			// Keep track of all the weights for this set of forwardTos. This will be
			// used later to understand if all the weights are set to zero.
			totalWeight += routeWeight

			// https://github.com/projectcontour/contour/issues/3593
			service.Weighted.Weight = routeWeight
			clusters = append(clusters, p.cluster(headerPolicy, service, routeWeight))
		}

		var headerPolicy *HeadersPolicy
		for _, filter := range rule.Filters {
			switch filter.Type {
			case gatewayapi_v1alpha2.HTTPRouteFilterRequestHeaderModifier:
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
		for host := range hosts {
			for _, route := range routes {
				// If there aren't any valid services, or the total weight of all of
				// them equal zero, then return 503 responses to the caller.
				if len(clusters) == 0 || totalWeight == 0 {
					// Configure a direct response HTTP status code of 503 so the
					// route still matches the configured conditions since the
					// service is missing or invalid.
					route.DirectResponse = &DirectResponse{
						StatusCode: http.StatusServiceUnavailable,
					}
				}

				// If we have a wildcard match, add a header match regex rule to match the
				// hostname so we can be sure to only match one DNS label. This is required
				// as Envoy's virtualhost hostname wildcard matching can match multiple
				// labels. This match ignores a port in the hostname in case it is present.
				if strings.HasPrefix(host, "*.") {
					route.HeaderMatchConditions = append(route.HeaderMatchConditions, HeaderMatchCondition{
						// Internally Envoy uses the HTTP/2 ":authority" header in
						// place of the HTTP/1 "host" header.
						// See: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#config-route-v3-headermatcher
						Name:      ":authority",
						MatchType: HeaderMatchTypeRegex,
						Value:     singleDNSLabelWildcardRegex + regexp.QuoteMeta(host[1:]),
					})
				}

				switch {
				case listenerSecret != nil:
					svhost := p.dag.EnsureSecureVirtualHost(ListenerName{Name: host, ListenerName: "ingress_https"})
					svhost.Secret = listenerSecret
					svhost.addRoute(route)
				default:
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
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionTrue, status.ReasonValid, "Valid HTTPRoute")
	default:
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
	}
}

// validateBackendRef verifies that the specified BackendRef is valid.
// Returns an error if not or the service found in the cache.
func (p *GatewayAPIProcessor) validateBackendRef(backendRef gatewayapi_v1alpha2.BackendRef, routeNamespace string) (*Service, error) {
	if !(backendRef.Group == nil || *backendRef.Group == "" || *backendRef.Group == "core") {
		return nil, fmt.Errorf("Spec.Rules.BackendRef.Group must be empty or 'core'")
	}

	if !(backendRef.Kind != nil && *backendRef.Kind == "Service") {
		return nil, fmt.Errorf("Spec.Rules.BackendRef.Kind must be 'Service'")
	}

	// TODO: Do not require port to be present (#3352).
	if backendRef.Port == nil {
		return nil, fmt.Errorf("Spec.Rules.BackendRef.Port must be specified")
	}

	// If the BackendRef does not specify a namespace,
	// the Service must be in the same namespace as
	// the route itself. If the BackendRef specifies
	// a namespace then the Service must be in that
	// namespace.
	namespace := routeNamespace
	if backendRef.Namespace != nil {
		namespace = string(*backendRef.Namespace)
	}

	meta := types.NamespacedName{Name: backendRef.Name, Namespace: namespace}

	// TODO: Refactor EnsureService to take an int32 so conversion to intstr is not needed.
	service, err := p.dag.EnsureService(meta, intstr.FromInt(int(*backendRef.Port)), p.source, p.EnableExternalNameService)
	if err != nil {
		return nil, fmt.Errorf("service %q is invalid: %s", meta.Name, err)
	}

	return service, nil
}

func pathMatchCondition(mc *matchConditions, match *gatewayapi_v1alpha2.HTTPPathMatch) error {

	if match == nil {
		mc.pathMatchConditions = append(mc.pathMatchConditions, &PrefixMatchCondition{Prefix: "/"})
		return nil
	}

	path := pointer.StringDeref(match.Value, "/")

	if match.Type == nil {
		// If path match type is not defined, default to 'PrefixMatch'.
		mc.pathMatchConditions = append(mc.pathMatchConditions, &PrefixMatchCondition{Prefix: path})
	} else {
		switch *match.Type {
		case gatewayapi_v1alpha2.PathMatchPrefix:
			mc.pathMatchConditions = append(mc.pathMatchConditions, &PrefixMatchCondition{Prefix: path})
		case gatewayapi_v1alpha2.PathMatchExact:
			mc.pathMatchConditions = append(mc.pathMatchConditions, &ExactMatchCondition{Path: path})
		default:
			return fmt.Errorf("HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported")
		}
	}
	return nil
}

func headerMatchCondition(mc *matchConditions, matches []gatewayapi_v1alpha2.HTTPHeaderMatch) error {

	for _, match := range matches {
		// HeaderMatchTypeExact is the default if not defined in the object.
		headerMatchType := HeaderMatchTypeExact
		if match.Type != nil {
			switch *match.Type {
			case gatewayapi_v1alpha2.HeaderMatchExact:
				headerMatchType = HeaderMatchTypeExact
			default:
				return fmt.Errorf("HTTPRoute.Spec.Rules.HeaderMatch: Only Exact match type is supported")
			}
		}

		mc.headerMatchCondition = append(mc.headerMatchCondition, HeaderMatchCondition{MatchType: headerMatchType, Name: string(match.Name), Value: match.Value})
	}

	return nil
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
func (p *GatewayAPIProcessor) cluster(headerPolicy *HeadersPolicy, service *Service, weight uint32) *Cluster {
	return &Cluster{
		Upstream:             service,
		Weight:               weight,
		Protocol:             service.Protocol,
		RequestHeadersPolicy: headerPolicy,
	}
}

func pathMatchTypePtr(pmt gatewayapi_v1alpha2.PathMatchType) *gatewayapi_v1alpha2.PathMatchType {
	return &pmt
}

func headerMatchTypePtr(hmt gatewayapi_v1alpha2.HeaderMatchType) *gatewayapi_v1alpha2.HeaderMatchType {
	return &hmt
}
