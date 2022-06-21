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
	"k8s.io/apimachinery/pkg/util/sets"
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
	path        MatchCondition
	headers     []HeaderMatchCondition
	queryParams []QueryParamMatchCondition
}

// Run translates Gateway API types into DAG objects and
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
		p.Info("GatewayClass not found in cache.")
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

	// Validate listener protocols, ports and hostnames and add conditions
	// for all invalid listeners.
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

	// Map routes to the listeners that they can attach to.
	var (
		httpRoutesToListeners = map[*gatewayapi_v1alpha2.HTTPRoute][]*listenerInfo{}
		tlsRoutesToListeners  = map[*gatewayapi_v1alpha2.TLSRoute][]*listenerInfo{}
	)

	for _, listener := range p.source.gateway.Spec.Listeners {
		httpRoutes, tlsRoutes, secret := p.computeListener(listener, gwAccessor, validateListenersResult)

		listenerInfo := &listenerInfo{
			listener: listener,
			secret:   secret,
		}

		for _, route := range httpRoutes {
			httpRoutesToListeners[route] = append(httpRoutesToListeners[route], listenerInfo)
		}

		for _, route := range tlsRoutes {
			tlsRoutesToListeners[route] = append(tlsRoutesToListeners[route], listenerInfo)
		}
	}

	// Keep track of the number of routes attached
	// to each Listener so we can set status properly.
	listenerAttachedRoutes := map[string]int{}

	// Compute each HTTPRoute for each Listener that it potentially
	// attaches to.
	for httpRoute, listeners := range httpRoutesToListeners {
		func() {
			routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(
				k8s.NamespacedNameOf(httpRoute),
				httpRoute.Generation,
				&gatewayapi_v1alpha2.HTTPRoute{},
				httpRoute.Status.Parents,
			)
			defer commit()

			// If the Gateway is invalid, set status on the route and we're done.
			if gatewayNotReadyCondition != nil {
				routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
				return
			}

			// Keep track of the number of intersecting hosts
			// between the route and all listeners so that we
			// can set the appropriate route condition if there
			// were none across all listeners.
			hostCount := 0

			for _, listener := range listeners {
				attached, hosts := p.computeHTTPRoute(httpRoute, routeAccessor, listener)

				if attached {
					listenerAttachedRoutes[string(listener.listener.Name)]++
				}

				hostCount += hosts.Len()
			}

			if hostCount == 0 {
				routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonNoIntersectingHostnames, "No intersecting hostnames were found between the listener and the route.")
			} else {
				// Determine if any errors exist in conditions and set the "Accepted"
				// condition accordingly.
				switch len(routeAccessor.Conditions) {
				case 0:
					routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionTrue, status.ReasonValid, "Valid HTTPRoute")
				default:
					routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
				}
			}
		}()
	}

	// Compute each TLSRoute for each Listener that it potentially
	// attaches to.
	for tlsRoute, listeners := range tlsRoutesToListeners {
		func() {
			routeAccessor, commit := p.dag.StatusCache.RouteConditionsAccessor(
				k8s.NamespacedNameOf(tlsRoute),
				tlsRoute.Generation,
				&gatewayapi_v1alpha2.TLSRoute{},
				tlsRoute.Status.Parents,
			)
			defer commit()

			// If the Gateway is invalid, set status on the route.
			if gatewayNotReadyCondition != nil {
				routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
				return
			}

			// Keep track of the number of intersecting hosts
			// between the route and all listeners so that we
			// can set the appropriate route condition if there
			// were none across all listeners.
			hostCount := 0

			for _, listener := range listeners {
				attached, hosts := p.computeTLSRoute(tlsRoute, routeAccessor, listener)

				if attached {
					listenerAttachedRoutes[string(listener.listener.Name)]++
				}

				hostCount += hosts.Len()
			}

			if hostCount == 0 {
				routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonNoIntersectingHostnames, "No intersecting hostnames were found between the listener and the route.")
			} else {
				// Determine if any errors exist in conditions and set the "Accepted"
				// condition accordingly.
				switch len(routeAccessor.Conditions) {
				case 0:
					routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionTrue, status.ReasonValid, "Valid TLSRoute")
				default:
					routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonErrorsExist, "Errors found, check other Conditions for details.")
				}
			}
		}()
	}

	for listenerName, attachedRoutes := range listenerAttachedRoutes {
		gwAccessor.SetListenerAttachedRoutes(listenerName, attachedRoutes)
	}

	p.computeGatewayConditions(gwAccessor, gatewayNotReadyCondition)
}

