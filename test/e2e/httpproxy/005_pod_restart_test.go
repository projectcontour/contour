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

// +build e2e

package httpproxy

import (
	"context"
	"time"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testPodRestart(fx *e2e.Framework) {
	t := fx.T()
	namespace := "005-pod-restart"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Deploy(namespace, "echo")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "pod-restart",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "podrestart.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo",
							Port: 80,
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	res, ok := fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

	body := fx.GetEchoResponseBody(res.Body)
	assert.Equal(t, namespace, body.Namespace)
	assert.Equal(t, "echo", body.Service)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      body.Pod,
		},
	}
	require.NoError(t, fx.Client.Delete(context.TODO(), pod))

	// This step occasionally takes longer than the default 60s timeout
	// so give it 2 minutes to succeed.
	require.Eventually(t, func() bool {
		var res corev1.Pod
		err := fx.Client.Get(context.TODO(), client.ObjectKeyFromObject(pod), &res)

		// we want a non-nil, "not found" error to confirm the pod was deleted
		return err != nil && errors.IsNotFound(err)
	}, 2*time.Minute, fx.RetryInterval)

	// now make HTTP requests again and confirm we eventually get a 200
	res, ok = fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

	// should be a different pod than the original request
	require.NotEqual(t, pod.Name, fx.GetEchoResponseBody(res.Body).Pod)
}
