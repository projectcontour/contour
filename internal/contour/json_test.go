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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heptio/contour/internal/log/stdlog"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestAPIServer(t *testing.T) {
	tests := []struct {
		name      string
		services  []*v1.Service
		endpoints []*v1.Endpoints
		ingresses []*v1beta1.Ingress
		path      string
		want      string
	}{{
		name: "cds/empty cache",
		path: "/v1/clusters/cluster0/node0",
		want: `{"clusters":[]}` + "\n",
	}, {
		name: "cds/single service",
		services: []*v1.Service{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}},
			},
		}},
		path: "/v1/clusters/cluster0/node0",
		want: `{"clusters":[{"name":"default/simple/80","type":"sds","connect_timeout_ms":250,"lb_type":"round_robin","service_name":"default/simple/6502"}]}` + "\n",
	}, {
		name: "cds/single service, two tcp ports",
		services: []*v1.Service{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}, {
					Protocol:   "TCP",
					Port:       8000,
					TargetPort: intstr.FromString("90"),
				}},
			},
		}},
		path: "/v1/clusters/cluster0/node0",
		want: `{"clusters":[{"name":"default/simple/80","type":"sds","connect_timeout_ms":250,"lb_type":"round_robin","service_name":"default/simple/6502"},{"name":"default/simple/8000","type":"sds","connect_timeout_ms":250,"lb_type":"round_robin","service_name":"default/simple/90"}]}` + "\n",
	}, {
		name: "cds/single service, one udp port",
		services: []*v1.Service{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Protocol:   "UDP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}},
			},
		}},
		path: "/v1/clusters/cluster0/node0",
		want: `{"clusters":[]}` + "\n",
	}, {
		name: "sds/empty cache",
		path: "/v1/registration/default/simple/80",
		want: `{"hosts":[]}` + "\n",
	}, {
		name: "sds/empty registration",
		endpoints: []*v1.Endpoints{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Subsets: []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: "192.168.183.24",
				}},
				Ports: []v1.EndpointPort{{
					Port: 8080,
				}},
			}},
		}},
		path: "/v1/registration/default/otherservice/8080",
		want: `{"hosts":[]}` + "\n",
	}, {
		name: "sds/simple",
		endpoints: []*v1.Endpoints{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Subsets: []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: "192.168.183.24",
				}},
				Ports: []v1.EndpointPort{{
					Port: 8080,
				}},
			}},
		}},
		path: "/v1/registration/default/simple/8080",
		want: `{"hosts":[{"ip_address":"192.168.183.24","port":8080,"tags":{}}]}` + "\n",
	}, {
		name: "sds/multiple endpoint addresses",
		endpoints: []*v1.Endpoints{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin-org",
				Namespace: "default",
			},
			Subsets: []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: "23.23.247.89",
				}, {
					IP: "50.17.192.147",
				}, {
					IP: "50.17.206.192",
				}, {
					IP: "50.19.99.160",
				}},
				Ports: []v1.EndpointPort{{
					Port: 80,
				}},
			}},
		}},
		path: "/v1/registration/default/httpbin-org/80",
		want: `{"hosts":[{"ip_address":"23.23.247.89","port":80,"tags":{}},{"ip_address":"50.17.192.147","port":80,"tags":{}},{"ip_address":"50.17.206.192","port":80,"tags":{}},{"ip_address":"50.19.99.160","port":80,"tags":{}}]}` + "\n",
	}, {
		name: "sds/multiple endpoint ports",
		endpoints: []*v1.Endpoints{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "heptio-com",
				Namespace: "default",
			},
			Subsets: []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: "130.211.139.167",
				}},
				Ports: []v1.EndpointPort{{
					Port: 80,
				}, {
					Port: 443,
				}},
			}},
		}},
		path: "/v1/registration/default/heptio-com/443",
		want: `{"hosts":[{"ip_address":"130.211.139.167","port":443,"tags":{}}]}` + "\n",
	}, {
		name: "rds/no routes",
		path: "/v1/routes/ingress_http/cluster0/node0",
		want: `{"virtual_hosts":[]}` + "\n",
	}, {
		name: "rds/default backend",
		ingresses: []*v1beta1.Ingress{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httbin-org",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		}},
		path: "/v1/routes/ingress_http/cluster0/node0",
		want: `{"virtual_hosts":[{"name":"default/httbin-org","domains":["*"],"routes":[{"prefix":"/","cluster":"default/backend/80"}]}]}` + "\n",
	}, {
		name: "rds/long vhost name",
		ingresses: []*v1beta1.Ingress{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-service-name",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "my-very-very-long-service-host-name.my.domainname",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "my-service-name",
									ServicePort: intstr.FromInt(8080),
								},
							}},
						},
					},
				}},
			},
		}},
		path: "/v1/routes/ingress_http/cluster0/node0",
		want: `{"virtual_hosts":[{"name":"default/my-service-name/my-very-very--c4d2d4","domains":["my-very-very-long-service-host-name.my.domainname"],"routes":[{"prefix":"/","cluster":"default/my-service-name/8080"}]}]}` + "\n",
	}, {
		name: "rds/ingress class",
		ingresses: []*v1beta1.Ingress{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "incorrect",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "nginx",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "incorrect",
					ServicePort: intstr.FromInt(80),
				},
			},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "correct",
					ServicePort: intstr.FromInt(80),
				},
			},
		}},
		path: "/v1/routes/ingress_http/cluster0/node0",
		want: `{"virtual_hosts":[{"name":"default/correct","domains":["*"],"routes":[{"prefix":"/","cluster":"default/correct/80"}]}]}` + "\n",
	}, {
		name: "rds/vhost proxy",
		ingresses: []*v1beta1.Ingress{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin-org",
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
		}},
		path: "/v1/routes/ingress_http/cluster0/node0",
		want: `{"virtual_hosts":[{"name":"default/httpbin-org/httpbin.org","domains":["httpbin.org"],"routes":[{"prefix":"/","cluster":"default/httpbin-org/80"}]}]}` + "\n",
	}, {
		name: "rds/multiple routes",
		ingresses: []*v1beta1.Ingress{{
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
		}},
		path: "/v1/routes/ingress_http/cluster0/node0",
		want: `{"virtual_hosts":[{"name":"default/httpbin/httpbin.org","domains":["httpbin.org"],"routes":[{"prefix":"/peter","cluster":"default/peter/80"},{"prefix":"/paul","cluster":"default/paul/paul"}]}]}` + "\n",
	}, {
		name: "rds/multiple rules",
		ingresses: []*v1beta1.Ingress{{
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
		}},
		path: "/v1/routes/ingress_http/cluster0/node0",
		want: `{"virtual_hosts":[{"name":"default/httpbin/httpbin.org","domains":["httpbin.org"],"routes":[{"prefix":"/","cluster":"default/peter/80"}]},{"name":"default/httpbin/admin.httpbin.org","domains":["admin.httpbin.org"],"routes":[{"prefix":"/","cluster":"default/paul/paul"}]}]}` + "\n",
	}, {
		name: "lds/0.0.0.0:8080",
		path: "/v1/listeners/cluster0/node0",
		want: `{"listeners":[{"name":"ingress_http","address":"tcp://0.0.0.0:8080","filters":[{"type":"read","name":"http_connection_manager","config":{"codec_type":"http1","stat_prefix":"ingress_http","rds":{"cluster":"rds","route_config_name":"ingress_http","refresh_delay_ms":1000},"filters":[{"type":"decoder","name":"router","config":{}}],"access_log":[{"path":"/dev/stdout"}],"use_remote_address":true}}]}]}` + "\n",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ds DataSource
			for _, s := range tc.services {
				ds.AddService(s)
			}
			for _, e := range tc.endpoints {
				ds.AddEndpoints(e)
			}
			for _, i := range tc.ingresses {
				ds.AddIngress(i)
			}
			var w discardWriter
			api := NewJSONAPI(stdlog.New(w, w, 0), &ds)
			got := request(t, tc.path, api)
			if tc.want != got {
				t.Fatalf("%q: expected: %q, got %q", tc.path, tc.want, got)
			}
		})
	}
}