type listenerInfo struct {
	listener gatewayapi_v1alpha2.Listener
	secret   *Secret
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

// computeListener processes a Listener's spec, including TLS details,
// allowed routes, etc., and sets the appropriate conditions on it in
// the Gateway's .status.listeners. It returns lists of the HTTPRoutes
// and TLSRoutes that select the Listener and are allowed by it, as well
// as the TLS secret to use for the Listener (if any).
func (p *GatewayAPIProcessor) computeListener(listener gatewayapi_v1alpha2.Listener, gwAccessor *status.GatewayStatusUpdate, validateListenersResult gatewayapi.ValidateListenersResult) ([]*gatewayapi_v1alpha2.HTTPRoute, []*gatewayapi_v1alpha2.TLSRoute, *Secret) {
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

	// If the listener had an invalid protocol/port/hostname, we don't need to go
	// any further.
	if _, ok := validateListenersResult.InvalidListenerConditions[listener.Name]; ok {
		return nil, nil, nil
	}

	// Get a list of the route kinds that the listener accepts.
	listenerRouteKinds := p.getListenerRouteKinds(listener, gwAccessor)
	gwAccessor.SetListenerSupportedKinds(string(listener.Name), listenerRouteKinds)

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
			return nil, nil, nil
		}

		// Check for valid TLS configuration on the Gateway.
		if listenerSecret = p.validGatewayTLS(*listener.TLS, string(listener.Name), gwAccessor); listenerSecret == nil {
			// If TLS was configured on the Listener, but it's invalid, don't allow any
			// routes to be bound to this listener since it can't serve TLS traffic.
			return nil, nil, nil
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
			return nil, nil, nil
		}

		if listener.TLS.Mode != nil {
			switch *listener.TLS.Mode {
			case gatewayapi_v1alpha2.TLSModeTerminate:
				// Check for valid TLS configuration on the Gateway.
				if listenerSecret = p.validGatewayTLS(*listener.TLS, string(listener.Name), gwAccessor); listenerSecret == nil {
					// If TLS was configured on the Listener, but it's invalid, don't allow any
					// routes to be bound to this listener since it can't serve TLS traffic.
					return nil, nil, nil
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
					return nil, nil, nil
				}
			}
		}
	}

	var httpRoutes []*gatewayapi_v1alpha2.HTTPRoute
	var tlsRoutes []*gatewayapi_v1alpha2.TLSRoute

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

				// If the Listener allows the HTTPRoute, check to see if the HTTPRoute selects
				// the Gateway/Listener.
				if !routeSelectsGatewayListener(p.source.gateway, listener, route.Spec.ParentRefs, route.Namespace) {
					continue
				}

				httpRoutes = append(httpRoutes, route)
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

				// If the Listener allows the TLSRoute, check to see if the TLSRoute selects
				// the Gateway/Listener.
				if !routeSelectsGatewayListener(p.source.gateway, listener, route.Spec.ParentRefs, route.Namespace) {
					continue
				}

				tlsRoutes = append(tlsRoutes, route)
			}
		}
	}

	return httpRoutes, tlsRoutes, listenerSecret
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
	if !isSecretRef(certificateRef) {
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
	// check for a ReferencePolicy or ReferenceGrant that allows the reference.
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
				fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q namespace must match the Gateway's namespace or be covered by a ReferencePolicy/ReferenceGrant", certificateRef.Name),
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

	for _, referenceGrant := range p.source.referencegrants {
		// The ReferenceGrant must be defined in the namespace of
		// the "to" (the referent).
		if referenceGrant.Namespace != to.namespace {
			continue
		}

		// Check if the ReferenceGrant has a matching "from".
		var fromAllowed bool
		for _, refGrantFrom := range referenceGrant.Spec.From {
			if string(refGrantFrom.Namespace) == from.namespace && string(refGrantFrom.Group) == from.group && string(refGrantFrom.Kind) == from.kind {
				fromAllowed = true
				break
			}
		}
		if !fromAllowed {
			continue
		}

		// Check if the ReferenceGrant has a matching "to".
		var toAllowed bool
		for _, refGrantTo := range referenceGrant.Spec.To {
			if string(refGrantTo.Group) == to.group && string(refGrantTo.Kind) == to.kind && (refGrantTo.Name == nil || *refGrantTo.Name == "" || string(*refGrantTo.Name) == to.name) {
				toAllowed = true
				break
			}
		}
		if !toAllowed {
			continue
		}

		// If we got here, both the "from" and the "to" were allowed by this
		// reference grant.
		return true
	}

	// If we got here, no reference policy or reference grant allowed both the "from" and "to".
	return false
}

