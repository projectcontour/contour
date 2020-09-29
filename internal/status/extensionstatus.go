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

	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/k8s"
)

func ExtensionStatusMutator(a *Accessor) k8s.StatusMutator {
	return k8s.StatusMutatorFunc(func(obj interface{}) interface{} {
		o, ok := obj.(*v1alpha1.ExtensionService)
		if !ok {
			panic(fmt.Sprintf("unsupported %T object %q in status mutator", obj, a.Name))
		}

		ext := o.DeepCopy()

		for condType, cond := range a.Conditions {
			cond.ObservedGeneration = a.Generation
			cond.LastTransitionTime = a.TransitionTime

			currCond := ext.Status.GetConditionFor(string(condType))
			if currCond == nil {
				ext.Status.Conditions = append(ext.Status.Conditions, *cond)
				continue
			}

			cond.DeepCopyInto(currCond)
		}

		return ext
	})
}
