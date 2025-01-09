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
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/timeout"
)

const (
	KindHTTPRoute = "HTTPRoute"
	KindTLSRoute  = "TLSRoute"
	KindGRPCRoute = "GRPCRoute"
	KindTCPRoute  = "TCPRoute"
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

	// MaxRequestsPerConnection defines the maximum number of requests per connection to the upstream before it is closed.
	MaxRequestsPerConnection *uint32

	// PerConnectionBufferLimitBytes defines the soft limit on size of the cluster’s new connection read and write buffers.
	PerConnectionBufferLimitBytes *uint32

	// SetSourceMetadataOnRoutes defines whether to set the Kind,
	// Namespace and Name fields on generated DAG routes. This is
	// configurable and off by default in order to support the feature
	// without requiring all existing test cases to change.
	SetSourceMetadataOnRoutes bool

	// GlobalCircuitBreakerDefaults defines global circuit breaker defaults.
	GlobalCircuitBreakerDefaults *contour_v1alpha1.CircuitBreakers

	// UpstreamTLS defines the TLS settings like min/max version
	// and cipher suites for upstream connections.
	UpstreamTLS *UpstreamTLS
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

	var gatewayNotProgrammedCondition *meta_v1.Condition

	if !isAddressAssigned(p.source.gateway.Spec.Addresses, p.source.gateway.Status.Addresses) {
		// TODO(sk) resolve condition type-reason mismatch
		gatewayNotProgrammedCondition = &meta_v1.Condition{
			Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
			Status:  meta_v1.ConditionFalse,
			Reason:  string(gatewayapi_v1.GatewayReasonAddressNotAssigned),
			Message: "None of the addresses in Spec.Addresses have been assigned to the Gateway",
		}
	}

	// Validate listener protocols, ports and hostnames and add conditions
	// for all invalid listeners.
	validateListenersResult := gatewayapi.ValidateListeners(p.source.gateway.Spec.Listeners)
	for name, cond := range validateListenersResult.InvalidListenerConditions {
		gwAccessor.AddListenerCondition(
			string(name),
			gatewayapi_v1.ListenerConditionType(cond.Type),
			cond.Status,
			gatewayapi_v1.ListenerConditionReason(cond.Reason),
			cond.Message,
		)
	}

	// Compute listeners and save a list of the valid/ready ones.
	var listenerInfos []*listenerInfo
	for _, listener := range p.source.gateway.Spec.Listeners {
		listenerInfos = append(listenerInfos, p.computeListener(listener, gwAccessor, validateListenersResult))
	}

	// Keep track of the number of routes attached
	// to each Listener so we can set status properly.
	listenerAttachedRoutes := map[string]int{}

	// Process sorted HTTPRoutes.
	for _, httpRoute := range sortHTTPRoutes(p.source.httproutes) {
		p.processRoute(KindHTTPRoute, httpRoute, httpRoute.Spec.ParentRefs, gatewayNotProgrammedCondition, listenerInfos, listenerAttachedRoutes, &gatewayapi_v1.HTTPRoute{})
	}

	// Process TLSRoutes.
	for _, tlsRoute := range p.source.tlsroutes {
		p.processRoute(KindTLSRoute, tlsRoute, tlsRoute.Spec.ParentRefs, gatewayNotProgrammedCondition, listenerInfos, listenerAttachedRoutes, &gatewayapi_v1alpha2.TLSRoute{})
	}

	// Process sorted GRPCRoutes.
	for _, grpcRoute := range sortGRPCRoutes(p.source.grpcroutes) {
		p.processRoute(KindGRPCRoute, grpcRoute, grpcRoute.Spec.ParentRefs, gatewayNotProgrammedCondition, listenerInfos, listenerAttachedRoutes, &gatewayapi_v1.GRPCRoute{})
	}

	// Process TCPRoutes.
	for _, tcpRoute := range p.source.tcproutes {
		p.processRoute(KindTCPRoute, tcpRoute, tcpRoute.Spec.ParentRefs, gatewayNotProgrammedCondition, listenerInfos, listenerAttachedRoutes, &gatewayapi_v1alpha2.TCPRoute{})
	}

	for listenerName, attachedRoutes := range listenerAttachedRoutes {
		gwAccessor.SetListenerAttachedRoutes(listenerName, int32(attachedRoutes)) //nolint:gosec // disable G115
	}

	p.computeGatewayConditions(gwAccessor, gatewayNotProgrammedCondition)
}

func (p *GatewayAPIProcessor) processRoute(
	routeKind gatewayapi_v1.Kind,
	route client.Object,
	parentRefs []gatewayapi_v1.ParentReference,
	gatewayNotProgrammedCondition *meta_v1.Condition,
	listeners []*listenerInfo,
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
			routeParentStatus.AddCondition(gatewayapi_v1.RouteConditionAccepted, meta_v1.ConditionFalse, status.ReasonInvalidGateway, "Invalid Gateway")
			continue
		}

		// Get the list of listeners that are
		// (a) included by this parent ref, and
		// (b) allow the route (based on kind, namespace), and
		// (c) the 'listenerInfo.ready' is true
		allowedListeners := p.getListenersForRouteParentRef(routeParentRef, route.GetNamespace(), routeKind, listeners, listenerAttachedRoutes, routeParentStatus)
		if len(allowedListeners) == 0 {
			p.resolveRouteRefs(route, routeParentStatus)
		}

		// Collect other Listeners with configured hostnames so we can
		// calculate Listener/Route hostname intersection properly.
		otherListenerHostnames := []string{}
		for _, listener := range listeners {
			name := string(listener.listener.Name)
			if _, ok := allowedListeners[name]; !ok && listener.listener.Hostname != nil && len(*listener.listener.Hostname) > 0 {
				otherListenerHostnames = append(otherListenerHostnames, string(*listener.listener.Hostname))
			}
		}

		// Keep track of the number of intersecting hosts
		// between the route and all allowed listeners for
		// this parent ref so that we can set the appropriate
		// route parent status condition if there were none.
		hostCount := 0

		for _, listener := range allowedListeners {
			var hosts sets.Set[string]
			var errs []error

			// TCPRoutes don't have hostnames.
			if routeKind != KindTCPRoute {
				var routeHostnames []gatewayapi_v1.Hostname

				switch route := route.(type) {
				case *gatewayapi_v1.HTTPRoute:
					routeHostnames = route.Spec.Hostnames
				case *gatewayapi_v1alpha2.TLSRoute:
					routeHostnames = route.Spec.Hostnames
				case *gatewayapi_v1.GRPCRoute:
					routeHostnames = route.Spec.Hostnames
				}

				hosts, errs = p.computeHosts(routeHostnames, string(ptr.Deref(listener.listener.Hostname, "")), otherListenerHostnames)
				for _, err := range errs {
					// The Gateway API spec does not indicate what to do if syntactically
					// invalid hostnames make it through, we're using our best judgment here.
					// Theoretically these should be prevented by the combination of kubebuilder
					// and admission webhook validations.
					routeParentStatus.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, err.Error())
				}

				// If there were no intersections between the listener hostname and the
				// route hostnames, the route is not programmed for this listener.
				if len(hosts) == 0 {
					continue
				}
			}

			switch route := route.(type) {
			case *gatewayapi_v1.HTTPRoute:
				p.computeHTTPRouteForListener(route, routeParentStatus, routeParentRef, listener, hosts)
			case *gatewayapi_v1alpha2.TLSRoute:
				p.computeTLSRouteForListener(route, routeParentStatus, listener, hosts)
			case *gatewayapi_v1.GRPCRoute:
				p.computeGRPCRouteForListener(route, routeParentStatus, listener, hosts)
			case *gatewayapi_v1alpha2.TCPRoute:
				p.computeTCPRouteForListener(route, routeParentStatus, listener)
			}

			hostCount += hosts.Len()
		}

		if routeKind != KindTCPRoute && hostCount == 0 && !routeParentStatus.ConditionExists(gatewayapi_v1.RouteConditionAccepted) {
			routeParentStatus.AddCondition(
				gatewayapi_v1.RouteConditionAccepted,
				meta_v1.ConditionFalse,
				gatewayapi_v1.RouteReasonNoMatchingListenerHostname,
				"No intersecting hostnames were found between the listener and the route.",
			)
		}

		// Check for an existing "ResolvedRefs" condition, add one if one does
		// not already exist.
		if !routeParentStatus.ConditionExists(gatewayapi_v1.RouteConditionResolvedRefs) {
			routeParentStatus.AddCondition(
				gatewayapi_v1.RouteConditionResolvedRefs,
				meta_v1.ConditionTrue,
				gatewayapi_v1.RouteReasonResolvedRefs,
				"References resolved")
		}

		// Check for an existing "Accepted" condition, add one if one does
		// not already exist.
		if !routeParentStatus.ConditionExists(gatewayapi_v1.RouteConditionAccepted) {
			routeParentStatus.AddCondition(
				gatewayapi_v1.RouteConditionAccepted,
				meta_v1.ConditionTrue,
				gatewayapi_v1.RouteReasonAccepted,
				fmt.Sprintf("Accepted %s", routeKind),
			)
		}
	}
}

