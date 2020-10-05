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

	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ConditionType is used to ensure we only use a limited set of possible values
// for DetailedCondition types. It's cast back to a string before sending off to
// HTTPProxy structs, as those use upstream types which we can't alias easily.
type ConditionType string

// ValidCondition is the ConditionType for Valid.
const ValidCondition ConditionType = "Valid"

// ProxyUpdate holds status updates for a particular HTTPProxy object
type ProxyUpdate struct {
	Fullname       types.NamespacedName
	Generation     int64
	TransitionTime v1.Time
	Vhost          string

	// Conditions holds all the DetailedConditions to add to the object
	// keyed by the Type (since that's what the apiserver will end up
	// doing.)
	Conditions map[ConditionType]*projectcontour.DetailedCondition
}

// ConditionFor returns a DetailedCondition for a given ConditionType.
// Currently only "Valid" is used.
func (pu *ProxyUpdate) ConditionFor(cond ConditionType) *projectcontour.DetailedCondition {
	dc, ok := pu.Conditions[cond]
	if !ok {
		newDc := &projectcontour.DetailedCondition{}
		newDc.Type = string(cond)
		newDc.ObservedGeneration = pu.Generation
		if cond == ValidCondition {
			newDc.Status = projectcontour.ConditionTrue
			newDc.Reason = "Valid"
			newDc.Message = "Valid HTTPProxy"
		} else {
			newDc.Status = projectcontour.ConditionFalse
		}
		pu.Conditions[cond] = newDc
		return newDc
	}
	return dc

}

func (pu *ProxyUpdate) Mutate(obj interface{}) interface{} {
	o, ok := obj.(*projectcontour.HTTPProxy)
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
	validCond := proxy.Status.GetConditionFor(projectcontour.ValidConditionType)

	switch validCond.Status {
	case projectcontour.ConditionTrue:
		// TODO(youngnick): bring the string(ProxyStatusValid) constants in here?
		proxy.Status.CurrentStatus = string(ProxyStatusValid)
		proxy.Status.Description = validCond.Message
	case projectcontour.ConditionFalse:
		if orphanCond, ok := validCond.GetError(string(OrphanedConditionType)); ok {
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
