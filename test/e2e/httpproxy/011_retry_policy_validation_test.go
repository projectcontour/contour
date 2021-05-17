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

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testRetryPolicyValidation(fx *e2e.Framework) {
	t := fx.T()
	ns := "011-retry-policy-validation"

	fx.CreateNamespace(ns)
	defer fx.DeleteNamespace(ns)

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "invalid-retry-on-condition",
		},
		Spec: contourv1.HTTPProxySpec{
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "foo",
							Port: 80,
						},
					},
					RetryPolicy: &contourv1.RetryPolicy{
						RetryOn: []contourv1.RetryOn{
							"foobar",
						},
					},
				},
			},
		},
	}
	err := fx.Client.Create(context.TODO(), p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.routes.retryPolicy.retryOn: Unsupported value")
}
