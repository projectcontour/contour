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
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses/status,verbs=create;get;update

// +kubebuilder:rbac:groups="projectcontour.io",resources=httpproxies;tlscertificatedelegations,verbs=get;list;watch
// +kubebuilder:rbac:groups="projectcontour.io",resources=httpproxies/status,verbs=create;get;update
// +kubebuilder:rbac:groups="projectcontour.io",resources=extensionservices,verbs=get;list;watch
// +kubebuilder:rbac:groups="projectcontour.io",resources=extensionservices/status,verbs=create;get;update

// DefaultResources ...
func DefaultResources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{
		contour_api_v1.HTTPProxyGVR,
		contour_api_v1.TLSCertificateDelegationGVR,
		contour_api_v1alpha1.ExtensionServiceGVR,
		corev1.SchemeGroupVersion.WithResource("services"),
	}
}

func IngressV1Resources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{
		networking_v1.SchemeGroupVersion.WithResource("ingresses"),
		networking_v1.SchemeGroupVersion.WithResource("ingressclasses"),
	}
}

// +kubebuilder:rbac:groups="networking.x-k8s.io",resources=gatewayclasses;gateways;httproutes;backendpolicies;tlsroutes;tcproutes;udproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups="networking.x-k8s.io",resources=gatewayclasses/status;gateways/status;httproutes/status;backendpolicies/status;tlsroutes/status;tcproutes/status;udproutes/status,verbs=update

// GatewayAPIResources returns a list of Gateway API group/version resources.
func GatewayAPIResources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{{
		Group:    gatewayapi_v1alpha1.GroupVersion.Group,
		Version:  gatewayapi_v1alpha1.GroupVersion.Version,
		Resource: "gatewayclasses",
	}, {
		Group:    gatewayapi_v1alpha1.GroupVersion.Group,
		Version:  gatewayapi_v1alpha1.GroupVersion.Version,
		Resource: "gateways",
	}, {
		Group:    gatewayapi_v1alpha1.GroupVersion.Group,
		Version:  gatewayapi_v1alpha1.GroupVersion.Version,
		Resource: "httproutes",
	}, {
		Group:    gatewayapi_v1alpha1.GroupVersion.Group,
		Version:  gatewayapi_v1alpha1.GroupVersion.Version,
		Resource: "backendpolicies",
	}, {
		Group:    gatewayapi_v1alpha1.GroupVersion.Group,
		Version:  gatewayapi_v1alpha1.GroupVersion.Version,
		Resource: "tlsroutes",
	}, {
		Group:    gatewayapi_v1alpha1.GroupVersion.Group,
		Version:  gatewayapi_v1alpha1.GroupVersion.Version,
		Resource: "tcproutes",
	}, {
		Group:    gatewayapi_v1alpha1.GroupVersion.Group,
		Version:  gatewayapi_v1alpha1.GroupVersion.Version,
		Resource: "udproutes",
	}}
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// SecretsResources ...
func SecretsResources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{
		corev1.SchemeGroupVersion.WithResource("secrets"),
	}
}

// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch

// EndpointsResources ...
func EndpointsResources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{
		corev1.SchemeGroupVersion.WithResource("endpoints"),
	}
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

// ServicesResources ...
func ServicesResources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{
		corev1.SchemeGroupVersion.WithResource("services"),
	}
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// NamespacesResource ...
func NamespacesResource() schema.GroupVersionResource {
	return corev1.SchemeGroupVersion.WithResource("namespaces")
}
