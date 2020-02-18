// Copyright © 2020 VMware
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

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

			converter := NewUnstructuredConverter()
			got, err := converter.Convert(tc.obj)

			assert.Equal(t, tc.wantError, err)
			assert.Equal(t, tc.want, got)
		})
	}

	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}, {
				Match: "/prefix",
				Delegate: &ingressroutev1.Delegate{
					Name: "delegated",
				}},
			},
		},
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
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	irTLSCert1 := &ingressroutev1.TLSCertificateDelegation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delegation",
			Namespace: "example",
		},
		Spec: ingressroutev1.TLSCertificateDelegationSpec{
			Delegations: []ingressroutev1.CertificateDelegation{{
				SecretName: "sec1",
				TargetNamespaces: []string{
					"targetns",
				},
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

	irUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "contour.heptio.com/v1beta1",
			"kind":       "IngressRoute",
			"metadata": map[string]interface{}{
				"name":      "example",
				"namespace": "roots",
			},
			"spec": map[string]interface{}{
				"virtualhost": map[string]interface{}{
					"fqdn": "example.com",
				},
				"routes": []map[string]interface{}{{
					"match": "/foo",
					"services": []map[string]interface{}{{
						"name": "home",
						"port": 8080,
					}},
				}, {
					"match": "/prefix",
					"delegate": map[string]interface{}{
						"name": "delegated",
					},
				}},
			},
		},
	}

	irTLSCertUnstructured := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "contour.heptio.com/v1beta1",
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

	run(t, "proxyunstructured", testcase{
		obj:       proxyUnstructured,
		want:      proxy1,
		wantError: nil,
	})

	run(t, "irunstructured", testcase{
		obj:       irUnstructured,
		want:      ir1,
		wantError: nil,
	})

	run(t, "irtlscertunstructured", testcase{
		obj:       irTLSCertUnstructured,
		want:      irTLSCert1,
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
		wantError: errors.New("unsupported object type: *unstructured.Unstructured"),
	})

	run(t, "invalidunstructured", testcase{
		obj:       proxyInvalidUnstructured,
		want:      &projectcontour.HTTPProxy{},
		wantError: errors.New("unable to convert unstructured object to projectcontour.io/v1, Kind=HTTPProxy: cannot convert int to string"),
	})

	run(t, "notunstructured", testcase{
		obj:       proxy1,
		want:      nil,
		wantError: errors.New("unable to convert unstructured object to projectcontour.io/v1, Kind=HTTPProxy: cannot convert int to string"),
	})
}

var _ cache.ResourceEventHandler = &DynamicClientHandler{}
