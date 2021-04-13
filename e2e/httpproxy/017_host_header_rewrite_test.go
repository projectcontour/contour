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
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testHostHeaderRewrite(t *testing.T, fx *e2e.Framework) {
	namespace := "017-host-header-rewrite"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "ingress-conformance-echo")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "host-header-rewrite",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "hostheaderrewrite.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "ingress-conformance-echo",
							Port: 80,
						},
					},
					RequestHeadersPolicy: &contourv1.HeadersPolicy{
						Set: []contourv1.HeaderValue{
							{
								Name:  "Host",
								Value: "rewritten.com",
							},
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(p, HTTPProxyValid)

	res, ok := fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.True(t, ok, "did not get 200 response")

	assert.Equal(t, "rewritten.com", fx.GetEchoResponseBody(res.Body).Host)
}
