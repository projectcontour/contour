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
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	KindHTTPRoute = "HTTPRoute"
	KindTLSRoute  = "TLSRoute"
)

var (
	HTTPProtocolType  = string(gatewayapi_v1alpha1.HTTPProtocolType)
	HTTPSProtocolType = string(gatewayapi_v1alpha1.HTTPSProtocolType)
	TLSProtocolType   = string(gatewayapi_v1alpha1.TLSProtocolType)
)

// GatewayAPIProcessor translates Gateway API types into DAG
// objects and adds them to the DAG.
type GatewayAPIProcessor struct {
	logrus.FieldLogger

	dag    *DAG
	source *KubernetesCache

	errs field.ErrorList

	// listeners represent valid listeners for the cached Gateway.
	listeners []gatewayListener
}

type gatewayListener struct {
	matchingHTTPRoutes []*gatewayapi_v1alpha1.HTTPRoute
	matchingTLSRoutes  []*gatewayapi_v1alpha1.TLSRoute
	secret             *Secret
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
	p.errs = field.ErrorList{}

	// reset the processor when we're done
	defer func() {
		p.dag = nil
		p.source = nil
		p.errs = nil
		p.listeners = nil
	}()

	if p.source.gateway == nil {
		p.FieldLogger.Info("No gateway defined.")
		return
	}

	p.listeners = make([]gatewayListener, len(p.source.gateway.Spec.Listeners))

	// Validate gateway and calculate status.
	p.computeGateway(p.source.gateway, p.validateGateway())
}

// validateGateway validates gw according to the Gateway API specification.
// For additional details of the Gateway spec, refer to:
//   https://gateway-api.sigs.k8s.io/spec/#networking.x-k8s.io/v1alpha1.Gateway
func (p *GatewayAPIProcessor) validateGateway() field.ErrorList {
	return p.validateGatewaySpec(field.NewPath("spec"))
}

// validateGatewaySpec validates whether required fields of spec are set according
// to the Gateway API specification.
func (p *GatewayAPIProcessor) validateGatewaySpec(path *field.Path) field.ErrorList {
	errs := p.errs

	errs = append(errs, p.validateGatewayClassName(path.Child("gatewayClassName"))...)
	errs = append(errs, p.validateGatewayListeners(path.Child("listeners"))...)
	errs = append(errs, p.validateGatewayAddresses(path.Child("addresses"))...)

	return errs
}

// validateGatewayClassName validates the referenced gatewayclass is valid.
func (p *GatewayAPIProcessor) validateGatewayClassName(path *field.Path) field.ErrorList {
	errs := p.errs

	if p.source.gatewayclass == nil {
		errs = append(errs, field.InternalError(path, fmt.Errorf("gatewayclass %q doesn't exist",
			p.source.gateway.Spec.GatewayClassName)))
		return errs
	}

	// See if the referenced gatewayclass is admitted.
	gcAdmitted := false
	for _, c := range p.source.gatewayclass.Status.Conditions {
		if c.Type == string(gatewayapi_v1alpha1.ConditionRouteAdmitted) && c.Status == metav1.ConditionTrue {
			gcAdmitted = true
		}
	}

	if !gcAdmitted {
		errs = append(errs, field.InternalError(path, fmt.Errorf("gatewayclass %q is not admitted",
			p.source.gateway.Spec.GatewayClassName)))
	}

	return errs
}

// validateGatewayListeners validates whether required fields of listeners are set according
// to the Gateway API specification.
func (p *GatewayAPIProcessor) validateGatewayListeners(path *field.Path) field.ErrorList {
	errs := p.errs

	errs = append(errs, p.validateListenersProtocol(p.source.gateway.Spec.Listeners, path)...)
	errs = append(errs, p.validateListenersTLS(p.source.gateway.Spec.Listeners, path)...)
	errs = append(errs, p.validateListenersRoutes(p.source.gateway.Spec.Listeners, path)...)

	return errs
}