func (p *GatewayAPIProcessor) getListenersForRouteParentRef(
	routeParentRef gatewayapi_v1.ParentReference,
	routeNamespace string,
	routeKind gatewayapi_v1.Kind,
	listeners []*listenerInfo,
	attachedRoutes map[string]int,
	routeParentStatusAccessor *status.RouteParentStatusUpdate,
) map[string]*listenerInfo {
	// Find the set of valid listeners that are relevant given this
	// parent ref (either all of them, if the ref is to the entire
	// gateway, or one of them, if the ref is to a specific listener,
	// or none of them, if the listener(s) the ref targets are invalid).
	var selectedListeners []*listenerInfo
	for _, listener := range listeners {
		// We've already verified the parent ref is for this Gateway,
		// now check if it has a listener name and port specified.
		// Both need to match the listener if specified.
		if (routeParentRef.SectionName == nil || *routeParentRef.SectionName == listener.listener.Name) &&
			(routeParentRef.Port == nil || *routeParentRef.Port == listener.listener.Port) {
			selectedListeners = append(selectedListeners, listener)
		}
	}

	// Now find the subset of those listeners that allow this route
	// to select them, based on route kind and namespace.
	allowedListeners := map[string]*listenerInfo{}

	readyListenerCount := 0

	for _, selectedListener := range selectedListeners {

		// for compute the AttachedRoutes, the listener that not passed its check(s), had been selected too
		// so ignore it.
		if selectedListener.ready {
			readyListenerCount++
		}

		// Check if the listener allows routes of this kind
		if !selectedListener.AllowsKind(routeKind) {
			continue
		}

		// Check if the route is in a namespace that the listener allows.
		if !p.namespaceMatches(selectedListener.listener.AllowedRoutes.Namespaces, selectedListener.namespaceSelector, routeNamespace) {
			continue
		}

		attachedRoutes[string(selectedListener.listener.Name)]++

		if selectedListener.ready {
			allowedListeners[string(selectedListener.listener.Name)] = selectedListener
		}

	}
	if readyListenerCount == 0 {
		routeParentStatusAccessor.AddCondition(
			gatewayapi_v1.RouteConditionAccepted,
			meta_v1.ConditionFalse,
			gatewayapi_v1.RouteReasonNoMatchingParent,
			"No listeners match this parent ref",
		)
		return nil
	}

	if len(allowedListeners) == 0 {
		routeParentStatusAccessor.AddCondition(
			gatewayapi_v1.RouteConditionAccepted,
			meta_v1.ConditionFalse,
			gatewayapi_v1.RouteReasonNotAllowedByListeners,
			"No listeners included by this parent ref allowed this attachment.",
		)
		return nil
	}

	return allowedListeners
}

type listenerInfo struct {
	listener          gatewayapi_v1.Listener
	dagListenerName   string
	allowedKinds      []gatewayapi_v1.Kind
	namespaceSelector labels.Selector
	tlsSecret         *Secret
	ready             bool
}

func (l *listenerInfo) AllowsKind(kind gatewayapi_v1.Kind) bool {
	for _, allowedKind := range l.allowedKinds {
		if allowedKind == kind {
			return true
		}
	}

	return false
}

// isAddressAssigned returns true if either there are no addresses requested in specAddresses,
// or if at least one address from specAddresses appears in statusAddresses.
func isAddressAssigned(specAddresses []gatewayapi_v1.GatewayAddress, statusAddresses []gatewayapi_v1.GatewayStatusAddress) bool {
	if len(specAddresses) == 0 {
		return true
	}

	for _, specAddress := range specAddresses {
		for _, statusAddress := range statusAddresses {
			// Types must match
			if ptr.Deref(specAddress.Type, gatewayapi_v1.IPAddressType) != ptr.Deref(statusAddress.Type, gatewayapi_v1.IPAddressType) {
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
	listener gatewayapi_v1.Listener,
	gwAccessor *status.GatewayStatusUpdate,
	validateListenersResult gatewayapi.ValidateListenersResult,
) *listenerInfo {
	info := &listenerInfo{
		listener:        listener,
		dagListenerName: validateListenersResult.ListenerNames[string(listener.Name)],
	}

	addInvalidListenerCondition := func(msg string) {
		gwAccessor.AddListenerCondition(
			string(listener.Name),
			gatewayapi_v1.ListenerConditionProgrammed,
			meta_v1.ConditionFalse,
			gatewayapi_v1.ListenerReasonInvalid,
			msg,
		)
	}

	// Set required Listener conditions (Programmed, Accepted, ResolvedRefs)
	// if they haven't already been set.
	defer func() {
		listenerStatus := gwAccessor.ListenerStatus[string(listener.Name)]

		if listenerStatus == nil || len(listenerStatus.Conditions) == 0 {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1.ListenerConditionProgrammed,
				meta_v1.ConditionTrue,
				gatewayapi_v1.ListenerReasonProgrammed,
				"Valid listener",
			)
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1.ListenerConditionAccepted,
				meta_v1.ConditionTrue,
				gatewayapi_v1.ListenerReasonAccepted,
				"Listener accepted",
			)
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1.ListenerConditionResolvedRefs,
				meta_v1.ConditionTrue,
				gatewayapi_v1.ListenerReasonResolvedRefs,
				"Listener references resolved",
			)
		} else {
			programmedConditionExists := false
			acceptedConditionExists := false
			resolvedRefsConditionExists := false

			for _, cond := range listenerStatus.Conditions {
				if cond.Type == string(gatewayapi_v1.ListenerConditionProgrammed) {
					programmedConditionExists = true
				}
				if cond.Type == string(gatewayapi_v1.ListenerConditionAccepted) {
					acceptedConditionExists = true
				}
				if cond.Type == string(gatewayapi_v1.ListenerConditionResolvedRefs) {
					resolvedRefsConditionExists = true
				}
			}

			// Only set the required Listener conditions if
			// they don't already exist in the status update, since
			// if they do exist, they will contain more specific
			// information in the reason, message, etc.
			if !programmedConditionExists {
				addInvalidListenerCondition("Invalid listener, see other listener conditions for details")
			}
			// Set Accepted condition to true if not
			// explicitly set otherwise.
			if !acceptedConditionExists {
				gwAccessor.AddListenerCondition(
					string(listener.Name),
					gatewayapi_v1.ListenerConditionAccepted,
					meta_v1.ConditionTrue,
					gatewayapi_v1.ListenerReasonAccepted,
					"Listener accepted",
				)
			}
			// Set ResolvedRefs condition to true if not
			// explicitly set otherwise.
			if !resolvedRefsConditionExists {
				gwAccessor.AddListenerCondition(
					string(listener.Name),
					gatewayapi_v1.ListenerConditionResolvedRefs,
					meta_v1.ConditionTrue,
					gatewayapi_v1.ListenerReasonResolvedRefs,
					"Listener references resolved",
				)
			}
		}
	}()

	// Get a list of the route kinds that the listener accepts.
	info.allowedKinds = p.getListenerRouteKinds(listener, gwAccessor)
	gwAccessor.SetListenerSupportedKinds(string(listener.Name), info.allowedKinds)

	if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces != nil &&
		listener.AllowedRoutes.Namespaces.From != nil && *listener.AllowedRoutes.Namespaces.From == gatewayapi_v1.NamespacesFromSelector {

		if listener.AllowedRoutes.Namespaces.Selector == nil {
			addInvalidListenerCondition("Listener.AllowedRoutes.Namespaces.Selector is required when Listener.AllowedRoutes.Namespaces.From is set to \"Selector\".")
			return info
		}

		if len(listener.AllowedRoutes.Namespaces.Selector.MatchExpressions)+len(listener.AllowedRoutes.Namespaces.Selector.MatchLabels) == 0 {
			addInvalidListenerCondition("Listener.AllowedRoutes.Namespaces.Selector must specify at least one MatchLabel or MatchExpression.")
			return info
		}

		var err error
		info.namespaceSelector, err = meta_v1.LabelSelectorAsSelector(listener.AllowedRoutes.Namespaces.Selector)
		if err != nil {
			addInvalidListenerCondition(fmt.Sprintf("Error parsing Listener.AllowedRoutes.Namespaces.Selector: %v.", err))
			return info
		}
	}

	// If the listener had an invalid protocol/port/hostname, we reach here just for pick the information to compute the AttachedRoutes later,
	// we don't need to go any further.
	if _, invalid := validateListenersResult.InvalidListenerConditions[listener.Name]; invalid {
		return info
	}

	var listenerSecret *Secret

	// Validate TLS details for HTTPS/TLS protocol listeners.
	switch listener.Protocol {
	case gatewayapi_v1.HTTPSProtocolType:
		// The HTTPS protocol is used for HTTP traffic encrypted with TLS,
		// which is to be TLS-terminated at the proxy and then routed to
		// backends using HTTPRoutes.

		if listener.TLS == nil {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS is required when protocol is %q.", listener.Protocol))
			return info
		}

		if listener.TLS.Mode != nil && *listener.TLS.Mode != gatewayapi_v1.TLSModeTerminate {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS.Mode must be %q when protocol is %q.", gatewayapi_v1.TLSModeTerminate, listener.Protocol))
			return info
		}

		// Resolve the TLS secret.
		if listenerSecret = p.resolveListenerSecret(listener.TLS.CertificateRefs, string(listener.Name), gwAccessor); listenerSecret == nil {
			// If TLS was configured on the Listener, but the secret ref is invalid, don't allow any
			// routes to be bound to this listener since it can't serve TLS traffic.
			return info
		}
	case gatewayapi_v1.TLSProtocolType:
		// The TLS protocol is used for TCP traffic encrypted with TLS.
		// Gateway API allows TLS to be either terminated at the proxy
		// or passed through to the backend.
		if listener.TLS == nil {
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS is required when protocol is %q.", listener.Protocol))
			return info
		}

		switch {
		case listener.TLS.Mode == nil || *listener.TLS.Mode == gatewayapi_v1.TLSModeTerminate:
			// Resolve the TLS secret.
			if listenerSecret = p.resolveListenerSecret(listener.TLS.CertificateRefs, string(listener.Name), gwAccessor); listenerSecret == nil {
				// If TLS was configured on the Listener, but the secret ref is invalid, don't allow any
				// routes to be bound to this listener since it can't serve TLS traffic.
				return info
			}
		case *listener.TLS.Mode == gatewayapi_v1.TLSModePassthrough:
			if len(listener.TLS.CertificateRefs) != 0 {
				addInvalidListenerCondition(fmt.Sprintf("Listener.TLS.CertificateRefs cannot be defined when Listener.TLS.Mode is %q.", gatewayapi_v1.TLSModePassthrough))
				return info
			}
		default:
			addInvalidListenerCondition(fmt.Sprintf("Listener.TLS.Mode must be %q or %q.", gatewayapi_v1.TLSModeTerminate, gatewayapi_v1.TLSModePassthrough))
			return info
		}
	}

	info.tlsSecret = listenerSecret
	info.ready = true
	return info
}

