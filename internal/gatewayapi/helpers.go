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
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
)

func CertificateRef(name, namespace string) gatewayapi_v1.SecretObjectReference {
	secretRef := gatewayapi_v1.SecretObjectReference{
		Group: ptr.To(gatewayapi_v1.Group("")),
		Kind:  ptr.To(gatewayapi_v1.Kind("Secret")),
		Name:  gatewayapi_v1.ObjectName(name),
	}

	if namespace != "" {
		secretRef.Namespace = ptr.To(gatewayapi_v1.Namespace(namespace))
	}

	return secretRef
}

func GatewayParentRef(namespace, name string) gatewayapi_v1.ParentReference {
	parentRef := gatewayapi_v1.ParentReference{
		Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
		Kind:  ptr.To(gatewayapi_v1.Kind("Gateway")),
		Name:  gatewayapi_v1.ObjectName(name),
	}

	if namespace != "" {
		parentRef.Namespace = ptr.To(gatewayapi_v1.Namespace(namespace))
	}

	return parentRef
}

func GatewayListenerParentRef(namespace, name, listener string, port uint16) gatewayapi_v1.ParentReference {
	parentRef := GatewayParentRef(namespace, name)

	if listener != "" {
		parentRef.SectionName = ptr.To(gatewayapi_v1.SectionName(listener))
	}

	if port != 0 {
		parentRef.Port = ptr.To(gatewayapi_v1.PortNumber(port))
	}

	return parentRef
}

func ServiceBackendObjectRef(name string, port uint16) gatewayapi_v1.BackendObjectReference {
	return gatewayapi_v1.BackendObjectReference{
		Group: ptr.To(gatewayapi_v1.Group("")),
		Kind:  ptr.To(gatewayapi_v1.Kind("Service")),
		Name:  gatewayapi_v1.ObjectName(name),
		Port:  ptr.To(gatewayapi_v1.PortNumber(port)),
	}
}

func HTTPRouteMatch(pathType gatewayapi_v1.PathMatchType, value string) []gatewayapi_v1.HTTPRouteMatch {
	return []gatewayapi_v1.HTTPRouteMatch{
		{
			Path: &gatewayapi_v1.HTTPPathMatch{
				Type:  ptr.To(pathType),
				Value: ptr.To(value),
			},
		},
	}
}

func HTTPHeaderMatch(matchType gatewayapi_v1.HeaderMatchType, name, value string) []gatewayapi_v1.HTTPHeaderMatch {
	return []gatewayapi_v1.HTTPHeaderMatch{
		{
			Type:  ptr.To(matchType),
			Name:  gatewayapi_v1.HTTPHeaderName(name),
			Value: value,
		},
	}
}

func HTTPQueryParamMatches(namesAndValues map[string]string) []gatewayapi_v1.HTTPQueryParamMatch {
	var matches []gatewayapi_v1.HTTPQueryParamMatch

	for name, val := range namesAndValues {
		matches = append(matches, gatewayapi_v1.HTTPQueryParamMatch{
			Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
			Name:  gatewayapi_v1.HTTPHeaderName(name),
			Value: val,
		})
	}

	return matches
}

func HTTPBackendRefs(backendRefs ...[]gatewayapi_v1.HTTPBackendRef) []gatewayapi_v1.HTTPBackendRef {
	var res []gatewayapi_v1.HTTPBackendRef

	for _, ref := range backendRefs {
		res = append(res, ref...)
	}
	return res
}

func HTTPBackendRef(serviceName string, port uint16, weight int32) []gatewayapi_v1.HTTPBackendRef {
	return []gatewayapi_v1.HTTPBackendRef{
		{
			BackendRef: gatewayapi_v1.BackendRef{
				BackendObjectReference: ServiceBackendObjectRef(serviceName, port),
				Weight:                 &weight,
			},
		},
	}
}

func TLSRouteBackendRefs(backendRefs ...[]gatewayapi_v1.BackendRef) []gatewayapi_v1.BackendRef {
	var res []gatewayapi_v1.BackendRef

	for _, ref := range backendRefs {
		res = append(res, ref...)
	}
	return res
}

func TLSRouteBackendRef(serviceName string, port uint16, weight *int32) []gatewayapi_v1.BackendRef {
	return []gatewayapi_v1.BackendRef{
		{
			BackendObjectReference: gatewayapi_v1.BackendObjectReference{
				Group: ptr.To(gatewayapi_v1.Group("")),
				Kind:  ptr.To(gatewayapi_v1.Kind("Service")),
				Name:  gatewayapi_v1.ObjectName(serviceName),
				Port:  ptr.To(gatewayapi_v1.PortNumber(port)),
			},
			Weight: weight,
		},
	}
}

func GRPCRouteBackendRef(serviceName string, port uint16, weight int32) []gatewayapi_v1.GRPCBackendRef {
	return []gatewayapi_v1.GRPCBackendRef{
		{
			BackendRef: gatewayapi_v1.BackendRef{
				BackendObjectReference: gatewayapi_v1.BackendObjectReference{
					Group: ptr.To(gatewayapi_v1.Group("")),
					Kind:  ptr.To(gatewayapi_v1.Kind("Service")),
					Name:  gatewayapi_v1.ObjectName(serviceName),
					Port:  ptr.To(gatewayapi_v1.PortNumber(port)),
				},
				Weight: &weight,
			},
			Filters: []gatewayapi_v1.GRPCRouteFilter{},
		},
	}
}

func GRPCMethodMatch(matchType gatewayapi_v1.GRPCMethodMatchType, service, method string) *gatewayapi_v1.GRPCMethodMatch {
	return &gatewayapi_v1.GRPCMethodMatch{
		Type:    ptr.To(matchType),
		Service: ptr.To(service),
		Method:  ptr.To(method),
	}
}

func GRPCHeaderMatch(matchType gatewayapi_v1.GRPCHeaderMatchType, name, value string) []gatewayapi_v1.GRPCHeaderMatch {
	return []gatewayapi_v1.GRPCHeaderMatch{
		{
			Type:  ptr.To(matchType),
			Name:  gatewayapi_v1.GRPCHeaderName(name),
			Value: value,
		},
	}
}

// IsRefToGateway returns whether the provided parent ref is a reference
// to a Gateway with the given namespace/name, irrespective of whether a
// section/listener name has been specified (i.e. a parent ref to a listener
// on the specified gateway will return "true").
func IsRefToGateway(parentRef gatewayapi_v1.ParentReference, gateway types.NamespacedName) bool {
	if parentRef.Group != nil && string(*parentRef.Group) != gatewayapi_v1.GroupName {
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
