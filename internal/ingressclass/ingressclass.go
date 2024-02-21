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
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
)

// DefaultClassName is the default IngressClass name that Contour will match
// Ingress/HTTPProxy resources against if no specific IngressClass name is
// configured.
const DefaultClassName = "contour"

// MatchesIngress returns true if the passed in Ingress annotations
// or Spec.IngressClassName match the passed in ingress class name.
// Annotations take precedence over spec field if both are set.
func MatchesIngress(obj *networking_v1.Ingress, ingressClassNames []string) bool {
	if annotationClass := annotation.IngressClass(obj); annotationClass != "" {
		return matches(annotationClass, ingressClassNames)
	}

	return matches(ptr.Deref(obj.Spec.IngressClassName, ""), ingressClassNames)
}

// MatchesHTTPProxy returns true if the passed in HTTPProxy annotations
// or Spec.IngressClassName match the passed in ingress class name.
// Annotations take precedence over spec field if both are set.
func MatchesHTTPProxy(obj *contour_v1.HTTPProxy, ingressClassNames []string) bool {
	if annotationClass := annotation.IngressClass(obj); annotationClass != "" {
		return matches(annotationClass, ingressClassNames)
	}

	return matches(obj.Spec.IngressClassName, ingressClassNames)
}

func matches(objIngressClass string, contourIngressClasses []string) bool {
	// If Contour has no configured ingress class, the object can either
	// not have an ingress class, or can have a "contour" ingress class.
	if len(contourIngressClasses) == 0 {
		return objIngressClass == "" || objIngressClass == DefaultClassName
	}

	// Otherwise, the object's ingress class must match one of Contour's.
	for _, contourIngressClass := range contourIngressClasses {
		if objIngressClass == contourIngressClass {
			return true
		}
	}
	return false
}
