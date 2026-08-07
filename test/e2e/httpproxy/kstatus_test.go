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

//go:build e2e

package httpproxy

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

// These tests call the kstatus library to compute the status of an HTTPProxy.
// A client uses that status to know when Contour has fully reconciled the resource.
//
// kstatus does not know the Contour `Valid` condition.
// It computes the status from `status.observedGeneration` and from the `Stalled` condition instead.
// https://github.com/kubernetes-sigs/cli-utils/tree/master/pkg/kstatus

// computeStatus converts the HTTPProxy to Unstructured and computes its status.
func computeStatus(proxy *contour_v1.HTTPProxy) *status.Result {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(proxy)
	require.NoError(f.T(), err)

	result, err := status.Compute(&unstructured.Unstructured{Object: content})
	require.NoError(f.T(), err)

	return result
}

func testKstatusCurrentProxy(namespace string) {
	Specify("kstatus reports Current for a valid HTTPProxy", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		proxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "kstatus-valid",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "kstatus-valid.projectcontour.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: "echo",
						Port: 80,
					}},
				}},
			},
		}
		require.True(t, f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid))

		assert.Equal(t, proxy.Generation, proxy.Status.ObservedGeneration, "Contour must record the generation that it reconciled")

		stalled := proxy.Status.GetConditionFor(contour_v1.StalledConditionType)
		require.NotNil(t, stalled, "the Stalled condition must be present")
		assert.Equal(t, contour_v1.ConditionFalse, stalled.Status)
		assert.Equal(t, "Valid", stalled.Reason)
		assert.Equal(t, "No errors present", stalled.Message)

		assert.Equal(t, status.CurrentStatus, computeStatus(proxy).Status)
	})
}

func testKstatusFailedProxy(namespace string) {
	Specify("kstatus reports Failed for an invalid HTTPProxy", func() {
		t := f.T()

		proxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "kstatus-invalid",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "kstatus-invalid.projectcontour.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: "does-not-exist",
						Port: 80,
					}},
				}},
			},
		}
		require.True(t, f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyInvalid))

		valid := proxy.Status.GetConditionFor(contour_v1.ValidConditionType)
		require.NotNil(t, valid, "the Valid condition must be present")

		stalled := proxy.Status.GetConditionFor(contour_v1.StalledConditionType)
		require.NotNil(t, stalled, "the Stalled condition must be present")
		assert.Equal(t, contour_v1.ConditionTrue, stalled.Status)
		assert.Equal(t, valid.Reason, stalled.Reason)
		assert.Equal(t, valid.Message, stalled.Message)
		assert.Equal(t, "ErrorPresent", stalled.Reason)

		result := computeStatus(proxy)
		assert.Equal(t, status.FailedStatus, result.Status)
		assert.Equal(t, stalled.Message, result.Message)
	})
}

func testKstatusInProgressStatus(namespace string) {
	Specify("kstatus reports InProgress while the status is old", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		proxy := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "kstatus-stale",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "kstatus-stale.projectcontour.io",
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{
						Name: "echo",
						Port: 80,
					}},
				}},
			},
		}
		require.True(t, f.CreateHTTPProxyAndWaitFor(proxy, e2e.HTTPProxyValid))
		require.Equal(t, status.CurrentStatus, computeStatus(proxy).Status)

		// Fake unreconciled resource by stepping `metadata.generation` so that it is greater than `status.observedGeneration`.
		// Contour reconciles fast so this is the only way to get a stable stale status.
		stale := proxy.DeepCopy()
		stale.Generation = proxy.Status.ObservedGeneration + 1

		assert.Equal(t, status.InProgressStatus, computeStatus(stale).Status)
	})
}
