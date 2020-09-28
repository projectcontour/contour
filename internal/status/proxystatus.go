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
	"github.com/projectcontour/contour/internal/k8s"
)

type ConditionType string

const ValidCondition ConditionType = "Valid"

// ProxyUpdate holds status updates for a particular HTTPProxy object
type ProxyUpdate struct {
	// Object holds a copy of the HTTPProxy object that the Conditions refer to.
	// This is intended for read-only use; any changes made to this object will
	// be lost.
	Object projectcontour.HTTPProxy
	// Conditions holds all the DetailedConditions to add to the object
	// keyed byt the Type (since that's what the apiserver will end up
	// doing.)
	Conditions map[ConditionType]*projectcontour.DetailedCondition
}

// ConditionFor returns a DetailedCondition for a given ConditionType.
// Currently only "Valid" is supported.
func (pu *ProxyUpdate) ConditionFor(cond ConditionType) *projectcontour.DetailedCondition {

	if cond == "" {
		return nil
	}

	dc, ok := pu.Conditions[cond]
	if !ok {
		newDc := &projectcontour.DetailedCondition{}
		newDc.Type = string(cond)

		pu.Conditions[cond] = newDc
		return newDc
	}
	return dc
}

func (pu *ProxyUpdate) StatusMutatorFunc() k8s.StatusMutator {

	return k8s.StatusMutatorFunc(func(obj interface{}) interface{} {
		switch o := obj.(type) {
		case *projectcontour.HTTPProxy:
			proxy := o.DeepCopy()

			currentGeneration := proxy.Generation

			for condType, cond := range pu.Conditions {
				// Set the ObservedGeneration correctly
				cond.ObservedGeneration = currentGeneration

				condIndex := projectcontour.GetConditionIndex(string(condType), proxy.Status.Conditions)
				if condIndex >= 0 {
					proxy.Status.Conditions[condIndex] = *cond
				} else {
					proxy.Status.Conditions = append(proxy.Status.Conditions, *cond)
				}

			}

			// Set the old status fields using the Valid DetailedCondition's details.
			// Other conditions are not relevant for these two fields.
			validCondIndex := projectcontour.GetConditionIndex(string(ValidCondition), proxy.Status.Conditions)
			if validCondIndex >= 0 {
				validCond := proxy.Status.Conditions[validCondIndex]
				switch validCond.Status {
				case projectcontour.ConditionTrue:
					orphanCond, orphaned := validCond.GetError(k8s.StatusOrphaned)
					if orphaned {
						proxy.Status.CurrentStatus = k8s.StatusOrphaned
						proxy.Status.CurrentStatus = orphanCond.Message
					}
					proxy.Status.CurrentStatus = k8s.StatusValid
					proxy.Status.Description = validCond.Reason + ": " + validCond.Message
				case projectcontour.ConditionFalse:
					proxy.Status.CurrentStatus = k8s.StatusInvalid
					proxy.Status.Description = validCond.Reason + ": " + validCond.Message
				}
			}

			return proxy
		default:
			panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
				pu.Object.Namespace, pu.Object.Name,
			))
		}

	})
}
