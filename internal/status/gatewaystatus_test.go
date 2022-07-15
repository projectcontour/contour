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
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestGatewayAddCondition(t *testing.T) {
	var testGeneration int64 = 7

	simpleValidCondition := metav1.Condition{
		Type:               string(gatewayapi_v1beta1.GatewayConditionScheduled),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayapi_v1beta1.GatewayReasonScheduled),
		Message:            MessageValidGateway,
		ObservedGeneration: testGeneration,
	}

	gatewayUpdate := GatewayStatusUpdate{
		FullName:           k8s.NamespacedNameFrom("test/test"),
		Conditions:         make(map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition),
		ExistingConditions: nil,
		Generation:         testGeneration,
		TransitionTime:     metav1.Time{},
	}

	got := gatewayUpdate.AddCondition(
		gatewayapi_v1beta1.GatewayConditionScheduled,
		metav1.ConditionTrue,
		gatewayapi_v1beta1.GatewayReasonScheduled,
		MessageValidGateway,
	)

	assert.Equal(t, simpleValidCondition.Message, got.Message)
	assert.Equal(t, simpleValidCondition.Reason, got.Reason)
	assert.Equal(t, simpleValidCondition.Type, got.Type)
	assert.Equal(t, simpleValidCondition.Status, got.Status)
	assert.Equal(t, simpleValidCondition.ObservedGeneration, got.ObservedGeneration)
}

func TestGatewaySetListenerSupportedKinds(t *testing.T) {
	var gsu GatewayStatusUpdate

	gsu.SetListenerSupportedKinds("http", []gatewayapi_v1beta1.Kind{"HTTPRoute"})
	gsu.SetListenerSupportedKinds("https", []gatewayapi_v1beta1.Kind{"HTTPRoute", "TLSRoute"})

	assert.Len(t, gsu.ListenerStatus, 2)

	require.NotNil(t, gsu.ListenerStatus["http"])
	require.NotNil(t, gsu.ListenerStatus["https"])

	assert.ElementsMatch(t,
		[]gatewayapi_v1beta1.RouteGroupKind{
			{Group: gatewayapi.GroupPtr(gatewayapi_v1beta1.GroupName), Kind: "HTTPRoute"},
		},
		gsu.ListenerStatus["http"].SupportedKinds,
	)

	assert.ElementsMatch(t,
		[]gatewayapi_v1beta1.RouteGroupKind{
			{Group: gatewayapi.GroupPtr(gatewayapi_v1beta1.GroupName), Kind: "HTTPRoute"},
			{Group: gatewayapi.GroupPtr(gatewayapi_v1beta1.GroupName), Kind: "TLSRoute"},
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

	gsu.ListenerStatus = map[string]*gatewayapi_v1beta1.ListenerStatus{
		"http": {
			Name:           "http",
			AttachedRoutes: 7,
			SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
				{
					Group: gatewayapi.GroupPtr(gatewayapi_v1beta1.GroupName),
					Kind:  gatewayapi_v1beta1.Kind("FooRoute"),
				},
				{
					Group: gatewayapi.GroupPtr(gatewayapi_v1beta1.GroupName),
					Kind:  gatewayapi_v1beta1.Kind("BarRoute"),
				},
			},
			Conditions: []metav1.Condition{},
		},
		"https": {
			Name:           "https",
			AttachedRoutes: 77,
			SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
				{
					Group: gatewayapi.GroupPtr(gatewayapi_v1beta1.GroupName),
					Kind:  gatewayapi_v1beta1.Kind("TLSRoute"),
				},
			},
			Conditions: []metav1.Condition{},
		},
	}

	gw := &gatewayapi_v1beta1.Gateway{
		Status: gatewayapi_v1beta1.GatewayStatus{
			Listeners: []gatewayapi_v1beta1.ListenerStatus{
				{
					Name:           "http",
					AttachedRoutes: 3,
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: gatewayapi.GroupPtr(gatewayapi_v1beta1.GroupName),
							Kind:  gatewayapi_v1beta1.Kind("HTTPRoute"),
						},
					},
					Conditions: []metav1.Condition{},
				},
			},
		},
	}

	got, ok := gsu.Mutate(gw).(*gatewayapi_v1beta1.Gateway)
	require.True(t, ok)

	assert.Len(t, got.Status.Listeners, 2)

	var want []gatewayapi_v1beta1.ListenerStatus
	for _, v := range gsu.ListenerStatus {
		want = append(want, *v)
	}
	assert.ElementsMatch(t, want, got.Status.Listeners)
}

