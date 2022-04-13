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
	"time"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/status"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	KindHTTPRoute = "HTTPRoute"
	KindTLSRoute  = "TLSRoute"
	KindGateway   = "Gateway"
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

	// ConnectTimeout defines how long the proxy should wait when establishing connection to upstream service.
	ConnectTimeout time.Duration
}

// matchConditions holds match rules.
type matchConditions struct {
	path    MatchCondition
	headers []HeaderMatchCondition
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

	// Gateway and GatewayClass must be defined for resources to be processed.
	if p.source.gateway == nil {
		p.Info("Gateway not found in cache.")
		return
	}
	if p.source.gatewayclass == nil {
		p.Info("Gatewayclass not found in cache.")
		return
	}

	gwAccessor, commit := p.dag.StatusCache.GatewayStatusAccessor(
		k8s.NamespacedNameOf(p.source.gateway),
		p.source.gateway.Generation,
		&p.source.gateway.Status,
	)
	defer commit()

	var gatewayNotReadyCondition *metav1.Condition

	if !isAddressAssigned(p.source.gateway.Spec.Addresses, p.source.gateway.Status.Addresses) {
		gatewayNotReadyCondition = &metav1.Condition{
			Type:    string(gatewayapi_v1alpha2.GatewayConditionReady),
			Status:  metav1.ConditionFalse,
			Reason:  string(gatewayapi_v1alpha2.GatewayReasonAddressNotAssigned),
			Message: "None of the addresses in Spec.Addresses have been assigned to the Gateway",
		}
	}

	// Add conditions for invalid listeners
	validateListenersResult := gatewayapi.ValidateListeners(p.source.gateway.Spec.Listeners)
	for name, cond := range validateListenersResult.InvalidListenerConditions {
		gwAccessor.AddListenerCondition(
			string(name),
			gatewayapi_v1alpha2.ListenerConditionType(cond.Type),
			cond.Status,
			gatewayapi_v1alpha2.ListenerConditionReason(cond.Reason),
			cond.Message,
		)
	}

	for _, listener := range p.source.gateway.Spec.Listeners {
		p.computeListener(listener, gwAccessor, gatewayNotReadyCondition == nil, validateListenersResult)
	}

	p.computeGatewayConditions(gwAccessor, gatewayNotReadyCondition)
}

// isAddressAssigned returns true if either there are no addresses requested in specAddresses,
// or if at least one address from specAddresses appears in statusAddresses.
func isAddressAssigned(specAddresses, statusAddresses []gatewayapi_v1alpha2.GatewayAddress) bool {
	if len(specAddresses) == 0 {
		return true
	}

	for _, specAddress := range specAddresses {
		for _, statusAddress := range statusAddresses {
			// Types must match
			if addressTypeDerefOr(specAddress.Type, gatewayapi_v1alpha2.IPAddressType) != addressTypeDerefOr(statusAddress.Type, gatewayapi_v1alpha2.IPAddressType) {
				continue
			}

			// Values must match
			if specAddress.Value != statusAddress.Value {
				continue
			}

			return true
		}
	}

	// No match found, so no spec address is assigned.
	return false
}

func addressTypeDerefOr(addressType *gatewayapi_v1alpha2.AddressType, defaultAddressType gatewayapi_v1alpha2.AddressType) gatewayapi_v1alpha2.AddressType {
	if addressType != nil {
		return *addressType
	}
	return defaultAddressType
}

