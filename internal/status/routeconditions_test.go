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
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
)

func TestHTTPRouteAddCondition(t *testing.T) {
	httpRouteUpdate := RouteStatusUpdate{
		FullName:   k8s.NamespacedNameFrom("test/test"),
		Generation: 7,
	}

	parentRef := gatewayapi.GatewayParentRef("projectcontour", "contour")

	rpsUpdate := httpRouteUpdate.StatusUpdateFor(parentRef)

	rpsUpdate.AddCondition(gatewayapi_v1.RouteConditionAccepted, meta_v1.ConditionTrue, "Valid", "Valid HTTPRoute")

	require.Len(t, httpRouteUpdate.ConditionsForParentRef(parentRef), 1)
	got := httpRouteUpdate.ConditionsForParentRef(parentRef)[0]

	assert.EqualValues(t, gatewayapi_v1.RouteConditionAccepted, got.Type)
	assert.EqualValues(t, meta_v1.ConditionTrue, got.Status)
	assert.EqualValues(t, "Valid", got.Reason)
	assert.EqualValues(t, "Valid HTTPRoute", got.Message)
	assert.EqualValues(t, 7, got.ObservedGeneration)
}

func newCondition(t string, status meta_v1.ConditionStatus, reason, msg string, lt time.Time) meta_v1.Condition {
	return meta_v1.Condition{
		Type:               t,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: meta_v1.NewTime(lt),
	}
}