// validateListenersTLS validates the TLS configuration of the provided listener. If the listener
// specifies a valid TLS configuration, the Secret referenced by the listener is returned.
func (p *GatewayAPIProcessor) validateListenersProtocol(listeners []gatewayapi_v1alpha1.Listener, path *field.Path) field.ErrorList {
	errs := p.errs

	for i, listener := range listeners {
		switch listener.Protocol {
		case gatewayapi_v1alpha1.HTTPProtocolType, gatewayapi_v1alpha1.HTTPSProtocolType, gatewayapi_v1alpha1.TLSProtocolType:
			continue
		case gatewayapi_v1alpha1.TCPProtocolType, gatewayapi_v1alpha1.UDPProtocolType:
			p.Errorf("Unsupported protocol type %q for listener", listener.Protocol)
			errs = append(errs, field.NotSupported(path.Index(i).Child("protocol"), listener.Protocol,
				[]string{HTTPProtocolType, HTTPSProtocolType, TLSProtocolType}))
		default:
			p.Errorf("Listener.Protocol %q is invalid.", listener.Protocol)
			errs = append(errs, field.Invalid(path.Index(i).Child("protocol"), listener.Protocol, "invalid type"))
		}
	}
	return errs
}
// validateListenersTLS validates the TLS configuration of the provided listener. If the listener
// specifies a valid TLS configuration, the Secret referenced by the listener is returned.
func (p *GatewayAPIProcessor) validateListenersTLS(listeners []gatewayapi_v1alpha1.Listener, path *field.Path) field.ErrorList {
	errs := p.errs
	var sec *Secret

	for i, listener := range listeners {
		// Validate the Protocol on the selector is a supported type.
		switch listener.Protocol {
		case gatewayapi_v1alpha1.HTTPProtocolType:
			break
		case gatewayapi_v1alpha1.HTTPSProtocolType:
			// TLS is required
			if listener.TLS == nil {
				p.Errorf("TLS required for listeners with protocol type %q", listener.Protocol)
				errs = append(errs, field.Required(path.Index(i).Child("tls"),
					fmt.Sprintf("required for listeners with protocol type %q", listener.Protocol)))
				// return early since additional validation requires TLS to be set.
				return errs
			}
			// Validate the certificateRef is configured.
			sec, errs = p.validateGatewayCertRef(listener, path.Index(i).Child("tls").Child("certificateRef"))
			if len(errs) == 0 {
				p.listeners[i].secret = sec
			}
		case gatewayapi_v1alpha1.TLSProtocolType:
			// TLS only required for mode "Terminate".
			if listener.TLS != nil {
				if listener.TLS.Mode != nil {
					mode := *listener.TLS.Mode
					switch mode {
					case gatewayapi_v1alpha1.TLSModeTerminate:
						// A certificate must be specified for mode "Terminate".
						sec, errs = p.validateGatewayCertRef(listener, path.Index(i).Child("tls").Child("certificateRef"))
						if len(errs) == 0 {
							p.listeners[i].secret = sec
						}
					case gatewayapi_v1alpha1.TLSModePassthrough:
						if listener.TLS.CertificateRef != nil {
							p.Errorf("CertificateRef is not supported when TLS Mode is %q.", mode)
							errs = append(errs, field.NotSupported(path.Index(i).Child("tls").Child("certificateRef"),
								fmt.Sprintf("not supported when TLS Mode is %q", mode), []string{}))
						}
					}
				}
			}
		}
	}

	return errs
}

func (p *GatewayAPIProcessor) validateGatewayCertRef(listener gatewayapi_v1alpha1.Listener, path *field.Path) (*Secret, field.ErrorList) {
	errs := p.errs

	if listener.TLS.CertificateRef == nil {
		p.Error("CertificateRef required for listeners with TLS")
		errs = append(errs, field.Required(path, "required for listeners with TLS"))
		return nil, errs
	}

	// Validate a v1.Secret is referenced which can be kind: secret & group: core.
	// ref: https://github.com/kubernetes-sigs/gateway-api/pull/562
	if !isSecretKind(listener.TLS.CertificateRef) {
		p.Error("invalid certificateRef kind; must be \"secret\"")
		errs = append(errs, field.InternalError(path.Child("certificateRef"),
			fmt.Errorf("invalid kind %q; must be \"secret\"", listener.TLS.CertificateRef.Kind)))
	}
	if !isCoreGroup(listener.TLS.CertificateRef) {
		p.Error("invalid certificateRef group; must be \"core\"")
		errs = append(errs, field.InternalError(path.Child("certificateRef"),
			fmt.Errorf("invalid group %q; must be \"core\"", listener.TLS.CertificateRef.Group)))
	}

	var sec *Secret
	if errs == nil {
		var err error
		sec, err = p.source.LookupSecret(types.NamespacedName{Name: listener.TLS.CertificateRef.Name, Namespace: p.source.gateway.Namespace}, validSecret)
		if err != nil {
			p.Errorf("failed to validate secret %s/%s: %w", p.source.gateway.Namespace, listener.TLS.CertificateRef.Name, err)
			errs = append(errs, field.InternalError(path.Child("name"),
				fmt.Errorf("failed to validate secret %s/%s: %w", p.source.gateway.Namespace, listener.TLS.CertificateRef.Name, err)))
		}
	}
	return sec, errs
}

