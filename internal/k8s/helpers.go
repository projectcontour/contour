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
	"fmt"
	"reflect"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// isStatusEqual checks that two objects of supported Kubernetes types
// have equivalent Status structs.
func isStatusEqual(objA, objB any) bool {
	switch a := objA.(type) {
	case *networking_v1.Ingress:
		if b, ok := objB.(*networking_v1.Ingress); ok {
			if cmp.Equal(a.Status, b.Status) {
				return true
			}
		}
	case *contour_v1.HTTPProxy:
		if b, ok := objB.(*contour_v1.HTTPProxy); ok {
			// Compare the status of the object ignoring the LastTransitionTime which is always
			// updated on each DAG rebuild regardless if the status of object changed or not.
			// Not ignoring this causes each status to be updated each time since the objects
			// are always different for each DAG rebuild (Issue #2979).
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(contour_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	case *contour_v1alpha1.ExtensionService:
		if b, ok := objB.(*contour_v1alpha1.ExtensionService); ok {
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(contour_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	case *gatewayapi_v1.GatewayClass:
		if b, ok := objB.(*gatewayapi_v1.GatewayClass); ok {
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	case *gatewayapi_v1.Gateway:
		if b, ok := objB.(*gatewayapi_v1.Gateway); ok {
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	case *gatewayapi_v1.HTTPRoute:
		if b, ok := objB.(*gatewayapi_v1.HTTPRoute); ok {
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	case *gatewayapi_v1alpha2.TLSRoute:
		if b, ok := objB.(*gatewayapi_v1alpha2.TLSRoute); ok {
			if cmp.Equal(a.Status, b.Status,
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime")) {
				return true
			}
		}
	}
	return false
}

// IsObjectEqual checks if objects received during update are equal.
//
// Make an attempt to avoid comparing full objects since it can be very CPU intensive.
// Prefer comparing Generation when only interested in spec changes.
func IsObjectEqual(oldObj, newObj client.Object) (bool, error) {
	// Fast path for any object: when ResourceVersions are equal, the objects are equal.
	// NOTE: This optimizes the case when controller-runtime executes full sync and sends updates for all objects.
	if isResourceVersionEqual(oldObj, newObj) {
		return true, nil
	}

	switch oldObj := oldObj.(type) {

	// Fast path for objects that implement Generation and where only spec changes matter.
	// Status/annotations/labels changes are ignored.
	// Generation is implemented in CRDs, Ingress and IngressClass.
	case *contour_v1alpha1.ExtensionService,
		*contour_v1.TLSCertificateDelegation:
		return isGenerationEqual(oldObj, newObj), nil

	case *gatewayapi_v1.GatewayClass,
		*gatewayapi_v1.Gateway,
		*gatewayapi_v1beta1.ReferenceGrant,
		*gatewayapi_v1.HTTPRoute,
		*gatewayapi_v1alpha2.TLSRoute,
		*gatewayapi_v1alpha2.GRPCRoute,
		*gatewayapi_v1alpha2.TCPRoute,
		*gatewayapi_v1alpha2.BackendTLSPolicy:
		return isGenerationEqual(oldObj, newObj), nil

	// Slow path: compare the content of the objects.
	case *contour_v1.HTTPProxy,
		*networking_v1.Ingress:
		return isGenerationEqual(oldObj, newObj) &&
			apiequality.Semantic.DeepEqual(oldObj.GetAnnotations(), newObj.GetAnnotations()), nil
	case *core_v1.Secret:
		if newObj, ok := newObj.(*core_v1.Secret); ok {
			return reflect.DeepEqual(oldObj.Data, newObj.Data), nil
		}
	case *core_v1.ConfigMap:
		if newObj, ok := newObj.(*core_v1.ConfigMap); ok {
			return reflect.DeepEqual(oldObj.Data, newObj.Data), nil
		}
	case *core_v1.Service:
		if newObj, ok := newObj.(*core_v1.Service); ok {
			return apiequality.Semantic.DeepEqual(oldObj.Spec, newObj.Spec) &&
				apiequality.Semantic.DeepEqual(oldObj.Status, newObj.Status) &&
				apiequality.Semantic.DeepEqual(oldObj.GetAnnotations(), newObj.GetAnnotations()), nil
		}
	case *core_v1.Endpoints:
		if newObj, ok := newObj.(*core_v1.Endpoints); ok {
			return apiequality.Semantic.DeepEqual(oldObj.Subsets, newObj.Subsets), nil
		}
	case *core_v1.Namespace:
		if newObj, ok := newObj.(*core_v1.Namespace); ok {
			return apiequality.Semantic.DeepEqual(oldObj.Labels, newObj.Labels), nil
		}
	}

	// ResourceVersions are not equal and we don't know how to compare the object type.
	// This should never happen and indicates that new type was added to the code but is missing in the switch above.
	return false, fmt.Errorf("do not know how to compare %T", newObj)
}

func isGenerationEqual(a, b client.Object) bool {
	return a.GetGeneration() == b.GetGeneration()
}

func isResourceVersionEqual(a, b client.Object) bool {
	return a.GetResourceVersion() == b.GetResourceVersion()
}
