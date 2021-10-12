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
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	networking_v1 "k8s.io/api/networking/v1"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// isStatusEqual checks that two objects of supported Kubernetes types
// have equivalent Status structs.
//
// Currently supports:
// networking.k8s.io/ingress/v1
// projectcontour.io/v1 (HTTPProxy only)
// networking.x-k8s.io/v1alpha1 (GatewayClass and Gateway only)
func isStatusEqual(objA, objB interface{}) bool {
	switch a := objA.(type) {
	case *networking_v1.Ingress:
		if b, ok := objB.(*networking_v1.Ingress); ok {
			if cmp.Equal(a.Status, b.Status) {
				return true
			}
		}
	case *contour_api_v1.HTTPProxy:
		if b, ok := objB.(*contour_api_v1.HTTPProxy); ok {
			// Compare the status of the object ignoring the LastTransitionTime which is always
			// updated on each DAG rebuild regardless if the status of object changed or not.
			// Not ignoring this causes each status to be updated each time since the objects
			// are always different for each DAG rebuild (Issue #2979).
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(contour_api_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	case *gatewayapi_v1alpha1.GatewayClass:
		if b, ok := objB.(*gatewayapi_v1alpha1.GatewayClass); ok {
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(contour_api_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	case *gatewayapi_v1alpha1.Gateway:
		if b, ok := objB.(*gatewayapi_v1alpha1.Gateway); ok {
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(contour_api_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	}
	return false
}
