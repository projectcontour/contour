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
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
)

// IsStatusEqual checks that two objects of supported Kubernetes types
// have equivalent Status structs.
// Currently supports:
// networking.k8s.io/ingress/v1beta1
func IsStatusEqual(objA, objB interface{}) bool {

	switch a := objA.(type) {
	case *v1beta1.Ingress:
		switch b := objB.(type) {
		case *v1beta1.Ingress:
			return equality.Semantic.DeepEqual(a.Status, b.Status)
		}
	case *contour_api_v1.HTTPProxy:
		switch b := objB.(type) {
		case *contour_api_v1.HTTPProxy:
			return equality.Semantic.DeepEqual(a.Status, b.Status)
		}
	}

	return false
}
