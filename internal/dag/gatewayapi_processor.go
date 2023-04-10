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
	"regexp"
	"strings"
	"time"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/internal/status"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	KindHTTPRoute = "HTTPRoute"
	KindTLSRoute  = "TLSRoute"
	KindGRPCRoute = "GRPCRoute"
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

	var gatewayNotProgrammedCondition *metav1.Condition

	if !isAddressAssigned(p.source.gateway.Spec.Addresses, p.source.gateway.Status.Addresses) {
		// TODO(sk) resolve condition type-reason mismatch
		gatewayNotProgrammedCondition = &metav1.Condition{
			Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
			Status:  metav1.ConditionFalse,
			Reason:  string(gatewayapi_v1beta1.GatewayReasonAddressNotAssigned),
			Message: "None of the addresses in Spec.Addresses have been assigned to the Gateway",
		}
	}

	// Validate listener protocols, ports and hostnames and add conditions
	// for all invalid listeners.
	validateListenersResult := gatewayapi.ValidateListeners(p.source.gateway.Spec.Listeners)
	for name, cond := range validateListenersResult.InvalidListenerConditions {
		gwAccessor.AddListenerCondition(
			string(name),
			gatewayapi_v1beta1.ListenerConditionType(cond.Type),
			cond.Status,
			gatewayapi_v1beta1.ListenerConditionReason(cond.Reason),
			cond.Message,
		)
	}

	// Compute listeners and save a list of the valid/ready ones.
	var readyListeners []*listenerInfo

	for _, listener := range p.source.gateway.Spec.Listeners {
		if ready, listenerInfo := p.computeListener(listener, gwAccessor, validateListenersResult); ready {
			readyListeners = append(readyListeners, listenerInfo)
		}
	}

	// Keep track of the number of routes attached
	// to each Listener so we can set status properly.
	listenerAttachedRoutes := map[string]int{}

	// Process HTTPRoutes.
	for _, httpRoute := range p.source.httproutes {
		p.processRoute(KindHTTPRoute, httpRoute, httpRoute.Spec.ParentRefs, gatewayNotProgrammedCondition, readyListeners, listenerAttachedRoutes, &gatewayapi_v1beta1.HTTPRoute{})
	}

	// Process TLSRoutes.
	for _, tlsRoute := range p.source.tlsroutes {
		p.processRoute(KindTLSRoute, tlsRoute, tlsRoute.Spec.ParentRefs, gatewayNotProgrammedCondition, readyListeners, listenerAttachedRoutes, &gatewayapi_v1alpha2.TLSRoute{})
	}

	// Process GRPCRoutes.
	for _, grpcRoute := range p.source.grpcroutes {
		p.processRoute(KindGRPCRoute, grpcRoute, grpcRoute.Spec.ParentRefs, gatewayNotProgrammedCondition, readyListeners, listenerAttachedRoutes, &gatewayapi_v1alpha2.GRPCRoute{})

	}

	for listenerName, attachedRoutes := range listenerAttachedRoutes {
		gwAccessor.SetListenerAttachedRoutes(listenerName, attachedRoutes)
	}

	p.computeGatewayConditions(gwAccessor, gatewayNotProgrammedCondition)
}

func (p *GatewayAPIProcessor) processRoute(
	routeKind gatewayapi_v1beta1.Kind,
	route client.Object,
	parentRefs []gatewayapi_v1beta1.ParentReference,
	gatewayNotProgrammedCondition *metav1.Condition,
	readyListeners []*listenerInfo,
	listenerAttachedRoutes map[string]int,
	emptyResource client.Object,
) {
	routeStatus, commit := p.dag.StatusCache.RouteConditionsAccessor(
		k8s.NamespacedNameOf(route),
		route.GetGeneration(),
		emptyResource,
	)
	defer commit()

	for _, routeParentRef := range parentRefs {
		// If this parent ref is to a different Gateway, ignore it.
		if !gatewayapi.IsRefToGateway(routeParentRef, k8s.NamespacedNameOf(p.source.gateway)) {
			continue
		}

		routeParentStatus := routeStatus.StatusUpdateFor(routeParentRef)

		// If the Gateway is invalid, set status on the route and we're done.
		if gatewayNotProgrammedCondition != nil {
			routeParentStatus.AddCondition(gatewayapi_v1beta1.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
			continue
		}

		// Get the list of listeners that are (a) included by this parent ref, and
		// (b) allow the route (based on kind, namespace).
		allowedListeners := p.getListenersForRouteParentRef(routeParentRef, route.GetNamespace(), routeKind, readyListeners, routeParentStatus)
		if len(allowedListeners) == 0 {
			p.resolveRouteRefs(route, routeParentStatus)
		}

		// Keep track of the number of intersecting hosts
		// between the route and all allowed listeners for
		// this parent ref so that we can set the appropriate
		// route parent status condition if there were none.
		hostCount := 0

		for _, listener := range allowedListeners {
			var routeHostnames []gatewayapi_v1beta1.Hostname

			switch route := route.(type) {
			case *gatewayapi_v1beta1.HTTPRoute:
				routeHostnames = route.Spec.Hostnames
			case *gatewayapi_v1alpha2.TLSRoute:
				routeHostnames = route.Spec.Hostnames
			case *gatewayapi_v1alpha2.GRPCRoute:
				routeHostnames = route.Spec.Hostnames
			}

			hosts, errs := p.computeHosts(routeHostnames, gatewayapi.HostnameDeref(listener.listener.Hostname))
			for _, err := range errs {
				// The Gateway API spec does not indicate what to do if syntactically
				// invalid hostnames make it through, we're using our best judgment here.
				// Theoretically these should be prevented by the combination of kubebuilder
				// and admission webhook validations.
				routeParentStatus.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, err.Error())
			}

			// If there were no intersections between the listener hostname and the
			// route hostnames, the route is not programmed for this listener.
			if len(hosts) == 0 {
				continue
			}

			var attached bool

			switch route := route.(type) {
			case *gatewayapi_v1beta1.HTTPRoute:
				attached = p.computeHTTPRouteForListener(route, routeParentStatus, listener, hosts)
			case *gatewayapi_v1alpha2.TLSRoute:
				attached = p.computeTLSRouteForListener(route, routeParentStatus, listener, hosts)
			case *gatewayapi_v1alpha2.GRPCRoute:
				attached = p.computeGRPCRouteForListener(route, routeParentStatus, listener, hosts)
			}

			if attached {
				listenerAttachedRoutes[string(listener.listener.Name)]++
			}

			hostCount += hosts.Len()
		}

		if hostCount == 0 && !routeParentStatus.ConditionExists(gatewayapi_v1beta1.RouteConditionAccepted) {
			routeParentStatus.AddCondition(
				gatewayapi_v1beta1.RouteConditionAccepted,
				metav1.ConditionFalse,
				gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname,
				"No intersecting hostnames were found between the listener and the route.",
			)
		}

		// Check for an existing "ResolvedRefs" condition, add one if one does
		// not already exist.
		if !routeParentStatus.ConditionExists(gatewayapi_v1beta1.RouteConditionResolvedRefs) {
			routeParentStatus.AddCondition(
				gatewayapi_v1beta1.RouteConditionResolvedRefs,
				metav1.ConditionTrue,
				gatewayapi_v1beta1.RouteReasonResolvedRefs,
				"References resolved")
		}

		// Check for an existing "Accepted" condition, add one if one does
		// not already exist.
		if !routeParentStatus.ConditionExists(gatewayapi_v1beta1.RouteConditionAccepted) {
			routeParentStatus.AddCondition(
				gatewayapi_v1beta1.RouteConditionAccepted,
				metav1.ConditionTrue,
				gatewayapi_v1beta1.RouteReasonAccepted,
				fmt.Sprintf("Accepted %s", routeKind),
			)
		}
	}
}