func isSecretRef(certificateRef gatewayapi_v1alpha2.SecretObjectReference) bool {
	return certificateRef.Group != nil && *certificateRef.Group == "" &&
		certificateRef.Kind != nil && *certificateRef.Kind == "Secret"
}

// computeHosts returns the set of hostnames to match for a route. Both the result
// and the error slice should be considered:
// 	- if the set of hostnames is non-empty, it should be used for matching (may be ["*"]).
//	- if the set of hostnames is empty, there was no intersection between the listener
//	  hostname and the route hostnames, and the route should be marked "Accepted: false".
//	- if the list of errors is non-empty, one or more hostnames was syntactically
//	  invalid and some condition should be added to the route. This shouldn't be
//	  possible because of kubebuilder+admission webhook validation but we're being
//    defensive here.
func (p *GatewayAPIProcessor) computeHosts(routeHostnames []gatewayapi_v1alpha2.Hostname, listenerHostname string) (sets.String, []error) {
	// The listener hostname is assumed to be valid because it's been run
	// through the `gatewayapi.ValidateListeners` logic, so we don't need
	// to validate it here.

	// No route hostnames specified: use the listener hostname if specified,
	// or else match all hostnames.
	if len(routeHostnames) == 0 {
		if len(listenerHostname) > 0 {
			return sets.NewString(listenerHostname), nil
		}

		return sets.NewString("*"), nil
	}

	hostnames := sets.NewString()
	var errs []error

	for i := range routeHostnames {
		routeHostname := string(routeHostnames[i])

		// If the route hostname is not valid, record an error and skip it.
		if err := gatewayapi.IsValidHostname(routeHostname); err != nil {
			errs = append(errs, err)
			continue
		}

		switch {
		// No listener hostname: use the route hostname.
		case len(listenerHostname) == 0:
			hostnames.Insert(routeHostname)

		// Listener hostname matches the route hostname: use it.
		case listenerHostname == routeHostname:
			hostnames.Insert(routeHostname)

		// Listener has a wildcard hostname: check if the route hostname matches.
		case strings.HasPrefix(listenerHostname, "*"):
			if hostnameMatchesWildcardHostname(routeHostname, listenerHostname) {
				hostnames.Insert(routeHostname)
			}

		// Route has a wildcard hostname: check if the listener hostname matches.
		case strings.HasPrefix(routeHostname, "*"):
			if hostnameMatchesWildcardHostname(listenerHostname, routeHostname) {
				hostnames.Insert(listenerHostname)
			}

		}
	}

	if len(hostnames) == 0 {
		return nil, errs
	}

	return hostnames, errs
}