// validateListenersRoutes...
func (p *GatewayAPIProcessor) validateListenersRoutes(listeners []gatewayapi_v1alpha1.Listener, path *field.Path) field.ErrorList {
	errs := p.errs

	// Validate the Group on the selector is a supported type.
	for i, listener := range listeners {
		if listener.Routes.Group != nil {
			if *listener.Routes.Group != gatewayapi_v1alpha1.GroupName {
				p.Errorf("Listener.Routes.Group %q is not supported.", *listener.Routes.Group)
				errs = append(errs, field.NotSupported(path.Index(i).Child("routes").Child("group"),
					*listener.Routes.Group, []string{gatewayapi_v1alpha1.GroupName}))
			}
		}

		// Validate the Kind on the selector is a supported type.
		if listener.Routes.Kind != KindHTTPRoute && listener.Routes.Kind != KindTLSRoute {
			p.Errorf("Listener.Routes.Kind %q is not supported.", listener.Routes.Kind)
			errs = append(errs, field.NotSupported(path.Index(i).Child("routes").Child("kind"),
				listener.Routes.Kind, []string{KindHTTPRoute, KindTLSRoute}))
		}

		switch listener.Routes.Kind {
		case KindHTTPRoute:
			for _, route := range p.source.httproutes {

				// Filter the HTTPRoutes that match the gateway which Contour is configured to watch.
				// RouteBindingSelector defines a schema for associating routes with the Gateway.
				// If Namespaces and Selector are defined, only routes matching both selectors are associated with the Gateway.

				// ## RouteBindingSelector ##
				//
				// Selector specifies a set of route labels used for selecting routes to associate
				// with the Gateway. If this Selector is defined, only routes matching the Selector
				// are associated with the Gateway. An empty Selector matches all routes.

				nsMatches, err := p.namespaceMatches(listener.Routes.Namespaces, route.Namespace)
				if err != nil {
					p.Errorf("error validating namespaces against Listener.Routes.Namespaces: %s", err)
					errs = append(errs, field.InternalError(path.Index(i).Child("routes").Child("namespaces"), err))
				}

				selMatches, err := selectorMatches(listener.Routes.Selector, route.Labels)
				if err != nil {
					p.Errorf("error validating routes against Listener.Routes.Selector: %s", err)
					errs = append(errs, field.InternalError(path.Index(i).Child("routes").Child("selector"), err))
				}

				// If all the match criteria for this HTTPRoute match the Gateway, then add
				// the route to the set of matchingRoutes.
				if selMatches && nsMatches {

					if !p.gatewayMatches(route.Spec.Gateways, route.Namespace) {

						// If a label selector or namespace selector matches, but the gateway Allow doesn't
						// then set the "Admitted: false" for the route.
						routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, status.ResourceHTTPRoute, route.Status.Gateways)
						routeAccessor.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonGatewayAllowMismatch, "Gateway RouteSelector matches, but GatewayAllow has mismatch.")
						commit()
						continue
					}
					// Empty Selector matches all routes.
					if len(errs) == 0 {
						p.listeners[i].matchingHTTPRoutes = append(p.listeners[i].matchingHTTPRoutes, route)
					}
				}
			}
		case KindTLSRoute:
			// Validate the listener protocol is type=TLS.
			if listener.Protocol != gatewayapi_v1alpha1.TLSProtocolType {
				p.Errorf("invalid listener protocol %q for Kind: TLSRoute", listener.Protocol)
				errs = append(errs, field.Invalid(path.Index(i).Child("protocol"), listener.Protocol,
					fmt.Sprintf("invalid listener protocol %q", listener.Protocol)))
			}

			for _, route := range p.source.tlsroutes {
				// Filter the TLSRoutes that match the gateway which Contour is configured to watch.
				// RouteBindingSelector defines a schema for associating routes with the Gateway.
				// If Namespaces and Selector are defined, only routes matching both selectors are associated with the Gateway.

				// ## RouteBindingSelector ##
				//
				// Selector specifies a set of route labels used for selecting routes to associate
				// with the Gateway. If this Selector is defined, only routes matching the Selector
				// are associated with the Gateway. An empty Selector matches all routes.

				nsMatches, err := p.namespaceMatches(listener.Routes.Namespaces, route.Namespace)
				if err != nil {
					p.Errorf("error validating namespaces against Listener.Routes.Namespaces: %s", err)
					errs = append(errs, field.InternalError(path.Child("namespaces"), err))
				}

				selMatches, err := selectorMatches(listener.Routes.Selector, route.Labels)
				if err != nil {
					p.Errorf("error validating routes against Listener.Routes.Selector: %s", err)
					errs = append(errs, field.InternalError(path.Child("selector"), err))
				}

				if selMatches && nsMatches {

					if !p.gatewayMatches(route.Spec.Gateways, route.Namespace) {

						// If a label selector or namespace selector matches, but the gateway Allow doesn't
						// then set the "Admitted: false" for the route.
						routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, status.ResourceTLSRoute, route.Status.Gateways)
						routeAccessor.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonGatewayAllowMismatch, "Gateway RouteSelector matches, but GatewayAllow has mismatch.")
						commit()
						continue
					}
					// Empty Selector matches all routes.
					if len(errs) == 0 {
						p.listeners[i].matchingTLSRoutes = append(p.listeners[i].matchingTLSRoutes, route)
					}
				}
			}
		}
		// Process all the HTTPRoutes that match this Gateway.
		if p.listeners[i].matchingHTTPRoutes != nil && len(errs) == 0 {
			for _, route := range p.listeners[i].matchingHTTPRoutes {
				p.computeHTTPRoute(route, p.listeners[i].secret, listener.Hostname)
			}
		}
		// Process all the TLSRoutes that match this Gateway.
		if len(p.listeners[i].matchingTLSRoutes) > 0 && len(errs) == 0 {
			for _, route := range p.listeners[i].matchingTLSRoutes {
				p.computeTLSRoute(route)
			}
		}
	}
	return errs
}

