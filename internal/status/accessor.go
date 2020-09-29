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
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NewAccessor returns a new status condition accessor for this Kubernetes object.
func NewAccessor(obj k8s.Object) *Accessor {
	return &Accessor{
		Name:           k8s.NamespacedNameOf(obj),
		Generation:     obj.GetObjectMeta().GetGeneration(),
		TransitionTime: v1.NewTime(time.Now()),
		Conditions:     make(map[ConditionType]*contour_api_v1.DetailedCondition),
	}
}

// Accessor holds status updates for a particular object.
type Accessor struct {
	Name           types.NamespacedName
	Generation     int64
	TransitionTime v1.Time

	// Conditions holds all the DetailedConditions to add to the object
	// keyed by the Type (since that's what the API server will end up
	// doing.)
	Conditions map[ConditionType]*contour_api_v1.DetailedCondition
}

// ConditionFor returns a DetailedCondition for a given ConditionType.
func (a *Accessor) ConditionFor(cond ConditionType) *contour_api_v1.DetailedCondition {
	dc, ok := a.Conditions[cond]
	if !ok {
		newDc := &contour_api_v1.DetailedCondition{}
		newDc.Type = string(cond)

		a.Conditions[cond] = newDc
		return newDc
	}

	return dc
}
