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
	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	serviceapis "sigs.k8s.io/service-apis/api/v1alpha1"
)

// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses/status,verbs=create;get;update

// +kubebuilder:rbac:groups="projectcontour.io",resources=httpproxies;tlscertificatedelegations,verbs=get;list;watch
// +kubebuilder:rbac:groups="projectcontour.io",resources=httpproxies/status,verbs=create;get;update

// DefaultResources ...
func DefaultResources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{
		projectcontour.HTTPProxyGVR,
		projectcontour.TLSCertificateDelegationGVR,
		corev1.SchemeGroupVersion.WithResource("services"),
		v1beta1.SchemeGroupVersion.WithResource("ingresses"),
	}
}

// +kubebuilder:rbac:groups="networking.k8s.io",resources=gatewayclasses;gateways;httproutes;tcproutes,verbs=get;list;watch

// ServiceAPIResources ...
func ServiceAPIResources() []schema.GroupVersionResource {
	return []schema.GroupVersionResource{
		serviceapis.GroupVersion.WithResource("gatewayclasses"),
		serviceapis.GroupVersion.WithResource("gateways"),
		serviceapis.GroupVersion.WithResource("httproutes"),
		serviceapis.GroupVersion.WithResource("tcproutes"),
	}
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