func (p *GatewayAPIProcessor) getListenersForRouteParentRef(
	routeParentRef gatewayapi_v1beta1.ParentReference,
	routeNamespace string,
	routeKind gatewayapi_v1beta1.Kind,
	validListeners []*listenerInfo,
	routeParentStatusAccessor *status.RouteParentStatusUpdate,
) []*listenerInfo {

	// Find the set of valid listeners that are relevant given this
	// parent ref (either all of them, if the ref is to the entire
	// gateway, or one of them, if the ref is to a specific listener,
	// or none of them, if the listener(s) the ref targets are invalid).
	var selectedListeners []*listenerInfo
	for _, validListener := range validListeners {
		// We've already verified the parent ref is for this Gateway,
		// now check if it has a listener name and port specified.
		// Both need to match the listener if specified.
		if (routeParentRef.SectionName == nil || *routeParentRef.SectionName == validListener.listener.Name) &&
			(routeParentRef.Port == nil || *routeParentRef.Port == validListener.listener.Port) {
			selectedListeners = append(selectedListeners, validListener)
		}
	}

	if len(selectedListeners) == 0 {
		routeParentStatusAccessor.AddCondition(
			gatewayapi_v1beta1.RouteConditionAccepted,
			metav1.ConditionFalse,
			gatewayapi_v1beta1.RouteReasonNoMatchingParent,
			"No listeners match this parent ref",
		)
		return nil
	}

	// Now find the subset of those listeners that allow this route
	// to select them, based on route kind and namespace.
	var allowedListeners []*listenerInfo
	for _, selectedListener := range selectedListeners {
		// Check if the listener allows routes of this kind
		if !selectedListener.AllowsKind(routeKind) {
			continue
		}

		// Check if the route is in a namespace that the listener allows.
		if !p.namespaceMatches(selectedListener.listener.AllowedRoutes.Namespaces, selectedListener.namespaceSelector, routeNamespace) {
			continue
		}

		allowedListeners = append(allowedListeners, selectedListener)
	}

	if len(allowedListeners) == 0 {
		routeParentStatusAccessor.AddCondition(
			gatewayapi_v1beta1.RouteConditionAccepted,
			metav1.ConditionFalse,
			gatewayapi_v1beta1.RouteReasonNotAllowedByListeners,
			"No listeners included by this parent ref allowed this attachment.",
		)
		return nil
	}

	return allowedListeners
}

type listenerInfo struct {
	listener          gatewayapi_v1beta1.Listener
	allowedKinds      []gatewayapi_v1beta1.Kind
	namespaceSelector labels.Selector
	tlsSecret         *Secret
}

func (l *listenerInfo) AllowsKind(kind gatewayapi_v1beta1.Kind) bool {
	for _, allowedKind := range l.allowedKinds {
		if allowedKind == kind {
			return true
		}
	}

	return false
}

