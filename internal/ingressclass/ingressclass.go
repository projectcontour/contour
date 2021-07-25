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

package ingressclass

import (
	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/utils/pointer"
)

// DefaultClassName is the default IngressClass name that Contour will match
// Ingress/HTTPProxy resources against if no specific IngressClass name is
// configured.
const DefaultClassName = "contour"

// MatchesIngress returns true if the passed in Ingress annotations
// or Spec.IngressClassName match the passed in ingress class name.
// Annotations take precedence over spec field if both are set.
func MatchesIngress(obj *networking_v1.Ingress, ingressClassName string) bool {
	if annotationClass := annotation.IngressClass(obj); annotationClass != "" {
		return matches(annotationClass, ingressClassName)
	}

	return matches(pointer.StringPtrDerefOr(obj.Spec.IngressClassName, ""), ingressClassName)
}

// MatchesHTTPProxy returns true if the passed in HTTPProxy annotations
// or Spec.IngressClassName match the passed in ingress class name.
// Annotations take precedence over spec field if both are set.
func MatchesHTTPProxy(obj *contour_v1.HTTPProxy, ingressClassName string) bool {
	if annotationClass := annotation.IngressClass(obj); annotationClass != "" {
		return matches(annotationClass, ingressClassName)
	}

	return matches(obj.Spec.IngressClassName, ingressClassName)
}

func matches(objIngressClass, contourIngressClass string) bool {
	// If Contour's configured ingress class is empty, the object can either
	// not have an ingress class, or can have a "contour" ingress class.
	if contourIngressClass == "" {
		return objIngressClass == "" || objIngressClass == DefaultClassName
	}

	// Otherwise, the object's ingress class must match Contour's.
	return objIngressClass == contourIngressClass
}