// validateGatewayAddresses validates that the gateway addresses field is unspecified.
func (p *GatewayAPIProcessor) validateGatewayAddresses(path *field.Path) field.ErrorList {
	errs := p.errs

	if len(p.source.gateway.Spec.Addresses) > 0 {
		p.Error("Spec.Addresses is unsupported")
		errs = append(errs, field.NotSupported(path, p.source.gateway.Spec.Addresses, []string{}))
	}

	return errs
}

func isSecretKind(certificateRef *gatewayapi_v1alpha1.LocalObjectReference) bool {
	return strings.ToLower(certificateRef.Kind) == "secret"
}

func isCoreGroup(certificateRef *gatewayapi_v1alpha1.LocalObjectReference) bool {
	return strings.ToLower(certificateRef.Group) == "core"
}

// computeHosts validates the hostnames for a HTTPRoute as well as validating
// that the hostname on the HTTPRoute matches what is optionally defined on the
// listener.hostname.
func (p *GatewayAPIProcessor) computeHosts(hostnames []gatewayapi_v1alpha1.Hostname, listenerHostname *gatewayapi_v1alpha1.Hostname) (map[string]struct{}, []error) {

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
// the HTTPRoute that is being processed.
func (p *GatewayAPIProcessor) namespaceMatches(namespaces *gatewayapi_v1alpha1.RouteNamespaces, namespace string) (bool, error) {
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
	case gatewayapi_v1alpha1.RouteSelectAll:
		return true, nil
	case gatewayapi_v1alpha1.RouteSelectSame:
		return p.source.ConfiguredGateway.Namespace == namespace, nil
	case gatewayapi_v1alpha1.RouteSelectSelector:
		if len(namespaces.Selector.MatchLabels) == 0 && len(namespaces.Selector.MatchExpressions) == 0 {
			return false, fmt.Errorf("RouteNamespaces selector must be specified when `RouteSelectType=Selector`")
		}

		// Look up the HTTPRoute's namespace in the list of cached namespaces.
		if ns := p.source.namespaces[namespace]; ns != nil {

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

// gatewayMatches returns true if "AllowAll" is set, the "SameNamespace" is set and the HTTPRoute
// matches the Gateway's namespace, or the "FromList" is set and the gateway Contour is watching
// matches one from the list.
func (p *GatewayAPIProcessor) gatewayMatches(routeGateways *gatewayapi_v1alpha1.RouteGateways, namespace string) bool {

	if routeGateways == nil || routeGateways.Allow == nil {
		return true
	}

	switch *routeGateways.Allow {
	case gatewayapi_v1alpha1.GatewayAllowAll:
		return true
	case gatewayapi_v1alpha1.GatewayAllowFromList:
		for _, gateway := range routeGateways.GatewayRefs {
			if gateway.Name == p.source.ConfiguredGateway.Name && gateway.Namespace == p.source.ConfiguredGateway.Namespace {
				return true
			}
		}
	case gatewayapi_v1alpha1.GatewayAllowSameNamespace:
		return p.source.ConfiguredGateway.Namespace == namespace
	}
	return false
}

// selectorMatches returns true if the selector matches the labels on the object or is not defined.
func selectorMatches(selector *metav1.LabelSelector, objLabels map[string]string) (bool, error) {

	if selector == nil {
		return true, nil
	}

	// If a selector is defined then check that it matches the labels on the object.
	if len(selector.MatchLabels) > 0 || len(selector.MatchExpressions) > 0 {
		l, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return false, err
		}

		// Look for matching labels on Selector.
		return l.Matches(labels.Set(objLabels)), nil
	}
	// If no selector is defined then it matches by default.
	return true, nil
}

// TODO: Pass in the status of the Envoy infra to compute "Ready" condition.
// TODO: Add gateway listener status support.
func (p *GatewayAPIProcessor) computeGateway(gw *gatewayapi_v1alpha1.Gateway, errs field.ErrorList) {
	gwAccessor, commit := p.dag.StatusCache.GatewayConditionsAccessor(k8s.NamespacedNameOf(gw), gw.Generation, status.ResourceGateway, &gw.Status)
	defer commit()

	// Determine the gateway status based on errs.
	switch len(errs) {
	case 0:
		gwAccessor.AddCondition(gatewayapi_v1alpha1.GatewayConditionScheduled, metav1.ConditionTrue, status.ReasonValidGateway, "Valid Gateway")
	default:
		gwAccessor.AddCondition(gatewayapi_v1alpha1.GatewayConditionScheduled, metav1.ConditionFalse, status.ReasonInvalidGateway, errors.ParseFieldErrors(errs))
	}
}

func (p *GatewayAPIProcessor) computeTLSRoute(route *gatewayapi_v1alpha1.TLSRoute) {

	routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, status.ResourceTLSRoute, route.Status.Gateways)
	defer commit()

	for _, rule := range route.Spec.Rules {
		var hosts []string
		var matchErrors []error
		totalSnis := 0

		// Build the set of SNIs that are applied to this TLSRoute.
		for _, match := range rule.Matches {
			for _, snis := range match.SNIs {
				totalSnis++
				if err := validHostName(string(snis)); err != nil {
					matchErrors = append(matchErrors, err)
					continue
				}
				hosts = append(hosts, string(snis))
			}
		}

		// If there are any errors with the supplied hostnames, then
		// add a condition to the route.
		for _, err := range matchErrors {
			routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
		}

		// If all the supplied SNIs are invalid, then this route is invalid
		// and should be dropped.
		if len(matchErrors) != 0 && len(matchErrors) == totalSnis {
			continue
		}

		// If SNIs is unspecified, then all
		// requests associated with the gateway TLS listener will match.
		// This can be used to define a default backend for a TLS listener.
		if len(hosts) == 0 {
			hosts = []string{"*"}
		}

		if len(rule.ForwardTo) == 0 {
			routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.ForwardTo must be specified.")
			continue
		}

		var proxy TCPProxy
		for _, forward := range rule.ForwardTo {

			service, err := p.validateForwardTo(forward.ServiceName, forward.Port, route.Namespace)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
				continue
			}

			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream: service,
				SNI:      service.ExternalName,
			})
		}

		if len(proxy.Clusters) == 0 {
			// No valid clusters so the route should get rejected.
			continue
		}

		for _, host := range hosts {
			secure := p.dag.EnsureSecureVirtualHost(ListenerName{Name: host, ListenerName: "ingress_https"})
			secure.TCPProxy = &proxy
		}
	}

	// Determine if any errors exist in conditions and set the "Admitted"
	// condition accordingly.
	switch len(routeAccessor.Conditions) {
	case 0:
		routeAccessor.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionTrue, status.ReasonValid, "Valid TLSRoute")
	default:
		routeAccessor.AddCondition(gatewayapi_v1alpha1.ConditionRouteAdmitted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
	}
}

