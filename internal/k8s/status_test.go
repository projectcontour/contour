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

package k8s

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s/mocks"
)

func TestStatusUpdateHandlerRequiresLeaderElection(t *testing.T) {
	var s manager.LeaderElectionRunnable = &StatusUpdateHandler{}
	require.True(t, s.NeedLeaderElection())
}

func TestStatusUpdateHandlerApplyOutputsMetrics(t *testing.T) {
	fooIngress := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: "somens",
		},
	}
	c := fake.NewClientBuilder().WithObjects(fooIngress)

	mockStatusMetrics := mocks.NewStatusMetrics(t)

	suh := NewStatusUpdateHandler(fixture.NewTestLogger(t), c.Build(), mockStatusMetrics)

	// Ingress with no status changes.
	mockStatusMetrics.On("SetStatusUpdateTotal", "Ingress").Once()
	mockStatusMetrics.On("SetStatusUpdateDuration", mock.Anything, "Ingress", false).Once()
	mockStatusMetrics.On("SetStatusUpdateSuccess", "Ingress").Once()
	mockStatusMetrics.On("SetStatusUpdateNoop", "Ingress").Once()

	suh.apply(NewStatusUpdate(
		fooIngress.Name,
		fooIngress.Namespace,
		fooIngress,
		StatusMutatorFunc(func(obj client.Object) client.Object {
			return obj
		}),
	))

	// Resource does not exist, so update fails.
	mockStatusMetrics.On("SetStatusUpdateTotal", "Ingress").Once()
	mockStatusMetrics.On("SetStatusUpdateDuration", mock.Anything, "Ingress", true).Once()
	mockStatusMetrics.On("SetStatusUpdateFailed", "Ingress").Once()

	suh.apply(NewStatusUpdate(
		"bar",
		"somens",
		&networking_v1.Ingress{},
		StatusMutatorFunc(func(obj client.Object) client.Object {
			return obj
		}),
	))

	// Resource has a status update.
	mockStatusMetrics.On("SetStatusUpdateTotal", "Ingress").Once()
	mockStatusMetrics.On("SetStatusUpdateDuration", mock.Anything, "Ingress", false).Once()
	mockStatusMetrics.On("SetStatusUpdateSuccess", "Ingress").Once()

	suh.apply(NewStatusUpdate(
		fooIngress.Name,
		fooIngress.Namespace,
		fooIngress,
		StatusMutatorFunc(func(obj client.Object) client.Object {
			i := obj.DeepCopyObject().(*networking_v1.Ingress)
			i.Status.LoadBalancer.Ingress = []networking_v1.IngressLoadBalancerIngress{
				{IP: "1.1.1.1"},
			}
			return i
		}),
	))
}
