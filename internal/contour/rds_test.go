// Copyright © 2017 Heptio
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
	"reflect"
	"testing"

	"github.com/heptio/contour/internal/envoy"
	"github.com/pkg/errors"

	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestIngressToVirtualHost(t *testing.T) {
	tests := []struct {
		name string
		i    *v1beta1.Ingress
		want []*envoy.VirtualHost
	}{{
		name: "default backend",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		want: []*envoy.VirtualHost{{
			Name:    "default/simple",
			Domains: domains("*"),
			Routes: routes(envoy.Route{
				Prefix:  "/",
				Cluster: "default/backend/80",
			}),
		}},
	}, {
		name: "incorrect ingress class",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "incorrect",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "nginx",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		want: nil,
	}, {
		name: "explicit ingress class",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		want: []*envoy.VirtualHost{{
			Name:    "default/correct",
			Domains: domains("*"),
			Routes: routes(envoy.Route{
				Prefix:  "/",
				Cluster: "default/backend/80",
			}),
		}},
	}, {
		name: "name based vhost",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "httpbin-org",
									ServicePort: intstr.FromInt(80),
								},
							}},
						},
					},
				}},
			},
		},
		want: []*envoy.VirtualHost{{
			Name:    "default/httpbin/httpbin.org",
			Domains: domains("httpbin.org"),
			Routes: routes(envoy.Route{
				Prefix:  "/",
				Cluster: "default/httpbin-org/80",
			}),
		}},
	}, {
		name: "regex vhost",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path: "/ip", // this field _is_ a regex
								Backend: v1beta1.IngressBackend{
									ServiceName: "httpbin-org",
									ServicePort: intstr.FromInt(80),
								},
							}},
						},
					},
				}},
			},
		},
		want: []*envoy.VirtualHost{{
			Name:    "default/httpbin/httpbin.org",
			Domains: domains("httpbin.org"),
			Routes: routes(envoy.Route{
				Prefix:  "/ip",
				Cluster: "default/httpbin-org/80",
			}),
		}},
	}, {
		name: "named service port",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "httpbin-org",
									ServicePort: intstr.FromString("http"),
								},
							}},
						},
					},
				}},
			},
		},
		want: []*envoy.VirtualHost{{
			Name:    "default/httpbin/httpbin.org",
			Domains: domains("httpbin.org"),
			Routes: routes(envoy.Route{
				Prefix:  "/",
				Cluster: "default/httpbin-org/http",
			}),
		}},
	}, {
		name: "multiple routes",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path: "/peter",
								Backend: v1beta1.IngressBackend{
									ServiceName: "peter",
									ServicePort: intstr.FromInt(80),
								},
							}, {
								Path: "/paul",
								Backend: v1beta1.IngressBackend{
									ServiceName: "paul",
									ServicePort: intstr.FromString("paul"),
								},
							}},
						},
					},
				}},
			},
		},
		want: []*envoy.VirtualHost{{
			Name:    "default/httpbin/httpbin.org",
			Domains: domains("httpbin.org"),
			Routes: routes(envoy.Route{
				Prefix:  "/peter",
				Cluster: "default/peter/80",
			}, envoy.Route{
				Prefix:  "/paul",
				Cluster: "default/paul/paul",
			}),
		}},
	}, {
		name: "multiple rules",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "peter",
									ServicePort: intstr.FromInt(80),
								},
							}},
						},
					},
				}, {
					Host: "admin.httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "paul",
									ServicePort: intstr.FromString("paul"),
								},
							}},
						},
					},
				}},
			},
		},
		want: []*envoy.VirtualHost{{
			Name:    "default/httpbin/httpbin.org",
			Domains: domains("httpbin.org"),
			Routes: routes(envoy.Route{
				Prefix:  "/",
				Cluster: "default/peter/80",
			}),
		}, {
			Name:    "default/httpbin/admin.httpbin.org",
			Domains: domains("admin.httpbin.org"),
			Routes: routes(envoy.Route{
				Prefix:  "/",
				Cluster: "default/paul/paul",
			}),
		}},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IngressToVirtualHosts(tc.i)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got: %#v, want: %#v", got, tc.want)
			}
		})
	}
}

func TestIngressBackendToVirtualHostName(t *testing.T) {
	tests := []struct {
		name string
		i    v1beta1.Ingress
		b    v1beta1.IngressBackend
		want string
	}{{
		name: "integer port",
		i: v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
		},
		b: v1beta1.IngressBackend{
			ServiceName: "httpbin-svc",
			ServicePort: intstr.FromInt(80),
		},
		want: "default/httpbin-svc/80",
	}, {
		name: "named port",
		i: v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
		},
		b: v1beta1.IngressBackend{
			ServiceName: "httpbin-svc",
			ServicePort: intstr.FromString("gunicorn"),
		},
		want: "default/httpbin-svc/gunicorn",
	}, {
		name: `port "80"`,
		i: v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
		},
		b: v1beta1.IngressBackend{
			ServiceName: "httpbin-svc",
			ServicePort: intstr.FromString("80"),
		},
		want: "default/httpbin-svc/80",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ingressBackendToClusterName(&tc.i, &tc.b)
			if got != tc.want {
				t.Fatalf("ingressBackendToCluster(%v, %v, %v): got: %q, want: %q", tc.i.Namespace, tc.b.ServiceName, tc.b.ServicePort.String(), got, tc.want)
			}
		})
	}
}

func TestValidateIngress(t *testing.T) {
	tests := []struct {
		name string
		i    *v1beta1.Ingress
		want error
	}{{
		name: "missing Ingress.Meta.Name",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "",
			},
		},
		want: errors.New("Ingress.Meta.Name is blank"),
	}, {
		name: "missing Ingress.Meta.Namespace",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "simple",
			},
		},
		want: errors.New("Ingress.Meta.Namespace is blank"),
	}, {
		name: "missing Ingress.Spec.Backend and Ingress.Spec.Rules",
		i: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
		},
		want: errors.New("Ingress.Spec.Backend and Ingress.Spec.Rules is blank"),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateIngress(tc.i)
			if tc.want != nil && got == nil || got.Error() != tc.want.Error() {
				t.Errorf("got: %v, expected: %v", tc.want, got)
			}
		})
	}
}

func domains(ds ...string) []string          { return ds }
func routes(rs ...envoy.Route) []envoy.Route { return rs }
