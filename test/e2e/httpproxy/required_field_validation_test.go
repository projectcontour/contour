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
// +build e2e

package httpproxy

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testRequiredFieldValidation(namespace string) {
	Specify("required fields are validated on creation", func() {
		t := f.T()

		// This HTTPProxy is expressed as an Unstructured because the JSON
		// tags for the relevant field do not include "omitempty", so when
		// a typed Go struct is serialized to JSON the field *is* included
		// and therefore does not fail validation.
		missingConditionHeaderName := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "projectcontour.io/v1",
				"kind":       "HTTPProxy",
				"metadata": map[string]interface{}{
					"name":      "missing-condition-header-name",
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"routes": []map[string]interface{}{
						{
							"conditions": []map[string]interface{}{
								{
									"header": map[string]interface{}{
										"present": true,
									},
								},
							},
							"services": []map[string]interface{}{
								{
									"name": "foo",
									"port": 80,
								},
							},
						},
					},
				},
			},
		}

		err := f.Client.Create(context.TODO(), missingConditionHeaderName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.routes.conditions.header.name: Required value")

		// This HTTPProxy is expressed as an Unstructured because the JSON
		// tags for the relevant field do not include "omitempty", so when
		// a typed Go struct is serialized to JSON the field *is* included
		// and therefore does not fail validation.
		missingVirtualHostName := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "projectcontour.io/v1",
				"kind":       "HTTPProxy",
				"metadata": map[string]interface{}{
					"name":      "missing-virtualhost-name",
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"virtualhost": map[string]interface{}{
						"tls": map[string]interface{}{
							"passthrough": true,
						},
					},
				},
			},
		}

		err = f.Client.Create(context.TODO(), missingVirtualHostName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.virtualhost.fqdn: Required value")

		// This HTTPProxy is expressed as an Unstructured because the JSON
		// tags for the relevant field do not include "omitempty", so when
		// a typed Go struct is serialized to JSON the field *is* included
		// and therefore does not fail validation.
		missingIncludesName := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "projectcontour.io/v1",
				"kind":       "HTTPProxy",
				"metadata": map[string]interface{}{
					"name":      "missing-includes-name",
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"includes": []map[string]interface{}{
						{
							"namespace": "foo",
						},
					},
				},
			},
		}

		err = f.Client.Create(context.TODO(), missingIncludesName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.includes.name: Required value")

		servicePortRange := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "service-port-range",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "ports.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "any-service-name",
								Port: 80000,
							},
						},
					},
				},
			},
		}
		err = f.Client.Create(context.TODO(), servicePortRange)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.routes.services.port: Invalid value")
	})
}
