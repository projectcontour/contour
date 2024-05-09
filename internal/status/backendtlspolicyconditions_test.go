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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
)

func TestBackendTLSPolicyAddCondition(t *testing.T) {
	backendTLSPolicyUpdate := BackendTLSPolicyStatusUpdate{
		FullName:   k8s.NamespacedNameFrom("test/test"),
		Generation: 7,
	}

	ancestorRef := gatewayapi.GatewayParentRef("projectcontour", "contour")

	basUpdate := backendTLSPolicyUpdate.StatusUpdateFor(ancestorRef)

	basUpdate.AddCondition(gatewayapi_v1alpha2.PolicyConditionAccepted, meta_v1.ConditionTrue, gatewayapi_v1alpha2.PolicyReasonAccepted, "Valid BackendTLSPolicy")

	require.Len(t, backendTLSPolicyUpdate.ConditionsForAncestorRef(ancestorRef), 1)
	got := backendTLSPolicyUpdate.ConditionsForAncestorRef(ancestorRef)[0]

	assert.EqualValues(t, gatewayapi_v1alpha2.PolicyConditionAccepted, got.Type)
	assert.EqualValues(t, meta_v1.ConditionTrue, got.Status)
	assert.EqualValues(t, gatewayapi_v1alpha2.PolicyReasonAccepted, got.Reason)
	assert.EqualValues(t, "Valid BackendTLSPolicy", got.Message)
	assert.EqualValues(t, 7, got.ObservedGeneration)
}

func TestBackendTLSPolicyMutate(t *testing.T) {
	testTransitionTime := meta_v1.NewTime(time.Now())
	var testGeneration int64 = 7

	bsu := BackendTLSPolicyStatusUpdate{
		FullName:       k8s.NamespacedNameFrom("test/test"),
		Generation:     testGeneration,
		TransitionTime: testTransitionTime,
		PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
			{
				AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
				Conditions: []meta_v1.Condition{
					{
						Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
						Status:  contour_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1alpha2.PolicyReasonAccepted),
						Message: "Accepted BackendTLSPolicy",
					},
				},
			},
		},
	}

	btp := &gatewayapi_v1alpha3.BackendTLSPolicy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Status: gatewayapi_v1alpha2.PolicyStatus{
			Ancestors: []gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("externalgateway", "some-gateway"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonAccepted),
							Message: "This was added by some other gateway and should not be removed.",
						},
					},
				},
			},
		},
	}

	wantBackendTLSPolicy := &gatewayapi_v1alpha3.BackendTLSPolicy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Status: gatewayapi_v1alpha2.PolicyStatus{
			Ancestors: []gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							ObservedGeneration: testGeneration,
							LastTransitionTime: testTransitionTime,
							Type:               string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:             contour_v1.ConditionTrue,
							Reason:             string(gatewayapi_v1alpha2.PolicyReasonAccepted),
							Message:            "Accepted BackendTLSPolicy",
						},
					},
				},
				{
					AncestorRef: gatewayapi.GatewayParentRef("externalgateway", "some-gateway"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonAccepted),
							Message: "This was added by some other gateway and should not be removed.",
						},
					},
				},
			},
		},
	}

	btp, ok := bsu.Mutate(btp).(*gatewayapi_v1alpha3.BackendTLSPolicy)
	require.True(t, ok)
	assert.Equal(t, wantBackendTLSPolicy, btp, 1)
}