// getListenerRouteKinds gets a list of the valid route kinds that
// the listener accepts.
func (p *GatewayAPIProcessor) getListenerRouteKinds(listener gatewayapi_v1.Listener, gwAccessor *status.GatewayStatusUpdate) []gatewayapi_v1.Kind {
	// None specified on the listener: return the default based on
	// the listener's protocol.
	if len(listener.AllowedRoutes.Kinds) == 0 {
		switch listener.Protocol {
		case gatewayapi_v1.HTTPProtocolType:
			return []gatewayapi_v1.Kind{KindHTTPRoute, KindGRPCRoute}
		case gatewayapi_v1.HTTPSProtocolType:
			return []gatewayapi_v1.Kind{KindHTTPRoute, KindGRPCRoute}
		case gatewayapi_v1.TLSProtocolType:
			return []gatewayapi_v1.Kind{KindTLSRoute, KindTCPRoute}
		case gatewayapi_v1.TCPProtocolType:
			return []gatewayapi_v1.Kind{KindTCPRoute}
		}
	}

	var routeKinds []gatewayapi_v1.Kind

	for _, routeKind := range listener.AllowedRoutes.Kinds {
		if routeKind.Group != nil && *routeKind.Group != gatewayapi_v1.GroupName {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1.ListenerConditionResolvedRefs,
				meta_v1.ConditionFalse,
				gatewayapi_v1.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("Group %q is not supported, group must be %q", *routeKind.Group, gatewayapi_v1.GroupName),
			)
			continue
		}
		if routeKind.Kind != KindHTTPRoute && routeKind.Kind != KindTLSRoute && routeKind.Kind != KindGRPCRoute && routeKind.Kind != KindTCPRoute {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1.ListenerConditionResolvedRefs,
				meta_v1.ConditionFalse,
				gatewayapi_v1.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("Kind %q is not supported, kind must be %q, %q, %q or %q", routeKind.Kind, KindHTTPRoute, KindTLSRoute, KindGRPCRoute, KindTCPRoute),
			)
			continue
		}
		if routeKind.Kind == KindTLSRoute && listener.Protocol != gatewayapi_v1.TLSProtocolType {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1.ListenerConditionResolvedRefs,
				meta_v1.ConditionFalse,
				gatewayapi_v1.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("TLSRoutes are incompatible with listener protocol %q", listener.Protocol),
			)
			continue
		}
		if routeKind.Kind == KindTCPRoute && listener.Protocol != gatewayapi_v1.TCPProtocolType && listener.Protocol != gatewayapi_v1.TLSProtocolType {
			gwAccessor.AddListenerCondition(
				string(listener.Name),
				gatewayapi_v1.ListenerConditionResolvedRefs,
				meta_v1.ConditionFalse,
				gatewayapi_v1.ListenerReasonInvalidRouteKinds,
				fmt.Sprintf("TCPRoutes are incompatible with listener protocol %q", listener.Protocol),
			)
			continue
		}

		routeKinds = append(routeKinds, routeKind.Kind)
	}

	return routeKinds
}

