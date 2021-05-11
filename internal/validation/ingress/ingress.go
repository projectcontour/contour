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

package ingress

import (
	"github.com/projectcontour/contour/internal/annotation"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/utils/pointer"
)

// DefaultClassName is the default IngressClass name that Contour will match
// Ingress resources against if no specific IngressClass name is configured.
const DefaultClassName = "contour"

// MatchesIngressClassName returns true if the passed in Ingress annotations
// or spec ingress class name matche the passed in ingress class name.
// Annotations take precedence over spec field if both are set.
func MatchesIngressClassName(obj *networking_v1.Ingress, ingressClassName string) bool {
	annotationClass := annotation.IngressClass(obj)
	specClass := pointer.StringPtrDerefOr(obj.Spec.IngressClassName, "")

	// If annotation is set, check if it matches.
	if annotationClass != "" {
		return annotation.MatchesIngressClass(obj, ingressClassName)
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
