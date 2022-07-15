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

package gatewayapi

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func SectionNamePtr(sectionName string) *gatewayapi_v1beta1.SectionName {
	gwSectionName := gatewayapi_v1beta1.SectionName(sectionName)
	return &gwSectionName
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func SectionNamePtrV1Alpha2(sectionName string) *gatewayapi_v1alpha2.SectionName {
	gwSectionName := gatewayapi_v1alpha2.SectionName(sectionName)
	return &gwSectionName
}

func PortNumPtr(port int) *gatewayapi_v1beta1.PortNumber {
	pn := gatewayapi_v1beta1.PortNumber(port)
	return &pn
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func PortNumPtrV1Alpha2(port int) *gatewayapi_v1alpha2.PortNumber {
	pn := gatewayapi_v1alpha2.PortNumber(port)
	return &pn
}

func FromNamespacesPtr(val gatewayapi_v1beta1.FromNamespaces) *gatewayapi_v1beta1.FromNamespaces {
	return &val
}

func PathMatchTypePtr(val gatewayapi_v1beta1.PathMatchType) *gatewayapi_v1beta1.PathMatchType {
	return &val
}

func HeaderMatchTypePtr(val gatewayapi_v1beta1.HeaderMatchType) *gatewayapi_v1beta1.HeaderMatchType {
	return &val
}

func QueryParamMatchTypePtr(val gatewayapi_v1beta1.QueryParamMatchType) *gatewayapi_v1beta1.QueryParamMatchType {
	return &val
}

func TLSModeTypePtr(mode gatewayapi_v1beta1.TLSModeType) *gatewayapi_v1beta1.TLSModeType {
	return &mode
}

func HTTPMethodPtr(method gatewayapi_v1beta1.HTTPMethod) *gatewayapi_v1beta1.HTTPMethod {
	return &method
}

func AddressTypePtr(addressType gatewayapi_v1beta1.AddressType) *gatewayapi_v1beta1.AddressType {
	return &addressType
}

func ListenerHostname(host string) *gatewayapi_v1beta1.Hostname {
	h := gatewayapi_v1beta1.Hostname(host)
	return &h
}

func PreciseHostname(host string) *gatewayapi_v1beta1.PreciseHostname {
	h := gatewayapi_v1beta1.PreciseHostname(host)
	return &h
}

func CertificateRef(name, namespace string) gatewayapi_v1beta1.SecretObjectReference {
	ref := gatewayapi_v1beta1.SecretObjectReference{
		Group: GroupPtr(""),
		Kind:  KindPtr("Secret"),
		Name:  gatewayapi_v1beta1.ObjectName(name),
	}

	if namespace != "" {
		ref.Namespace = NamespacePtr(namespace)
	}

	return ref
}

func GatewayParentRef(namespace, name string) gatewayapi_v1beta1.ParentReference {
	parentRef := gatewayapi_v1beta1.ParentReference{
		Group: GroupPtr(gatewayapi_v1alpha2.GroupName),
		Kind:  KindPtr("Gateway"),
		Name:  gatewayapi_v1beta1.ObjectName(name),
	}

	if namespace != "" {
		parentRef.Namespace = NamespacePtr(namespace)
	}

	return parentRef
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func GatewayParentRefV1Alpha2(namespace, name string) gatewayapi_v1alpha2.ParentReference {
	parentRef := gatewayapi_v1alpha2.ParentReference{
		Group: GroupPtrV1Alpha2(gatewayapi_v1alpha2.GroupName),
		Kind:  KindPtrV1Alpha2("Gateway"),
		Name:  gatewayapi_v1alpha2.ObjectName(name),
	}

	if namespace != "" {
		parentRef.Namespace = NamespacePtrV1Alpha2(namespace)
	}

	return parentRef
}

func GatewayListenerParentRef(namespace, name, listener string) gatewayapi_v1beta1.ParentReference {
	parentRef := GatewayParentRef(namespace, name)

	if listener != "" {
		parentRef.SectionName = SectionNamePtr(listener)
	}

	return parentRef
}

func GroupPtr(group string) *gatewayapi_v1beta1.Group {
	gwGroup := gatewayapi_v1beta1.Group(group)
	return &gwGroup
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func GroupPtrV1Alpha2(group string) *gatewayapi_v1alpha2.Group {
	gwGroup := gatewayapi_v1alpha2.Group(group)
	return &gwGroup
}

func KindPtr(kind string) *gatewayapi_v1beta1.Kind {
	gwKind := gatewayapi_v1beta1.Kind(kind)
	return &gwKind
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func KindPtrV1Alpha2(kind string) *gatewayapi_v1alpha2.Kind {
	gwKind := gatewayapi_v1alpha2.Kind(kind)
	return &gwKind
}

func NamespacePtr(namespace string) *gatewayapi_v1beta1.Namespace {
	gwNamespace := gatewayapi_v1beta1.Namespace(namespace)
	return &gwNamespace
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func NamespacePtrV1Alpha2(namespace string) *gatewayapi_v1alpha2.Namespace {
	gwNamespace := gatewayapi_v1alpha2.Namespace(namespace)
	return &gwNamespace
}

func ObjectNamePtr(name string) *gatewayapi_v1alpha2.ObjectName {
	objectName := gatewayapi_v1alpha2.ObjectName(name)
	return &objectName
}

func ServiceBackendObjectRef(name string, port int) gatewayapi_v1beta1.BackendObjectReference {
	return gatewayapi_v1beta1.BackendObjectReference{
		Group: GroupPtr(""),
		Kind:  KindPtr("Service"),
		Name:  gatewayapi_v1beta1.ObjectName(name),
		Port:  PortNumPtr(port),
	}
}

func GatewayAddressTypePtr(addr gatewayapi_v1beta1.AddressType) *gatewayapi_v1beta1.AddressType {
	return &addr
}

func HTTPRouteMatch(pathType gatewayapi_v1beta1.PathMatchType, value string) []gatewayapi_v1beta1.HTTPRouteMatch {
	return []gatewayapi_v1beta1.HTTPRouteMatch{
		{
			Path: &gatewayapi_v1beta1.HTTPPathMatch{
				Type:  PathMatchTypePtr(pathType),
				Value: pointer.StringPtr(value),
			},
		},
	}
}

func HTTPHeaderMatch(matchType gatewayapi_v1beta1.HeaderMatchType, name, value string) []gatewayapi_v1beta1.HTTPHeaderMatch {
	return []gatewayapi_v1beta1.HTTPHeaderMatch{
		{
			Type:  HeaderMatchTypePtr(gatewayapi_v1beta1.HeaderMatchExact),
			Name:  gatewayapi_v1beta1.HTTPHeaderName(name),
			Value: value,
		},
	}
}

func HTTPQueryParamMatches(namesAndValues map[string]string) []gatewayapi_v1beta1.HTTPQueryParamMatch {
	var matches []gatewayapi_v1beta1.HTTPQueryParamMatch

	for name, val := range namesAndValues {
		matches = append(matches, gatewayapi_v1beta1.HTTPQueryParamMatch{
			Type:  QueryParamMatchTypePtr(gatewayapi_v1beta1.QueryParamMatchExact),
			Name:  name,
			Value: val,
		})
	}

	return matches
}

func HTTPBackendRefs(backendRefs ...[]gatewayapi_v1beta1.HTTPBackendRef) []gatewayapi_v1beta1.HTTPBackendRef {
	var res []gatewayapi_v1beta1.HTTPBackendRef

	for _, ref := range backendRefs {
		res = append(res, ref...)
	}
	return res
}

func HTTPBackendRef(serviceName string, port int, weight int32) []gatewayapi_v1beta1.HTTPBackendRef {
	return []gatewayapi_v1beta1.HTTPBackendRef{
		{
			BackendRef: gatewayapi_v1beta1.BackendRef{
				BackendObjectReference: ServiceBackendObjectRef(serviceName, port),
				Weight:                 &weight,
			},
		},
	}
}

func TLSRouteBackendRefs(backendRefs ...[]gatewayapi_v1alpha2.BackendRef) []gatewayapi_v1alpha2.BackendRef {
	var res []gatewayapi_v1alpha2.BackendRef

	for _, ref := range backendRefs {
		res = append(res, ref...)
	}
	return res
}

func TLSRouteBackendRef(serviceName string, port int, weight *int32) []gatewayapi_v1alpha2.BackendRef {
	return []gatewayapi_v1alpha2.BackendRef{
		{
			BackendObjectReference: gatewayapi_v1alpha2.BackendObjectReference{
				Group: GroupPtrV1Alpha2(""),
				Kind:  KindPtrV1Alpha2("Service"),
				Name:  gatewayapi_v1alpha2.ObjectName(serviceName),
				Port:  PortNumPtrV1Alpha2(port),
			},
			Weight: weight,
		},
	}
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func UpgradeParentRefs(parentRefs []gatewayapi_v1alpha2.ParentReference) []gatewayapi_v1beta1.ParentReference {
	var res []gatewayapi_v1beta1.ParentReference

	for _, parentRef := range parentRefs {
		res = append(res, UpgradeParentRef(parentRef))
	}

	return res
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func UpgradeParentRef(parentRef gatewayapi_v1alpha2.ParentReference) gatewayapi_v1beta1.ParentReference {
	upgraded := gatewayapi_v1beta1.ParentReference{}

	if parentRef.Group != nil {
		upgraded.Group = GroupPtr(string(*parentRef.Group))
	}

	if parentRef.Kind != nil {
		upgraded.Kind = KindPtr(string(*parentRef.Kind))
	}

	if parentRef.Namespace != nil {
		upgraded.Namespace = NamespacePtr(string(*parentRef.Namespace))
	}

	upgraded.Name = gatewayapi_v1beta1.ObjectName(parentRef.Name)

	if parentRef.SectionName != nil {
		upgraded.SectionName = SectionNamePtr(string(*parentRef.SectionName))
	}

	if parentRef.Port != nil {
		upgraded.Port = PortNumPtr(int(*parentRef.Port))
	}

	return upgraded
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func UpgradeRouteParentStatuses(routeParentStatuses []gatewayapi_v1alpha2.RouteParentStatus) []gatewayapi_v1beta1.RouteParentStatus {
	var res []gatewayapi_v1beta1.RouteParentStatus

	for _, rps := range routeParentStatuses {
		res = append(res, UpgradeRouteParentStatus(rps))
	}

	return res
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func UpgradeRouteParentStatus(routeParentStatus gatewayapi_v1alpha2.RouteParentStatus) gatewayapi_v1beta1.RouteParentStatus {
	return gatewayapi_v1beta1.RouteParentStatus{
		ParentRef:      UpgradeParentRef(routeParentStatus.ParentRef),
		ControllerName: gatewayapi_v1beta1.GatewayController(routeParentStatus.ControllerName),
		Conditions:     routeParentStatus.Conditions,
	}
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func DowngradeRouteParentStatuses(routeParentStatuses []gatewayapi_v1beta1.RouteParentStatus) []gatewayapi_v1alpha2.RouteParentStatus {
	var res []gatewayapi_v1alpha2.RouteParentStatus

	for _, rps := range routeParentStatuses {
		res = append(res, gatewayapi_v1alpha2.RouteParentStatus{
			ParentRef:      downgradeParentRef(rps.ParentRef),
			ControllerName: gatewayapi_v1alpha2.GatewayController(rps.ControllerName),
			Conditions:     rps.Conditions,
		})
	}

	return res
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func downgradeParentRef(parentRef gatewayapi_v1beta1.ParentReference) gatewayapi_v1alpha2.ParentReference {
	downgraded := gatewayapi_v1alpha2.ParentReference{}

	if parentRef.Group != nil {
		downgraded.Group = GroupPtrV1Alpha2(string(*parentRef.Group))
	}

	if parentRef.Kind != nil {
		downgraded.Kind = KindPtrV1Alpha2(string(*parentRef.Kind))
	}

	if parentRef.Namespace != nil {
		downgraded.Namespace = NamespacePtrV1Alpha2(string(*parentRef.Namespace))
	}

	downgraded.Name = gatewayapi_v1alpha2.ObjectName(parentRef.Name)

	if parentRef.SectionName != nil {
		downgraded.SectionName = SectionNamePtrV1Alpha2(string(*parentRef.SectionName))
	}

	if parentRef.Port != nil {
		downgraded.Port = PortNumPtrV1Alpha2(int(*parentRef.Port))
	}

	return downgraded
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func UpgradeHostnames(hostnames []gatewayapi_v1alpha2.Hostname) []gatewayapi_v1beta1.Hostname {
	var res []gatewayapi_v1beta1.Hostname

	for _, hostname := range hostnames {
		res = append(res, gatewayapi_v1beta1.Hostname(hostname))
	}

	return res
}

// TODO(sk): delete when Gateway API v1alpha2 support is dropped
func UpgradeBackendRef(backendRef gatewayapi_v1alpha2.BackendRef) gatewayapi_v1beta1.BackendRef {
	upgraded := gatewayapi_v1beta1.BackendRef{}

	if backendRef.Group != nil {
		upgraded.Group = GroupPtr(string(*backendRef.Group))
	}

	if backendRef.Kind != nil {
		upgraded.Kind = KindPtr(string(*backendRef.Kind))
	}

	upgraded.Name = gatewayapi_v1beta1.ObjectName(backendRef.Name)

	if backendRef.Namespace != nil {
		upgraded.Namespace = NamespacePtr(string(*backendRef.Namespace))
	}

	if backendRef.Port != nil {
		upgraded.Port = PortNumPtr(int(*backendRef.Port))
	}

	upgraded.Weight = backendRef.Weight

	return upgraded
}

// IsRefToGateway returns whether the provided parent ref is a reference
// to a Gateway with the given namespace/name, irrespective of whether a
// section/listener name has been specified (i.e. a parent ref to a listener
// on the specified gateway will return "true").
func IsRefToGateway(parentRef gatewayapi_v1beta1.ParentReference, gateway types.NamespacedName) bool {
	if parentRef.Group != nil && string(*parentRef.Group) != gatewayapi_v1beta1.GroupName {
		return false
	}

	if parentRef.Kind != nil && string(*parentRef.Kind) != "Gateway" {
		return false
	}

	if parentRef.Namespace != nil && string(*parentRef.Namespace) != gateway.Namespace {
		return false
	}

	return string(parentRef.Name) == gateway.Name
}