// isAddressAssigned returns true if either there are no addresses requested in specAddresses,
// or if at least one address from specAddresses appears in statusAddresses.
func isAddressAssigned(specAddresses, statusAddresses []gatewayapi_v1beta1.GatewayAddress) bool {
	if len(specAddresses) == 0 {
		return true
	}

	for _, specAddress := range specAddresses {
		for _, statusAddress := range statusAddresses {
			// Types must match
			if ref.Val(specAddress.Type, gatewayapi_v1beta1.IPAddressType) != ref.Val(statusAddress.Type, gatewayapi_v1beta1.IPAddressType) {
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

// computeListener processes a Listener's spec, including TLS details,
// allowed routes, etc., and sets the appropriate conditions on it in
// the Gateway's .status.listeners. It returns a listenerInfo struct with
// the allowed route kinds and TLS secret (if any).
func (p *GatewayAPIProcessor) computeListener(
	listener gatewayapi_v1beta1.Listener,
	gwAccessor *status.GatewayStatusUpdate,
	validateListenersResult gatewayapi.ValidateListenersResult,
) (bool, *listenerInfo) {

	addInvalidListenerCondition := func(msg string) {
		gwAccessor.AddListenerCondition(
			string(listener.Name),
			gatewayapi_v1beta1.ListenerConditionProgrammed,
			metav1.ConditionFalse,
			gatewayapi_v1beta1.ListenerReasonInvalid,
			msg,
		)
	}
	// set the listener's "Programmed" condition based on whether we've
	// added any other conditions for the listener. The assumption
	// here is that if another condition is set, the listener is
	// invalid/not programmed.
	defer func() {
		listenerStatus := gwAccessor.ListenerStatus[string(listener.Name)]

		if listenerStatus == nil || len(listenerStatus.Conditions) == 0 {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1beta1.ListenerConditionProgrammed,
				metav1.ConditionTrue,
				gatewayapi_v1beta1.ListenerReasonProgrammed,
				"Valid listener",
			)
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1beta1.ListenerConditionAccepted,
				metav1.ConditionTrue,
				gatewayapi_v1beta1.ListenerReasonAccepted,
				"Listener accepted",
			)
		} else {
			programmedConditionExists := false
			acceptedConditionExists := false
			for _, cond := range listenerStatus.Conditions {
				if cond.Type == string(gatewayapi_v1beta1.ListenerConditionProgrammed) {
					programmedConditionExists = true
				}
				if cond.Type == string(gatewayapi_v1beta1.ListenerConditionAccepted) {
					acceptedConditionExists = true
				}
			}

			// Only set the Programmed or Accepted conditions if
			// they don't already exist in the status update, since
			// if they do exist, they will contain more specific
			// information in the reason, message, etc.
			if !programmedConditionExists {
				addInvalidListenerCondition("Invalid listener, see other listener conditions for details")
			}
			// Accepted condition is always true for now if not
			// explicitly set.
			if !acceptedConditionExists {
				gwAccessor.AddListenerCondition(
					string(listener.Name),
					gatewayapi_v1beta1.ListenerConditionAccepted,
					metav1.ConditionTrue,
					gatewayapi_v1beta1.ListenerReasonAccepted,
					"Listener accepted",
				)
			}
		}
	}()

	// If the listener had an invalid protocol/port/hostname, we don't need to go
	// any further.
	if _, ok := validateListenersResult.InvalidListenerConditions[listener.Name]; ok {
		return false, nil
	}

	// Get a list of the route kinds that the listener accepts.
	listenerRouteKinds := p.getListenerRouteKinds(listener, gwAccessor)
	gwAccessor.SetListenerSupportedKinds(string(listener.Name), listenerRouteKinds)

	var selector labels.Selector

	if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces != nil &&
		listener.AllowedRoutes.Namespaces.From != nil && *listener.AllowedRoutes.Namespaces.From == gatewayapi_v1beta1.NamespacesFromSelector {

		if listener.AllowedRoutes.Namespaces.Selector == nil {
			addInvalidListenerCondition("Listener.AllowedRoutes.Namespaces.Selector is required when Listener.AllowedRoutes.Namespaces.From is set to \"Selector\".")
			return false, nil
		}

		if len(listener.AllowedRoutes.Namespaces.Selector.MatchExpressions)+len(listener.AllowedRoutes.Namespaces.Selector.MatchLabels) == 0 {
			addInvalidListenerCondition("Listener.AllowedRoutes.Namespaces.Selector must specify at least one MatchLabel or MatchExpression.")
			return false, nil
		}

		var err error
		selector, err = metav1.LabelSelectorAsSelector(listener.AllowedRoutes.Namespaces.Selector)
		if err != nil {
			addInvalidListenerCondition(fmt.Sprintf("Error parsing Listener.AllowedRoutes.Namespaces.Selector: %v.", err))
			return false, nil
		}
	}

	var listenerSecret *Secret

	// Validate TLS details for HTTPS/TLS protocol listeners.
	switch listener.Protocol {
	case gatewayapi_v1beta1.HTTPSProtocolType:
		// The HTTPS protocol is used for HTTP traffic encrypted with TLS,
		// which is to be TLS-terminated at the proxy and then routed to
		// backends using HTTPRoutes.

		if listener.TLS == nil {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS is required when protocol is %q.", listener.Protocol))
			return false, nil
		}

		if listener.TLS.Mode != nil && *listener.TLS.Mode != gatewayapi_v1beta1.TLSModeTerminate {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS.Mode must be %q when protocol is %q.", gatewayapi_v1beta1.TLSModeTerminate, listener.Protocol))
			return false, nil
		}

		// Resolve the TLS secret.
		if listenerSecret = p.resolveListenerSecret(listener.TLS.CertificateRefs, string(listener.Name), gwAccessor); listenerSecret == nil {
			// If TLS was configured on the Listener, but the secret ref is invalid, don't allow any
			// routes to be bound to this listener since it can't serve TLS traffic.
			return false, nil
		}
	case gatewayapi_v1beta1.TLSProtocolType:
		// The TLS protocol is used for TCP traffic encrypted with TLS.
		// Gateway API allows TLS to be either terminated at the proxy
		// or passed through to the backend, but the former requires using
		// TCPRoute to route traffic since the underlying protocol is TCP
		// not HTTP, which Contour doesn't support. Therefore, we only
		// support "Passthrough" with the TLS protocol, which requires
		// the use of TLSRoute to route to backends since the traffic is
		// still encrypted.

		if listener.TLS == nil {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS is required when protocol is %q.", listener.Protocol))
			return false, nil
		}

		if listener.TLS.Mode == nil || *listener.TLS.Mode != gatewayapi_v1beta1.TLSModePassthrough {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS.Mode must be %q when protocol is %q.", gatewayapi_v1beta1.TLSModePassthrough, listener.Protocol))
			return false, nil
		}

		if len(listener.TLS.CertificateRefs) != 0 {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS.CertificateRefs cannot be defined when Listener.TLS.Mode is %q.", gatewayapi_v1beta1.TLSModePassthrough))
			return false, nil
		}
	}

	return true, &listenerInfo{
		listener:          listener,
		allowedKinds:      listenerRouteKinds,
		tlsSecret:         listenerSecret,
		namespaceSelector: selector,
	}
}

// getListenerRouteKinds gets a list of the valid route kinds that
// the listener accepts.
func (p *GatewayAPIProcessor) getListenerRouteKinds(listener gatewayapi_v1beta1.Listener, gwAccessor *status.GatewayStatusUpdate) []gatewayapi_v1beta1.Kind {
	// None specified on the listener: return the default based on
	// the listener's protocol.
	if len(listener.AllowedRoutes.Kinds) == 0 {
		switch listener.Protocol {
		case gatewayapi_v1beta1.HTTPProtocolType:
			return []gatewayapi_v1beta1.Kind{KindHTTPRoute, KindGRPCRoute}
		case gatewayapi_v1beta1.HTTPSProtocolType:
			return []gatewayapi_v1beta1.Kind{KindHTTPRoute, KindGRPCRoute}
		case gatewayapi_v1beta1.TLSProtocolType:
			return []gatewayapi_v1beta1.Kind{KindTLSRoute}
		}
	}

	var routeKinds []gatewayapi_v1beta1.Kind

	for _, routeKind := range listener.AllowedRoutes.Kinds {
		if routeKind.Group != nil && *routeKind.Group != gatewayapi_v1beta1.GroupName {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1beta1.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1beta1.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("Group %q is not supported, group must be %q", *routeKind.Group, gatewayapi_v1beta1.GroupName),
			)
			continue
		}
		if routeKind.Kind != KindHTTPRoute && routeKind.Kind != KindTLSRoute && routeKind.Kind != KindGRPCRoute {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1beta1.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1beta1.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("Kind %q is not supported, kind must be %q or %q or %q", routeKind.Kind, KindHTTPRoute, KindTLSRoute, KindGRPCRoute),
			)
			continue
		}
		if routeKind.Kind == KindTLSRoute && listener.Protocol != gatewayapi_v1beta1.TLSProtocolType {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1beta1.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1beta1.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("TLSRoutes are incompatible with listener protocol %q", listener.Protocol),
			)
			continue
		}

		routeKinds = append(routeKinds, routeKind.Kind)
	}

	return routeKinds
}

