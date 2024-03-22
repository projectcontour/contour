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
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func testRequiredFieldValidation(namespace string) {
	Specify("required fields are validated on creation", func() {
		t := f.T()

		// This HTTPProxy is expressed as an Unstructured because the JSON
		// tags for the relevant field do not include "omitempty", so when
		// a typed Go struct is serialized to JSON the field *is* included
		// and therefore does not fail validation.
		missingConditionHeaderName := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "projectcontour.io/v1",
				"kind":       "HTTPProxy",
				"metadata": map[string]any{
					"name":      "missing-condition-header-name",
					"namespace": namespace,
				},
				"spec": map[string]any{
					"routes": []map[string]any{
						{
							"conditions": []map[string]any{
								{
									"header": map[string]any{
										"present": true,
									},
								},
							},
							"services": []map[string]any{
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

		// Kubernetes 1.24 adds array indexes to the error message, so allow
		// either format for now.
		isExpectedErr := func(err error) bool {
			return strings.Contains(err.Error(), "spec.routes.conditions.header.name: Required value") ||
				strings.Contains(err.Error(), "spec.routes[0].conditions[0].header.name: Required value")
		}
		assert.True(t, isExpectedErr(err))

		// This HTTPProxy is expressed as an Unstructured because the JSON
		// tags for the relevant field do not include "omitempty", so when
		// a typed Go struct is serialized to JSON the field *is* included
		// and therefore does not fail validation.
		missingVirtualHostName := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "projectcontour.io/v1",
				"kind":       "HTTPProxy",
				"metadata": map[string]any{
					"name":      "missing-virtualhost-name",
					"namespace": namespace,
				},
				"spec": map[string]any{
					"virtualhost": map[string]any{
						"tls": map[string]any{
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
			Object: map[string]any{
				"apiVersion": "projectcontour.io/v1",
				"kind":       "HTTPProxy",
				"metadata": map[string]any{
					"name":      "missing-includes-name",
					"namespace": namespace,
				},
				"spec": map[string]any{
					"includes": []map[string]any{
						{
							"namespace": "foo",
						},
					},
				},
			},
		}

		err = f.Client.Create(context.TODO(), missingIncludesName)
		require.Error(t, err)

		// Kubernetes 1.24 adds array indexes to the error message, so allow
		// either format for now.
		isExpectedErr = func(err error) bool {
			return strings.Contains(err.Error(), "spec.includes.name: Required value") ||
				strings.Contains(err.Error(), "spec.includes[0].name: Required value")
		}
		assert.True(t, isExpectedErr(err))

		servicePortRange := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "service-port-range",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "ports.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
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

		// Kubernetes 1.24 adds array indexes to the error message, so allow
		// either format for now.
		isExpectedErr = func(err error) bool {
			return strings.Contains(err.Error(), "spec.routes.services.port: Invalid value") ||
				strings.Contains(err.Error(), "spec.routes[0].services[0].port: Invalid value")
		}
		assert.True(t, isExpectedErr(err))
	})
}