func (p *GatewayAPIProcessor) computeListener(listener gatewayapi_v1alpha2.Listener, gwAccessor *status.GatewayStatusUpdate, isGatewayValid bool, validateListenersResult gatewayapi.ValidateListenersResult) {
	// set the listener's "Ready" condition based on whether we've
	// added any other conditions for the listener. The assumption
	// here is that if another condition is set, the listener is
	// invalid/not ready.
	defer func() {
		listenerStatus := gwAccessor.ListenerStatus[string(listener.Name)]

		if listenerStatus == nil || len(listenerStatus.Conditions) == 0 {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1alpha2.ListenerConditionReady,
				metav1.ConditionTrue,
				gatewayapi_v1alpha2.ListenerReasonReady,
				"Valid listener",
			)
		} else {
			readyConditionExists := false
			for _, cond := range listenerStatus.Conditions {
				if cond.Type == string(gatewayapi_v1alpha2.ListenerConditionReady) {
					readyConditionExists = true
					break
				}
			}

			// Only set the Ready condition if it doesn't already
			// exist in the status update, since if it does exist,
			// it will contain more specific information about what
			// was invalid.
			if !readyConditionExists {
				gwAccessor.AddListenerCondition(
					string(listener.Name),
					gatewayapi_v1alpha2.ListenerConditionReady,
					metav1.ConditionFalse,
					gatewayapi_v1alpha2.ListenerReasonInvalid,
					"Invalid listener, see other listener conditions for details",
				)
			}
		}
	}()

	if _, ok := validateListenersResult.InvalidListenerConditions[listener.Name]; ok {
		// Listener had an invalid protocol/port/hostname, don't need to inspect further.
		return
	}

	var listenerSecret *Secret

	// Validate TLS details for HTTPS/TLS protocol listeners.
	switch listener.Protocol {
	case gatewayapi_v1alpha2.HTTPSProtocolType:
		// Validate that if protocol is type HTTPS, that TLS is defined.
		if listener.TLS == nil {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1alpha2.ListenerConditionReady,
				metav1.ConditionFalse,
				gatewayapi_v1alpha2.ListenerReasonInvalid,
				fmt.Sprintf("Listener.TLS is required when protocol is %q.", listener.Protocol),
			)
			return
		}

		// Check for valid TLS configuration on the Gateway.
		if listenerSecret = p.validGatewayTLS(*listener.TLS, string(listener.Name), gwAccessor); listenerSecret == nil {
			// If TLS was configured on the Listener, but it's invalid, don't allow any
			// routes to be bound to this listener since it can't serve TLS traffic.
			return
		}
	case gatewayapi_v1alpha2.TLSProtocolType:
		// TLS is required for the type TLS.
		if listener.TLS == nil {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1alpha2.ListenerConditionReady,
				metav1.ConditionFalse,
				gatewayapi_v1alpha2.ListenerReasonInvalid,
				fmt.Sprintf("Listener.TLS is required when protocol is %q.", listener.Protocol),
			)
			return
		}

		if listener.TLS.Mode != nil {
			switch *listener.TLS.Mode {
			case gatewayapi_v1alpha2.TLSModeTerminate:
				// Check for valid TLS configuration on the Gateway.
				if listenerSecret = p.validGatewayTLS(*listener.TLS, string(listener.Name), gwAccessor); listenerSecret == nil {
					// If TLS was configured on the Listener, but it's invalid, don't allow any
					// routes to be bound to this listener since it can't serve TLS traffic.
					return
				}
			case gatewayapi_v1alpha2.TLSModePassthrough:
				if len(listener.TLS.CertificateRefs) > 0 {
					gwAccessor.AddListenerCondition(
						string(listener.Name),
						gatewayapi_v1alpha2.ListenerConditionReady,
						metav1.ConditionFalse,
						gatewayapi_v1alpha2.ListenerReasonInvalid,
						fmt.Sprintf("Listener.TLS.CertificateRefs cannot be defined when TLS Mode is %q.", *listener.TLS.Mode),
					)
					return
				}
			}
		}
	}

	// Get a list of the route kinds that the listener accepts.
	listenerRouteKinds := p.getListenerRouteKinds(listener, gwAccessor)
	gwAccessor.SetListenerSupportedKinds(string(listener.Name), listenerRouteKinds)

	attachedRoutes := 0
	for _, routeKind := range listenerRouteKinds {
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

				if p.computeHTTPRoute(route, listenerSecret, listener.Hostname, isGatewayValid) {
					attachedRoutes++
				}
			}
		case KindTLSRoute:
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

				if p.computeTLSRoute(route, listenerSecret, listener.Hostname, isGatewayValid) {
					attachedRoutes++
				}
			}
		}
	}

	gwAccessor.SetListenerAttachedRoutes(string(listener.Name), attachedRoutes)
}

