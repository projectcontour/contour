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

package gateway

import (
	"context"
	"testing"

	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestGatewayPathConditionMatch(t *testing.T) {
	t.Parallel()

	// Start by assuming install-contour-working.sh has been run, so we
	// have Contour running in a cluster. Later we may want to move part
	// or all of that script into the E2E framework.

	var (
		fx        = e2e.NewFramework(t)
		namespace = "gateway-001-path-condition-match"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo-slash-prefix")
	fx.CreateEchoWorkload(namespace, "echo-slash-noprefix")
	fx.CreateEchoWorkload(namespace, "echo-slash-default")
	fx.CreateEchoWorkload(namespace, "echo-slash-exact")

	// GatewayClass
	// TODO since this is cluster-scoped it doesn't get cleaned up after the test run
	gatewayClass := &gatewayv1alpha1.GatewayClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GatewayClass",
			APIVersion: gatewayv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "contour-class",
		},
		Spec: gatewayv1alpha1.GatewayClassSpec{
			Controller: "projectcontour.io/ingress-controller",
		},
	}

	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gatewayClass)
	require.NoError(t, err)

	client := fx.Clients.DynamicClient().Resource(schema.GroupVersionResource{Group: gatewayv1alpha1.GroupVersion.Group, Version: gatewayv1alpha1.GroupVersion.Version, Resource: "gatewayclasses"})
	_, err = client.Create(context.TODO(), &unstructured.Unstructured{Object: u}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Gateway
	gateway := &gatewayv1alpha1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: gatewayv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "projectcontour", // TODO needs to be this to match default settings, but need to clean it up!
			Name:      "contour",
		},
		Spec: gatewayv1alpha1.GatewaySpec{
			GatewayClassName: "contour-class",
			Listeners: []gatewayv1alpha1.Listener{
				{
					Protocol: gatewayv1alpha1.HTTPProtocolType,
					Port:     gatewayv1alpha1.PortNumber(80),
					Routes: gatewayv1alpha1.RouteBindingSelector{
						Kind: "HTTPRoute",
						Namespaces: gatewayv1alpha1.RouteNamespaces{
							From: gatewayv1alpha1.RouteSelectAll,
						},
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "filter"},
						},
					},
				},
			},
		},
	}

	u, err = runtime.DefaultUnstructuredConverter.ToUnstructured(gateway)
	require.NoError(t, err)

	client = fx.Clients.DynamicClient().Resource(schema.GroupVersionResource{Group: gatewayv1alpha1.GroupVersion.Group, Version: gatewayv1alpha1.GroupVersion.Version, Resource: "gateways"})
	_, err = client.Namespace(gateway.Namespace).Create(context.TODO(), &unstructured.Unstructured{Object: u}, metav1.CreateOptions{})
	require.NoError(t, err)

	// HTTPRoute
	route := &gatewayv1alpha1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: gatewayv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "http-filter-1",
			Labels:    map[string]string{"app": "filter"},
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Hostnames: []gatewayv1alpha1.Hostname{"gatewaypathconditions.projectcontour.io"},
			Rules: []gatewayv1alpha1.HTTPRouteRule{
				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/path/prefix/",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-prefix"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/path/prefix",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-noprefix"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchExact,
								Value: "/path/exact",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-exact"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/",
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-default"),
							Port:        portNumPtr(80),
						},
					},
				},
			},
		},
	}

	u, err = runtime.DefaultUnstructuredConverter.ToUnstructured(route)
	require.NoError(t, err)

	client = fx.Clients.DynamicClient().Resource(schema.GroupVersionResource{Group: gatewayv1alpha1.GroupVersion.Group, Version: gatewayv1alpha1.GroupVersion.Version, Resource: "httproutes"})
	_, err = client.Namespace(route.Namespace).Create(context.TODO(), &unstructured.Unstructured{Object: u}, metav1.CreateOptions{})
	require.NoError(t, err)

	// TODO should wait until HTTPRoute has a status of valid

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

		res, ok := fx.HTTPRequestUntil(e2e.IsOK, path, string(route.Spec.Hostnames[0]))
		if !assert.True(t, ok, "did not get 200 response") {
			continue
		}

		body := fx.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, expectedService, body.Service)
	}
}

func stringPtr(s string) *string {
	return &s
}

func portNumPtr(port int) *gatewayv1alpha1.PortNumber {
	pn := gatewayv1alpha1.PortNumber(port)
	return &pn
}
