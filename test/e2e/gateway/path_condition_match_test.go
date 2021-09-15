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

package gateway

import (
	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func gatewayParentRef(namespace, name string) gatewayapi_v1alpha2.ParentRef {
	parentRef := gatewayapi_v1alpha2.ParentRef{
		Group: groupPtr(gatewayapi_v1alpha2.GroupName),
		Kind:  kindPtr("Gateway"),
		Name:  name,
	}

	if namespace != "" {
		parentRef.Namespace = namespacePtr(namespace)
	}

	return parentRef
}

func groupPtr(group string) *gatewayapi_v1alpha2.Group {
	gwGroup := gatewayapi_v1alpha2.Group(group)
	return &gwGroup
}

func kindPtr(kind string) *gatewayapi_v1alpha2.Kind {
	gwKind := gatewayapi_v1alpha2.Kind(kind)
	return &gwKind
}

func namespacePtr(namespace string) *gatewayapi_v1alpha2.Namespace {
	gwNamespace := gatewayapi_v1alpha2.Namespace(namespace)
	return &gwNamespace
}

func serviceBackendObjectRef(name string, port int) gatewayapi_v1alpha2.BackendObjectReference {
	return gatewayapi_v1alpha2.BackendObjectReference{
		Kind: kindPtr("Service"),
		Name: name,
		Port: portNumPtr(port),
	}
}

func testGatewayPathConditionMatch(namespace string) {
	Specify("path match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-slash-prefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-noprefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-default")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-exact")

		route := &gatewayapi_v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-filter-1",
			},
			Spec: gatewayapi_v1alpha2.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1alpha2.Hostname{"gatewaypathconditions.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentRef{
						gatewayParentRef("", "http"), // TODO need a better way to inform the test case of the Gateway it should use
					},
				},
				Rules: []gatewayapi_v1alpha2.HTTPRouteRule{
					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPrefix),
									Value: stringPtr("/path/prefix/"),
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: serviceBackendObjectRef("echo-slash-prefix", 80),
								},
							},
						},
					},

					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPrefix),
									Value: stringPtr("/path/prefix"),
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: serviceBackendObjectRef("echo-slash-noprefix", 80),
								},
							},
						},
					},

					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchExact),
									Value: stringPtr("/path/exact"),
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: serviceBackendObjectRef("echo-slash-exact", 80),
								},
							},
						},
					},

					{
						Matches: []gatewayapi_v1alpha2.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1alpha2.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha2.PathMatchPrefix),
									Value: stringPtr("/"),
								},
							},
						},
						BackendRefs: []gatewayapi_v1alpha2.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1alpha2.BackendRef{
									BackendObjectReference: serviceBackendObjectRef("echo-slash-default", 80),
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

		cases := map[string]string{
			"/":                "echo-slash-default",
			"/foo":             "echo-slash-default",
			"/path/prefix":     "echo-slash-noprefix",
			"/path/prefixfoo":  "echo-slash-noprefix",
			"/path/prefix/":    "echo-slash-prefix",
			"/path/prefix/foo": "echo-slash-prefix",
			"/path/exact":      "echo-slash-exact",
			"/path/exactfoo":   "echo-slash-default",
			"/path/exact/":     "echo-slash-default",
			"/path/exact/foo":  "echo-slash-default",
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      string(route.Spec.Hostnames[0]),
				Path:      path,
				Condition: e2e.HasStatusCode(200),
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, expectedService, body.Service)
		}
	})
}
