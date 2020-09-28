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
	"time"

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
		Fullname: k8s.NamespacedNameFrom("test/test"),
		Conditions: map[ConditionType]*projectcontour.DetailedCondition{
			ValidCondition: &simpleValidCondition,
		},
	}

	got := pu.ConditionFor(ValidCondition)

	assert.Equal(t, simpleValidCondition, *got.DeepCopy())

	var emptyCondition ConditionType
	gotNil := pu.ConditionFor(emptyCondition)
	assert.Nil(t, gotNil)

	emptyProxyUpdate := ProxyUpdate{
		Fullname:   k8s.NamespacedNameFrom("test/test"),
		Conditions: make(map[ConditionType]*projectcontour.DetailedCondition),
	}

	newDc := projectcontour.DetailedCondition{
		Condition: projectcontour.Condition{
			Type: string(ValidCondition),
		},
	}
	gotEmpty := emptyProxyUpdate.ConditionFor(ValidCondition)
	assert.Equal(t, newDc, *gotEmpty)
}

func TestStatusMutator(t *testing.T) {
	type testcase struct {
		testProxy         projectcontour.HTTPProxy
		proxyUpdate       ProxyUpdate
		wantConditions    []projectcontour.DetailedCondition
		wantCurrentStatus string
		wantDescription   string
	}

	testTransitionTime := v1.NewTime(time.Now())
	var testGeneration int64 = 7

	run := func(desc string, tc testcase) {
		newProxy := tc.proxyUpdate.Mutate(&tc.testProxy)

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
		testProxy: projectcontour.HTTPProxy{
			ObjectMeta: v1.ObjectMeta{
				Name:       "test",
				Namespace:  "test",
				Generation: testGeneration,
			},
		},
		proxyUpdate: ProxyUpdate{
			Fullname:       k8s.NamespacedNameFrom("test/test"),
			Generation:     testGeneration,
			TransitionTime: testTransitionTime,
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
					ObservedGeneration: testGeneration,
					LastTransitionTime: testTransitionTime,
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
		testProxy: projectcontour.HTTPProxy{
			ObjectMeta: v1.ObjectMeta{
				Name:       "test",
				Namespace:  "test",
				Generation: 6,
			},
		},
		proxyUpdate: ProxyUpdate{
			Fullname:       k8s.NamespacedNameFrom("test/test"),
			Generation:     testGeneration,
			TransitionTime: testTransitionTime,
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
					ObservedGeneration: testGeneration,
					LastTransitionTime: testTransitionTime,
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
		testProxy: projectcontour.HTTPProxy{
			ObjectMeta: v1.ObjectMeta{
				Name:       "test",
				Namespace:  "test",
				Generation: testGeneration,
			},
		},
		proxyUpdate: ProxyUpdate{
			Fullname:       k8s.NamespacedNameFrom("test/test"),
			Generation:     testGeneration,
			TransitionTime: testTransitionTime,
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
					ObservedGeneration: testGeneration,
					LastTransitionTime: testTransitionTime,
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
		testProxy: projectcontour.HTTPProxy{
			ObjectMeta: v1.ObjectMeta{
				Name:       "test",
				Namespace:  "test",
				Generation: testGeneration,
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
		proxyUpdate: ProxyUpdate{
			Fullname:       k8s.NamespacedNameFrom("test/test"),
			Generation:     testGeneration,
			TransitionTime: testTransitionTime,
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
					ObservedGeneration: testGeneration,
					LastTransitionTime: testTransitionTime,
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