// getListenerRouteKinds gets a list of the valid route kinds that
// the listener accepts.
func (p *GatewayAPIProcessor) getListenerRouteKinds(listener gatewayapi_v1alpha2.Listener, gwAccessor *status.GatewayStatusUpdate) []gatewayapi_v1alpha2.Kind {
	// None specified on the listener: return the default based on
	// the listener's protocol.
	if len(listener.AllowedRoutes.Kinds) == 0 {
		switch listener.Protocol {
		case gatewayapi_v1alpha2.HTTPProtocolType:
			return []gatewayapi_v1alpha2.Kind{KindHTTPRoute}
		case gatewayapi_v1alpha2.HTTPSProtocolType:
			return []gatewayapi_v1alpha2.Kind{KindHTTPRoute}
		case gatewayapi_v1alpha2.TLSProtocolType:
			return []gatewayapi_v1alpha2.Kind{KindTLSRoute}
		}
	}

	var routeKinds []gatewayapi_v1alpha2.Kind

	for _, routeKind := range listener.AllowedRoutes.Kinds {
		if routeKind.Group != nil && *routeKind.Group != gatewayapi_v1alpha2.GroupName {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1alpha2.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("Group %q is not supported, group must be %q", *routeKind.Group, gatewayapi_v1alpha2.GroupName),
			)
			continue
		}
		if routeKind.Kind != KindHTTPRoute && routeKind.Kind != KindTLSRoute {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1alpha2.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("Kind %q is not supported, kind must be %q or %q", routeKind.Kind, KindHTTPRoute, KindTLSRoute),
			)
			continue
		}
		if routeKind.Kind == KindTLSRoute && listener.Protocol != gatewayapi_v1alpha2.TLSProtocolType {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1alpha2.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("TLSRoutes are incompatible with listener protocol %q", listener.Protocol),
			)
			continue
		}

		routeKinds = append(routeKinds, routeKind.Kind)
	}

	return routeKinds
}

func (p *GatewayAPIProcessor) validGatewayTLS(listenerTLS gatewayapi_v1alpha2.GatewayTLSConfig, listenerName string, gwAccessor *status.GatewayStatusUpdate) *Secret {
	if len(listenerTLS.CertificateRefs) != 1 {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1alpha2.ListenerConditionReady,
			metav1.ConditionFalse,
			gatewayapi_v1alpha2.ListenerReasonInvalid,
			"Listener.TLS.CertificateRefs must contain exactly one entry",
		)
		return nil
	}

	certificateRef := listenerTLS.CertificateRefs[0]

	// Validate a v1.Secret is referenced which can be kind: secret & group: core.
	// ref: https://github.com/kubernetes-sigs/gateway-api/pull/562
	if !isSecretRef(*certificateRef) {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
			metav1.ConditionFalse,
			gatewayapi_v1alpha2.ListenerReasonInvalidCertificateRef,
			fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q must contain a reference to a core.Secret", certificateRef.Name),
		)
		return nil
	}

	// If the secret is in a different namespace than the gateway, then we need to
	// check for a ReferencePolicy that allows the reference.
	if certificateRef.Namespace != nil && string(*certificateRef.Namespace) != p.source.gateway.Namespace {
		if !p.validCrossNamespaceRef(
			crossNamespaceFrom{
				group:     gatewayapi_v1alpha2.GroupName,
				kind:      KindGateway,
				namespace: p.source.gateway.Namespace,
			},
			crossNamespaceTo{
				group:     "",
				kind:      "Secret",
				namespace: string(*certificateRef.Namespace),
				name:      string(certificateRef.Name),
			},
		) {
			gwAccessor.AddListenerCondition(
				listenerName,
				gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1alpha2.ListenerReasonInvalidCertificateRef,
				fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q namespace must match the Gateway's namespace or be covered by a ReferencePolicy", certificateRef.Name),
			)
			return nil
		}
	}

	var meta types.NamespacedName
	if certificateRef.Namespace != nil {
		meta = types.NamespacedName{Name: string(certificateRef.Name), Namespace: string(*certificateRef.Namespace)}
	} else {
		meta = types.NamespacedName{Name: string(certificateRef.Name), Namespace: p.source.gateway.Namespace}
	}

	listenerSecret, err := p.source.LookupSecret(meta, validTLSSecret)
	if err != nil {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
			metav1.ConditionFalse,
			gatewayapi_v1alpha2.ListenerReasonInvalidCertificateRef,
			fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q referent is invalid: %s", certificateRef.Name, err),
		)
		return nil
	}
	return listenerSecret
}

