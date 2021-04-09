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
	"context"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIncludePrefixCondition(t *testing.T) {
	t.Parallel()

	// Start by assuming install-contour-working.sh has been run, so we
	// have Contour running in a cluster. Later we may want to move part
	// or all of that script into the E2E framework.

	var (
		fx             = NewFramework(t)
		baseNamespace  = "010-include-prefix-condition"
		appNamespace   = "010-include-prefix-condition-app"
		adminNamespace = "010-include-prefix-condition-admin"
	)

	for _, ns := range []string{baseNamespace, appNamespace, adminNamespace} {
		fx.CreateNamespace(ns)
		defer fx.DeleteNamespace(ns)
	}

	fx.CreateEchoWorkload(appNamespace, "echo-app")
	fx.CreateEchoWorkload(adminNamespace, "echo-admin")

	appProxy := &contourv1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPProxy",
			APIVersion: "projectcontour.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: appNamespace,
			Name:      "echo-app",
		},
		Spec: contourv1.HTTPProxySpec{
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo-app",
							Port: 80,
						},
					},
				},
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), appProxy))

	adminProxy := &contourv1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPProxy",
			APIVersion: "projectcontour.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: adminNamespace,
			Name:      "echo-admin",
		},
		Spec: contourv1.HTTPProxySpec{
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo-admin",
							Port: 80,
						},
					},
				},
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), adminProxy))

	baseProxy := &contourv1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPProxy",
			APIVersion: "projectcontour.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: baseNamespace,
			Name:      "echo",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "includeprefixcondition.projectcontour.io",
			},
			Includes: []contourv1.Include{
				{
					Name:      appProxy.Name,
					Namespace: appProxy.Namespace,
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/",
						},
					},
				},
				{
					Name:      adminProxy.Name,
					Namespace: adminProxy.Namespace,
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/admin",
						},
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(baseProxy, HTTPProxyValid)

	// TODO should check for appProxy/adminProxy valid too

	cases := map[string]string{
		"/":          "echo-app",
		"/app":       "echo-app",
		"/admin":     "echo-admin",
		"/admin/":    "echo-admin",
		"/admin/app": "echo-admin",
	}

	for path, expectedService := range cases {
		t.Logf("Querying %q, expecting service %q", path, expectedService)

		res, ok := fx.HTTPRequestUntil(IsOK, path, baseProxy.Spec.VirtualHost.Fqdn)
		if !assert.True(t, ok, "did not get 200 response") {
			continue
		}

		assert.Equal(t, expectedService, fx.GetEchoResponseBody(res.Body).Service)
	}
}
