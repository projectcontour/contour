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

package status

import (
	"fmt"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

type ProxyStatus string

const (
	ProxyStatusValid    ProxyStatus = "valid"
	ProxyStatusInvalid  ProxyStatus = "invalid"
	ProxyStatusOrphaned ProxyStatus = "orphaned"
)

// ProxyUpdate holds status updates for a particular HTTPProxy object
type ProxyUpdate struct {
	Fullname       types.NamespacedName
	Generation     int64
	TransitionTime meta_v1.Time
	Vhost          string

	// Conditions holds all the DetailedConditions to add to the object
	// keyed by the Type (since that's what the apiserver will end up
	// doing.)
	Conditions map[ConditionType]*contour_v1.DetailedCondition
}

// ConditionFor returns a DetailedCondition for a given ConditionType.
// Currently only "Valid" is used.
func (pu *ProxyUpdate) ConditionFor(cond ConditionType) *contour_v1.DetailedCondition {
	dc, ok := pu.Conditions[cond]
	if !ok {
		newDc := &contour_v1.DetailedCondition{}
		newDc.Type = string(cond)
		newDc.ObservedGeneration = pu.Generation
		if cond == ValidCondition {
			newDc.Status = contour_v1.ConditionTrue
			newDc.Reason = "Valid"
			newDc.Message = "Valid HTTPProxy"
		} else {
			newDc.Status = contour_v1.ConditionFalse
		}
		pu.Conditions[cond] = newDc
		return newDc
	}
	return dc
}

func (pu *ProxyUpdate) Mutate(obj client.Object) client.Object {
	o, ok := obj.(*contour_v1.HTTPProxy)
	if !ok {
		panic(fmt.Sprintf("Unsupported %T object %s/%s in status mutator",
			obj, pu.Fullname.Namespace, pu.Fullname.Name,
		))
	}

	proxy := o.DeepCopy()

	for condType, cond := range pu.Conditions {
		cond.ObservedGeneration = pu.Generation
		cond.LastTransitionTime = pu.TransitionTime

		currCond := proxy.Status.GetConditionFor(string(condType))
		if currCond == nil {
			proxy.Status.Conditions = append(proxy.Status.Conditions, *cond)
			continue
		}

		// Don't update the condition if our observation is stale.
		if currCond.ObservedGeneration > cond.ObservedGeneration {
			continue
		}

		cond.DeepCopyInto(currCond)

	}

	// Set the old status fields using the Valid DetailedCondition's details.
	// Other conditions are not relevant for these two fields.
	validCond := proxy.Status.GetConditionFor(contour_v1.ValidConditionType)

	switch validCond.Status {
	case contour_v1.ConditionTrue:
		// TODO(youngnick): bring the string(ProxyStatusValid) constants in here?
		proxy.Status.CurrentStatus = string(ProxyStatusValid)
		proxy.Status.Description = validCond.Message
	case contour_v1.ConditionFalse:
		if orphanCond, ok := validCond.GetError(contour_v1.ConditionTypeOrphanedError); ok {
			proxy.Status.CurrentStatus = string(ProxyStatusOrphaned)
			proxy.Status.Description = orphanCond.Message
			break
		}
		proxy.Status.CurrentStatus = string(ProxyStatusInvalid)

		// proxy.Status.Description = validCond.Reason + ": " + validCond.Message
		proxy.Status.Description = validCond.Message
	}

	return proxy
}