// hostnameMatchesWildcardHostname returns true if hostname has the non-wildcard
// portion of wildcardHostname as a suffix, plus at least one DNS label matching the
// wildcard.
func hostnameMatchesWildcardHostname(hostname, wildcardHostname string) bool {
	if !strings.HasSuffix(hostname, strings.TrimPrefix(wildcardHostname, "*")) {
		return false
	}

	wildcardMatch := strings.TrimSuffix(hostname, strings.TrimPrefix(wildcardHostname, "*"))
	return len(wildcardMatch) > 0
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
func routeSelectsGatewayListener(gateway *gatewayapi_v1alpha2.Gateway, listener gatewayapi_v1alpha2.Listener, routeParentRefs []gatewayapi_v1alpha2.ParentReference, routeNamespace string) bool {
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
			if *ref.SectionName == listener.Name {
				return true
			}
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

func (p *GatewayAPIProcessor) computeTLSRoute(route *gatewayapi_v1alpha2.TLSRoute, routeAccessor *status.RouteConditionsUpdate, listener *listenerInfo) (bool, sets.String) {
	hosts, errs := p.computeHosts(route.Spec.Hostnames, gatewayapi.HostnameDeref(listener.listener.Hostname))
	for _, err := range errs {
		// The Gateway API spec does not indicate what to do if syntactically
		// invalid hostnames make it through, we're using our best judgment here.
		// Theoretically these should be prevented by the combination of kubebuilder
		// and admission webhook validations.
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
	}

	// If there were no intersections between the listener hostname and the
	// route hostnames, the route is not programmed for this listener.
	if len(hosts) == 0 {
		return false, nil
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

			service, cond := p.validateBackendRef(backendRef, KindTLSRoute, route.Namespace)
			if cond != nil {
				routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionType(cond.Type), cond.Status, status.RouteReasonType(cond.Reason), cond.Message)
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

			if listener.secret != nil {
				secure.Secret = listener.secret
			}

			secure.TCPProxy = &proxy

			programmed = true
		}
	}

	return programmed, hosts
}

func (p *GatewayAPIProcessor) computeHTTPRoute(route *gatewayapi_v1alpha2.HTTPRoute, routeAccessor *status.RouteConditionsUpdate, listener *listenerInfo) (bool, sets.String) {
	hosts, errs := p.computeHosts(route.Spec.Hostnames, gatewayapi.HostnameDeref(listener.listener.Hostname))
	for _, err := range errs {
		// The Gateway API spec does not indicate what to do if syntactically
		// invalid hostnames make it through, we're using our best judgment here.
		// Theoretically these should be prevented by the combination of kubebuilder
		// and admission webhook validations.
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
	}

	// If there were no intersections between the listener hostname and the
	// route hostnames, the route is not programmed for this listener.
	if len(hosts) == 0 {
		return false, nil
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
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHeaderMatchType, err.Error())
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

			queryParamMatches, err := gatewayQueryParamMatchConditions(match.QueryParams)
			if err != nil {
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonQueryParamMatchType, err.Error())
				continue
			}

			matchconditions = append(matchconditions, &matchConditions{
				path:        pathMatch,
				headers:     headerMatches,
				queryParams: queryParamMatches,
			})
		}

		// Process rule-level filters.
		var (
			headerPolicy       *HeadersPolicy
			headerModifierSeen bool
			redirect           *gatewayapi_v1alpha2.HTTPRequestRedirectFilter
			mirrorPolicy       *MirrorPolicy
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
			case gatewayapi_v1alpha2.HTTPRouteFilterRequestMirror:
				// Get the mirror filter if there is one. If there are more than one
				// mirror filters, "NotImplemented" condition on the Route is set to
				// status: True, with the "NotImplemented" reason.
				if mirrorPolicy != nil {
					routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonNotImplemented, "HTTPRoute.Spec.Rules.Filters: Only one mirror filter is supported.")
					continue
				}

				if filter.RequestMirror != nil {
					mirrorService, cond := p.validateBackendObjectRef(filter.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindHTTPRoute, route.Namespace)
					if cond != nil {
						routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionType(cond.Type), cond.Status, status.RouteReasonType(cond.Reason), cond.Message)
						continue
					}
					mirrorPolicy = &MirrorPolicy{
						Cluster: &Cluster{
							Upstream: mirrorService,
						},
					}
				}

			default:
				routeAccessor.AddCondition(status.ConditionNotImplemented, metav1.ConditionTrue, status.ReasonHTTPRouteFilterType,
					fmt.Sprintf("HTTPRoute.Spec.Rules.Filters: invalid type %q: only RequestHeaderModifier, RequestRedirect and RequestMirror are supported.", filter.Type))
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
			routes = p.clusterRoutes(route.Namespace, matchconditions, headerPolicy, mirrorPolicy, rule.BackendRefs, routeAccessor)
		}

		// Add each route to the relevant vhost(s)/svhosts(s).
		for host := range hosts {
			for _, route := range routes {
				switch {
				case listener.secret != nil:
					svhost := p.dag.EnsureSecureVirtualHost(host)
					svhost.Secret = listener.secret
					svhost.AddRoute(route)
				default:
					vhost := p.dag.EnsureVirtualHost(host)
					vhost.AddRoute(route)
				}

				programmed = true
			}
		}
	}

	return programmed, hosts
}