type crossNamespaceFrom struct {
	group     string
	kind      string
	namespace string
}

type crossNamespaceTo struct {
	group     string
	kind      string
	namespace string
	name      string
}

func (p *GatewayAPIProcessor) validCrossNamespaceRef(from crossNamespaceFrom, to crossNamespaceTo) bool {
	for _, referencePolicy := range p.source.referencepolicies {
		// The ReferencePolicy must be defined in the namespace of
		// the "to" (the referent).
		if referencePolicy.Namespace != to.namespace {
			continue
		}

		// Check if the ReferencePolicy has a matching "from".
		var fromAllowed bool
		for _, refPolicyFrom := range referencePolicy.Spec.From {
			if string(refPolicyFrom.Namespace) == from.namespace && string(refPolicyFrom.Group) == from.group && string(refPolicyFrom.Kind) == from.kind {
				fromAllowed = true
				break
			}
		}
		if !fromAllowed {
			continue
		}

		// Check if the ReferencePolicy has a matching "to".
		var toAllowed bool
		for _, refPolicyTo := range referencePolicy.Spec.To {
			if string(refPolicyTo.Group) == to.group && string(refPolicyTo.Kind) == to.kind && (refPolicyTo.Name == nil || *refPolicyTo.Name == "" || string(*refPolicyTo.Name) == to.name) {
				toAllowed = true
				break
			}
		}
		if !toAllowed {
			continue
		}

		// If we got here, both the "from" and the "to" were allowed by this
		// reference policy.
		return true
	}

	// If we got here, no reference policy allowed both the "from" and "to".
	return false
}

