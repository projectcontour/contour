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

package k8s

import (
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// KindOf returns the kind string for the given Kubernetes object.
//
// The API machinery doesn't populate the meta_v1.TypeMeta field for
// objects, so we have to use a type assertion to detect kinds that
// we care about.
func KindOf(obj any) string {
	object, ok := obj.(runtime.Object)
	if !ok {
		return ""
	}
	gvk, _, err := scheme.Scheme.ObjectKinds(object)
	if err != nil {
		switch obj := obj.(type) {
		case *core_v1.Secret:
			return "Secret"
		case *core_v1.Service:
			return "Service"
		case *core_v1.Endpoints:
			return "Endpoints"
		case *networking_v1.Ingress:
			return "Ingress"
		case *contour_v1.HTTPProxy:
			return "HTTPProxy"
		case *gatewayapi_v1.HTTPRoute:
			return "HTTPRoute"
		case *gatewayapi_v1.GRPCRoute:
			return "GRPCRoute"
		case *gatewayapi_v1alpha2.TLSRoute:
			return "TLSRoute"
		case *gatewayapi_v1alpha2.TCPRoute:
			return "TCPRoute"
		case *gatewayapi_v1.Gateway:
			return "Gateway"
		case *gatewayapi_v1.GatewayClass:
			return "GatewayClass"
		case *gatewayapi_v1beta1.ReferenceGrant:
			return "ReferenceGrant"
		case *gatewayapi_v1alpha3.BackendTLSPolicy:
			return "BackendTLSPolicy"
		case *contour_v1.TLSCertificateDelegation:
			return "TLSCertificateDelegation"
		case *contour_v1alpha1.ExtensionService:
			return "ExtensionService"
		case *contour_v1alpha1.ContourConfiguration:
			return "ContourConfiguration"
		case *contour_v1alpha1.ContourDeployment:
			return "ContourDeployment"
		case *core_v1.Namespace:
			return "Namespace"
		case *unstructured.Unstructured:
			return obj.GetKind()
		default:
			return ""
		}
	}
	for _, gv := range gvk {
		return gv.GroupKind().Kind
	}
	return ""
}

// VersionOf returns the GroupVersion string for the given Kubernetes object.
func VersionOf(obj any) string {
	// If err is not nil we have the GVK and we can use it. Otherwise we're going to use switch case method as failover
	gvk, _, err := scheme.Scheme.ObjectKinds(obj.(runtime.Object))
	if err != nil {
		switch obj := obj.(type) {
		case *core_v1.Secret, *core_v1.Service, *core_v1.Endpoints:
			return core_v1.SchemeGroupVersion.String()
		case *networking_v1.Ingress:
			return networking_v1.SchemeGroupVersion.String()
		case *contour_v1.HTTPProxy, *contour_v1.TLSCertificateDelegation:
			return contour_v1.GroupVersion.String()
		case *contour_v1alpha1.ExtensionService:
			return contour_v1alpha1.GroupVersion.String()
		case *unstructured.Unstructured:
			return obj.GetAPIVersion()
		default:
			return ""
		}
	}
	for _, gv := range gvk {
		return gv.GroupVersion().String()
	}
	return ""
}
