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

package contour

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "k8s.io/api/core/v1"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/projectcontour/contour/internal/assert"
)

func TestConvertUnstructured(t *testing.T) {
	type testcase struct {
		obj  interface{}
		want interface{}
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			eh := EventHandler{}
			got := eh.ConvertUnstructured(tc.obj)
			if tc.want != got {
				assert.Equal(t, tc.want, got)
			}
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

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "foo",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     12345678,
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

	run(t, "ingressroute", testcase{
		obj:  ir1,
		want: ir1,
	})

	run(t, "proxy", testcase{
		obj:  proxy1,
		want: proxy1,
	})

	run(t, "service", testcase{
		obj:  s1,
		want: s1,
	})

	run(t, "irtlscertificatedelegation", testcase{
		obj:  irTLSCert1,
		want: irTLSCert1,
	})

	run(t, "proxytlscertificatedelegation", testcase{
		obj:  proxyTLSCert1,
		want: proxyTLSCert1,
	})

	run(t, "secret", testcase{
		obj:  secret("default/secret-a/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
		want: secret("default/secret-a/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
	})

	run(t, "proxyunstructured", testcase{
		obj:  proxyUnstructured,
		want: proxy1,
	})

	run(t, "irunstructured", testcase{
		obj:  irUnstructured,
		want: ir1,
	})

	run(t, "irtlscertunstructured", testcase{
		obj:  irTLSCertUnstructured,
		want: irTLSCert1,
	})

	run(t, "proxytlscertunstructured", testcase{
		obj:  proxyTLSCertUnstructured,
		want: proxyTLSCert1,
	})
}