func isSecretRef(certificateRef gatewayapi_v1alpha2.SecretObjectReference) bool {
	return certificateRef.Group != nil && *certificateRef.Group == "" &&
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
			switch {
			case lhn == "*":
				hosts[hostname] = struct{}{}
				continue
			case lhn == hostname:
				// If the listener.hostname matches then no need to
				// do any other validation.
				hosts[hostname] = struct{}{}
				continue
			case strings.Contains(lhn, "*"):

				if removeFirstDNSLabel(lhn) != removeFirstDNSLabel(hostname) {
					errors = append(errors, fmt.Errorf("gateway hostname %q does not match route hostname %q", lhn, hostname))
					continue
				}
			default:
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

		if *ref.Group == gatewayapi_v1alpha2.GroupName && *ref.Kind == "Gateway" && refNamespace == gateway.Namespace && string(ref.Name) == gateway.Name {
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

func (p *GatewayAPIProcessor) computeGatewayConditions(gwAccessor *status.GatewayStatusUpdate, gatewayNotReadyCondition *metav1.Condition) {
	// If Contour's running, the Gateway is considered scheduled.
	gwAccessor.AddCondition(
		gatewayapi_v1alpha2.GatewayConditionScheduled,
		metav1.ConditionTrue,
		status.GatewayReasonType(gatewayapi_v1alpha2.GatewayReasonScheduled),
		"Gateway is scheduled",
	)

	switch {
	case gatewayNotReadyCondition != nil:
		gwAccessor.AddCondition(
			gatewayapi_v1alpha2.GatewayConditionType(gatewayNotReadyCondition.Type),
			gatewayNotReadyCondition.Status,
			status.GatewayReasonType(gatewayNotReadyCondition.Reason),
			gatewayNotReadyCondition.Message,
		)
	default:
		// Check for any listeners with a Ready: false condition.
		allListenersReady := true
		for _, ls := range gwAccessor.ListenerStatus {
			if ls == nil {
				continue
			}

			for _, cond := range ls.Conditions {
				if cond.Type == string(gatewayapi_v1alpha2.ListenerConditionReady) && cond.Status == metav1.ConditionFalse {
					allListenersReady = false
					break
				}
			}

			if !allListenersReady {
				break
			}
		}

		if !allListenersReady {
			// If we have invalid listeners, set Ready=false.
			gwAccessor.AddCondition(gatewayapi_v1alpha2.GatewayConditionReady, metav1.ConditionFalse, status.GatewayReasonType(gatewayapi_v1alpha2.GatewayReasonListenersNotValid), "Listeners are not valid")
		} else {
			// Otherwise, Ready=true.
			gwAccessor.AddCondition(gatewayapi_v1alpha2.GatewayConditionReady, metav1.ConditionTrue, status.ReasonValidGateway, "Valid Gateway")
		}
	}
}

func (p *GatewayAPIProcessor) computeTLSRoute(route *gatewayapi_v1alpha2.TLSRoute, listenerSecret *Secret, listenerHostname *gatewayapi_v1alpha2.Hostname, validGateway bool) bool {

	routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, &gatewayapi_v1alpha2.TLSRoute{}, route.Status.Parents)
	defer commit()

	// If the Gateway is invalid, set status on the route.
	if !validGateway {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
		return false
	}

	hosts, errs := p.computeHosts(route.Spec.Hostnames, listenerHostname)
	for _, err := range errs {
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
	}

	// Check if all the hostnames are invalid.
	if len(hosts) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
		return false
	}

	var programmed bool
	for _, rule := range route.Spec.Rules {
		if len(rule.BackendRefs) == 0 {
			routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
			continue
		}

		var proxy TCPProxy
		var totalWeight uint32

		for _, backendRef := range rule.BackendRefs {

			service, err := p.validateBackendRef(backendRef, KindTLSRoute, route.Namespace)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
				continue
			}

			// Route defaults to a weight of "1" unless otherwise specified.
			routeWeight := uint32(1)
			if backendRef.Weight != nil {
				routeWeight = uint32(*backendRef.Weight)
			}

			// Keep track of all the weights for this set of backendRefs. This will be
			// used later to understand if all the weights are set to zero.
			totalWeight += routeWeight

			// https://github.com/projectcontour/contour/issues/3593
			service.Weighted.Weight = routeWeight
			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream:      service,
				SNI:           service.ExternalName,
				Weight:        routeWeight,
				TimeoutPolicy: ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
			})
		}

		// No clusters added: they were all invalid, so reject
		// the route (it already has a relevant condition set).
		if len(proxy.Clusters) == 0 {
			continue
		}

		// If we have valid clusters but they all have a zero
		// weight, reject the route.
		if totalWeight == 0 {
			routeAccessor.AddCondition(status.ConditionValidBackendRefs, metav1.ConditionFalse, status.ReasonAllBackendRefsHaveZeroWeights, "At least one Spec.Rules.BackendRef must have a non-zero weight.")
			continue
		}

		for host := range hosts {
			secure := p.dag.EnsureSecureVirtualHost(host)

			if listenerSecret != nil {
				secure.Secret = listenerSecret
			}

			secure.TCPProxy = &proxy

			programmed = true
		}
	}

	// Determine if any errors exist in conditions and set the "Accepted"
	// condition accordingly.
	switch len(routeAccessor.Conditions) {
	case 0:
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionTrue, status.ReasonValid, "Valid TLSRoute")
	default:
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
	}

	return programmed
}