func TestGatewayAddListenerCondition(t *testing.T) {
	var gsu GatewayStatusUpdate

	// first condition for listener-1
	res := gsu.AddListenerCondition("listener-1", gatewayapi_v1beta1.ListenerConditionReady, metav1.ConditionFalse, gatewayapi_v1beta1.ListenerReasonInvalid, "message 1")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 1)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerConditionReady), res.Type)
	assert.Equal(t, metav1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerReasonInvalid), res.Reason)
	assert.Equal(t, "message 1", res.Message)

	// second condition (different type) for listener-1
	res = gsu.AddListenerCondition("listener-1", gatewayapi_v1beta1.ListenerConditionDetached, metav1.ConditionTrue, gatewayapi_v1beta1.ListenerReasonUnsupportedProtocol, "message 2")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerConditionDetached), res.Type)
	assert.Equal(t, metav1.ConditionTrue, res.Status)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerReasonUnsupportedProtocol), res.Reason)
	assert.Equal(t, "message 2", res.Message)

	// first condition for listener-2
	res = gsu.AddListenerCondition("listener-2", gatewayapi_v1beta1.ListenerConditionReady, metav1.ConditionFalse, gatewayapi_v1beta1.ListenerReasonInvalid, "message 3")
	assert.Len(t, gsu.ListenerStatus["listener-2"].Conditions, 1)
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerConditionReady), res.Type)
	assert.Equal(t, metav1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerReasonInvalid), res.Reason)
	assert.Equal(t, "message 3", res.Message)

	// third condition (pre-existing type) for listener-1
	res = gsu.AddListenerCondition("listener-1", gatewayapi_v1beta1.ListenerConditionDetached, metav1.ConditionTrue, gatewayapi_v1beta1.ListenerReasonUnsupportedProtocol, "message 4")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerConditionDetached), res.Type)
	assert.Equal(t, metav1.ConditionTrue, res.Status)
	assert.Equal(t, string(gatewayapi_v1beta1.ListenerReasonUnsupportedProtocol), res.Reason)
	assert.Equal(t, "message 2, message 4", res.Message)
}

func TestGetGatewayConditions(t *testing.T) {
	tests := map[string]struct {
		conditions []metav1.Condition
		want       map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition
	}{
		"no gateway conditions": {
			conditions: nil,
			want:       map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{},
		},
		"one gateway condition": {
			conditions: []metav1.Condition{
				{Type: string(gatewayapi_v1beta1.GatewayConditionReady)},
			},
			want: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionReady: {Type: string(gatewayapi_v1beta1.GatewayConditionReady)},
			},
		},
		"multiple gateway conditions": {
			conditions: []metav1.Condition{
				{Type: string(gatewayapi_v1beta1.GatewayConditionReady)},
				{Type: string(gatewayapi_v1beta1.GatewayConditionScheduled)},
			},
			want: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionReady:     {Type: string(gatewayapi_v1beta1.GatewayConditionReady)},
				gatewayapi_v1beta1.GatewayConditionScheduled: {Type: string(gatewayapi_v1beta1.GatewayConditionScheduled)},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := getGatewayConditions(&gatewayapi_v1beta1.GatewayStatus{Conditions: tc.conditions})
			assert.Equal(t, tc.want, got)
		})
	}
}
