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

package e2e

import (
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPathConditionMatch(t *testing.T) {
	t.Parallel()

	// Start by assuming install-contour-working.sh has been run, so we
	// have Contour running in a cluster. Later we may want to move part
	// or all of that script into the E2E framework.

	var (
		fx        = NewFramework(t)
		namespace = "003-path-condition-match"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo-slash-prefix")
	fx.CreateEchoWorkload(namespace, "echo-slash-noprefix")
	fx.CreateEchoWorkload(namespace, "echo-slash-default")

	p := &contourv1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPProxy",
			APIVersion: "projectcontour.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "path-conditions",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "pathconditions.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo-slash-prefix",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/path/prefix/",
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo-slash-noprefix",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/path/prefix",
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo-slash-default",
							Port: 80,
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(p, HTTPProxyValid)

	cases := map[string]string{
		"/":                "echo-slash-default",
		"/foo":             "echo-slash-default",
		"/path/prefix":     "echo-slash-noprefix",
		"/path/prefixfoo":  "echo-slash-noprefix",
		"/path/prefix/":    "echo-slash-prefix",
		"/path/prefix/foo": "echo-slash-prefix",
	}

	for path, expectedService := range cases {
		t.Logf("Querying %q, expecting service %q", path, expectedService)

		res, ok := fx.HTTPRequestUntil(IsOK, path, p.Spec.VirtualHost.Fqdn)
		if !assert.True(t, ok, "did not get 200 response") {
			continue
		}

		body := fx.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, expectedService, body.Service)
	}
}