func (p *GatewayAPIProcessor) computeHTTPRoute(route *gatewayapi_v1alpha2.HTTPRoute, listenerSecret *Secret, listenerHostname *gatewayapi_v1alpha2.Hostname, validGateway bool) bool {
	routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(k8s.NamespacedNameOf(route), route.Generation, &gatewayapi_v1alpha2.HTTPRoute{}, route.Status.Parents)
	defer commit()

	// If the Gateway is invalid, set status on the route.
	if !validGateway {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
		return false
	}

	hosts, errs := p.computeHosts(route.Spec.Hostnames, listenerHostname)
	for _, err := range errs {
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
	}

	// Check if all the hostnames are invalid.
	if len(hosts) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
		return false
	}

	var programmed bool
	for _, rule := range route.Spec.Rules {
		// Get match conditions for the rule.
		var matchconditions []*matchConditions
		for _, match := range rule.Matches {
			pathMatch, ok := gatewayPathMatchCondition(match.Path, routeAccessor)
			if !ok {
				continue
			}

			headerMatches, err := gatewayHeaderMatchConditions(match.Headers)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHeaderMatchType, "HTTPRoute.Spec.Rules.HeaderMatch: Only Exact match type is supported.")
				continue
			}

			// Envoy uses the HTTP/2 ":method" header internally
			// for both HTTP/1 and HTTP/2 method matching.
			if match.Method != nil {
				headerMatches = append(headerMatches, HeaderMatchCondition{
					Name:      ":method",
					Value:     string(*match.Method),
					MatchType: HeaderMatchTypeExact,
				})
			}

			matchconditions = append(matchconditions, &matchConditions{
				path:    pathMatch,
				headers: headerMatches,
			})
		}

		// Process rule-level filters.
		var (
			headerPolicy       *HeadersPolicy
			headerModifierSeen bool
			redirect           *gatewayapi_v1alpha2.HTTPRequestRedirectFilter
		)

		for _, filter := range rule.Filters {
			switch filter.Type {
			case gatewayapi_v1alpha2.HTTPRouteFilterRequestHeaderModifier:
				// Per Gateway API docs, "specifying a core filter multiple times has
				// unspecified or custom conformance.", here we choose to just process
				// the first one.
				if headerModifierSeen {
					continue
				}

				headerModifierSeen = true

				var err error
				headerPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier)
				if err != nil {
					routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1alpha2.HTTPRouteFilterRequestRedirect:
				// Get the redirect filter if there is one. Note that per Gateway API
				// docs, "specifying a core filter multiple times has unspecified or
				// custom conformance.", here we choose to just select the first one.
				if redirect == nil && filter.RequestRedirect != nil {
					redirect = filter.RequestRedirect
				}
			default:
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHTTPRouteFilterType,
					fmt.Sprintf("HTTPRoute.Spec.Rules.Filters: invalid type %q: only RequestHeaderModifier and RequestRedirect are supported.", filter.Type))
			}
		}

		// Get our list of routes based on whether it's a redirect or a cluster-backed route.
		// Note that we can end up with multiple routes here since the match conditions are
		// logically "OR"-ed, which we express as multiple routes, each with one of the
		// conditions, all with the same action.
		var routes []*Route
		if redirect != nil {
			routes = p.redirectRoutes(matchconditions, headerPolicy, redirect)
		} else {
			routes = p.clusterRoutes(route.Namespace, matchconditions, headerPolicy, rule.BackendRefs, routeAccessor)
		}

		// Add each route to the relevant vhost(s)/svhosts(s).
		for host := range hosts {
			for _, route := range routes {
				// If we have a wildcard match, add a header match regex rule to match the
				// hostname so we can be sure to only match one DNS label. This is required
				// as Envoy's virtualhost hostname wildcard matching can match multiple
				// labels. This match ignores a port in the hostname in case it is present.
				if strings.HasPrefix(host, "*.") {
					route.HeaderMatchConditions = append(route.HeaderMatchConditions, wildcardDomainHeaderMatch(host))
				}

				switch {
				case listenerSecret != nil:
					svhost := p.dag.EnsureSecureVirtualHost(host)
					svhost.Secret = listenerSecret
					svhost.AddRoute(route)
				default:
					vhost := p.dag.EnsureVirtualHost(host)
					vhost.AddRoute(route)
				}

				programmed = true
			}
		}
	}

	// Determine if any errors exist in conditions and set the "Accepted"
	// condition accordingly.
	switch len(routeAccessor.Conditions) {
	case 0:
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionTrue, status.ReasonValid, "Valid HTTPRoute")
	default:
		routeAccessor.AddCondition(gatewayapi_v1alpha2.ConditionRouteAccepted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
	}

	return programmed
}

