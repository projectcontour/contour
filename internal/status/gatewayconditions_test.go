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

	"github.com/projectcontour/contour/internal/k8s"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestGatewayAddCondition(t *testing.T) {

	var testGeneration int64 = 7

	simpleValidCondition := metav1.Condition{
		Type:               string(gatewayapi_v1alpha2.GatewayConditionScheduled),
		Status:             metav1.ConditionTrue,
		Reason:             ReasonValidGateway,
		Message:            "Valid Gateway",
		ObservedGeneration: testGeneration,
	}

	gatewayUpdate := GatewayConditionsUpdate{
		FullName:           k8s.NamespacedNameFrom("test/test"),
		Conditions:         make(map[gatewayapi_v1alpha2.GatewayConditionType]metav1.Condition),
		ExistingConditions: nil,
		GatewayRef:         types.NamespacedName{},
		Resource:           "",
		Generation:         testGeneration,
		TransitionTime:     metav1.Time{},
	}

	got := gatewayUpdate.AddCondition(gatewayapi_v1alpha2.GatewayConditionScheduled, metav1.ConditionTrue, ReasonValidGateway,
		"Valid Gateway")

	assert.Equal(t, simpleValidCondition.Message, got.Message)
	assert.Equal(t, simpleValidCondition.Reason, got.Reason)
	assert.Equal(t, simpleValidCondition.Type, got.Type)
	assert.Equal(t, simpleValidCondition.Status, got.Status)
	assert.Equal(t, simpleValidCondition.ObservedGeneration, got.ObservedGeneration)
}