// resolveListenerSecret validates and resolves a Listener TLS secret
// from a given list of certificateRefs. There must be exactly one
// certificate ref, to a v1.Secret, that exists, is allowed to be referenced
// based on namespace and ReferenceGrants, and is a valid TLS secret.
// Conditions are set if any of these requirements are not met.
func (p *GatewayAPIProcessor) resolveListenerSecret(certificateRefs []gatewayapi_v1beta1.SecretObjectReference, listenerName string, gwAccessor *status.GatewayStatusUpdate) *Secret {
	if len(certificateRefs) != 1 {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1beta1.ListenerConditionProgrammed,
			metav1.ConditionFalse,
			gatewayapi_v1beta1.ListenerReasonInvalid,
			"Listener.TLS.CertificateRefs must contain exactly one entry",
		)
		return nil
	}

	certificateRef := certificateRefs[0]

	// Validate a v1.Secret is referenced which can be kind: secret & group: core.
	// ref: https://github.com/kubernetes-sigs/gateway-api/pull/562
	if !isSecretRef(certificateRef) {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1beta1.ListenerConditionResolvedRefs,
			metav1.ConditionFalse,
			gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef,
			fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q must contain a reference to a core.Secret", certificateRef.Name),
		)
		return nil
	}

	// If the secret is in a different namespace than the gateway, then we need to
	// check for a ReferenceGrant that allows the reference.
	if certificateRef.Namespace != nil && string(*certificateRef.Namespace) != p.source.gateway.Namespace {
		if !p.validCrossNamespaceRef(
			crossNamespaceFrom{
				group:     gatewayapi_v1beta1.GroupName,
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
				gatewayapi_v1beta1.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1beta1.ListenerReasonRefNotPermitted,
				fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q namespace must match the Gateway's namespace or be covered by a ReferenceGrant", certificateRef.Name),
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

	listenerSecret, err := p.source.LookupTLSSecret(meta)
	if err != nil {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1beta1.ListenerConditionResolvedRefs,
			metav1.ConditionFalse,
			gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef,
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

func isSecretRef(certificateRef gatewayapi_v1beta1.SecretObjectReference) bool {
	return certificateRef.Group != nil && *certificateRef.Group == "" &&
		certificateRef.Kind != nil && *certificateRef.Kind == "Secret"
}

// computeHosts returns the set of hostnames to match for a route. Both the result
// and the error slice should be considered:
//   - if the set of hostnames is non-empty, it should be used for matching (may be ["*"]).
//   - if the set of hostnames is empty, there was no intersection between the listener
//     hostname and the route hostnames, and the route should be marked "Accepted: false".
//   - if the list of errors is non-empty, one or more hostnames was syntactically
//     invalid and some condition should be added to the route. This shouldn't be
//     possible because of kubebuilder+admission webhook validation but we're being
//     defensive here.
func (p *GatewayAPIProcessor) computeHosts(routeHostnames []gatewayapi_v1beta1.Hostname, listenerHostname string) (sets.Set[string], []error) {
	// The listener hostname is assumed to be valid because it's been run
	// through the `gatewayapi.ValidateListeners` logic, so we don't need
	// to validate it here.

	// No route hostnames specified: use the listener hostname if specified,
	// or else match all hostnames.
	if len(routeHostnames) == 0 {
		if len(listenerHostname) > 0 {
			return sets.New(listenerHostname), nil
		}

		return sets.New("*"), nil
	}

	hostnames := sets.New[string]()
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

// namespaceMatches returns true if namespaces allows
// the provided route namespace.
func (p *GatewayAPIProcessor) namespaceMatches(namespaces *gatewayapi_v1beta1.RouteNamespaces, namespaceSelector labels.Selector, routeNamespace string) bool {
	// From indicates where Routes will be selected for this Gateway.
	// Possible values are:
	//   * All: Routes in all namespaces may be used by this Gateway.
	//   * Selector: Routes in namespaces selected by the selector may be used by
	//     this Gateway.
	//   * Same: Only Routes in the same namespace may be used by this Gateway.

	if namespaces == nil || namespaces.From == nil {
		return true
	}

	switch *namespaces.From {
	case gatewayapi_v1beta1.NamespacesFromAll:
		return true
	case gatewayapi_v1beta1.NamespacesFromSame:
		return p.source.gateway.Namespace == routeNamespace
	case gatewayapi_v1beta1.NamespacesFromSelector:
		// Look up the route's namespace in the list of cached namespaces.
		if ns := p.source.namespaces[routeNamespace]; ns != nil {

			// Check that the route's namespace is included in the Gateway's
			// namespace selector.
			return namespaceSelector.Matches(labels.Set(ns.Labels))
		}
	}

	return true
}

func (p *GatewayAPIProcessor) computeGatewayConditions(gwAccessor *status.GatewayStatusUpdate, gatewayNotProgrammedCondition *metav1.Condition) {
	// If Contour's running, the Gateway is considered accepted.
	gwAccessor.AddCondition(
		gatewayapi_v1beta1.GatewayConditionAccepted,
		metav1.ConditionTrue,
		gatewayapi_v1beta1.GatewayReasonAccepted,
		"Gateway is accepted",
	)

	switch {
	case gatewayNotProgrammedCondition != nil:
		gwAccessor.AddCondition(
			gatewayapi_v1beta1.GatewayConditionType(gatewayNotProgrammedCondition.Type),
			gatewayNotProgrammedCondition.Status,
			gatewayapi_v1beta1.GatewayConditionReason(gatewayNotProgrammedCondition.Reason),
			gatewayNotProgrammedCondition.Message,
		)
	default:
		// Check for any listeners with a Programmed: false condition.
		allListenersProgrammed := true
		for _, ls := range gwAccessor.ListenerStatus {
			if ls == nil {
				continue
			}

			for _, cond := range ls.Conditions {
				if cond.Type == string(gatewayapi_v1beta1.ListenerConditionProgrammed) && cond.Status == metav1.ConditionFalse {
					allListenersProgrammed = false
					break
				}
			}

			if !allListenersProgrammed {
				break
			}
		}

		if !allListenersProgrammed {
			// If we have invalid listeners, set Programmed=false.
			// TODO(sk) resolve condition type-reason mismatch
			gwAccessor.AddCondition(gatewayapi_v1beta1.GatewayConditionProgrammed, metav1.ConditionFalse, gatewayapi_v1beta1.GatewayReasonListenersNotValid, "Listeners are not valid")
		} else {
			// Otherwise, Programmed=true.
			gwAccessor.AddCondition(gatewayapi_v1beta1.GatewayConditionProgrammed, metav1.ConditionTrue, gatewayapi_v1beta1.GatewayReasonProgrammed, status.MessageValidGateway)
		}
	}
}

func (p *GatewayAPIProcessor) computeTLSRouteForListener(route *gatewayapi_v1alpha2.TLSRoute, routeAccessor *status.RouteParentStatusUpdate, listener *listenerInfo, hosts sets.Set[string]) bool {
	var programmed bool
	for _, rule := range route.Spec.Rules {
		if len(rule.BackendRefs) == 0 {
			routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
			continue
		}

		var proxy TCPProxy
		var totalWeight uint32

		for _, backendRef := range rule.BackendRefs {

			service, cond := p.validateBackendRef(backendRef, KindTLSRoute, route.Namespace)
			if cond != nil {
				routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
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
			secure := p.dag.EnsureSecureVirtualHost(HTTPS_LISTENER_NAME, host)

			if listener.tlsSecret != nil {
				secure.Secret = listener.tlsSecret
			}

			secure.TCPProxy = &proxy

			programmed = true
		}
	}

	return programmed
}

// Resolve route references for a route and do not program any routes.
func (p *GatewayAPIProcessor) resolveRouteRefs(route interface{}, routeAccessor *status.RouteParentStatusUpdate) {
	switch route := route.(type) {
	case *gatewayapi_v1beta1.HTTPRoute:
		for _, r := range route.Spec.Rules {
			for _, f := range r.Filters {
				if f.Type == gatewayapi_v1beta1.HTTPRouteFilterRequestMirror && f.RequestMirror != nil {
					_, cond := p.validateBackendObjectRef(f.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindHTTPRoute, route.Namespace)
					if cond != nil {
						routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
					}
				}
			}

			// TODO: validate filter extension refs if they become relevant

			for _, br := range r.BackendRefs {
				_, cond := p.validateBackendRef(br.BackendRef, KindHTTPRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
				}

				// RequestMirror filter is not supported so we don't check it here

				// TODO: validate filter extension refs if they become relevant
			}
		}
	case *gatewayapi_v1alpha2.TLSRoute:
		for _, r := range route.Spec.Rules {
			for _, b := range r.BackendRefs {
				_, cond := p.validateBackendRef(b, KindTLSRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
				}
			}
		}
	case *gatewayapi_v1alpha2.GRPCRoute:
		for _, r := range route.Spec.Rules {
			for _, f := range r.Filters {
				if f.Type == gatewayapi_v1alpha2.GRPCRouteFilterRequestMirror && f.RequestMirror != nil {
					_, cond := p.validateBackendObjectRef(f.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindGRPCRoute, route.Namespace)
					if cond != nil {
						routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
					}
				}
			}

			// TODO: validate filter extension refs if they become relevant

			for _, br := range r.BackendRefs {
				_, cond := p.validateBackendRef(br.BackendRef, KindGRPCRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
				}

				// RequestMirror filter is not supported so we don't check it here

				// TODO: validate filter extension refs if they become relevant
			}
		}
	}
}

func (p *GatewayAPIProcessor) computeHTTPRouteForListener(route *gatewayapi_v1beta1.HTTPRoute, routeAccessor *status.RouteParentStatusUpdate, listener *listenerInfo, hosts sets.Set[string]) bool {
	var programmed bool
	for ruleIndex, rule := range route.Spec.Rules {
		// Get match conditions for the rule.
		var matchconditions []*matchConditions
		for _, match := range rule.Matches {
			pathMatch, ok := gatewayPathMatchCondition(match.Path, routeAccessor)
			if !ok {
				continue
			}

			headerMatches, err := gatewayHeaderMatchConditions(match.Headers)
			if err != nil {
				routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionAccepted, metav1.ConditionFalse, gatewayapi_v1beta1.RouteReasonUnsupportedValue, err.Error())
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
				routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionAccepted, metav1.ConditionFalse, gatewayapi_v1beta1.RouteReasonUnsupportedValue, err.Error())
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
			requestHeaderPolicy  *HeadersPolicy
			responseHeaderPolicy *HeadersPolicy
			redirect             *Redirect
			mirrorPolicy         *MirrorPolicy
			pathRewritePolicy    *PathRewritePolicy
			urlRewriteHostname   string
		)

		// Per Gateway API docs: "Specifying a core filter multiple times
		// has unspecified or implementation-specific conformance." Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range rule.Filters {
			switch filter.Type {
			case gatewayapi_v1beta1.HTTPRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || requestHeaderPolicy != nil {
					continue
				}

				var err error
				requestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1beta1.HTTPRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || responseHeaderPolicy != nil {
					continue
				}

				var err error
				responseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			case gatewayapi_v1beta1.HTTPRouteFilterRequestRedirect:
				if filter.RequestRedirect == nil || redirect != nil {
					continue
				}

				var hostname string
				if filter.RequestRedirect.Hostname != nil {
					hostname = string(*filter.RequestRedirect.Hostname)
				}

				var portNumber uint32
				if filter.RequestRedirect.Port != nil {
					portNumber = uint32(*filter.RequestRedirect.Port)
				}

				var scheme string
				if filter.RequestRedirect.Scheme != nil {
					scheme = *filter.RequestRedirect.Scheme
				}

				var statusCode int
				if filter.RequestRedirect.StatusCode != nil {
					statusCode = *filter.RequestRedirect.StatusCode
				}

				var pathRewritePolicy *PathRewritePolicy

				if filter.RequestRedirect.Path != nil {
					var prefixRewrite, fullPathRewrite string

					switch filter.RequestRedirect.Path.Type {
					case gatewayapi_v1beta1.PrefixMatchHTTPPathModifier:
						if filter.RequestRedirect.Path.ReplacePrefixMatch == nil || len(*filter.RequestRedirect.Path.ReplacePrefixMatch) == 0 {
							prefixRewrite = "/"
						} else {
							prefixRewrite = *filter.RequestRedirect.Path.ReplacePrefixMatch
						}
					case gatewayapi_v1beta1.FullPathHTTPPathModifier:
						if filter.RequestRedirect.Path.ReplaceFullPath == nil || len(*filter.RequestRedirect.Path.ReplaceFullPath) == 0 {
							fullPathRewrite = "/"
						} else {
							fullPathRewrite = *filter.RequestRedirect.Path.ReplaceFullPath
						}
					default:
						routeAccessor.AddCondition(
							gatewayapi_v1beta1.RouteConditionAccepted,
							metav1.ConditionFalse,
							gatewayapi_v1beta1.RouteReasonUnsupportedValue,
							fmt.Sprintf("HTTPRoute.Spec.Rules.Filters.RequestRedirect.Path.Type: invalid type %q: only ReplacePrefixMatch and ReplaceFullPath are supported.", filter.RequestRedirect.Path.Type),
						)
						continue
					}

					pathRewritePolicy = &PathRewritePolicy{
						PrefixRewrite:   prefixRewrite,
						FullPathRewrite: fullPathRewrite,
					}
				}

				redirect = &Redirect{
					Hostname:          hostname,
					PortNumber:        portNumber,
					Scheme:            scheme,
					StatusCode:        statusCode,
					PathRewritePolicy: pathRewritePolicy,
				}
			case gatewayapi_v1beta1.HTTPRouteFilterRequestMirror:
				if filter.RequestMirror == nil || mirrorPolicy != nil {
					continue
				}

				mirrorService, cond := p.validateBackendObjectRef(filter.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindHTTPRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
					continue
				}
				mirrorPolicy = &MirrorPolicy{
					Cluster: &Cluster{
						Upstream: mirrorService,
					},
				}
			case gatewayapi_v1beta1.HTTPRouteFilterURLRewrite:
				if filter.URLRewrite == nil || pathRewritePolicy != nil {
					continue
				}

				if filter.URLRewrite.Hostname != nil {
					urlRewriteHostname = string(*filter.URLRewrite.Hostname)
				}

				if filter.URLRewrite.Path == nil {
					continue
				}

				var prefixRewrite, fullPathRewrite string

				switch filter.URLRewrite.Path.Type {
				case gatewayapi_v1beta1.PrefixMatchHTTPPathModifier:
					if filter.URLRewrite.Path.ReplacePrefixMatch == nil || len(*filter.URLRewrite.Path.ReplacePrefixMatch) == 0 {
						prefixRewrite = "/"
					} else {
						prefixRewrite = *filter.URLRewrite.Path.ReplacePrefixMatch
					}
				case gatewayapi_v1beta1.FullPathHTTPPathModifier:
					if filter.URLRewrite.Path.ReplaceFullPath == nil || len(*filter.URLRewrite.Path.ReplaceFullPath) == 0 {
						fullPathRewrite = "/"
					} else {
						fullPathRewrite = *filter.URLRewrite.Path.ReplaceFullPath
					}
				default:
					routeAccessor.AddCondition(
						gatewayapi_v1beta1.RouteConditionAccepted,
						metav1.ConditionFalse,
						gatewayapi_v1beta1.RouteReasonUnsupportedValue,
						fmt.Sprintf("HTTPRoute.Spec.Rules.Filters.URLRewrite.Path.Type: invalid type %q: only ReplacePrefixMatch and ReplaceFullPath are supported.", filter.URLRewrite.Path.Type),
					)
					continue
				}

				pathRewritePolicy = &PathRewritePolicy{
					PrefixRewrite:   prefixRewrite,
					FullPathRewrite: fullPathRewrite,
				}
			default:
				routeAccessor.AddCondition(
					gatewayapi_v1beta1.RouteConditionAccepted,
					metav1.ConditionFalse,
					gatewayapi_v1beta1.RouteReasonUnsupportedValue,
					fmt.Sprintf("HTTPRoute.Spec.Rules.Filters: invalid type %q: only RequestHeaderModifier, ResponseHeaderModifier, RequestRedirect, RequestMirror and URLRewrite are supported.", filter.Type),
				)
			}
		}

		// If a URLRewrite filter specified a hostname rewrite,
		// add it to the request headers policy. The API spec does
		// not indicate how to resolve conflicts in rewriting the
		// Host header between a URLRewrite filter and a RequestHeaderModifier
		// filter so here we are choosing to prioritize the URLRewrite
		// filter.
		if len(urlRewriteHostname) > 0 {
			if requestHeaderPolicy == nil {
				requestHeaderPolicy = &HeadersPolicy{}
			}
			requestHeaderPolicy.HostRewrite = urlRewriteHostname
		}

		// Priority is used to ensure if there are multiple matching route rules
		// within an HTTPRoute, the one that comes first in the list has
		// precedence. We treat lower values as higher priority so we use the
		// index of the rule to ensure rules that come first have a higher
		// priority. All dag.Routes generated from a single HTTPRoute rule have
		// the same priority.
		priority := uint8(ruleIndex)

		// Get our list of routes based on whether it's a redirect or a cluster-backed route.
		// Note that we can end up with multiple routes here since the match conditions are
		// logically "OR"-ed, which we express as multiple routes, each with one of the
		// conditions, all with the same action.
		var routes []*Route

		if redirect != nil {
			routes = p.redirectRoutes(matchconditions, requestHeaderPolicy, responseHeaderPolicy, redirect, priority)
		} else {
			// Get clusters from rule backendRefs
			clusters, totalWeight, ok := p.httpClusters(route.Namespace, rule.BackendRefs, routeAccessor)
			if !ok {
				continue
			}
			routes = p.clusterRoutes(matchconditions, requestHeaderPolicy, responseHeaderPolicy, mirrorPolicy, clusters, totalWeight, priority, pathRewritePolicy)
		}

		// Add each route to the relevant vhost(s)/svhosts(s).
		for host := range hosts {
			for _, route := range routes {
				switch {
				case listener.tlsSecret != nil:
					svhost := p.dag.EnsureSecureVirtualHost(HTTPS_LISTENER_NAME, host)
					svhost.Secret = listener.tlsSecret
					svhost.AddRoute(route)
				default:
					vhost := p.dag.EnsureVirtualHost(HTTP_LISTENER_NAME, host)
					vhost.AddRoute(route)
				}

				programmed = true
			}
		}
	}

	return programmed
}

func (p *GatewayAPIProcessor) computeGRPCRouteForListener(route *gatewayapi_v1alpha2.GRPCRoute, routeAccessor *status.RouteParentStatusUpdate, listener *listenerInfo, hosts sets.Set[string]) bool {
	var programmed bool
	for ruleIndex, rule := range route.Spec.Rules {
		// Get match conditions for the rule.
		var matchconditions []*matchConditions
		for _, match := range rule.Matches {
			// Convert method match to path match
			pathMatch, ok := gatewayGRPCMethodMatchCondition(match.Method, routeAccessor)
			if !ok {
				continue
			}

			headerMatches, err := gatewayGRPCHeaderMatchConditions(match.Headers)
			if err != nil {
				routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, metav1.ConditionFalse, gatewayapi_v1beta1.RouteReasonUnsupportedValue, err.Error())
				continue
			}

			matchconditions = append(matchconditions, &matchConditions{
				path:    pathMatch,
				headers: headerMatches,
			})
		}

		// If no matches are specified, the implementation MUST match every gRPC request.
		if len(rule.Matches) == 0 {
			matchconditions = append(matchconditions, &matchConditions{
				path: &PrefixMatchCondition{Prefix: "/"},
			})
		}

		// Process rule-level filters.
		var (
			requestHeaderPolicy, responseHeaderPolicy *HeadersPolicy
			mirrorPolicy                              *MirrorPolicy
		)

		// Per Gateway API docs: "Specifying a core filter multiple times
		// has unspecified or implementation-specific conformance." Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range rule.Filters {
			switch filter.Type {
			case gatewayapi_v1alpha2.GRPCRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || requestHeaderPolicy != nil {
					continue
				}

				var err error
				requestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1alpha2.GRPCRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || responseHeaderPolicy != nil {
					continue
				}

				var err error
				responseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			case gatewayapi_v1alpha2.GRPCRouteFilterRequestMirror:
				if filter.RequestMirror == nil || mirrorPolicy != nil {
					continue
				}

				mirrorService, cond := p.validateBackendObjectRef(filter.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindGRPCRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
					continue
				}
				// If protocol is not set on the service, need to set a default one based on listener's protocol type.
				setDefaultServiceProtocol(mirrorService, listener.listener.Protocol)
				mirrorPolicy = &MirrorPolicy{
					Cluster: &Cluster{
						Upstream: mirrorService,
					},
				}
			default:
				routeAccessor.AddCondition(
					gatewayapi_v1beta1.RouteConditionAccepted,
					metav1.ConditionFalse,
					gatewayapi_v1beta1.RouteReasonUnsupportedValue,
					fmt.Sprintf("GRPCRoute.Spec.Rules.Filters: invalid type %q: only RequestHeaderModifier, ResponseHeaderModifier and RequestMirror are supported.", filter.Type),
				)
			}
		}

		// Priority is used to ensure if there are multiple matching route rules
		// within an GRPCRoute, the one that comes first in the list has
		// precedence. We treat lower values as higher priority so we use the
		// index of the rule to ensure rules that come first have a higher
		// priority. All dag.Routes generated from a single GRPCRoute rule have
		// the same priority.
		priority := uint8(ruleIndex)

		// Note that we can end up with multiple routes here since the match conditions are
		// logically "OR"-ed, which we express as multiple routes, each with one of the
		// conditions, all with the same action.
		var routes []*Route

		clusters, totalWeight, ok := p.grpcClusters(route.Namespace, rule.BackendRefs, routeAccessor, listener.listener.Protocol)
		if !ok {
			continue
		}
		routes = p.clusterRoutes(matchconditions, requestHeaderPolicy, responseHeaderPolicy, mirrorPolicy, clusters, totalWeight, priority, nil)

		// Add each route to the relevant vhost(s)/svhosts(s).
		for host := range hosts {
			for _, route := range routes {
				switch {
				case listener.tlsSecret != nil:
					svhost := p.dag.EnsureSecureVirtualHost(HTTPS_LISTENER_NAME, host)
					svhost.Secret = listener.tlsSecret
					svhost.AddRoute(route)
				default:
					vhost := p.dag.EnsureVirtualHost(HTTP_LISTENER_NAME, host)
					vhost.AddRoute(route)
				}

				programmed = true
			}
		}
	}

	return programmed
}

func gatewayGRPCMethodMatchCondition(match *gatewayapi_v1alpha2.GRPCMethodMatch, routeAccessor *status.RouteParentStatusUpdate) (MatchCondition, bool) {
	// If method match is not specified, all services and methods will match.
	if match == nil {
		return &PrefixMatchCondition{Prefix: "/"}, true
	}

	// Type specifies how to match against the service and/or method.
	// Support: Core (Exact with service and method specified)
	// Not Support: Implementation-specific (Exact with method specified but no service specified)
	// Not Support: Implementation-specific (RegularExpression)

	// Support "Exact" match type only. If match type is not specified, use "Exact" as default.
	if match.Type != nil && *match.Type != gatewayapi_v1alpha2.GRPCMethodMatchExact {
		routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionAccepted, metav1.ConditionFalse, gatewayapi_v1beta1.RouteReasonUnsupportedValue, "GRPCRoute.Spec.Rules.Matches.Method: Only Exact match type is supported.")
		return nil, false
	}

	if match.Service == nil || isBlank(*match.Service) || match.Method == nil || isBlank(*match.Method) {
		routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionAccepted, metav1.ConditionFalse, status.ReasonInvalidMethodMatch, "GRPCRoute.Spec.Rules.Matches.Method: Both Service and Method need be configured.")
		return nil, false
	}

	// Convert service and method into path
	path := "/" + *match.Service + "/" + *match.Method

	return &ExactMatchCondition{Path: path}, true
}