// validateBackendRef verifies that the specified BackendRef is valid.
// Returns an error if not or the service found in the cache.
func (p *GatewayAPIProcessor) validateBackendRef(backendRef gatewayapi_v1alpha2.BackendRef, routeKind, routeNamespace string) (*Service, error) {
	if !(backendRef.Group == nil || *backendRef.Group == "") {
		return nil, fmt.Errorf("Spec.Rules.BackendRef.Group must be \"\"")
	}

	if !(backendRef.Kind != nil && *backendRef.Kind == "Service") {
		return nil, fmt.Errorf("Spec.Rules.BackendRef.Kind must be 'Service'")
	}

	if backendRef.Name == "" {
		return nil, fmt.Errorf("Spec.Rules.BackendRef.Name must be specified")
	}

	if backendRef.Port == nil {
		return nil, fmt.Errorf("Spec.Rules.BackendRef.Port must be specified")
	}

	// If the backend is in a different namespace than the route, then we need to
	// check for a ReferencePolicy that allows the reference.
	if backendRef.Namespace != nil && string(*backendRef.Namespace) != routeNamespace {
		if !p.validCrossNamespaceRef(
			crossNamespaceFrom{
				group:     string(gatewayapi_v1alpha2.GroupName),
				kind:      routeKind,
				namespace: routeNamespace,
			},
			crossNamespaceTo{
				group:     "",
				kind:      "Service",
				namespace: string(*backendRef.Namespace),
				name:      string(backendRef.Name),
			},
		) {
			return nil, fmt.Errorf("Spec.Rules.BackendRef.Namespace must match the route's namespace or be covered by a ReferencePolicy")
		}
	}

	var meta types.NamespacedName
	if backendRef.Namespace != nil {
		meta = types.NamespacedName{Name: string(backendRef.Name), Namespace: string(*backendRef.Namespace)}
	} else {
		meta = types.NamespacedName{Name: string(backendRef.Name), Namespace: routeNamespace}
	}

	// TODO: Refactor EnsureService to take an int32 so conversion to intstr is not needed.
	service, err := p.dag.EnsureService(meta, intstr.FromInt(int(*backendRef.Port)), p.source, p.EnableExternalNameService)
	if err != nil {
		return nil, fmt.Errorf("service %q is invalid: %s", meta.Name, err)
	}

	return service, nil
}

