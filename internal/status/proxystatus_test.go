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
	"testing"

	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConditionFor(t *testing.T) {

	simpleValidCondition := projectcontour.DetailedCondition{
		Condition: projectcontour.Condition{
			Type: "Valid",
		},
	}

	pu := ProxyUpdate{
		Object: projectcontour.HTTPProxy{},
		Conditions: map[ConditionType]*projectcontour.DetailedCondition{
			ValidCondition: &simpleValidCondition,
		},
	}

	got := pu.ConditionFor(ValidCondition)

	assert.Equal(t, simpleValidCondition, *got.DeepCopy())

	var emptyCondition ConditionType
	gotNil := pu.ConditionFor(emptyCondition)
	assert.Nil(t, gotNil)

	var newCondition ConditionType = "NewCondition"
	newDc := projectcontour.DetailedCondition{
		Condition: projectcontour.Condition{
			Type: string(newCondition),
		},
	}
	gotEmpty := pu.ConditionFor(newCondition)
	assert.Equal(t, newDc, *gotEmpty)
}

func TestStatusMutatorFunc(t *testing.T) {

	proxyUpdate := ProxyUpdate{
		Object: projectcontour.HTTPProxy{
			ObjectMeta: v1.ObjectMeta{
				Name:       "test",
				Namespace:  "test",
				Generation: 5,
			},
		},
		Conditions: map[ConditionType]*projectcontour.DetailedCondition{
			ValidCondition: {
				Condition: projectcontour.Condition{
					Type:    string(ValidCondition),
					Status:  projectcontour.ConditionTrue,
					Reason:  "TLSErrorTLSConfigError",
					Message: "Syntax Error in TLS Config",
				},
				Warnings: []projectcontour.SubCondition{
					{
						Type:    "TLSError",
						Reason:  "TLSConfigError",
						Message: "Syntax Error in TLS Config",
					},
				},
			},
		},
	}

	wantConditions := []projectcontour.DetailedCondition{
		{
			Condition: projectcontour.Condition{
				Type:               string(ValidCondition),
				Status:             projectcontour.ConditionTrue,
				ObservedGeneration: 5,
				Reason:             "TLSErrorTLSConfigError",
				Message:            "Syntax Error in TLS Config",
			},
			Warnings: []projectcontour.SubCondition{
				{
					Type:    "TLSError",
					Reason:  "TLSConfigError",
					Message: "Syntax Error in TLS Config",
				},
			},
		},
	}

	mutator := proxyUpdate.StatusMutatorFunc()
	newProxy := mutator.Mutate(&proxyUpdate.Object)

	switch o := newProxy.(type) {
	case *projectcontour.HTTPProxy:
		assert.Equal(t, wantConditions, o.Status.Conditions)
		assert.Equal(t, "valid", o.Status.CurrentStatus)
		assert.Equal(t, "TLSErrorTLSConfigError: Syntax Error in TLS Config", o.Status.Description)
	default:
		t.Fatal("Got a non-HTTPProxy object, wow, impressive.")
	}

}
