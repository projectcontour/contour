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

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	gatewayUpdate := GatewayStatusUpdate{
		FullName:           k8s.NamespacedNameFrom("test/test"),
		Conditions:         make(map[gatewayapi_v1alpha2.GatewayConditionType]metav1.Condition),
		ExistingConditions: nil,
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

func TestGatewaySetListenerSupportedKinds(t *testing.T) {
	var gsu GatewayStatusUpdate

	gsu.SetListenerSupportedKinds("http", []gatewayapi_v1alpha2.Kind{"HTTPRoute"})
	gsu.SetListenerSupportedKinds("https", []gatewayapi_v1alpha2.Kind{"HTTPRoute", "TLSRoute"})

	assert.Len(t, gsu.ListenerStatus, 2)

	require.NotNil(t, gsu.ListenerStatus["http"])
	require.NotNil(t, gsu.ListenerStatus["https"])

	assert.ElementsMatch(t,
		[]gatewayapi_v1alpha2.RouteGroupKind{
			{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
		},
		gsu.ListenerStatus["http"].SupportedKinds,
	)

	assert.ElementsMatch(t,
		[]gatewayapi_v1alpha2.RouteGroupKind{
			{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
			{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
		},
		gsu.ListenerStatus["https"].SupportedKinds,
	)
}

func TestGatewaySetListenerAttachedRoutes(t *testing.T) {
	var gsu GatewayStatusUpdate

	gsu.SetListenerAttachedRoutes("http", 7)
	gsu.SetListenerAttachedRoutes("https", 77)

	assert.Len(t, gsu.ListenerStatus, 2)

	require.NotNil(t, gsu.ListenerStatus["http"])
	require.NotNil(t, gsu.ListenerStatus["https"])

	assert.Equal(t, int32(7), gsu.ListenerStatus["http"].AttachedRoutes)
	assert.Equal(t, int32(77), gsu.ListenerStatus["https"].AttachedRoutes)
}

func TestGatewayMutate(t *testing.T) {
	var gsu GatewayStatusUpdate

	gsu.ListenerStatus = map[string]*gatewayapi_v1alpha2.ListenerStatus{
		"http": {
			Name:           "http",
			AttachedRoutes: 7,
			SupportedKinds: []gatewayapi_v1alpha2.RouteGroupKind{
				{
					Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
					Kind:  gatewayapi_v1alpha2.Kind("FooRoute"),
				},
				{
					Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
					Kind:  gatewayapi_v1alpha2.Kind("BarRoute"),
				},
			},
			Conditions: []metav1.Condition{},
		},
		"https": {
			Name:           "https",
			AttachedRoutes: 77,
			SupportedKinds: []gatewayapi_v1alpha2.RouteGroupKind{
				{
					Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
					Kind:  gatewayapi_v1alpha2.Kind("TLSRoute"),
				},
			},
			Conditions: []metav1.Condition{},
		},
	}

	gw := &gatewayapi_v1alpha2.Gateway{
		Status: gatewayapi_v1alpha2.GatewayStatus{
			Listeners: []gatewayapi_v1alpha2.ListenerStatus{
				{
					Name:           "http",
					AttachedRoutes: 3,
					SupportedKinds: []gatewayapi_v1alpha2.RouteGroupKind{
						{
							Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
							Kind:  gatewayapi_v1alpha2.Kind("HTTPRoute"),
						},
					},
					Conditions: []metav1.Condition{},
				},
			},
		},
	}

	got, ok := gsu.Mutate(gw).(*gatewayapi_v1alpha2.Gateway)
	require.True(t, ok)

	assert.Len(t, got.Status.Listeners, 2)

	var want []gatewayapi_v1alpha2.ListenerStatus
	for _, v := range gsu.ListenerStatus {
		want = append(want, *v)
	}
	assert.ElementsMatch(t, want, got.Status.Listeners)
}

func TestGatewayAddListenerCondition(t *testing.T) {
	var gsu GatewayStatusUpdate

	// first condition for listener-1
	res := gsu.AddListenerCondition("listener-1", gatewayapi_v1alpha2.ListenerConditionReady, metav1.ConditionFalse, gatewayapi_v1alpha2.ListenerReasonInvalid, "message 1")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 1)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerConditionReady), res.Type)
	assert.Equal(t, metav1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerReasonInvalid), res.Reason)
	assert.Equal(t, "message 1", res.Message)

	// second condition (different type) for listener-1
	res = gsu.AddListenerCondition("listener-1", gatewayapi_v1alpha2.ListenerConditionDetached, metav1.ConditionTrue, gatewayapi_v1alpha2.ListenerReasonUnsupportedProtocol, "message 2")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerConditionDetached), res.Type)
	assert.Equal(t, metav1.ConditionTrue, res.Status)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerReasonUnsupportedProtocol), res.Reason)
	assert.Equal(t, "message 2", res.Message)

	// first condition for listener-2
	res = gsu.AddListenerCondition("listener-2", gatewayapi_v1alpha2.ListenerConditionReady, metav1.ConditionFalse, gatewayapi_v1alpha2.ListenerReasonInvalid, "message 3")
	assert.Len(t, gsu.ListenerStatus["listener-2"].Conditions, 1)
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerConditionReady), res.Type)
	assert.Equal(t, metav1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerReasonInvalid), res.Reason)
	assert.Equal(t, "message 3", res.Message)

	// third condition (pre-existing type) for listener-1
	res = gsu.AddListenerCondition("listener-1", gatewayapi_v1alpha2.ListenerConditionDetached, metav1.ConditionTrue, gatewayapi_v1alpha2.ListenerReasonUnsupportedProtocol, "message 4")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerConditionDetached), res.Type)
	assert.Equal(t, metav1.ConditionTrue, res.Status)
	assert.Equal(t, string(gatewayapi_v1alpha2.ListenerReasonUnsupportedProtocol), res.Reason)
	assert.Equal(t, "message 2, message 4", res.Message)
}
