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
	"github.com/projectcontour/contour/internal/k8s"
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

	type testcase struct {
		proxyUpdate       ProxyUpdate
		wantConditions    []projectcontour.DetailedCondition
		wantCurrentStatus string
		wantDescription   string
	}

	run := func(desc string, tc testcase) {
		mutator := tc.proxyUpdate.StatusMutatorFunc()
		newProxy := mutator.Mutate(&tc.proxyUpdate.Object)

		switch o := newProxy.(type) {
		case *projectcontour.HTTPProxy:
			assert.Equal(t, tc.wantConditions, o.Status.Conditions, desc)
			assert.Equal(t, tc.wantCurrentStatus, o.Status.CurrentStatus, desc)
			assert.Equal(t, tc.wantDescription, o.Status.Description, desc)
		default:
			t.Fatal("Got a non-HTTPProxy object, wow, impressive.")
		}
	}

	validConditionWarning := testcase{
		proxyUpdate: ProxyUpdate{
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
		},
		wantConditions: []projectcontour.DetailedCondition{
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
		},
		wantCurrentStatus: k8s.StatusValid,
		wantDescription:   "TLSErrorTLSConfigError: Syntax Error in TLS Config",
	}
	run("valid with one warning", validConditionWarning)

	inValidConditionError := testcase{
		proxyUpdate: ProxyUpdate{
			Object: projectcontour.HTTPProxy{
				ObjectMeta: v1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Generation: 6,
				},
			},
			Conditions: map[ConditionType]*projectcontour.DetailedCondition{
				ValidCondition: {
					Condition: projectcontour.Condition{
						Type:    string(ValidCondition),
						Status:  projectcontour.ConditionFalse,
						Reason:  "TLSErrorTLSConfigError",
						Message: "Syntax Error in TLS Config",
					},
					Errors: []projectcontour.SubCondition{
						{
							Type:    "TLSError",
							Reason:  "TLSConfigError",
							Message: "Syntax Error in TLS Config",
						},
					},
				},
			},
		},
		wantConditions: []projectcontour.DetailedCondition{
			{
				Condition: projectcontour.Condition{
					Type:               string(ValidCondition),
					Status:             projectcontour.ConditionFalse,
					ObservedGeneration: 6,
					Reason:             "TLSErrorTLSConfigError",
					Message:            "Syntax Error in TLS Config",
				},
				Errors: []projectcontour.SubCondition{
					{
						Type:    "TLSError",
						Reason:  "TLSConfigError",
						Message: "Syntax Error in TLS Config",
					},
				},
			},
		},
		wantCurrentStatus: k8s.StatusInvalid,
		wantDescription:   "TLSErrorTLSConfigError: Syntax Error in TLS Config",
	}
	run("invalid status, one error", inValidConditionError)

	orphanedCondition := testcase{
		proxyUpdate: ProxyUpdate{
			Object: projectcontour.HTTPProxy{
				ObjectMeta: v1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Generation: 6,
				},
			},
			Conditions: map[ConditionType]*projectcontour.DetailedCondition{
				ValidCondition: {
					Condition: projectcontour.Condition{
						Type:    string(ValidCondition),
						Status:  projectcontour.ConditionFalse,
						Reason:  "orphaned",
						Message: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
					},
					Errors: []projectcontour.SubCondition{
						{
							Type:    "orphaned",
							Reason:  "Orphaned",
							Message: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
						},
					},
				},
			},
		},
		wantConditions: []projectcontour.DetailedCondition{
			{
				Condition: projectcontour.Condition{
					Type:               string(ValidCondition),
					Status:             projectcontour.ConditionFalse,
					ObservedGeneration: 6,
					Reason:             "orphaned",
					Message:            "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				},
				Errors: []projectcontour.SubCondition{
					{
						Type:    "orphaned",
						Reason:  "Orphaned",
						Message: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
					},
				},
			},
		},
		wantCurrentStatus: k8s.StatusOrphaned,
		wantDescription:   "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
	}

	run("orphaned HTTPProxy", orphanedCondition)

	updateExistingValidCond := testcase{
		proxyUpdate: ProxyUpdate{
			Object: projectcontour.HTTPProxy{
				ObjectMeta: v1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Generation: 5,
				},
				Status: projectcontour.HTTPProxyStatus{
					Conditions: []projectcontour.DetailedCondition{
						{
							Condition: projectcontour.Condition{
								Type:   string(ValidCondition),
								Status: projectcontour.ConditionTrue,
							},
						},
					},
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
		},
		wantConditions: []projectcontour.DetailedCondition{
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
		},
		wantCurrentStatus: k8s.StatusValid,
		wantDescription:   "TLSErrorTLSConfigError: Syntax Error in TLS Config",
	}

	run("Test updating existing Valid Condition", updateExistingValidCond)
}