func gatewayGRPCHeaderMatchConditions(matches []gatewayapi_v1alpha2.GRPCHeaderMatch) ([]HeaderMatchCondition, error) {
	var headerMatchConditions []HeaderMatchCondition
	seenNames := sets.New[string]()

	for _, match := range matches {
		// "Exact" is the default if not defined in the object, and
		// the only supported match type.
		if match.Type != nil && *match.Type != gatewayapi_v1beta1.HeaderMatchExact {
			return nil, fmt.Errorf("GRPCRoute.Spec.Rules.Matches.Headers: Only Exact match type is supported")
		}

		// If multiple match conditions are found for the same header name (case-insensitive),
		// use the first one and ignore subsequent ones.
		upperName := strings.ToUpper(string(match.Name))
		if seenNames.Has(upperName) {
			continue
		}
		seenNames.Insert(upperName)

		headerMatchConditions = append(headerMatchConditions, HeaderMatchCondition{
			MatchType: HeaderMatchTypeExact,
			Name:      string(match.Name),
			Value:     match.Value,
		})
	}

	return headerMatchConditions, nil
}

// validateBackendRef verifies that the specified BackendRef is valid.
// Returns a metav1.Condition for the route if any errors are detected.
func (p *GatewayAPIProcessor) validateBackendRef(backendRef gatewayapi_v1beta1.BackendRef, routeKind, routeNamespace string) (*Service, *metav1.Condition) {
	return p.validateBackendObjectRef(backendRef.BackendObjectReference, "Spec.Rules.BackendRef", routeKind, routeNamespace)
}