// validateBackendRef verifies that the specified BackendRef is valid.
// Returns a metav1.Condition for the route if any errors are detected.
func (p *GatewayAPIProcessor) validateBackendRef(backendRef gatewayapi_v1alpha2.BackendRef, routeKind, routeNamespace string) (*Service, *metav1.Condition) {
	return p.validateBackendObjectRef(backendRef.BackendObjectReference, "Spec.Rules.BackendRef", routeKind, routeNamespace)
}

// validateBackendObjectRef verifies that the specified BackendObjectReference
// is valid. Returns a metav1.Condition for the route if any errors are detected.
// As BackendObjectReference is used in multiple fields, the given field is used
// to build the message in metav1.Condition.
func (p *GatewayAPIProcessor) validateBackendObjectRef(backendObjectRef gatewayapi_v1alpha2.BackendObjectReference, field string, routeKind, routeNamespace string) (*Service, *metav1.Condition) {
	degraded := func(msg string) *metav1.Condition {
		return &metav1.Condition{
			Type:    string(gatewayapi_v1alpha2.RouteConditionResolvedRefs),
			Status:  metav1.ConditionFalse,
			Reason:  string(status.ReasonDegraded),
			Message: msg,
		}
	}

	if !(backendObjectRef.Group == nil || *backendObjectRef.Group == "") {
		return nil, degraded(fmt.Sprintf("%s.Group must be \"\"", field))
	}

	if !(backendObjectRef.Kind != nil && *backendObjectRef.Kind == "Service") {
		return nil, degraded(fmt.Sprintf("%s.Kind must be 'Service'", field))
	}

	if backendObjectRef.Name == "" {
		return nil, degraded(fmt.Sprintf("%s.Name must be specified", field))
	}

	if backendObjectRef.Port == nil {
		return nil, degraded(fmt.Sprintf("%s.Port must be specified", field))
	}

	// If the backend is in a different namespace than the route, then we need to
	// check for a ReferencePolicy or ReferenceGrant that allows the reference.
	if backendObjectRef.Namespace != nil && string(*backendObjectRef.Namespace) != routeNamespace {
		if !p.validCrossNamespaceRef(
			crossNamespaceFrom{
				group:     string(gatewayapi_v1alpha2.GroupName),
				kind:      routeKind,
				namespace: routeNamespace,
			},
			crossNamespaceTo{
				group:     "",
				kind:      "Service",
				namespace: string(*backendObjectRef.Namespace),
				name:      string(backendObjectRef.Name),
			},
		) {
			return nil, &metav1.Condition{
				Type:    string(gatewayapi_v1alpha2.RouteConditionResolvedRefs),
				Status:  metav1.ConditionFalse,
				Reason:  string(gatewayapi_v1alpha2.ListenerReasonRefNotPermitted),
				Message: fmt.Sprintf("%s.Namespace must match the route's namespace or be covered by a ReferencePolicy/ReferenceGrant", field),
			}
		}
	}

	var meta types.NamespacedName
	if backendObjectRef.Namespace != nil {
		meta = types.NamespacedName{Name: string(backendObjectRef.Name), Namespace: string(*backendObjectRef.Namespace)}
	} else {
		meta = types.NamespacedName{Name: string(backendObjectRef.Name), Namespace: routeNamespace}
	}

	// TODO: Refactor EnsureService to take an int32 so conversion to intstr is not needed.
	service, err := p.dag.EnsureService(meta, intstr.FromInt(int(*backendObjectRef.Port)), p.source, p.EnableExternalNameService)
	if err != nil {
		return nil, degraded(fmt.Sprintf("service %q is invalid: %s", meta.Name, err))
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
				return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.Headers: Only Exact match type is supported")
			}
		}

		headerMatchConditions = append(headerMatchConditions, HeaderMatchCondition{MatchType: headerMatchType, Name: string(match.Name), Value: match.Value})
	}

	return headerMatchConditions, nil
}

