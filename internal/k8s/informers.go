// Copyright © 2020 VMware
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

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	"k8s.io/api/networking/v1beta1"

	serviceapis "sigs.k8s.io/service-apis/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

type gvrmap map[schema.GroupVersionResource]cache.SharedIndexInformer

// InformerSet stores a table of Kubernetes GVR objects to their associated informers.
type InformerSet struct {
	Informers gvrmap
}

// DefaultInformerSet creates a new InformerSet lookup table and populates with the default
// GVRs that Contour will try to watch.
func DefaultInformerSet(inffactory dynamicinformer.DynamicSharedInformerFactory, serviceAPIs bool) InformerSet {

	defaultGVRs := []schema.GroupVersionResource{
		ingressroutev1.IngressRouteGVR,
		ingressroutev1.TLSCertificateDelegationGVR,
		projectcontour.HTTPProxyGVR,
		projectcontour.TLSCertificateDelegationGVR,
		corev1.SchemeGroupVersion.WithResource("services"),
		v1beta1.SchemeGroupVersion.WithResource("ingresses"),
	}

	// TODO(youngnick): Remove this boolean once we have autodetection of available types (Further work on #2219).
	if serviceAPIs {
		defaultGVRs = append(defaultGVRs, serviceapis.GroupVersion.WithResource("gatewayclasses"))
		defaultGVRs = append(defaultGVRs, serviceapis.GroupVersion.WithResource("gateways"))
		defaultGVRs = append(defaultGVRs, serviceapis.GroupVersion.WithResource("httproutes"))
		defaultGVRs = append(defaultGVRs, serviceapis.GroupVersion.WithResource("tcproutes"))
	}

	gvri := InformerSet{
		Informers: make(gvrmap),
	}

	for _, gvr := range defaultGVRs {
		gvri.Informers[gvr] = inffactory.ForResource(gvr).Informer()
	}
	return gvri
}