// validateBackendObjectRef verifies that the specified BackendObjectReference
// is valid. Returns a metav1.Condition for the route if any errors are detected.
// As BackendObjectReference is used in multiple fields, the given field is used
// to build the message in metav1.Condition.
func (p *GatewayAPIProcessor) validateBackendObjectRef(backendObjectRef gatewayapi_v1beta1.BackendObjectReference, field string, routeKind, routeNamespace string) (*Service, *metav1.Condition) {
	resolvedRefsFalse := func(reason gatewayapi_v1beta1.RouteConditionReason, msg string) *metav1.Condition {
		return &metav1.Condition{
			Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
			Status:  metav1.ConditionFalse,
			Reason:  string(reason),
			Message: msg,
		}
	}

	if !(backendObjectRef.Group == nil || *backendObjectRef.Group == "") {
		return nil, resolvedRefsFalse(gatewayapi_v1beta1.RouteReasonInvalidKind, fmt.Sprintf("%s.Group must be \"\"", field))
	}

	if !(backendObjectRef.Kind != nil && *backendObjectRef.Kind == "Service") {
		return nil, resolvedRefsFalse(gatewayapi_v1beta1.RouteReasonInvalidKind, fmt.Sprintf("%s.Kind must be 'Service'", field))
	}

	if backendObjectRef.Name == "" {
		return nil, resolvedRefsFalse(status.ReasonDegraded, fmt.Sprintf("%s.Name must be specified", field))
	}

	if backendObjectRef.Port == nil {
		return nil, resolvedRefsFalse(status.ReasonDegraded, fmt.Sprintf("%s.Port must be specified", field))
	}

	// If the backend is in a different namespace than the route, then we need to
	// check for a ReferenceGrant that allows the reference.
	if backendObjectRef.Namespace != nil && string(*backendObjectRef.Namespace) != routeNamespace {
		if !p.validCrossNamespaceRef(
			crossNamespaceFrom{
				group:     string(gatewayapi_v1beta1.GroupName),
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
			return nil, resolvedRefsFalse(gatewayapi_v1beta1.RouteReasonRefNotPermitted, fmt.Sprintf("%s.Namespace must match the route's namespace or be covered by a ReferenceGrant", field))
		}
	}

	var meta types.NamespacedName
	if backendObjectRef.Namespace != nil {
		meta = types.NamespacedName{Name: string(backendObjectRef.Name), Namespace: string(*backendObjectRef.Namespace)}
	} else {
		meta = types.NamespacedName{Name: string(backendObjectRef.Name), Namespace: routeNamespace}
	}

	service, err := p.dag.EnsureService(meta, int(*backendObjectRef.Port), int(*backendObjectRef.Port), p.source, p.EnableExternalNameService)
	if err != nil {
		return nil, resolvedRefsFalse(gatewayapi_v1beta1.RouteReasonBackendNotFound, fmt.Sprintf("service %q is invalid: %s", meta.Name, err))
	}

	return service, nil
}

func gatewayPathMatchCondition(match *gatewayapi_v1beta1.HTTPPathMatch, routeAccessor *status.RouteParentStatusUpdate) (MatchCondition, bool) {
	if match == nil {
		return &PrefixMatchCondition{Prefix: "/"}, true
	}

	path := ref.Val(match.Value, "/")

	// If path match type is not defined, default to 'PathPrefix'.
	if match.Type == nil || *match.Type == gatewayapi_v1beta1.PathMatchPathPrefix {
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

	if *match.Type == gatewayapi_v1beta1.PathMatchExact {
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

	routeAccessor.AddCondition(
		gatewayapi_v1beta1.RouteConditionAccepted,
		metav1.ConditionFalse,
		gatewayapi_v1beta1.RouteReasonUnsupportedValue,
		"HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported.",
	)
	return nil, false
}

func gatewayHeaderMatchConditions(matches []gatewayapi_v1beta1.HTTPHeaderMatch) ([]HeaderMatchCondition, error) {
	var headerMatchConditions []HeaderMatchCondition
	seenNames := sets.New[string]()

	for _, match := range matches {
		// "Exact" is the default if not defined in the object, and
		// the only supported match type.
		if match.Type != nil && *match.Type != gatewayapi_v1beta1.HeaderMatchExact {
			return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.Headers: Only Exact match type is supported")
		}

		// If multiple match conditions are found for the same header name (case-insensitive),
		// use the first one and ignore subsequent ones.
		upperName := strings.ToUpper(string(match.Name))
		if seenNames.Has(upperName) {
			continue
		}
		seenNames.Insert(upperName)

		headerMatchConditions = append(headerMatchConditions, HeaderMatchCondition{
			MatchType: HeaderMatchTypeExact,
			Name:      string(match.Name),
			Value:     match.Value,
		})
	}

	return headerMatchConditions, nil
}

func gatewayQueryParamMatchConditions(matches []gatewayapi_v1beta1.HTTPQueryParamMatch) ([]QueryParamMatchCondition, error) {
	var dagMatchConditions []QueryParamMatchCondition
	seenNames := sets.New[string]()

	for _, match := range matches {
		// "Exact" is the default if not defined in the object, and
		// the only supported match type.
		if match.Type != nil && *match.Type != gatewayapi_v1beta1.QueryParamMatchExact {
			return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.QueryParams: Only Exact match type is supported")
		}

		// If multiple match conditions are found for the same value,
		// use the first one and ignore subsequent ones.
		if seenNames.Has(match.Name) {
			continue
		}
		seenNames.Insert(match.Name)

		dagMatchConditions = append(dagMatchConditions, QueryParamMatchCondition{
			MatchType: QueryParamMatchTypeExact,
			Name:      match.Name,
			Value:     match.Value,
		})
	}

	return dagMatchConditions, nil
}

// httpClusters builds clusters from backendRef.
func (p *GatewayAPIProcessor) httpClusters(routeNamespace string, backendRefs []gatewayapi_v1beta1.HTTPBackendRef, routeAccessor *status.RouteParentStatusUpdate) ([]*Cluster, uint32, bool) {
	totalWeight := uint32(0)

	if len(backendRefs) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
		return nil, totalWeight, false
	}

	var clusters []*Cluster

	// Validate the backend refs.
	for _, backendRef := range backendRefs {
		service, cond := p.validateBackendRef(backendRef.BackendRef, KindHTTPRoute, routeNamespace)
		if cond != nil {
			routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
			continue
		}

		var clusterRequestHeaderPolicy *HeadersPolicy
		var clusterResponseHeaderPolicy *HeadersPolicy

		// Per Gateway API docs: "Specifying a core filter multiple times
		// has unspecified or implementation-specific conformance." Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range backendRef.Filters {
			switch filter.Type {
			case gatewayapi_v1beta1.HTTPRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || clusterRequestHeaderPolicy != nil {
					continue
				}

				var err error
				clusterRequestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1beta1.HTTPRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || clusterResponseHeaderPolicy != nil {
					continue
				}

				var err error
				clusterResponseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			default:
				routeAccessor.AddCondition(
					gatewayapi_v1beta1.RouteConditionAccepted,
					metav1.ConditionFalse,
					gatewayapi_v1beta1.RouteReasonUnsupportedValue,
					"HTTPRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier and ResponseHeaderModifier type is supported.",
				)
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
			Upstream:              service,
			Weight:                routeWeight,
			Protocol:              service.Protocol,
			RequestHeadersPolicy:  clusterRequestHeaderPolicy,
			ResponseHeadersPolicy: clusterResponseHeaderPolicy,
			TimeoutPolicy:         ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
		})
	}
	return clusters, totalWeight, true
}

// grpcClusters builds clusters from backendRef.
func (p *GatewayAPIProcessor) grpcClusters(routeNamespace string, backendRefs []gatewayapi_v1alpha2.GRPCBackendRef, routeAccessor *status.RouteParentStatusUpdate, protocolType gatewayapi_v1beta1.ProtocolType) ([]*Cluster, uint32, bool) {
	totalWeight := uint32(0)

	if len(backendRefs) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
		return nil, totalWeight, false
	}

	var clusters []*Cluster

	// Validate the backend refs.
	for _, backendRef := range backendRefs {
		service, cond := p.validateBackendRef(backendRef.BackendRef, KindGRPCRoute, routeNamespace)
		if cond != nil {
			routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1beta1.RouteConditionReason(cond.Reason), cond.Message)
			continue
		}

		var clusterRequestHeaderPolicy, clusterResponseHeaderPolicy *HeadersPolicy

		// Per Gateway API docs: "Specifying a core filter multiple times
		// has unspecified or implementation-specific conformance." Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range backendRef.Filters {
			switch filter.Type {
			case gatewayapi_v1alpha2.GRPCRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || clusterRequestHeaderPolicy != nil {
					continue
				}

				var err error
				clusterRequestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1alpha2.GRPCRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || clusterResponseHeaderPolicy != nil {
					continue
				}

				var err error
				clusterResponseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1beta1.RouteConditionResolvedRefs, metav1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			default:
				routeAccessor.AddCondition(
					gatewayapi_v1beta1.RouteConditionAccepted,
					metav1.ConditionFalse,
					gatewayapi_v1beta1.RouteReasonUnsupportedValue,
					"GRPCRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier and ResponseHeaderModifier type is supported.",
				)
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

		// If protocol is not set on the service, need to set a default one based on listener's protocol type.
		setDefaultServiceProtocol(service, protocolType)

		// https://github.com/projectcontour/contour/issues/3593
		service.Weighted.Weight = routeWeight
		clusters = append(clusters, &Cluster{
			Upstream:              service,
			Weight:                routeWeight,
			Protocol:              service.Protocol,
			RequestHeadersPolicy:  clusterRequestHeaderPolicy,
			ResponseHeadersPolicy: clusterResponseHeaderPolicy,
			TimeoutPolicy:         ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
		})
	}
	return clusters, totalWeight, true
}