// resolveListenerSecret validates and resolves a Listener TLS secret
// from a given list of certificateRefs. There must be exactly one
// certificate ref, to a core_v1.Secret, that exists, is allowed to be referenced
// based on namespace and ReferenceGrants, and is a valid TLS secret.
// Conditions are set if any of these requirements are not met.
func (p *GatewayAPIProcessor) resolveListenerSecret(certificateRefs []gatewayapi_v1.SecretObjectReference, listenerName string, gwAccessor *status.GatewayStatusUpdate) *Secret {
	if len(certificateRefs) != 1 {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1.ListenerConditionProgrammed,
			meta_v1.ConditionFalse,
			gatewayapi_v1.ListenerReasonInvalid,
			"Listener.TLS.CertificateRefs must contain exactly one entry",
		)
		return nil
	}

	certificateRef := certificateRefs[0]

	// Validate a core_v1.Secret is referenced which can be kind: secret & group: core.
	// ref: https://github.com/kubernetes-sigs/gateway-api/pull/562
	if !isSecretRef(certificateRef) {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1.ListenerConditionResolvedRefs,
			meta_v1.ConditionFalse,
			gatewayapi_v1.ListenerReasonInvalidCertificateRef,
			fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q must contain a reference to a core.Secret", certificateRef.Name),
		)
		return nil
	}

	// If the secret is in a different namespace than the gateway, then we need to
	// check for a ReferenceGrant that allows the reference.
	if certificateRef.Namespace != nil && string(*certificateRef.Namespace) != p.source.gateway.Namespace {
		if !p.validCrossNamespaceRef(
			crossNamespaceFrom{
				group:     gatewayapi_v1.GroupName,
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
				gatewayapi_v1.ListenerConditionResolvedRefs,
				meta_v1.ConditionFalse,
				gatewayapi_v1.ListenerReasonRefNotPermitted,
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

	// Use LookupTLSSecretInsecure instead of LookupTLSSecret since Gateway API uses its own mechanism (ReferenceGrant, not TLSCertificateDelegation)
	// to control access to secrets across namespaces.
	listenerSecret, err := p.source.LookupTLSSecretInsecure(meta)
	if err != nil {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1.ListenerConditionResolvedRefs,
			meta_v1.ConditionFalse,
			gatewayapi_v1.ListenerReasonInvalidCertificateRef,
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

func isSecretRef(certificateRef gatewayapi_v1.SecretObjectReference) bool {
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
func (p *GatewayAPIProcessor) computeHosts(routeHostnames []gatewayapi_v1.Hostname, listenerHostname string, otherListenerHosts []string) (sets.Set[string], []error) {
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

	otherListenerIntersection := func(routeHostname, actualListenerHostname string) bool {
		for _, listenerHostname := range otherListenerHosts {
			if routeHostname == listenerHostname {
				return true
			}
			if strings.HasPrefix(listenerHostname, "*") &&
				hostnameMatchesWildcardHostname(routeHostname, listenerHostname) &&
				len(listenerHostname) > len(actualListenerHostname) {
				return true
			}
		}

		return false
	}

	for i := range routeHostnames {
		routeHostname := string(routeHostnames[i])

		// If the route hostname is not valid, record an error and skip it.
		if err := gatewayapi.IsValidHostname(routeHostname); err != nil {
			errs = append(errs, err)
			continue
		}

		switch {
		// No listener hostname: use the route hostname if
		// it does not also intersect with another Listener.
		case len(listenerHostname) == 0:
			if !otherListenerIntersection(routeHostname, listenerHostname) {
				hostnames.Insert(routeHostname)
			}

		// Listener hostname matches the route hostname: use it.
		case listenerHostname == routeHostname:
			hostnames.Insert(routeHostname)

		// Listener has a wildcard hostname: check if the route hostname matches
		// but do not use it if it intersects with a more specific other Listener.
		case strings.HasPrefix(listenerHostname, "*"):
			if hostnameMatchesWildcardHostname(routeHostname, listenerHostname) && !otherListenerIntersection(routeHostname, listenerHostname) {
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
func (p *GatewayAPIProcessor) namespaceMatches(namespaces *gatewayapi_v1.RouteNamespaces, namespaceSelector labels.Selector, routeNamespace string) bool {
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
	case gatewayapi_v1.NamespacesFromAll:
		return true
	case gatewayapi_v1.NamespacesFromSame:
		return p.source.gateway.Namespace == routeNamespace
	case gatewayapi_v1.NamespacesFromSelector:
		// Look up the route's namespace in the list of cached namespaces.
		if ns := p.source.namespaces[routeNamespace]; ns != nil {
			// Check that the route's namespace is included in the Gateway's
			// namespace selector.
			return namespaceSelector.Matches(labels.Set(ns.Labels))
		}
	}

	return true
}

func (p *GatewayAPIProcessor) computeGatewayConditions(gwAccessor *status.GatewayStatusUpdate, gatewayNotProgrammedCondition *meta_v1.Condition) {
	// If Contour's running, the Gateway is considered accepted.
	gwAccessor.AddCondition(
		gatewayapi_v1.GatewayConditionAccepted,
		meta_v1.ConditionTrue,
		gatewayapi_v1.GatewayReasonAccepted,
		"Gateway is accepted",
	)

	switch {
	case gatewayNotProgrammedCondition != nil:
		gwAccessor.AddCondition(
			gatewayapi_v1.GatewayConditionType(gatewayNotProgrammedCondition.Type),
			gatewayNotProgrammedCondition.Status,
			gatewayapi_v1.GatewayConditionReason(gatewayNotProgrammedCondition.Reason),
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
				if cond.Type == string(gatewayapi_v1.ListenerConditionProgrammed) && cond.Status == meta_v1.ConditionFalse {
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
			gwAccessor.AddCondition(gatewayapi_v1.GatewayConditionProgrammed, meta_v1.ConditionFalse, gatewayapi_v1.GatewayReasonListenersNotValid, "Listeners are not valid")
		} else {
			// Otherwise, Programmed=true.
			gwAccessor.AddCondition(gatewayapi_v1.GatewayConditionProgrammed, meta_v1.ConditionTrue, gatewayapi_v1.GatewayReasonProgrammed, status.MessageValidGateway)
		}
	}
}

func (p *GatewayAPIProcessor) computeTLSRouteForListener(route *gatewayapi_v1alpha2.TLSRoute, routeAccessor *status.RouteParentStatusUpdate, listener *listenerInfo, hosts sets.Set[string]) bool {
	var programmed bool
	for _, rule := range route.Spec.Rules {
		if len(rule.BackendRefs) == 0 {
			routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
			continue
		}

		var proxy TCPProxy
		var totalWeight uint32

		for _, backendRef := range rule.BackendRefs {

			service, cond := p.validateBackendRef(backendRef, KindTLSRoute, route.Namespace)
			if cond != nil {
				routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
				continue
			}

			// Route defaults to a weight of "1" unless otherwise specified.
			routeWeight := uint32(1)
			if backendRef.Weight != nil {
				routeWeight = uint32(*backendRef.Weight) //nolint:gosec // disable G115
			}

			// Keep track of all the weights for this set of backendRefs. This will be
			// used later to understand if all the weights are set to zero.
			totalWeight += routeWeight

			// https://github.com/projectcontour/contour/issues/3593
			service.Weighted.Weight = routeWeight
			proxy.Clusters = append(proxy.Clusters, &Cluster{
				Upstream:                      service,
				SNI:                           service.ExternalName,
				Weight:                        routeWeight,
				TimeoutPolicy:                 ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
				MaxRequestsPerConnection:      p.MaxRequestsPerConnection,
				PerConnectionBufferLimitBytes: p.PerConnectionBufferLimitBytes,
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
			routeAccessor.AddCondition(status.ConditionValidBackendRefs, meta_v1.ConditionFalse, status.ReasonAllBackendRefsHaveZeroWeights, "At least one Spec.Rules.BackendRef must have a non-zero weight.")
			continue
		}

		for host := range hosts {
			secure := p.dag.EnsureSecureVirtualHost(listener.dagListenerName, host)

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
func (p *GatewayAPIProcessor) resolveRouteRefs(route any, routeAccessor *status.RouteParentStatusUpdate) {
	switch route := route.(type) {
	case *gatewayapi_v1.HTTPRoute:
		for _, r := range route.Spec.Rules {
			for _, f := range r.Filters {
				if f.Type == gatewayapi_v1.HTTPRouteFilterRequestMirror && f.RequestMirror != nil {
					_, cond := p.validateBackendObjectRef(f.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindHTTPRoute, route.Namespace)
					if cond != nil {
						routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
					}
				}
			}

			// TODO: validate filter extension refs if they become relevant

			for _, br := range r.BackendRefs {
				_, cond := p.validateBackendRef(br.BackendRef, KindHTTPRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
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
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
				}
			}
		}
	case *gatewayapi_v1.GRPCRoute:
		for _, r := range route.Spec.Rules {
			for _, f := range r.Filters {
				if f.Type == gatewayapi_v1.GRPCRouteFilterRequestMirror && f.RequestMirror != nil {
					_, cond := p.validateBackendObjectRef(f.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindGRPCRoute, route.Namespace)
					if cond != nil {
						routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
					}
				}
			}

			// TODO: validate filter extension refs if they become relevant

			for _, br := range r.BackendRefs {
				_, cond := p.validateBackendRef(br.BackendRef, KindGRPCRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
				}

				// RequestMirror filter is not supported so we don't check it here

				// TODO: validate filter extension refs if they become relevant
			}
		}
	}
}

func parseHTTPRouteTimeouts(httpRouteTimeouts *gatewayapi_v1.HTTPRouteTimeouts) (*RouteTimeoutPolicy, error) {
	if httpRouteTimeouts == nil || (httpRouteTimeouts.Request == nil && httpRouteTimeouts.BackendRequest == nil) {
		return nil, nil
	}

	var responseTimeout timeout.Setting

	if httpRouteTimeouts.Request != nil {
		requestTimeout, err := timeout.Parse(string(*httpRouteTimeouts.Request))
		if err != nil {
			return nil, fmt.Errorf("invalid HTTPRoute.Spec.Rules.Timeouts.Request: %v", err)
		}

		// For Gateway API a zero-valued timeout means disable the timeout.
		if requestTimeout.Duration() == 0 {
			requestTimeout = timeout.DisabledSetting()
		}

		responseTimeout = requestTimeout
	}

	// Note, since retries are not yet implemented in Gateway API, the backend
	// request timeout is functionally equivalent to the request timeout for now.
	// The API spec requires that it be less than/equal to the request timeout if
	// both are specified. This implementation will change when retries are implemented.
	if httpRouteTimeouts.BackendRequest != nil {
		backendRequestTimeout, err := timeout.Parse(string(*httpRouteTimeouts.BackendRequest))
		if err != nil {
			return nil, fmt.Errorf("invalid HTTPRoute.Spec.Rules.Timeouts.BackendRequest: %v", err)
		}

		// For Gateway API a zero-valued timeout means disable the timeout.
		if backendRequestTimeout.Duration() == 0 {
			backendRequestTimeout = timeout.DisabledSetting()
		}

		// If Timeouts.Request was specified, then Timeouts.BackendRequest must be
		// less than/equal to it.
		if responseTimeout.Duration() > 0 && backendRequestTimeout.Duration() > responseTimeout.Duration() {
			return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Timeouts.BackendRequest must be less than/equal to HTTPRoute.Spec.Rules.Timeouts.Request when both are specified")
		}

		responseTimeout = backendRequestTimeout
	}

	return &RouteTimeoutPolicy{
		ResponseTimeout: responseTimeout,
	}, nil
}

func (p *GatewayAPIProcessor) computeHTTPRouteForListener(
	route *gatewayapi_v1.HTTPRoute,
	routeAccessor *status.RouteParentStatusUpdate,
	routeParentRef gatewayapi_v1.ParentReference,
	listener *listenerInfo,
	hosts sets.Set[string],
) {
	// Count number of rules under this Route that are invalid.
	invalidRuleCnt := 0
	for ruleIndex, rule := range route.Spec.Rules {
		// Get match conditions for the rule.
		var matchconditions []*matchConditions
		for _, match := range rule.Matches {
			pathMatch := gatewayPathMatchCondition(match.Path, routeAccessor)
			if pathMatch == nil {
				continue
			}

			headerMatches, err := gatewayHeaderMatchConditions(match.Headers)
			if err != nil {
				routeAccessor.AddCondition(gatewayapi_v1.RouteConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1.RouteReasonUnsupportedValue, err.Error())
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
				routeAccessor.AddCondition(gatewayapi_v1.RouteConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1.RouteReasonUnsupportedValue, err.Error())
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
			err                  error
			redirect             *Redirect
			urlRewriteHostname   string
			mirrorPolicies       []*MirrorPolicy
			requestHeaderPolicy  *HeadersPolicy
			responseHeaderPolicy *HeadersPolicy
			pathRewritePolicy    *PathRewritePolicy
			timeoutPolicy        *RouteTimeoutPolicy
		)

		timeoutPolicy, err = parseHTTPRouteTimeouts(rule.Timeouts)
		if err != nil {
			routeAccessor.AddCondition(gatewayapi_v1.RouteConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1.RouteReasonUnsupportedValue, err.Error())
			continue
		}

		// Per Gateway API docs: "Specifying the same filter multiple times is
		// not supported unless explicitly indicated in the filter." For filters
		// that can't be used multiple times within the same rule, Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range rule.Filters {
			switch filter.Type {
			case gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || requestHeaderPolicy != nil {
					continue
				}

				var err error
				requestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || responseHeaderPolicy != nil {
					continue
				}

				var err error
				responseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			case gatewayapi_v1.HTTPRouteFilterRequestRedirect:
				if filter.RequestRedirect == nil || redirect != nil {
					continue
				}

				var hostname string
				if filter.RequestRedirect.Hostname != nil {
					hostname = string(*filter.RequestRedirect.Hostname)
				}

				var portNumber uint32
				if filter.RequestRedirect.Port != nil {
					portNumber = uint32(*filter.RequestRedirect.Port) //nolint:gosec // disable G115
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
					case gatewayapi_v1.PrefixMatchHTTPPathModifier:
						if filter.RequestRedirect.Path.ReplacePrefixMatch == nil || len(*filter.RequestRedirect.Path.ReplacePrefixMatch) == 0 {
							prefixRewrite = "/"
						} else {
							prefixRewrite = *filter.RequestRedirect.Path.ReplacePrefixMatch
						}
					case gatewayapi_v1.FullPathHTTPPathModifier:
						if filter.RequestRedirect.Path.ReplaceFullPath == nil || len(*filter.RequestRedirect.Path.ReplaceFullPath) == 0 {
							fullPathRewrite = "/"
						} else {
							fullPathRewrite = *filter.RequestRedirect.Path.ReplaceFullPath
						}
					default:
						routeAccessor.AddCondition(
							gatewayapi_v1.RouteConditionAccepted,
							meta_v1.ConditionFalse,
							gatewayapi_v1.RouteReasonUnsupportedValue,
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
			case gatewayapi_v1.HTTPRouteFilterRequestMirror:
				if filter.RequestMirror == nil {
					continue
				}

				mirrorService, cond := p.validateBackendObjectRef(filter.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindHTTPRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
					continue
				}
				mirrorPolicies = append(mirrorPolicies, &MirrorPolicy{
					Cluster: &Cluster{
						Upstream: mirrorService,
					},
					Weight: 100,
				})
			case gatewayapi_v1.HTTPRouteFilterURLRewrite:
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
				case gatewayapi_v1.PrefixMatchHTTPPathModifier:
					if filter.URLRewrite.Path.ReplacePrefixMatch == nil || len(*filter.URLRewrite.Path.ReplacePrefixMatch) == 0 {
						prefixRewrite = "/"
					} else {
						prefixRewrite = *filter.URLRewrite.Path.ReplacePrefixMatch
					}
				case gatewayapi_v1.FullPathHTTPPathModifier:
					if filter.URLRewrite.Path.ReplaceFullPath == nil || len(*filter.URLRewrite.Path.ReplaceFullPath) == 0 {
						fullPathRewrite = "/"
					} else {
						fullPathRewrite = *filter.URLRewrite.Path.ReplaceFullPath
					}
				default:
					routeAccessor.AddCondition(
						gatewayapi_v1.RouteConditionAccepted,
						meta_v1.ConditionFalse,
						gatewayapi_v1.RouteReasonUnsupportedValue,
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
					gatewayapi_v1.RouteConditionAccepted,
					meta_v1.ConditionFalse,
					gatewayapi_v1.RouteReasonUnsupportedValue,
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
		priority := uint8(ruleIndex) //nolint:gosec // disable G115

		// Get our list of routes based on whether it's a redirect or a cluster-backed route.
		// Note that we can end up with multiple routes here since the match conditions are
		// logically "OR"-ed, which we express as multiple routes, each with one of the
		// conditions, all with the same action.
		var routes []*Route

		if redirect != nil {
			routes = p.redirectRoutes(
				matchconditions,
				requestHeaderPolicy,
				responseHeaderPolicy,
				redirect,
				priority,
				KindHTTPRoute,
				route.Namespace,
				route.Name,
			)
		} else {
			// Get clusters from rule backendRefs
			clusters, totalWeight, ok := p.httpClusters(route.Namespace, rule.BackendRefs, routeAccessor, routeParentRef)
			if !ok {
				continue
			}
			routes = p.clusterRoutes(
				matchconditions,
				clusters,
				totalWeight,
				priority,
				KindHTTPRoute,
				route.Namespace,
				route.Name,
				requestHeaderPolicy,
				responseHeaderPolicy,
				mirrorPolicies,
				pathRewritePolicy,
				timeoutPolicy)
		}

		// Check all the routes whether there is conflict against previous rules.
		if !p.hasConflictRoute(listener, hosts, routes) {
			// Add the route if there is no conflict at the same rule level.
			// Add each route to the relevant vhost(s)/svhosts(s).
			for host := range hosts {
				for _, route := range routes {
					switch {
					case listener.tlsSecret != nil:
						svhost := p.dag.EnsureSecureVirtualHost(listener.dagListenerName, host)
						svhost.Secret = listener.tlsSecret
						svhost.AddRoute(route)
					default:
						vhost := p.dag.EnsureVirtualHost(listener.dagListenerName, host)
						vhost.AddRoute(route)
					}
				}
			}
		} else {
			// Skip adding the routes under this rule.
			invalidRuleCnt++
		}
	}

	if invalidRuleCnt == len(route.Spec.Rules) {
		// No rules under the route is valid, mark it as not accepted.
		addRouteNotAcceptedConditionDueToMatchConflict(routeAccessor, KindHTTPRoute)
	} else if invalidRuleCnt > 0 {
		// Some of the rules are conflicted, mark it as partially invalid.
		addRoutePartiallyInvalidConditionDueToMatchPartiallyConflict(routeAccessor, KindHTTPRoute)
	}
}

func (p *GatewayAPIProcessor) hasConflictRoute(listener *listenerInfo, hosts sets.Set[string], routes []*Route) bool {
	// check if there is conflict match first
	for host := range hosts {
		for _, route := range routes {
			switch {
			case listener.tlsSecret != nil:
				svhost := p.dag.EnsureSecureVirtualHost(listener.dagListenerName, host)
				if svhost.HasConflictRoute(route) {
					return true
				}
			default:
				vhost := p.dag.EnsureVirtualHost(listener.dagListenerName, host)
				if vhost.HasConflictRoute(route) {
					return true
				}
			}
		}
	}
	return false
}

func (p *GatewayAPIProcessor) computeGRPCRouteForListener(route *gatewayapi_v1.GRPCRoute, routeAccessor *status.RouteParentStatusUpdate, listener *listenerInfo, hosts sets.Set[string]) bool {
	var programmed bool
	invalidRuleCnt := 0
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
				routeAccessor.AddCondition(gatewayapi_v1alpha2.RouteConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1.RouteReasonUnsupportedValue, err.Error())
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
			mirrorPolicies                            []*MirrorPolicy
		)

		// Per Gateway API docs: "Specifying the same filter multiple times is
		// not supported unless explicitly indicated in the filter." For filters
		// that can't be used multiple times within the same rule, Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range rule.Filters {
			switch filter.Type {
			case gatewayapi_v1.GRPCRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || requestHeaderPolicy != nil {
					continue
				}

				var err error
				requestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1.GRPCRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || responseHeaderPolicy != nil {
					continue
				}

				var err error
				responseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			case gatewayapi_v1.GRPCRouteFilterRequestMirror:
				if filter.RequestMirror == nil {
					continue
				}

				mirrorService, cond := p.validateBackendObjectRef(filter.RequestMirror.BackendRef, "Spec.Rules.Filters.RequestMirror.BackendRef", KindGRPCRoute, route.Namespace)
				if cond != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
					continue
				}
				// If protocol is not set on the service, need to set a default one based on listener's protocol type.
				setDefaultServiceProtocol(mirrorService, listener.listener.Protocol)
				mirrorPolicies = append(mirrorPolicies, &MirrorPolicy{
					Cluster: &Cluster{
						Upstream: mirrorService,
					},
					Weight: 100,
				})
			default:
				routeAccessor.AddCondition(
					gatewayapi_v1.RouteConditionAccepted,
					meta_v1.ConditionFalse,
					gatewayapi_v1.RouteReasonUnsupportedValue,
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
		priority := uint8(ruleIndex) //nolint:gosec // disable G115

		// Note that we can end up with multiple routes here since the match conditions are
		// logically "OR"-ed, which we express as multiple routes, each with one of the
		// conditions, all with the same action.
		var routes []*Route

		clusters, totalWeight, ok := p.grpcClusters(route.Namespace, rule.BackendRefs, routeAccessor, listener.listener.Protocol)
		if !ok {
			continue
		}
		routes = p.clusterRoutes(
			matchconditions,
			clusters,
			totalWeight,
			priority,
			KindGRPCRoute,
			route.Namespace,
			route.Name,
			requestHeaderPolicy,
			responseHeaderPolicy,
			mirrorPolicies,
			nil,
			nil,
		)

		// Check all the routes whether there is conflict against previous rules.
		if !p.hasConflictRoute(listener, hosts, routes) {
			// Add the route if there is no conflict at the same rule level.
			// Add each route to the relevant vhost(s)/svhosts(s).
			for host := range hosts {
				for _, route := range routes {
					switch {
					case listener.tlsSecret != nil:
						svhost := p.dag.EnsureSecureVirtualHost(listener.dagListenerName, host)
						svhost.Secret = listener.tlsSecret
						svhost.AddRoute(route)
					default:
						vhost := p.dag.EnsureVirtualHost(listener.dagListenerName, host)
						vhost.AddRoute(route)
					}
				}
			}
		} else {
			// Skip adding the routes under this rule.
			invalidRuleCnt++
		}
	}

	if invalidRuleCnt == len(route.Spec.Rules) {
		// No rules under the route is valid, mark it as not accepted.
		addRouteNotAcceptedConditionDueToMatchConflict(routeAccessor, KindGRPCRoute)
	} else if invalidRuleCnt > 0 {
		// Some of the rules are conflicted, mark it as partially invalid.
		addRoutePartiallyInvalidConditionDueToMatchPartiallyConflict(routeAccessor, KindGRPCRoute)
	}

	return programmed
}

func gatewayGRPCMethodMatchCondition(match *gatewayapi_v1.GRPCMethodMatch, routeAccessor *status.RouteParentStatusUpdate) (MatchCondition, bool) {
	// If method match is not specified, all services and methods will match.
	if match == nil {
		return &PrefixMatchCondition{Prefix: "/"}, true
	}

	// Type specifies how to match against the service and/or method.
	// Support: Core (Exact with service and method specified)
	// Not Support: Implementation-specific (Exact with method specified but no service specified)
	// Not Support: Implementation-specific (RegularExpression)

	// Support "Exact" match type only. If match type is not specified, use "Exact" as default.
	if match.Type != nil && *match.Type != gatewayapi_v1.GRPCMethodMatchExact {
		routeAccessor.AddCondition(gatewayapi_v1.RouteConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1.RouteReasonUnsupportedValue, "GRPCRoute.Spec.Rules.Matches.Method: Only Exact match type is supported.")
		return nil, false
	}

	if match.Service == nil || isBlank(*match.Service) || match.Method == nil || isBlank(*match.Method) {
		routeAccessor.AddCondition(gatewayapi_v1.RouteConditionAccepted, meta_v1.ConditionFalse, status.ReasonInvalidMethodMatch, "GRPCRoute.Spec.Rules.Matches.Method: Both Service and Method need be configured.")
		return nil, false
	}

	// Convert service and method into path
	path := "/" + *match.Service + "/" + *match.Method

	return &ExactMatchCondition{Path: path}, true
}

func gatewayGRPCHeaderMatchConditions(matches []gatewayapi_v1.GRPCHeaderMatch) ([]HeaderMatchCondition, error) {
	var headerMatchConditions []HeaderMatchCondition
	seenNames := sets.New[string]()

	for _, match := range matches {
		// "Exact" and "RegularExpression" are the only supported match types. If match type is not specified, use "Exact" as default.
		var matchType string
		switch ptr.Deref(match.Type, gatewayapi_v1.GRPCHeaderMatchExact) {
		case gatewayapi_v1.GRPCHeaderMatchExact:
			matchType = HeaderMatchTypeExact
		case gatewayapi_v1.GRPCHeaderMatchRegularExpression:
			if err := ValidateRegex(match.Value); err != nil {
				return nil, fmt.Errorf("GRPCRoute.Spec.Rules.Matches.Headers: Invalid value for RegularExpression match type is specified")
			}
			matchType = HeaderMatchTypeRegex
		default:
			return nil, fmt.Errorf("GRPCRoute.Spec.Rules.Matches.Headers: Only Exact match type and RegularExpression match type are supported")
		}

		// If multiple match conditions are found for the same header name (case-insensitive),
		// use the first one and ignore subsequent ones.
		upperName := strings.ToUpper(string(match.Name))
		if seenNames.Has(upperName) {
			continue
		}
		seenNames.Insert(upperName)

		headerMatchConditions = append(headerMatchConditions, HeaderMatchCondition{
			MatchType: matchType,
			Name:      string(match.Name),
			Value:     match.Value,
		})
	}

	return headerMatchConditions, nil
}

func (p *GatewayAPIProcessor) computeTCPRouteForListener(route *gatewayapi_v1alpha2.TCPRoute, routeAccessor *status.RouteParentStatusUpdate, listener *listenerInfo) bool {
	if len(route.Spec.Rules) != 1 {
		routeAccessor.AddCondition(
			gatewayapi_v1.RouteConditionAccepted,
			meta_v1.ConditionFalse,
			"InvalidRouteRules",
			"TCPRoute must have only a single rule defined",
		)

		return false
	}

	rule := route.Spec.Rules[0]

	if len(rule.BackendRefs) == 0 {
		routeAccessor.AddCondition(
			gatewayapi_v1.RouteConditionResolvedRefs,
			meta_v1.ConditionFalse,
			status.ReasonDegraded,
			"At least one Spec.Rules.BackendRef must be specified.",
		)
		return false
	}

	var proxy TCPProxy
	var totalWeight uint32

	for _, backendRef := range rule.BackendRefs {
		service, cond := p.validateBackendRef(backendRef, KindTCPRoute, route.Namespace)
		if cond != nil {
			routeAccessor.AddCondition(
				gatewayapi_v1.RouteConditionType(cond.Type),
				cond.Status,
				gatewayapi_v1.RouteConditionReason(cond.Reason),
				cond.Message,
			)
			continue
		}

		// Route defaults to a weight of "1" unless otherwise specified.
		routeWeight := uint32(1)
		if backendRef.Weight != nil {
			routeWeight = uint32(*backendRef.Weight) //nolint:gosec // disable G115
		}

		// Keep track of all the weights for this set of backendRefs. This will be
		// used later to understand if all the weights are set to zero.
		totalWeight += routeWeight

		// https://github.com/projectcontour/contour/issues/3593
		service.Weighted.Weight = routeWeight
		proxy.Clusters = append(proxy.Clusters, &Cluster{
			Upstream:                      service,
			SNI:                           service.ExternalName,
			Weight:                        routeWeight,
			TimeoutPolicy:                 ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
			MaxRequestsPerConnection:      p.MaxRequestsPerConnection,
			PerConnectionBufferLimitBytes: p.PerConnectionBufferLimitBytes,
		})
	}

	// No clusters added: they were all invalid, so reject
	// the route (it already has a relevant condition set).
	if len(proxy.Clusters) == 0 {
		return false
	}

	// If we have valid clusters but they all have a zero
	// weight, reject the route.
	if totalWeight == 0 {
		routeAccessor.AddCondition(
			status.ConditionValidBackendRefs,
			meta_v1.ConditionFalse,
			status.ReasonAllBackendRefsHaveZeroWeights,
			"At least one Spec.Rules.BackendRef must have a non-zero weight.",
		)
		return false
	}

	if listener.tlsSecret != nil {
		secure := p.dag.EnsureSecureVirtualHost(listener.dagListenerName, "*")
		secure.Secret = listener.tlsSecret
		secure.TCPProxy = &proxy
	} else {
		p.dag.Listeners[listener.dagListenerName].TCPProxy = &proxy
	}

	return true
}

// validateBackendRef verifies that the specified BackendRef is valid.
// Returns a meta_v1.Condition for the route if any errors are detected.
func (p *GatewayAPIProcessor) validateBackendRef(backendRef gatewayapi_v1.BackendRef, routeKind, routeNamespace string) (*Service, *meta_v1.Condition) {
	return p.validateBackendObjectRef(backendRef.BackendObjectReference, "Spec.Rules.BackendRef", routeKind, routeNamespace)
}

func resolvedRefsFalse(reason gatewayapi_v1.RouteConditionReason, msg string) meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.RouteConditionResolvedRefs),
		Status:  meta_v1.ConditionFalse,
		Reason:  string(reason),
		Message: msg,
	}
}

// validateBackendObjectRef verifies that the specified BackendObjectReference
// is valid. Returns a meta_v1.Condition for the route if any errors are detected.
// As BackendObjectReference is used in multiple fields, the given field is used
// to build the message in meta_v1.Condition.
func (p *GatewayAPIProcessor) validateBackendObjectRef(
	backendObjectRef gatewayapi_v1.BackendObjectReference,
	field string,
	routeKind string,
	routeNamespace string,
) (*Service, *meta_v1.Condition) {
	if !(backendObjectRef.Group == nil || *backendObjectRef.Group == "") {
		return nil, ptr.To(resolvedRefsFalse(gatewayapi_v1.RouteReasonInvalidKind, fmt.Sprintf("%s.Group must be \"\"", field)))
	}

	if !(backendObjectRef.Kind != nil && *backendObjectRef.Kind == "Service") {
		return nil, ptr.To(resolvedRefsFalse(gatewayapi_v1.RouteReasonInvalidKind, fmt.Sprintf("%s.Kind must be 'Service'", field)))
	}

	if backendObjectRef.Name == "" {
		return nil, ptr.To(resolvedRefsFalse(status.ReasonDegraded, fmt.Sprintf("%s.Name must be specified", field)))
	}

	if backendObjectRef.Port == nil {
		return nil, ptr.To(resolvedRefsFalse(status.ReasonDegraded, fmt.Sprintf("%s.Port must be specified", field)))
	}

	// If the backend is in a different namespace than the route, then we need to
	// check for a ReferenceGrant that allows the reference.
	if backendObjectRef.Namespace != nil && string(*backendObjectRef.Namespace) != routeNamespace {
		if !p.validCrossNamespaceRef(
			crossNamespaceFrom{
				group:     string(gatewayapi_v1.GroupName),
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
			return nil, ptr.To(resolvedRefsFalse(gatewayapi_v1.RouteReasonRefNotPermitted, fmt.Sprintf("%s.Namespace must match the route's namespace or be covered by a ReferenceGrant", field)))
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
		return nil, ptr.To(resolvedRefsFalse(gatewayapi_v1.RouteReasonBackendNotFound, fmt.Sprintf("service %q is invalid: %s", meta.Name, err)))
	}

	service = serviceCircuitBreakerPolicy(service, p.GlobalCircuitBreakerDefaults)
	if err = validateAppProtocol(&service.Weighted.ServicePort); err != nil {
		return nil, ptr.To(resolvedRefsFalse(gatewayapi_v1.RouteReasonUnsupportedProtocol, err.Error()))
	}

	return service, nil
}

func validateAppProtocol(svc *core_v1.ServicePort) error {
	if svc.AppProtocol == nil {
		return nil
	}
	if _, ok := toContourProtocol(*svc.AppProtocol); ok {
		return nil
	}
	return fmt.Errorf("AppProtocol: \"%s\" is unsupported", *svc.AppProtocol)
}

func gatewayPathMatchCondition(match *gatewayapi_v1.HTTPPathMatch, routeAccessor *status.RouteParentStatusUpdate) MatchCondition {
	if match == nil {
		return &PrefixMatchCondition{Prefix: "/"}
	}

	path := ptr.Deref(match.Value, "/")

	// If path match type is not defined, default to 'PathPrefix'.
	if match.Type == nil || *match.Type == gatewayapi_v1.PathMatchPathPrefix {
		if !strings.HasPrefix(path, "/") {
			routeAccessor.AddCondition(status.ConditionValidMatches, meta_v1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must start with '/'.")
			return nil
		}
		if strings.Contains(path, "//") {
			routeAccessor.AddCondition(status.ConditionValidMatches, meta_v1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must not contain consecutive '/' characters.")
			return nil
		}

		// As an optimization, if path is just "/", we can use
		// string prefix matching instead of segment prefix
		// matching which requires a regex.
		if path == "/" {
			return &PrefixMatchCondition{Prefix: path}
		}
		return &PrefixMatchCondition{Prefix: path, PrefixMatchType: PrefixMatchSegment}
	}

	if *match.Type == gatewayapi_v1.PathMatchExact {
		if !strings.HasPrefix(path, "/") {
			routeAccessor.AddCondition(status.ConditionValidMatches, meta_v1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must start with '/'.")
			return nil
		}
		if strings.Contains(path, "//") {
			routeAccessor.AddCondition(status.ConditionValidMatches, meta_v1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value must not contain consecutive '/' characters.")
			return nil
		}
		return &ExactMatchCondition{Path: path}
	}

	if *match.Type == gatewayapi_v1.PathMatchRegularExpression {
		if err := ValidateRegex(*match.Value); err != nil {
			routeAccessor.AddCondition(status.ConditionValidMatches, meta_v1.ConditionFalse, status.ReasonInvalidPathMatch, "Match.Path.Value is invalid for RegularExpression match type.")
			return nil
		}
		return &RegexMatchCondition{Regex: path}
	}

	routeAccessor.AddCondition(
		gatewayapi_v1.RouteConditionAccepted,
		meta_v1.ConditionFalse,
		gatewayapi_v1.RouteReasonUnsupportedValue,
		"HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type, Exact match type and RegularExpression match type are supported.",
	)
	return nil
}

func gatewayHeaderMatchConditions(matches []gatewayapi_v1.HTTPHeaderMatch) ([]HeaderMatchCondition, error) {
	var headerMatchConditions []HeaderMatchCondition
	seenNames := sets.New[string]()

	for _, match := range matches {
		// "Exact" and "RegularExpression" are the only supported match types. If match type is not specified, use "Exact" as default.
		var matchType string
		switch ptr.Deref(match.Type, gatewayapi_v1.HeaderMatchExact) {
		case gatewayapi_v1.HeaderMatchExact:
			matchType = HeaderMatchTypeExact
		case gatewayapi_v1.HeaderMatchRegularExpression:
			if err := ValidateRegex(match.Value); err != nil {
				return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.Headers: Invalid value for RegularExpression match type is specified")
			}
			matchType = HeaderMatchTypeRegex
		default:
			return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.Headers: Only Exact match type and RegularExpression match type are supported")
		}

		// If multiple match conditions are found for the same header name (case-insensitive),
		// use the first one and ignore subsequent ones.
		upperName := strings.ToUpper(string(match.Name))
		if seenNames.Has(upperName) {
			continue
		}
		seenNames.Insert(upperName)

		headerMatchConditions = append(headerMatchConditions, HeaderMatchCondition{
			MatchType: matchType,
			Name:      string(match.Name),
			Value:     match.Value,
		})
	}

	return headerMatchConditions, nil
}

func gatewayQueryParamMatchConditions(matches []gatewayapi_v1.HTTPQueryParamMatch) ([]QueryParamMatchCondition, error) {
	var dagMatchConditions []QueryParamMatchCondition
	seenNames := sets.New[gatewayapi_v1.HTTPHeaderName]()

	for _, match := range matches {
		var matchType string
		switch ptr.Deref(match.Type, gatewayapi_v1.QueryParamMatchExact) {
		case gatewayapi_v1.QueryParamMatchExact:
			matchType = HeaderMatchTypeExact
		case gatewayapi_v1.QueryParamMatchRegularExpression:
			if err := ValidateRegex(match.Value); err != nil {
				return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.QueryParams: Invalid value for RegularExpression match type is specified")
			}
			matchType = HeaderMatchTypeRegex
		default:
			return nil, fmt.Errorf("HTTPRoute.Spec.Rules.Matches.QueryParams: Only Exact and RegularExpression match types are supported")
		}

		// If multiple match conditions are found for the same value,
		// use the first one and ignore subsequent ones.
		if seenNames.Has(match.Name) {
			continue
		}
		seenNames.Insert(match.Name)

		dagMatchConditions = append(dagMatchConditions, QueryParamMatchCondition{
			MatchType: matchType,
			Name:      string(match.Name),
			Value:     match.Value,
		})
	}

	return dagMatchConditions, nil
}

// httpClusters builds clusters from backendRef.
func (p *GatewayAPIProcessor) httpClusters(routeNamespace string, backendRefs []gatewayapi_v1.HTTPBackendRef, routeAccessor *status.RouteParentStatusUpdate, routeParentRef gatewayapi_v1.ParentReference) ([]*Cluster, uint32, bool) {
	totalWeight := uint32(0)

	if len(backendRefs) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
		return nil, totalWeight, false
	}

	var clusters []*Cluster

	// Validate the backend refs.
	for _, backendRef := range backendRefs {
		service, cond := p.validateBackendRef(backendRef.BackendRef, KindHTTPRoute, routeNamespace)
		if cond != nil {
			routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
			continue
		}

		upstreamValidation, upstreamTLS := p.computeBackendTLSPolicies(routeNamespace, backendRef, service, routeParentRef)
		if upstreamValidation != nil {
			service.Protocol = "tls"
		}

		var clusterRequestHeaderPolicy *HeadersPolicy
		var clusterResponseHeaderPolicy *HeadersPolicy

		// Per Gateway API docs: "Specifying the same filter multiple times is
		// not supported unless explicitly indicated in the filter." For filters
		// that can't be used multiple times within the same rule, Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range backendRef.Filters {
			switch filter.Type {
			case gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || clusterRequestHeaderPolicy != nil {
					continue
				}

				var err error
				clusterRequestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || clusterResponseHeaderPolicy != nil {
					continue
				}

				var err error
				clusterResponseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			default:
				routeAccessor.AddCondition(
					gatewayapi_v1.RouteConditionAccepted,
					meta_v1.ConditionFalse,
					gatewayapi_v1.RouteReasonUnsupportedValue,
					"HTTPRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier and ResponseHeaderModifier type is supported.",
				)
			}
		}

		// Route defaults to a weight of "1" unless otherwise specified.
		routeWeight := uint32(1)
		if backendRef.Weight != nil {
			routeWeight = uint32(*backendRef.Weight) //nolint:gosec // disable G115
		}

		// Keep track of all the weights for this set of backend refs. This will be
		// used later to understand if all the weights are set to zero.
		totalWeight += routeWeight

		// https://github.com/projectcontour/contour/issues/3593
		service.Weighted.Weight = routeWeight
		clusters = append(clusters, &Cluster{
			Upstream:                      service,
			Weight:                        routeWeight,
			Protocol:                      service.Protocol,
			RequestHeadersPolicy:          clusterRequestHeaderPolicy,
			ResponseHeadersPolicy:         clusterResponseHeaderPolicy,
			TimeoutPolicy:                 ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
			MaxRequestsPerConnection:      p.MaxRequestsPerConnection,
			PerConnectionBufferLimitBytes: p.PerConnectionBufferLimitBytes,
			UpstreamValidation:            upstreamValidation,
			UpstreamTLS:                   upstreamTLS,
		})
	}
	return clusters, totalWeight, true
}

// computeBackendTLSPolicies returns the upstreamValidation and upstreamTLS
// fields for the cluster that is being calculated if there is an associated
// BackendTLSPolicy for the service being referenced.
//
// If no BackendTLSPolicy is found or the BackendTLSPolicy is invalid then nil
// is returned for both fields.
func (p *GatewayAPIProcessor) computeBackendTLSPolicies(routeNamespace string, backendRef gatewayapi_v1.HTTPBackendRef, service *Service, routeParentRef gatewayapi_v1.ParentReference) (*PeerValidationContext, *UpstreamTLS) {
	var upstreamValidation *PeerValidationContext
	var upstreamTLS *UpstreamTLS

	var backendRefGroup gatewayapi_v1.Group
	if backendRef.Group != nil {
		backendRefGroup = *backendRef.Group
	}

	var backendRefKind gatewayapi_v1alpha2.Kind
	if backendRef.Kind != nil {
		backendRefKind = *backendRef.Kind
	}

	var backendNamespace *gatewayapi_v1.Namespace
	if backendRef.Namespace != nil && *backendRef.Namespace != "" {
		backendNamespace = backendRef.Namespace
	} else {
		backendNamespace = ptr.To(gatewayapi_v1.Namespace(routeNamespace))
	}

	policyTargetRef := gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
		LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
			Group: backendRefGroup,
			Kind:  backendRefKind,
			Name:  backendRef.Name,
		},
		SectionName: ptr.To(gatewayapi_v1alpha2.SectionName(service.Weighted.ServicePort.Name)),
	}

	// Check to see if there is any BackendTLSPolicy matching this service and service port
	backendTLSPolicy, found := p.source.LookupBackendTLSPolicyByTargetRef(policyTargetRef, string(*backendNamespace))
	if found {
		backendTLSPolicyAccessor, commit := p.dag.StatusCache.BackendTLSPolicyConditionsAccessor(
			k8s.NamespacedNameOf(backendTLSPolicy),
			backendTLSPolicy.GetGeneration(),
		)
		defer commit()
		backendTLSPolicyAncestorStatus := backendTLSPolicyAccessor.StatusUpdateFor(routeParentRef)

		if backendTLSPolicy.Spec.Validation.WellKnownCACertificates != nil && *backendTLSPolicy.Spec.Validation.WellKnownCACertificates != "" {
			backendTLSPolicyAncestorStatus.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1alpha2.PolicyReasonInvalid, "BackendTLSPolicy.Spec.Validation.WellKnownCACertificates is unsupported.")
			return nil, nil
		}

		if err := gatewayapi.IsValidHostname(string(backendTLSPolicy.Spec.Validation.Hostname)); err != nil {
			backendTLSPolicyAncestorStatus.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1alpha2.PolicyReasonInvalid, fmt.Sprintf("BackendTLSPolicy.Spec.Validation.Hostname %q is invalid. Hostname must be a valid RFC 1123 fully qualified domain name. Wildcard domains and numeric IP addresses are not allowed", backendTLSPolicy.Spec.Validation.Hostname))
			return nil, nil
		}

		if strings.Contains(string(backendTLSPolicy.Spec.Validation.Hostname), "*") {
			backendTLSPolicyAncestorStatus.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1alpha2.PolicyReasonInvalid, fmt.Sprintf("BackendTLSPolicy.Spec.Validation.Hostname %q is invalid. Hostname must be a valid RFC 1123 fully qualified domain name. Wildcard domains and numeric IP addresses are not allowed", backendTLSPolicy.Spec.Validation.Hostname))
			return nil, nil
		}

		var isInvalidCertChain bool
		var caSecrets []*Secret
		for _, certRef := range backendTLSPolicy.Spec.Validation.CACertificateRefs {
			switch certRef.Kind {
			case "Secret":
				caSecret, err := p.source.LookupCASecret(types.NamespacedName{
					Name:      string(certRef.Name),
					Namespace: backendTLSPolicy.Namespace,
				}, backendTLSPolicy.Namespace)
				if err != nil {
					backendTLSPolicyAncestorStatus.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1alpha2.PolicyReasonInvalid, fmt.Sprintf("Could not find CACertificateRef Secret: %s/%s", backendTLSPolicy.Namespace, certRef.Name))
					isInvalidCertChain = true
					continue
				}
				caSecrets = append(caSecrets, caSecret)
			case "ConfigMap":
				caSecret, err := p.source.LookupCAConfigMap(types.NamespacedName{
					Name:      string(certRef.Name),
					Namespace: backendTLSPolicy.Namespace,
				})
				if err != nil {
					backendTLSPolicyAncestorStatus.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1alpha2.PolicyReasonInvalid, fmt.Sprintf("Could not find CACertificateRef ConfigMap: %s/%s", backendTLSPolicy.Namespace, certRef.Name))
					isInvalidCertChain = true
					continue
				}
				caSecrets = append(caSecrets, caSecret)
			default:
				backendTLSPolicyAncestorStatus.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1alpha2.PolicyReasonInvalid, fmt.Sprintf("BackendTLSPolicy.Spec.Validation.CACertificateRef.Kind %q is unsupported. Only ConfigMap or Secret Kind is supported.", certRef.Kind))
				isInvalidCertChain = true
				continue
			}
		}

		if isInvalidCertChain {
			return nil, nil
		}

		if len(caSecrets) != 0 {
			upstreamValidation = &PeerValidationContext{
				CACertificates: caSecrets,
				SubjectNames:   []string{string(backendTLSPolicy.Spec.Validation.Hostname)},
			}

			upstreamTLS = p.UpstreamTLS

			backendTLSPolicyAncestorStatus.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionTrue, gatewayapi_v1alpha2.PolicyReasonAccepted, "Accepted BackendTLSPolicy")
		}
	}

	return upstreamValidation, upstreamTLS
}

// grpcClusters builds clusters from backendRef.
func (p *GatewayAPIProcessor) grpcClusters(routeNamespace string, backendRefs []gatewayapi_v1.GRPCBackendRef, routeAccessor *status.RouteParentStatusUpdate, protocolType gatewayapi_v1.ProtocolType) ([]*Cluster, uint32, bool) {
	totalWeight := uint32(0)

	if len(backendRefs) == 0 {
		routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified.")
		return nil, totalWeight, false
	}

	var clusters []*Cluster

	// Validate the backend refs.
	for _, backendRef := range backendRefs {
		service, cond := p.validateBackendRef(backendRef.BackendRef, KindGRPCRoute, routeNamespace)
		if cond != nil {
			routeAccessor.AddCondition(gatewayapi_v1.RouteConditionType(cond.Type), cond.Status, gatewayapi_v1.RouteConditionReason(cond.Reason), cond.Message)
			continue
		}

		var clusterRequestHeaderPolicy, clusterResponseHeaderPolicy *HeadersPolicy

		// Per Gateway API docs: "Specifying the same filter multiple times is
		// not supported unless explicitly indicated in the filter." For filters
		// that can't be used multiple times within the same rule, Contour
		// chooses to use the first instance of each filter type and ignore
		// subsequent instances.
		for _, filter := range backendRef.Filters {
			switch filter.Type {
			case gatewayapi_v1.GRPCRouteFilterRequestHeaderModifier:
				if filter.RequestHeaderModifier == nil || clusterRequestHeaderPolicy != nil {
					continue
				}

				var err error
				clusterRequestHeaderPolicy, err = headersPolicyGatewayAPI(filter.RequestHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on request headers", err))
				}
			case gatewayapi_v1.GRPCRouteFilterResponseHeaderModifier:
				if filter.ResponseHeaderModifier == nil || clusterResponseHeaderPolicy != nil {
					continue
				}

				var err error
				clusterResponseHeaderPolicy, err = headersPolicyGatewayAPI(filter.ResponseHeaderModifier, string(filter.Type))
				if err != nil {
					routeAccessor.AddCondition(gatewayapi_v1.RouteConditionResolvedRefs, meta_v1.ConditionFalse, status.ReasonDegraded, fmt.Sprintf("%s on response headers", err))
				}
			default:
				routeAccessor.AddCondition(
					gatewayapi_v1.RouteConditionAccepted,
					meta_v1.ConditionFalse,
					gatewayapi_v1.RouteReasonUnsupportedValue,
					"GRPCRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier and ResponseHeaderModifier type is supported.",
				)
			}
		}

		// Route defaults to a weight of "1" unless otherwise specified.
		routeWeight := uint32(1)
		if backendRef.Weight != nil {
			routeWeight = uint32(*backendRef.Weight) //nolint:gosec // disable G115
		}

		// Keep track of all the weights for this set of backend refs. This will be
		// used later to understand if all the weights are set to zero.
		totalWeight += routeWeight

		// If protocol is not set on the service, need to set a default one based on listener's protocol type.
		setDefaultServiceProtocol(service, protocolType)

		// https://github.com/projectcontour/contour/issues/3593
		service.Weighted.Weight = routeWeight
		clusters = append(clusters, &Cluster{
			Upstream:                      service,
			Weight:                        routeWeight,
			Protocol:                      service.Protocol,
			RequestHeadersPolicy:          clusterRequestHeaderPolicy,
			ResponseHeadersPolicy:         clusterResponseHeaderPolicy,
			TimeoutPolicy:                 ClusterTimeoutPolicy{ConnectTimeout: p.ConnectTimeout},
			MaxRequestsPerConnection:      p.MaxRequestsPerConnection,
			PerConnectionBufferLimitBytes: p.PerConnectionBufferLimitBytes,
		})
	}
	return clusters, totalWeight, true
}

// clusterRoutes builds a []*dag.Route for the supplied set of matchConditions, headerPolicies and backendRefs.
func (p *GatewayAPIProcessor) clusterRoutes(
	matchConditions []*matchConditions,
	clusters []*Cluster,
	totalWeight uint32,
	priority uint8,
	kind string,
	namespace string,
	name string,
	requestHeaderPolicy *HeadersPolicy,
	responseHeaderPolicy *HeadersPolicy,
	mirrorPolicies []*MirrorPolicy,
	pathRewritePolicy *PathRewritePolicy,
	timeoutPolicy *RouteTimeoutPolicy,
) []*Route {
	var routes []*Route

	// Per Gateway API: "Each match is independent,
	// i.e. this rule will be matched if any one of
	// the matches is satisfied." To implement this,
	// we create a separate route per match.
	for _, mc := range matchConditions {
		// Re-configure the PathRewritePolicy if we're trying to remove
		// the prefix entirely.
		pathRewritePolicy = handlePathRewritePrefixRemoval(pathRewritePolicy, mc)

		route := &Route{
			Clusters:                  clusters,
			PathMatchCondition:        mc.path,
			HeaderMatchConditions:     mc.headers,
			QueryParamMatchConditions: mc.queryParams,
			RequestHeadersPolicy:      requestHeaderPolicy,
			ResponseHeadersPolicy:     responseHeaderPolicy,
			MirrorPolicies:            mirrorPolicies,
			Priority:                  priority,
			PathRewritePolicy:         pathRewritePolicy,
		}
		if timeoutPolicy != nil {
			route.TimeoutPolicy = *timeoutPolicy
		}

		if p.SetSourceMetadataOnRoutes {
			route.Kind = kind
			route.Namespace = namespace
			route.Name = name
		}

		routes = append(routes, route)
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

func setDefaultServiceProtocol(service *Service, protocolType gatewayapi_v1.ProtocolType) {
	// For GRPCRoute, if the protocol is not set on the Service via annotation,
	// we should assume a protocol that matches what listener the route was attached to
	if isBlank(service.Protocol) {
		if protocolType == gatewayapi_v1.HTTPProtocolType {
			service.Protocol = "h2c"
		} else if protocolType == gatewayapi_v1.HTTPSProtocolType {
			service.Protocol = "h2"
		}
	}
}

// redirectRoutes builds a []*dag.Route for the supplied set of matchConditions, headerPolicies and redirect.
func (p *GatewayAPIProcessor) redirectRoutes(
	matchConditions []*matchConditions,
	requestHeaderPolicy *HeadersPolicy,
	responseHeaderPolicy *HeadersPolicy,
	redirect *Redirect,
	priority uint8,
	kind string,
	namespace string,
	name string,
) []*Route {
	var routes []*Route

	// Per Gateway API: "Each match is independent,
	// i.e. this rule will be matched if any one of
	// the matches is satisfied." To implement this,
	// we create a separate route per match.
	for _, mc := range matchConditions {
		// Re-configure the PathRewritePolicy if we're trying to remove
		// the prefix entirely.
		redirect.PathRewritePolicy = handlePathRewritePrefixRemoval(redirect.PathRewritePolicy, mc)

		route := &Route{
			Priority:              priority,
			Redirect:              redirect,
			PathMatchCondition:    mc.path,
			HeaderMatchConditions: mc.headers,
			RequestHeadersPolicy:  requestHeaderPolicy,
			ResponseHeadersPolicy: responseHeaderPolicy,
		}

		if p.SetSourceMetadataOnRoutes {
			route.Kind = kind
			route.Namespace = namespace
			route.Name = name
		}

		routes = append(routes, route)
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

// sortHTTPRoutes sorts httproutes based on creationTimestamp in ascending order
// if creationTimestamps are the same, sort based on namespaced name ("<namespace>/<name>") in alphetical ascending order
func sortHTTPRoutes(m map[types.NamespacedName]*gatewayapi_v1.HTTPRoute) []*gatewayapi_v1.HTTPRoute {
	routes := []*gatewayapi_v1.HTTPRoute{}
	for _, r := range m {
		routes = append(routes, r)
	}
	sort.SliceStable(routes, func(i, j int) bool {
		// if the creation time is the same, compare the route name
		if routes[i].CreationTimestamp.Equal(&routes[j].CreationTimestamp) {
			return k8s.NamespacedNameOf(routes[i]).String() <
				k8s.NamespacedNameOf(routes[j]).String()
		}
		return routes[i].CreationTimestamp.Before(&routes[j].CreationTimestamp)
	})

	return routes
}

// sortGRPCRoutes sorts grpcroutes based on creationTimestamp in ascending order
// if creationTimestamps are the same, sort based on namespaced name ("<namespace>/<name>") in alphetical ascending order
func sortGRPCRoutes(m map[types.NamespacedName]*gatewayapi_v1.GRPCRoute) []*gatewayapi_v1.GRPCRoute {
	routes := []*gatewayapi_v1.GRPCRoute{}
	for _, r := range m {
		routes = append(routes, r)
	}
	sort.SliceStable(routes, func(i, j int) bool {
		// if the creation time is the same, compare the route name
		if routes[i].CreationTimestamp.Equal(&routes[j].CreationTimestamp) {
			return k8s.NamespacedNameOf(routes[i]).String() <
				k8s.NamespacedNameOf(routes[j]).String()
		}
		return routes[i].CreationTimestamp.Before(&routes[j].CreationTimestamp)
	})

	return routes
}

func addRouteNotAcceptedConditionDueToMatchConflict(routeAccessor *status.RouteParentStatusUpdate, routeKind string) {
	routeAccessor.AddCondition(
		gatewayapi_v1.RouteConditionAccepted,
		meta_v1.ConditionFalse,
		status.ReasonRouteRuleMatchConflict,
		fmt.Sprintf(status.MessageRouteRuleMatchConflict, routeKind, routeKind),
	)
}

func addRoutePartiallyInvalidConditionDueToMatchPartiallyConflict(routeAccessor *status.RouteParentStatusUpdate, routeKind string) {
	routeAccessor.AddCondition(
		gatewayapi_v1.RouteConditionPartiallyInvalid,
		meta_v1.ConditionTrue,
		status.ReasonRouteRuleMatchPartiallyConflict,
		fmt.Sprintf(status.MessageRouteRuleMatchPartiallyConflict, routeKind, routeKind),
	)
}