func (p *GatewayAPIProcessor) computeHTTPRoute(route *gatewayapi_v1alpha1.HTTPRoute, listenerSecret *Secret, listenerHostname *gatewayapi_v1alpha1.Hostname) {
	routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, status.ResourceHTTPRoute, route.Status.Gateways)
	defer commit()

	// Check if gateway status is reporting "Scheduled=true".
	// TODO: Update to "Ready=true" when the gateway controller supports provisioning Envoy infrastructure.
	if p.source.gateway != nil {
		for _, cond := range p.source.gateway.Status.Conditions {
			if cond.Type == string(gatewayapi_v1alpha1.GatewayConditionScheduled) &&
				cond.Status != metav1.ConditionTrue {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "gateway is not scheduled")
			}
		}
	}

	hosts, errs := p.computeHosts(route.Spec.Hostnames, listenerHostname)
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

		if len(rule.ForwardTo) == 0 {
			routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.ForwardTo must be specified.")
			continue
		}

		var clusters []*Cluster

		// Validate the ForwardTos.
		totalWeight := uint32(0)
		for _, forward := range rule.ForwardTo {

			service, err := p.validateForwardTo(forward.ServiceName, forward.Port, route.Namespace)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
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

			// Route defaults to a weight of "1" unless otherwise specified.
			routeWeight := uint32(1)
			if forward.Weight != nil {
				routeWeight = uint32(*forward.Weight)
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

				if len(p.errs) == 0 {
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

// validateForwardTo verifies that the specified forwardTo is valid.
// Returns an error if not or the service found in the cache.
func (p *GatewayAPIProcessor) validateForwardTo(serviceName *string, port *gatewayapi_v1alpha1.PortNumber, namespace string) (*Service, error) {
	// Verify the service is valid
	if serviceName == nil {
		return nil, fmt.Errorf("Spec.Rules.ForwardTo.ServiceName must be specified")
	}

	// TODO: Do not require port to be present (#3352).
	if port == nil {
		return nil, fmt.Errorf("Spec.Rules.ForwardTo.ServicePort must be specified")
	}

	meta := types.NamespacedName{Name: *serviceName, Namespace: namespace}

	// TODO: Refactor EnsureService to take an int32 so conversion to intstr is not needed.
	service, err := p.dag.EnsureService(meta, intstr.FromInt(int(*port)), p.source)
	if err != nil {
		return nil, fmt.Errorf("service %q does not exist", meta.Name)
	}

	return service, nil
}

func pathMatchCondition(mc *matchConditions, match *gatewayapi_v1alpha1.HTTPPathMatch) error {

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
		case gatewayapi_v1alpha1.PathMatchPrefix:
			mc.pathMatchConditions = append(mc.pathMatchConditions, &PrefixMatchCondition{Prefix: path})
		case gatewayapi_v1alpha1.PathMatchExact:
			mc.pathMatchConditions = append(mc.pathMatchConditions, &ExactMatchCondition{Path: path})
		default:
			return fmt.Errorf("HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported")
		}
	}
	return nil
}

func headerMatchCondition(mc *matchConditions, match *gatewayapi_v1alpha1.HTTPHeaderMatch) error {
	if match == nil {
		return nil
	}

	// HeaderMatchTypeExact is the default if not defined in the object.
	headerMatchType := HeaderMatchTypeExact
	if match.Type != nil {
		switch *match.Type {
		case gatewayapi_v1alpha1.HeaderMatchExact:
			headerMatchType = HeaderMatchTypeExact
		default:
			return fmt.Errorf("HTTPRoute.Spec.Rules.HeaderMatch: Only Exact match type is supported")
		}
	}

	for k, v := range match.Values {
		mc.headerMatchCondition = append(mc.headerMatchCondition, HeaderMatchCondition{MatchType: headerMatchType, Name: k, Value: v})
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

func pathMatchTypePtr(pmt gatewayapi_v1alpha1.PathMatchType) *gatewayapi_v1alpha1.PathMatchType {
	return &pmt
}

func headerMatchTypePtr(hmt gatewayapi_v1alpha1.HeaderMatchType) *gatewayapi_v1alpha1.HeaderMatchType {
	return &hmt
}

func gatewayAllowTypePtr(gwType gatewayapi_v1alpha1.GatewayAllowType) *gatewayapi_v1alpha1.GatewayAllowType {
	return &gwType
}