// clusterRoutes builds a []*dag.Route for the supplied set of matchConditions, headerPolicies and backendRefs.
func (p *GatewayAPIProcessor) clusterRoutes(matchConditions []*matchConditions, requestHeaderPolicy *HeadersPolicy, responseHeaderPolicy *HeadersPolicy,
	mirrorPolicy *MirrorPolicy, clusters []*Cluster, totalWeight uint32, priority uint8, pathRewritePolicy *PathRewritePolicy) []*Route {

	var routes []*Route

	// Per Gateway API: "Each match is independent,
	// i.e. this rule will be matched if any one of
	// the matches is satisfied." To implement this,
	// we create a separate route per match.
	for _, mc := range matchConditions {
		// Re-configure the PathRewritePolicy if we're trying to remove
		// the prefix entirely.
		pathRewritePolicy = handlePathRewritePrefixRemoval(pathRewritePolicy, mc)

		routes = append(routes, &Route{
			Clusters:                  clusters,
			PathMatchCondition:        mc.path,
			HeaderMatchConditions:     mc.headers,
			QueryParamMatchConditions: mc.queryParams,
			RequestHeadersPolicy:      requestHeaderPolicy,
			ResponseHeadersPolicy:     responseHeaderPolicy,
			MirrorPolicy:              mirrorPolicy,
			Priority:                  priority,
			PathRewritePolicy:         pathRewritePolicy,
		})
	}

	for _, route := range routes {
		// If there aren't any valid services, or the total weight of all of
		// them equal zero, then return 500 responses to the caller.
		if len(clusters) == 0 || totalWeight == 0 {
			// Configure a direct response HTTP status code of 500 so the
			// route still matches the configured conditions since the
			// service is missing or invalid.
			route.DirectResponse = &DirectResponse{
				StatusCode: http.StatusInternalServerError,
			}
		}
	}

	return routes
}

