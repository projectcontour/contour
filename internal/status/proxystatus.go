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

// ConditionType is used to ensure we only use a limited set of possible values
// for DetailedCondition types. It's cast back to a string before sending off to
// HTTPProxy structs, as those use upstream types which we can't alias easily.
type ConditionType string

// ValidCondition is the ConditionType for Valid.
const ValidCondition ConditionType = "Valid"

func ProxyStatusMutator(a *Accessor) k8s.StatusMutator {
	return k8s.StatusMutatorFunc(func(obj interface{}) interface{} {
		o, ok := obj.(*projectcontour.HTTPProxy)
		if !ok {
			panic(fmt.Sprintf("unsupported %T object %q in status mutator", obj, a.Name))
		}

		proxy := o.DeepCopy()

		for condType, cond := range a.Conditions {
			cond.ObservedGeneration = a.Generation
			cond.LastTransitionTime = a.TransitionTime

			currCond := proxy.Status.GetConditionFor(string(condType))
			if currCond == nil {
				proxy.Status.Conditions = append(proxy.Status.Conditions, *cond)
				continue
			}

			cond.DeepCopyInto(currCond)
		}

		// Set the old status fields using the Valid DetailedCondition's details.
		// Other conditions are not relevant for these two fields.
		validCond := proxy.Status.GetConditionFor(projectcontour.ValidConditionType)

		switch validCond.Status {
		case projectcontour.ConditionTrue:
			// TODO(youngnick): bring the k8s.StatusValid constants in here?
			proxy.Status.CurrentStatus = k8s.StatusValid
			proxy.Status.Description = validCond.Reason + ": " + validCond.Message
		case projectcontour.ConditionFalse:
			orphanCond, orphaned := validCond.GetError(k8s.StatusOrphaned)
			if orphaned {
				proxy.Status.CurrentStatus = k8s.StatusOrphaned
				proxy.Status.Description = orphanCond.Message
				break
			}
			proxy.Status.CurrentStatus = k8s.StatusInvalid
			proxy.Status.Description = validCond.Reason + ": " + validCond.Message
		}

		return proxy
	})
}
