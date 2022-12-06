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
	"github.com/projectcontour/contour/internal/ref"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

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

func GatewayListenerParentRef(namespace, name, listener string) gatewayapi_v1beta1.ParentReference {
	parentRef := GatewayParentRef(namespace, name)

	if listener != "" {
		parentRef.SectionName = ref.To(gatewayapi_v1beta1.SectionName(listener))
	}

	return parentRef
}

func GroupPtr(group string) *gatewayapi_v1beta1.Group {
	gwGroup := gatewayapi_v1beta1.Group(group)
	return &gwGroup
}

func KindPtr(kind string) *gatewayapi_v1beta1.Kind {
	gwKind := gatewayapi_v1beta1.Kind(kind)
	return &gwKind
}

func NamespacePtr(namespace string) *gatewayapi_v1beta1.Namespace {
	gwNamespace := gatewayapi_v1beta1.Namespace(namespace)
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
		Port:  ref.To(gatewayapi_v1beta1.PortNumber(port)),
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
				Group: GroupPtr(""),
				Kind:  KindPtr("Service"),
				Name:  gatewayapi_v1alpha2.ObjectName(serviceName),
				Port:  ref.To(gatewayapi_v1beta1.PortNumber(port)),
			},
			Weight: weight,
		},
	}
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