func setDefaultServiceProtocol(service *Service, protocolType gatewayapi_v1beta1.ProtocolType) {
	// For GRPCRoute, if the protocol is not set on the Service via annotation,
	// we should assume a protocol that matches what listener the route was attached to
	if isBlank(service.Protocol) {
		if protocolType == gatewayapi_v1beta1.HTTPProtocolType {
			service.Protocol = "h2c"
		} else if protocolType == gatewayapi_v1beta1.HTTPSProtocolType {
			service.Protocol = "h2"
		}
	}
}

// redirectRoutes builds a []*dag.Route for the supplied set of matchConditions, headerPolicies and redirect.
func (p *GatewayAPIProcessor) redirectRoutes(matchConditions []*matchConditions, requestHeaderPolicy *HeadersPolicy, responseHeaderPolicy *HeadersPolicy, redirect *Redirect, priority uint8) []*Route {
	var routes []*Route

	// Per Gateway API: "Each match is independent,
	// i.e. this rule will be matched if any one of
	// the matches is satisfied." To implement this,
	// we create a separate route per match.
	for _, mc := range matchConditions {
		// Re-configure the PathRewritePolicy if we're trying to remove
		// the prefix entirely.
		redirect.PathRewritePolicy = handlePathRewritePrefixRemoval(redirect.PathRewritePolicy, mc)

		routes = append(routes, &Route{
			Priority:              priority,
			Redirect:              redirect,
			PathMatchCondition:    mc.path,
			HeaderMatchConditions: mc.headers,
			RequestHeadersPolicy:  requestHeaderPolicy,
			ResponseHeadersPolicy: responseHeaderPolicy,
		})
	}

	return routes
}

func handlePathRewritePrefixRemoval(p *PathRewritePolicy, mc *matchConditions) *PathRewritePolicy {
	// Handle the case where the prefix is supposed to be rewritten to "/", i.e. removed.
	// This doesn't work out of the box in Envoy with path_separated_prefix
	// and prefix_rewrite, so we have to use a regex. Specifically, for a prefix
	// match of "/foo", a prefix rewrite of "/", and a request to "/foo/bar", Envoy
	// will rewrite the request path to "//bar" which is invalid. The regex handles matching
	// and removing any trailing slashes.
	//
	// This logic is implemented here rather than in internal/envoy because there
	// is already special handling at the DAG level for similar issues for HTTPProxy.
	if p != nil && p.PrefixRewrite == "/" {
		prefixMatch, ok := mc.path.(*PrefixMatchCondition)
		if ok {
			p.PrefixRewrite = ""
			// The regex below will capture/remove all consecutive trailing slashes
			// immediately after the prefix, to handle requests like /prefix///foo.
			p.PrefixRegexRemove = "^" + regexp.QuoteMeta(prefixMatch.Prefix) + "/*"
		}
	}

	return p
}
