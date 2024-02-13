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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/k8s"
)

func TestGatewayAddCondition(t *testing.T) {
	var testGeneration int64 = 7

	simpleValidCondition := meta_v1.Condition{
		Type:               string(gatewayapi_v1.GatewayConditionAccepted),
		Status:             meta_v1.ConditionTrue,
		Reason:             string(gatewayapi_v1.GatewayReasonAccepted),
		Message:            MessageValidGateway,
		ObservedGeneration: testGeneration,
	}

	gatewayUpdate := GatewayStatusUpdate{
		FullName:           k8s.NamespacedNameFrom("test/test"),
		Conditions:         make(map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition),
		ExistingConditions: nil,
		Generation:         testGeneration,
		TransitionTime:     meta_v1.Time{},
	}

	got := gatewayUpdate.AddCondition(
		gatewayapi_v1.GatewayConditionAccepted,
		meta_v1.ConditionTrue,
		gatewayapi_v1.GatewayReasonAccepted,
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

	gsu.SetListenerSupportedKinds("http", []gatewayapi_v1.Kind{"HTTPRoute"})
	gsu.SetListenerSupportedKinds("https", []gatewayapi_v1.Kind{"HTTPRoute", "TLSRoute"})

	assert.Len(t, gsu.ListenerStatus, 2)

	require.NotNil(t, gsu.ListenerStatus["http"])
	require.NotNil(t, gsu.ListenerStatus["https"])

	assert.ElementsMatch(t,
		[]gatewayapi_v1.RouteGroupKind{
			{Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)), Kind: "HTTPRoute"},
		},
		gsu.ListenerStatus["http"].SupportedKinds,
	)

	assert.ElementsMatch(t,
		[]gatewayapi_v1.RouteGroupKind{
			{Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)), Kind: "HTTPRoute"},
			{Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)), Kind: "TLSRoute"},
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

	gsu.ListenerStatus = map[string]*gatewayapi_v1.ListenerStatus{
		"http": {
			Name:           "http",
			AttachedRoutes: 7,
			SupportedKinds: []gatewayapi_v1.RouteGroupKind{
				{
					Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
					Kind:  gatewayapi_v1.Kind("FooRoute"),
				},
				{
					Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
					Kind:  gatewayapi_v1.Kind("BarRoute"),
				},
			},
			Conditions: []meta_v1.Condition{},
		},
		"https": {
			Name:           "https",
			AttachedRoutes: 77,
			SupportedKinds: []gatewayapi_v1.RouteGroupKind{
				{
					Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
					Kind:  gatewayapi_v1.Kind("TLSRoute"),
				},
			},
			Conditions: []meta_v1.Condition{},
		},
	}

	gw := &gatewayapi_v1.Gateway{
		Status: gatewayapi_v1.GatewayStatus{
			Listeners: []gatewayapi_v1.ListenerStatus{
				{
					Name:           "http",
					AttachedRoutes: 3,
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  gatewayapi_v1.Kind("HTTPRoute"),
						},
					},
					Conditions: []meta_v1.Condition{},
				},
			},
		},
	}

	got, ok := gsu.Mutate(gw).(*gatewayapi_v1.Gateway)
	require.True(t, ok)

	assert.Len(t, got.Status.Listeners, 2)

	var want []gatewayapi_v1.ListenerStatus
	for _, v := range gsu.ListenerStatus {
		want = append(want, *v)
	}
	assert.ElementsMatch(t, want, got.Status.Listeners)
}

func TestGatewayAddListenerCondition(t *testing.T) {
	var gsu GatewayStatusUpdate

	// first condition for listener-1
	res := gsu.AddListenerCondition("listener-1", gatewayapi_v1.ListenerConditionProgrammed, meta_v1.ConditionFalse, gatewayapi_v1.ListenerReasonInvalid, "message 1")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 1)
	assert.Equal(t, string(gatewayapi_v1.ListenerConditionProgrammed), res.Type)
	assert.Equal(t, meta_v1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1.ListenerReasonInvalid), res.Reason)
	assert.Equal(t, "message 1", res.Message)

	// second condition (different type) for listener-1
	res = gsu.AddListenerCondition("listener-1", gatewayapi_v1.ListenerConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1.ListenerReasonUnsupportedProtocol, "message 2")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1.ListenerConditionAccepted), res.Type)
	assert.Equal(t, meta_v1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1.ListenerReasonUnsupportedProtocol), res.Reason)
	assert.Equal(t, "message 2", res.Message)

	// first condition for listener-2
	res = gsu.AddListenerCondition("listener-2", gatewayapi_v1.ListenerConditionProgrammed, meta_v1.ConditionFalse, gatewayapi_v1.ListenerReasonInvalid, "message 3")
	assert.Len(t, gsu.ListenerStatus["listener-2"].Conditions, 1)
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1.ListenerConditionProgrammed), res.Type)
	assert.Equal(t, meta_v1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1.ListenerReasonInvalid), res.Reason)
	assert.Equal(t, "message 3", res.Message)

	// third condition (pre-existing type) for listener-1
	res = gsu.AddListenerCondition("listener-1", gatewayapi_v1.ListenerConditionAccepted, meta_v1.ConditionFalse, gatewayapi_v1.ListenerReasonUnsupportedProtocol, "message 4")
	assert.Len(t, gsu.ListenerStatus["listener-1"].Conditions, 2)
	assert.Equal(t, string(gatewayapi_v1.ListenerConditionAccepted), res.Type)
	assert.Equal(t, meta_v1.ConditionFalse, res.Status)
	assert.Equal(t, string(gatewayapi_v1.ListenerReasonUnsupportedProtocol), res.Reason)
	assert.Equal(t, "message 2, message 4", res.Message)
}

func TestGetGatewayConditions(t *testing.T) {
	tests := map[string]struct {
		conditions []meta_v1.Condition
		want       map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition
	}{
		"no gateway conditions": {
			conditions: nil,
			want:       map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{},
		},
		"one gateway condition": {
			conditions: []meta_v1.Condition{
				{Type: string(gatewayapi_v1.GatewayConditionProgrammed)},
			},
			want: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionProgrammed: {Type: string(gatewayapi_v1.GatewayConditionProgrammed)},
			},
		},
		"multiple gateway conditions": {
			conditions: []meta_v1.Condition{
				{Type: string(gatewayapi_v1.GatewayConditionProgrammed)},
				{Type: string(gatewayapi_v1.GatewayConditionAccepted)},
			},
			want: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionProgrammed: {Type: string(gatewayapi_v1.GatewayConditionProgrammed)},
				gatewayapi_v1.GatewayConditionAccepted:   {Type: string(gatewayapi_v1.GatewayConditionAccepted)},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := getGatewayConditions(&gatewayapi_v1.GatewayStatus{Conditions: tc.conditions})
			assert.Equal(t, tc.want, got)
		})
	}
}