func gatewayQueryParamMatchConditions(matches []gatewayapi_v1alpha2.HTTPQueryParamMatch) ([]QueryParamMatchCondition, error) {
	var dagMatchConditions []QueryParamMatchCondition

	for _, match := range matches {
		// QueryParamMatchTypeExact is the default if not defined in the object.
		queryParamMatchType := QueryParamMatchTypeExact

		if match.Type != nil && *match.Type != gatewayapi_v1alpha2.QueryParamMatchExact {
			return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.QueryParams: Only Exact match type is supported")
		}

		dagMatchConditions = append(dagMatchConditions, QueryParamMatchCondition{
			MatchType: queryParamMatchType,
			Name:      match.Name,
			Value:     match.Value,
		})
	}

	return dagMatchConditions, nil
}

// clusterRoutes builds a []*dag.Route for the supplied set of matchConditions, headerPolicy and backendRefs.
func (p *GatewayAPIProcessor) clusterRoutes(routeNamespace string, matchConditions []*matchConditions, headerPolicy *HeadersPolicy, mirrorPolicy *MirrorPolicy, backendRefs []gatewayapi_v1alpha2.HTTPBackendRef, routeAccessor *status.RouteConditionsUpdate) []*Route {
	if len(backendRefs) == 0 {
		routeAccessor.AddCondition(status.ConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
		return nil
	}

	var clusters []*Cluster

	// Validate the backend refs.
	totalWeight := uint32(0)
	for _, backendRef := range backendRefs {
		service, cond := p.validateBackendRef(backendRef.BackendRef, KindHTTPRoute, routeNamespace)
		if cond != nil {
			routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionType(cond.Type), cond.Status, status.RouteReasonType(cond.Reason), cond.Message)
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
			Clusters:                  clusters,
			PathMatchCondition:        mc.path,
			HeaderMatchConditions:     mc.headers,
			QueryParamMatchConditions: mc.queryParams,
			RequestHeadersPolicy:      headerPolicy,
			MirrorPolicy:              mirrorPolicy,
		})
	}

	for _, route := range routes {
		// If there aren't any valid services, or the total weight of all of
		// them equal zero, then return 404 responses to the caller.
		if len(clusters) == 0 || totalWeight == 0 {
			// Configure a direct response HTTP status code of 404 so the
			// route still matches the configured conditions since the
			// service is missing or invalid.
			route.DirectResponse = &DirectResponse{
				StatusCode: http.StatusNotFound,
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
