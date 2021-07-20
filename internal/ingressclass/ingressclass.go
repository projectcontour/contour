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
	"github.com/projectcontour/contour/internal/annotation"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	annotationClass := annotation.IngressClass(obj)
	specClass := pointer.StringPtrDerefOr(obj.Spec.IngressClassName, "")

	// If annotation is set, check if it matches.
	if annotationClass != "" {
		return MatchesAnnotation(obj, ingressClassName)
	}

	// If spec field is set, check if it matches.
	if specClass != "" {
		classToMatch := ingressClassName
		if classToMatch == "" {
			classToMatch = DefaultClassName
		}
		return specClass == classToMatch
	}

	// Matches if class is not set.
	return ingressClassName == ""
}

// MatchesAnnotation checks that the passed object has an ingress class annotation
// that matches either the passed ingress-class string, or DefaultClassName if it's
// empty.
func MatchesAnnotation(o meta_v1.Object, ic string) bool {
	ingressClassAnnotation := annotation.IngressClass(o)

	// If Contour's configured ingress class is empty, the object can either
	// not have an ingress class, or can have a "contour" ingress class.
	if ic == "" {
		return ingressClassAnnotation == "" || ingressClassAnnotation == DefaultClassName
	}

	// Otherwise, the object's ingress class must match Contour's.
	return ingressClassAnnotation == ic
}