func TestHashname(t *testing.T) {
	tests := []struct {
		name string
		l    int
		s    []string
		want string
	}{
		{name: "empty s", l: 99, s: nil, want: ""},
		{name: "single element", l: 99, s: []string{"alpha"}, want: "alpha"},
		{name: "long single element, hashed", l: 12, s: []string{"gammagammagamma"}, want: "0d350ea5c204"},
		{name: "single element, truncated", l: 4, s: []string{"alpha"}, want: "8ed3"},
		{name: "two elements, truncated", l: 19, s: []string{"gammagamma", "betabeta"}, want: "ga-edf159/betabeta"},
		{name: "three elements", l: 99, s: []string{"alpha", "beta", "gamma"}, want: "alpha/beta/gamma"},
		{name: "issue/25", l: 60, s: []string{"default", "my-sevice-name", "my-very-very-long-service-host-name.my.domainname"}, want: "default/my-sevice-name/my-very-very--665863"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hashname(tc.l, append([]string{}, tc.s...)...)
			if got != tc.want {
				t.Fatalf("hashname(%d, %q): got %q, want %q", tc.l, tc.s, got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		l      int
		s      string
		suffix string
		want   string
	}{
		{name: "no truncate", l: 10, s: "quijibo", suffix: "a8c5e6", want: "quijibo"},
		{name: "limit", l: len("quijibo"), s: "quijibo", suffix: "a8c5e6", want: "quijibo"},
		{name: "truncate some", l: 6, s: "quijibo", suffix: "a8c5", want: "q-a8c5"},
		{name: "truncate suffix", l: 4, s: "quijibo", suffix: "a8c5", want: "a8c5"},
		{name: "truncate more", l: 3, s: "quijibo", suffix: "a8c5", want: "a8c"},
		{name: "long single element, truncated", l: 9, s: "gammagamma", suffix: "0d350e", want: "ga-0d350e"},
		{name: "long single element, truncated", l: 12, s: "gammagammagamma", suffix: "0d350e", want: "gamma-0d350e"},
		{name: "issue/25", l: 60 / 3, s: "my-very-very-long-service-host-name.my.domainname", suffix: "a8c5e6", want: "my-very-very--a8c5e6"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.l, tc.s, tc.suffix)
			if got != tc.want {
				t.Fatalf("hashname(%d, %q, %q): got %q, want %q", tc.l, tc.s, tc.suffix, got, tc.want)
			}
		})
	}
}

type discardWriter struct{}

func (w discardWriter) Write(buf []byte) (int, error) { return len(buf), nil }

func request(t *testing.T, path string, h http.Handler) string {
	r := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("%v: got %d, want: 200", path, w.Code)
	}
	return w.Body.String()
}