func gatewayPathMatchCondition(match *gatewayapi_v1alpha2.HTTPPathMatch, routeAccessor *status.RouteConditionsUpdate) (MatchCondition, bool) {
	if match == nil {
		return &PrefixMatchCondition{Prefix: "/"}, true
	}

	path := pointer.StringDeref(match.Value, "/")

	// If path match type is not defined, default to 'PathPrefix'.
	if match.Type == nil || *match.Type == gatewayapi_v1alpha2.PathMatchPathPrefix {
		if !strings.HasPrefix(path, "/") {
			routeAccessor.AddCondition(status.ConditionValidMatches, metav1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must start with '/'.")
			return nil, false
		}
		if strings.Contains(path, "//") {
			routeAccessor.AddCondition(status.ConditionValidMatches, metav1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must not contain consecutive '/' characters.")
			return nil, false
		}

		// As an optimization, if path is just "/", we can use
		// string prefix matching instead of segment prefix
		// matching which requires a regex.
		if path == "/" {
			return &PrefixMatchCondition{Prefix: path}, true
		}
		return &PrefixMatchCondition{Prefix: path, PrefixMatchType: PrefixMatchSegment}, true
	}

	if *match.Type == gatewayapi_v1alpha2.PathMatchExact {
		if !strings.HasPrefix(path, "/") {
			routeAccessor.AddCondition(status.ConditionValidMatches, metav1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must start with '/'.")
			return nil, false
		}
		if strings.Contains(path, "//") {
			routeAccessor.AddCondition(status.ConditionValidMatches, metav1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must not contain consecutive '/' characters.")
			return nil, false
		}

		return &ExactMatchCondition{Path: path}, true
	}

	routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonPathMatchType, "HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported.")
	return nil, false
}

func gatewayHeaderMatchConditions(matches []gatewayapi_v1alpha2.HTTPHeaderMatch) ([]HeaderMatchCondition, error) {
	var headerMatchConditions []HeaderMatchCondition

	for _, match := range matches {
		// HeaderMatchTypeExact is the default if not defined in the object.
		headerMatchType := HeaderMatchTypeExact
		if match.Type != nil {
			switch *match.Type {
			case gatewayapi_v1alpha2.HeaderMatchExact:
				headerMatchType = HeaderMatchTypeExact
			default:
				return nil, fmt.Errorf("HTTPRoute.Spec.Rules.HeaderMatch: Only Exact match type is supported")
			}
		}

		headerMatchConditions = append(headerMatchConditions, HeaderMatchCondition{MatchType: headerMatchType, Name: string(match.Name), Value: match.Value})
	}

	return headerMatchConditions, nil
}

// clusterRoutes builds a []*dag.Route for the supplied set of matchConditions, headerPolicy and backendRefs.
func (p *GatewayAPIProcessor) clusterRoutes(routeNamespace string, matchConditions []*matchConditions, headerPolicy *HeadersPolicy, backendRefs []gatewayapi_v1alpha2.HTTPBackendRef, routeAccessor *status.RouteConditionsUpdate) []*Route {
	if len(backendRefs) == 0 {
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
		return nil
	}

	var clusters []*Cluster

	// Validate the backend refs.
	totalWeight := uint32(0)
	for _, backendRef := range backendRefs {
		service, err := p.validateBackendRef(backendRef.BackendRef, KindHTTPRoute, routeNamespace)
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
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHTTPRouteFilterType, "HTTPRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier type is supported.")
			}
		}

		// Route defaults to a weight of "1" unless otherwise specified.
		routeWeight := uint32(1)
		if backendRef.Weight != nil {
			routeWeight = uint32(*backendRef.Weight)
		}

		// Keep track of all the weights for this set of backend refs. This will be
		// used later to understand if all the weights are set to zero.
		totalWeight += routeWeight

		// https://github.com/projectcontour/contour/issues/3593
		service.Weighted.Weight = routeWeight
		clusters = append(clusters, &Cluster{
			Upstream:             service,
			Weight:               routeWeight,
			Protocol:             service.Protocol,
			RequestHeadersPolicy: headerPolicy,
			TimeoutPolicy:        ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
		})
	}

	var routes []*Route

	// Per Gateway API: "Each match is independent,
	// i.e. this rule will be matched if any one of
	// the matches is satisfied." To implement this,
	// we create a separate route per match.
	for _, mc := range matchConditions {
		routes = append(routes, &Route{
			Clusters:              clusters,
			PathMatchCondition:    mc.path,
			HeaderMatchConditions: mc.headers,
			RequestHeadersPolicy:  headerPolicy,
		})
	}

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
	}

	return routes
}

// redirectRoutes builds a []*dag.Route for the supplied set of matchConditions, headerPolicy and redirect.
func (p *GatewayAPIProcessor) redirectRoutes(matchConditions []*matchConditions, headerPolicy *HeadersPolicy, redirect *gatewayapi_v1alpha2.HTTPRequestRedirectFilter) []*Route {
	var hostname string
	if redirect.Hostname != nil {
		hostname = string(*redirect.Hostname)
	}

	var portNumber uint32
	if redirect.Port != nil {
		portNumber = uint32(*redirect.Port)
	}

	var scheme string
	if redirect.Scheme != nil {
		scheme = *redirect.Scheme
	}

	var statusCode int
	if redirect.StatusCode != nil {
		statusCode = *redirect.StatusCode
	}

	var routes []*Route

	// Per Gateway API: "Each match is independent,
	// i.e. this rule will be matched if any one of
	// the matches is satisfied." To implement this,
	// we create a separate route per match.
	for _, mc := range matchConditions {
		routes = append(routes, &Route{
			Redirect: &Redirect{
				Hostname:   hostname,
				Scheme:     scheme,
				PortNumber: portNumber,
				StatusCode: statusCode,
			},
			PathMatchCondition:    mc.path,
			HeaderMatchConditions: mc.headers,
			RequestHeadersPolicy:  headerPolicy,
		})
	}

	return routes
}
