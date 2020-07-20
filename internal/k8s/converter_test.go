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

package k8s

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	serviceapis "sigs.k8s.io/service-apis/api/v1alpha1"

	"github.com/projectcontour/contour/internal/assert"
)

func TestConvertUnstructured(t *testing.T) {
	type testcase struct {
		obj       interface{}
		want      interface{}
		wantError error
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			converter, err := NewUnstructuredConverter()
			if err != nil {
				t.Fatal(err)
			}
			got, err := converter.FromUnstructured(tc.obj)

			// Note we don't match error string values
			// because the actual values come from Kubernetes
			// internals and may not be stable.
			if tc.wantError == nil && err != nil {
				t.Errorf("wanted no error, got error %q", err)
			}

			if tc.wantError != nil && err == nil {
				t.Errorf("wanted error %q, got no error", tc.wantError)
			}

			assert.Equal(t, tc.want, got)
		})
	}

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTLSCert1 := &projcontour.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "example",
		},
		Spec: projcontour.TLSCertificateDelegationSpec{
			Delegations: []projcontour.CertificateDelegation{{
				SecretName: "sec1",
				TargetNamespaces: []string{
					"targetns",
				},
			}},
		},
	}

	proxyUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "projectcontour.io/v1",
			"kind":       "HTTPProxy",
			"metadata": map[string]interface{}{
				"name":      "example",
				"namespace": "roots",
			},
			"spec": map[string]interface{}{
				"virtualhost": map[string]interface{}{
					"fqdn": "example.com",
				},
				"routes": []map[string]interface{}{{
					"services": []map[string]interface{}{{
						"name": "home",
						"port": 8080,
					}},
					"conditions": []map[string]interface{}{{
						"prefix": "/foo",
					}},
				}},
			},
		},
	}

	proxyTLSCertUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "projectcontour.io/v1",
			"kind":       "TLSCertificateDelegation",
			"metadata": map[string]interface{}{
				"name":      "delegation",
				"namespace": "example",
			},
			"spec": map[string]interface{}{
				"delegations": []map[string]interface{}{{
					"secretName": "sec1",
					"targetNamespaces": []string{
						"targetns",
					},
				}},
			},
		},
	}

	unknownUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "invalid/-1",
			"kind":       "Broken",
			"metadata": map[string]interface{}{
				"name":      "invalid",
				"namespace": "example",
			},
			"spec": map[string]interface{}{
				"unknown": "field",
			},
		},
	}

	proxyInvalidUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "projectcontour.io/v1",
			"kind":       "HTTPProxy",
			"metadata": map[string]interface{}{
				"name":      "example",
				"namespace": "roots",
			},
			"spec": map[string]interface{}{
				"virtualhost": map[string]interface{}{
					"fqdn": "example.com",
				},
				"routes": []map[string]interface{}{{
					"services": []map[string]interface{}{{
						"name": 8080,
						"port": "bad",
					}},
					"conditions": []map[string]interface{}{{
						"prefix": "/foo",
					}},
				}},
			},
		},
	}

	gatewayClassUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.x.k8s.io/v1alpha1",
			"kind":       "GatewayClass",
			"metadata": map[string]interface{}{
				"name":      "gatewayclass",
				"namespace": "default",
			},
		},
	}

	gatewayclass1 := &serviceapis.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gatewayclass",
			Namespace: "default",
		},
	}

	gatewayUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.x.k8s.io/v1alpha1",
			"kind":       "Gateway",
			"metadata": map[string]interface{}{
				"name":      "gateway",
				"namespace": "default",
			},
		},
	}

	gateway1 := &serviceapis.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: "default",
		},
	}

	httpRouteUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.x.k8s.io/v1alpha1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      "httproute",
				"namespace": "default",
			},
		},
	}

	hpr1 := &serviceapis.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httproute",
			Namespace: "default",
		},
	}

	tcpRouteUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.x.k8s.io/v1alpha1",
			"kind":       "TcpRoute",
			"metadata": map[string]interface{}{
				"name":      "tcproute",
				"namespace": "default",
			},
		},
	}

	tr1 := &serviceapis.TcpRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcproute",
			Namespace: "default",
		},
	}

	run(t, "proxyunstructured", testcase{
		obj:       proxyUnstructured,
		want:      proxy1,
		wantError: nil,
	})

	run(t, "proxytlscertunstructured", testcase{
		obj:       proxyTLSCertUnstructured,
		want:      proxyTLSCert1,
		wantError: nil,
	})

	run(t, "unknownunstructured", testcase{
		obj:       unknownUnstructured,
		want:      nil,
		wantError: errors.New(`no kind "Broken" is registered for version "invalid/-1" in scheme "pkg/runtime/scheme.go:101"`),
	})

	run(t, "invalidunstructured", testcase{
		obj:       proxyInvalidUnstructured,
		want:      &projectcontour.HTTPProxy{},
		wantError: errors.New("unable to convert unstructured object to projectcontour.io/v1, Kind=HTTPProxy: cannot convert int to string"),
	})

	run(t, "notunstructured", testcase{
		obj:       proxy1,
		want:      proxy1,
		wantError: nil,
	})

	run(t, "gatewayclass", testcase{
		obj:       gatewayClassUnstructured,
		want:      gatewayclass1,
		wantError: nil,
	})

	run(t, "gateway", testcase{
		obj:       gatewayUnstructured,
		want:      gateway1,
		wantError: nil,
	})

	run(t, "httproute", testcase{
		obj:       httpRouteUnstructured,
		want:      hpr1,
		wantError: nil,
	})

	run(t, "tcproute", testcase{
		obj:       tcpRouteUnstructured,
		want:      tr1,
		wantError: nil,
	})

}

var _ cache.ResourceEventHandler = &DynamicClientHandler{}
