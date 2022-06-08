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
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func SectionNamePtr(sectionName string) *gatewayapi_v1alpha2.SectionName {
	gwSectionName := gatewayapi_v1alpha2.SectionName(sectionName)
	return &gwSectionName
}

func PortNumPtr(port int) *gatewayapi_v1alpha2.PortNumber {
	pn := gatewayapi_v1alpha2.PortNumber(port)
	return &pn
}

func FromNamespacesPtr(val gatewayapi_v1alpha2.FromNamespaces) *gatewayapi_v1alpha2.FromNamespaces {
	return &val
}

func PathMatchTypePtr(val gatewayapi_v1alpha2.PathMatchType) *gatewayapi_v1alpha2.PathMatchType {
	return &val
}

func HeaderMatchTypePtr(val gatewayapi_v1alpha2.HeaderMatchType) *gatewayapi_v1alpha2.HeaderMatchType {
	return &val
}

func TLSModeTypePtr(mode gatewayapi_v1alpha2.TLSModeType) *gatewayapi_v1alpha2.TLSModeType {
	return &mode
}

func HTTPMethodPtr(method gatewayapi_v1alpha2.HTTPMethod) *gatewayapi_v1alpha2.HTTPMethod {
	return &method
}

func AddressTypePtr(addressType gatewayapi_v1alpha2.AddressType) *gatewayapi_v1alpha2.AddressType {
	return &addressType
}

func ListenerHostname(host string) *gatewayapi_v1alpha2.Hostname {
	h := gatewayapi_v1alpha2.Hostname(host)
	return &h
}

func PreciseHostname(host string) *gatewayapi_v1alpha2.PreciseHostname {
	h := gatewayapi_v1alpha2.PreciseHostname(host)
	return &h
}

func CertificateRef(name, namespace string) *gatewayapi_v1alpha2.SecretObjectReference {
	ref := &gatewayapi_v1alpha2.SecretObjectReference{
		Group: GroupPtr(""),
		Kind:  KindPtr("Secret"),
		Name:  gatewayapi_v1alpha2.ObjectName(name),
	}

	if namespace != "" {
		ref.Namespace = NamespacePtr(namespace)
	}

	return ref
}

func GatewayParentRef(namespace, name string) gatewayapi_v1alpha2.ParentReference {
	parentRef := gatewayapi_v1alpha2.ParentReference{
		Group: GroupPtr(gatewayapi_v1alpha2.GroupName),
		Kind:  KindPtr("Gateway"),
		Name:  gatewayapi_v1alpha2.ObjectName(name),
	}

	if namespace != "" {
		parentRef.Namespace = NamespacePtr(namespace)
	}

	return parentRef
}

func GatewayListenerParentRef(namespace, name, listener string) gatewayapi_v1alpha2.ParentReference {
	parentRef := GatewayParentRef(namespace, name)

	if listener != "" {
		parentRef.SectionName = SectionNamePtr(listener)
	}

	return parentRef
}

func GroupPtr(group string) *gatewayapi_v1alpha2.Group {
	gwGroup := gatewayapi_v1alpha2.Group(group)
	return &gwGroup
}

func KindPtr(kind string) *gatewayapi_v1alpha2.Kind {
	gwKind := gatewayapi_v1alpha2.Kind(kind)
	return &gwKind
}

func NamespacePtr(namespace string) *gatewayapi_v1alpha2.Namespace {
	gwNamespace := gatewayapi_v1alpha2.Namespace(namespace)
	return &gwNamespace
}

func ObjectNamePtr(name string) *gatewayapi_v1alpha2.ObjectName {
	objectName := gatewayapi_v1alpha2.ObjectName(name)
	return &objectName
}

func ServiceBackendObjectRef(name string, port int) gatewayapi_v1alpha2.BackendObjectReference {
	return gatewayapi_v1alpha2.BackendObjectReference{
		Group: GroupPtr(""),
		Kind:  KindPtr("Service"),
		Name:  gatewayapi_v1alpha2.ObjectName(name),
		Port:  PortNumPtr(port),
	}
}

func GatewayAddressTypePtr(addr gatewayapi_v1alpha2.AddressType) *gatewayapi_v1alpha2.AddressType {
	return &addr
}

func HTTPRouteMatch(pathType gatewayapi_v1alpha2.PathMatchType, value string) []gatewayapi_v1alpha2.HTTPRouteMatch {
	return []gatewayapi_v1alpha2.HTTPRouteMatch{
		{
			Path: &gatewayapi_v1alpha2.HTTPPathMatch{
				Type:  PathMatchTypePtr(pathType),
				Value: pointer.StringPtr(value),
			},
		},
	}
}

func HTTPHeaderMatch(matchType gatewayapi_v1alpha2.HeaderMatchType, name, value string) []gatewayapi_v1alpha2.HTTPHeaderMatch {
	return []gatewayapi_v1alpha2.HTTPHeaderMatch{
		{
			Type:  HeaderMatchTypePtr(gatewayapi_v1alpha2.HeaderMatchExact),
			Name:  gatewayapi_v1alpha2.HTTPHeaderName(name),
			Value: value,
		},
	}
}

func HTTPBackendRefs(backendRefs ...[]gatewayapi_v1alpha2.HTTPBackendRef) []gatewayapi_v1alpha2.HTTPBackendRef {
	var res []gatewayapi_v1alpha2.HTTPBackendRef

	for _, ref := range backendRefs {
		res = append(res, ref...)
	}
	return res
}

func HTTPBackendRef(serviceName string, port int, weight int32) []gatewayapi_v1alpha2.HTTPBackendRef {
	return []gatewayapi_v1alpha2.HTTPBackendRef{
		{
			BackendRef: gatewayapi_v1alpha2.BackendRef{
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
			BackendObjectReference: ServiceBackendObjectRef(serviceName, port),
			Weight:                 weight,
		},
	}
}
