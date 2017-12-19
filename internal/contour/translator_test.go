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
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/heptio/contour/internal/log/stdlog"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTranslatorAddService(t *testing.T) {
	tests := []struct {
		name string
		svc  *v1.Service
		want []*v2.Cluster
	}{{
		name: "single service port",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []*v2.Cluster{
			cluster("default/simple/80", "default/simple/6502"),
		},
	}, {
		name: "long namespace and service name",
		svc: service(
			"beurocratic-company-test-domain-1",
			"tiny-cog-department-test-instance",
			v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			},
		),
		want: []*v2.Cluster{
			cluster(
				"beurocratic-company-test-domain-1/tiny-cog-depa-52e801/80",
				"beurocratic-company-test-domain-1/tiny-cog-department-test-instance/6502", // ServiceName is not subject to the 60 char limit
			),
		},
	}, {
		name: "single named service port",
		svc: service("default", "simple", v1.ServicePort{
			Name:       "http",
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []*v2.Cluster{
			cluster("default/simple/80", "default/simple/6502"),
			cluster("default/simple/http", "default/simple/6502"),
		},
	}, {
		name: "two service ports",
		svc: service("default", "simple", v1.ServicePort{
			Name:       "http",
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}, v1.ServicePort{
			Name:       "alt",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromString("9001"),
		}),
		want: []*v2.Cluster{
			cluster("default/simple/80", "default/simple/6502"),
			cluster("default/simple/8080", "default/simple/9001"),
			cluster("default/simple/alt", "default/simple/9001"),
			cluster("default/simple/http", "default/simple/6502"),
		},
	}, {
		name: "one tcp service, one udp service",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "UDP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}, v1.ServicePort{
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromString("9001"),
		}),
		want: []*v2.Cluster{
			cluster("default/simple/8080", "default/simple/9001"),
		},
	}, {
		name: "one udp service",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "UDP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []*v2.Cluster{},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := NewTranslator(stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS))
			tr.addService(tc.svc)
			got := tr.ClusterCache.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("addService(%v): got: %v, want: %v", tc.svc, got, tc.want)
			}
		})
	}
}

func TestTranslatorRemoveService(t *testing.T) {
	tests := map[string]struct {
		setup func(*Translator)
		svc   *v1.Service
		want  []*v2.Cluster
	}{
		"remove existing": {
			setup: func(tr *Translator) {
				tr.addService(service("default", "simple", v1.ServicePort{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}))
			},
			svc: service("default", "simple", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []*v2.Cluster{},
		},
		"remove named": {
			setup: func(tr *Translator) {
				tr.addService(service("default", "simple", v1.ServicePort{
					Name:       "kevin",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}))
			},
			svc: service("default", "simple", v1.ServicePort{
				Name:       "kevin",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []*v2.Cluster{},
		},
		"remove different": {
			setup: func(tr *Translator) {
				tr.addService(service("default", "simple", v1.ServicePort{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}))
			},
			svc: service("default", "different", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []*v2.Cluster{
				cluster("default/simple/80", "default/simple/6502"),
			},
		},
		"remove non existant": {
			setup: func(*Translator) {},
			svc: service("default", "simple", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []*v2.Cluster{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := NewTranslator(stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS))
			tc.setup(tr)
			tr.removeService(tc.svc)
			got := tr.ClusterCache.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("removeService(%v): got: %v, want: %v", tc.svc, got, tc.want)
			}
		})
	}
}

func TestTranslatorAddEndpoints(t *testing.T) {
	tests := []struct {
		name string
		ep   *v1.Endpoints
		want []*v2.ClusterLoadAssignment
	}{{
		name: "simple",
		ep: endpoints("default", "simple", v1.EndpointSubset{
			Addresses: addresses("192.168.183.24"),
			Ports:     ports(8080),
		}),
		want: []*v2.ClusterLoadAssignment{
			clusterloadassignment("default/simple/8080", lbendpoints(endpoint("192.168.183.24", 8080))),
		},
	}, {
		name: "multiple addresses",
		ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
			Addresses: addresses(
				"23.23.247.89",
				"50.17.192.147",
				"50.17.206.192",
				"50.19.99.160",
			),
			Ports: ports(80),
		}),
		want: []*v2.ClusterLoadAssignment{
			clusterloadassignment("default/httpbin-org/80", lbendpoints(
				endpoint("23.23.247.89", 80),
				endpoint("50.17.192.147", 80),
				endpoint("50.17.206.192", 80),
				endpoint("50.19.99.160", 80)),
			),
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := NewTranslator(stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS))
			tr.addEndpoints(tc.ep)
			got := tr.ClusterLoadAssignmentCache.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("addEndpoints(%v): got: %v, want: %v", tc.ep, got, tc.want)
			}
		})
	}
}

func TestTranslatorRemoveEndpoints(t *testing.T) {
	tests := map[string]struct {
		setup func(*Translator)
		ep    *v1.Endpoints
		want  []*v2.ClusterLoadAssignment
	}{
		"remove existing": {
			setup: func(tr *Translator) {
				tr.addEndpoints(endpoints("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
			},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []*v2.ClusterLoadAssignment{},
		},
		"remove different": {
			setup: func(tr *Translator) {
				tr.addEndpoints(endpoints("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
			},
			ep: endpoints("default", "different", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []*v2.ClusterLoadAssignment{
				clusterloadassignment("default/simple/8080", lbendpoints(endpoint("192.168.183.24", 8080))),
			},
		},
		"remove non existant": {
			setup: func(*Translator) {},
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []*v2.ClusterLoadAssignment{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := NewTranslator(stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS))
			tc.setup(tr)
			tr.removeEndpoints(tc.ep)
			got := tr.ClusterLoadAssignmentCache.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("removeEndpoints(%v): got: %v, want: %v", tc.ep, got, tc.want)
			}
		})
	}
}

func TestTranslatorAddIngress(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Translator)
		ing   *v1beta1.Ingress
		want  []*v2.VirtualHost
	}{{
		name: "default backend",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "*",
			Domains: []string{"*"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"),
				Action: clusteraction("default/backend/80"),
			}},
		}},
	}, {
		name: "incorrect ingress class",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "incorrect",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "nginx",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		want: []*v2.VirtualHost{}, // expected to be empty, the ingress class is ingnored
	}, {
		name: "explicit ingress class",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "*",
			Domains: []string{"*"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"), // match all
				Action: clusteraction("default/backend/80"),
			}},
		}},
	}, {
		name: "name based vhost",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host:             "httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("httpbin-org", intstr.FromInt(80))),
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "httpbin.org",
			Domains: []string{"httpbin.org"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"), // match all
				Action: clusteraction("default/httpbin-org/80"),
			}},
		}},
	}, {
		name: "regex vhost without match characters",
		ing: &v1beta1.Ingress{
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
								Path:    "/ip", // this field _is_ a regex
								Backend: *backend("httpbin-org", intstr.FromInt(80)),
							}},
						},
					},
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "httpbin.org",
			Domains: []string{"httpbin.org"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/ip"), // if the field does not contact any regex characters, we treat it as a prefix
				Action: clusteraction("default/httpbin-org/80"),
			}},
		}},
	}, {
		name: "regex vhost with match characters",
		ing: &v1beta1.Ingress{
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
								Path:    "/get.*", // this field _is_ a regex
								Backend: *backend("httpbin-org", intstr.FromInt(80)),
							}},
						},
					},
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "httpbin.org",
			Domains: []string{"httpbin.org"},
			Routes: []*v2.Route{{
				Match:  regexmatch("/get.*"),
				Action: clusteraction("default/httpbin-org/80"),
			}},
		}},
	}, {
		name: "named service port",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host:             "httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("httpbin-org", intstr.FromString("http"))),
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "httpbin.org",
			Domains: []string{"httpbin.org"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"),
				Action: clusteraction("default/httpbin-org/http"),
			}},
		}},
	}, {
		name: "multiple routes",
		ing: &v1beta1.Ingress{
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
								Path:    "/peter",
								Backend: *backend("peter", intstr.FromInt(80)),
							}, {
								Path:    "/paul",
								Backend: *backend("paul", intstr.FromString("paul")),
							}},
						},
					},
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "httpbin.org",
			Domains: []string{"httpbin.org"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/peter"),
				Action: clusteraction("default/peter/80"),
			}, {
				Match:  prefixmatch("/paul"),
				Action: clusteraction("default/paul/paul"),
			}},
		}},
	}, {
		name: "multiple rules",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host:             "httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
				}, {
					Host:             "admin.httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("paul", intstr.FromString("paul"))),
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "admin.httpbin.org",
			Domains: []string{"admin.httpbin.org"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"),
				Action: clusteraction("default/paul/paul"),
			}},
		}, {
			Name:    "httpbin.org",
			Domains: []string{"httpbin.org"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"),
				Action: clusteraction("default/peter/80"),
			}},
		}},
	}, {
		name: "vhost name exceeds 60 chars", // heptio/contour#25
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-service-name",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host:             "my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
					IngressRuleValue: ingressrulevalue(backend("my-service-name", intstr.FromInt(80))),
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "d31bb322ca62bb395acad00b3cbf45a3aa1010ca28dca7cddb4f7db786fa",
			Domains: []string{"my-very-very-long-service-host-name.subdomain.boring-dept.my.company"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"),
				Action: clusteraction("default/my-service-name/80"),
			}},
		}},
	}, {
		name: "second ingress object extends an existing vhost",
		setup: func(tr *Translator) {
			tr.OnAdd(&v1beta1.Ingress{
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
									Path:    "/",
									Backend: *backend("default", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			})
		},
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin-admin",
				Namespace: "kube-system",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path:    "/admin",
								Backend: *backend("admin", intstr.FromString("admin")),
							}},
						},
					},
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "httpbin.org",
			Domains: []string{"httpbin.org"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/admin"),
				Action: clusteraction("kube-system/admin/admin"),
			}, {
				Match:  prefixmatch("/"),
				Action: clusteraction("default/default/80"),
			}},
		}},
	}, {
		// kube-lego uses a single vhost in its own namespace to insert its
		// callback route for let's encrypt support.
		name: "kube-lego styleextend vhost definitions",
		setup: func(tr *Translator) {
			tr.OnAdd(&v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.davecheney.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/",
									Backend: *backend("httpbin", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			})
			tr.OnAdd(&v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin2",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin2.davecheney.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/",
									Backend: *backend("httpbin", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			})
		},
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-lego-nginx",
				Namespace: "kube-lego",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.davecheney.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path:    "/.well-known/acme-challenge",
								Backend: *backend("kube-lego-nginx", intstr.FromInt(8080)),
							}},
						},
					},
				}, {
					Host: "httpbin2.davecheney.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path:    "/.well-known/acme-challenge",
								Backend: *backend("kube-lego-nginx", intstr.FromInt(8080)),
							}},
						},
					},
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "httpbin.davecheney.com",
			Domains: []string{"httpbin.davecheney.com"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/.well-known/acme-challenge"),
				Action: clusteraction("kube-lego/kube-lego-nginx/8080"),
			}, {
				Match:  prefixmatch("/"),
				Action: clusteraction("default/httpbin/80"),
			}},
		}, {
			Name:    "httpbin2.davecheney.com",
			Domains: []string{"httpbin2.davecheney.com"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/.well-known/acme-challenge"),
				Action: clusteraction("kube-lego/kube-lego-nginx/8080"),
			}, {
				Match:  prefixmatch("/"),
				Action: clusteraction("default/httpbin/80"),
			}},
		}},
	}, {
		name: "IngressRuleValue without host should become the default vhost", // heptio/contour#101
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hello",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path: "/hello",
								Backend: v1beta1.IngressBackend{
									ServiceName: "hello",
									ServicePort: intstr.FromInt(80),
								},
							}},
						},
					},
				}},
			},
		},
		want: []*v2.VirtualHost{{
			Name:    "*",
			Domains: []string{"*"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/hello"),
				Action: clusteraction("default/hello/80"),
			}},
		}},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := NewTranslator(stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS))
			if tc.setup != nil {
				tc.setup(tr)
			}
			tr.addIngress(tc.ing)
			got := tr.VirtualHostCache.HTTP.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("addIngress(%v):\n (ingress_http) got: %v\nwant: %v", tc.ing, got, tc.want)
			}

			got = tr.VirtualHostCache.HTTPS.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("addIngress(%v):\n (ingress_https) got: %v\nwant: %v", tc.ing, got, tc.want)
			}
		})
	}
}

func TestTranslatorRemoveIngress(t *testing.T) {
	tests := map[string]struct {
		setup func(*Translator)
		ing   *v1beta1.Ingress
		want  []*v2.VirtualHost
	}{
		"remove existing": {
			setup: func(tr *Translator) {
				tr.addIngress(&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							Host:             "httpbin.org",
							IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
						}},
					},
				})
			},
			ing: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
					}},
				},
			},
			want: []*v2.VirtualHost{},
		},
		"remove different": {
			setup: func(tr *Translator) {
				tr.addIngress(&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							Host:             "httpbin.org",
							IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
						}},
					},
				})
			},
			ing: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "different",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "example.org",
						IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
					}},
				},
			},
			want: []*v2.VirtualHost{
				&v2.VirtualHost{
					Name:    "httpbin.org",
					Domains: []string{"httpbin.org"},
					Routes: []*v2.Route{{
						Match:  prefixmatch("/"),
						Action: clusteraction("default/peter/80"),
					}},
				},
			},
		},
		"remove non existant": {
			setup: func(*Translator) {},
			ing: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: backend("backend", intstr.FromInt(80)),
				},
			},
			want: []*v2.VirtualHost{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := NewTranslator(stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS))
			tc.setup(tr)
			tr.removeIngress(tc.ing)
			got := tr.VirtualHostCache.HTTPS.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("removeIngress(%v) (ingress_http): got: %v, want: %v", tc.ing, got, tc.want)
			}

			got = tr.VirtualHostCache.HTTPS.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("removeIngress(%v) (ingress_https): got: %v, want: %v", tc.ing, got, tc.want)
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

func service(ns, name string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

func cluster(name, servicename string) *v2.Cluster {
	return &v2.Cluster{
		Name: name,
		Type: v2.Cluster_EDS,
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig: &v2.ConfigSource{
				ConfigSourceSpecifier: &v2.ConfigSource_ApiConfigSource{
					ApiConfigSource: &v2.ApiConfigSource{
						ApiType:     v2.ApiConfigSource_GRPC,
						ClusterName: []string{"xds_cluster"},
					},
				},
			},
			ServiceName: servicename,
		},
		ConnectTimeout: 250 * time.Millisecond,
		LbPolicy:       v2.Cluster_ROUND_ROBIN,
	}
}

func endpoints(ns, name string, subsets ...v1.EndpointSubset) *v1.Endpoints {
	return &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Subsets: subsets,
	}
}

func addresses(ips ...string) []v1.EndpointAddress {
	var addrs []v1.EndpointAddress
	for _, ip := range ips {
		addrs = append(addrs, v1.EndpointAddress{IP: ip})
	}
	return addrs
}

func ports(ps ...int32) []v1.EndpointPort {
	var ports []v1.EndpointPort
	for _, p := range ps {
		ports = append(ports, v1.EndpointPort{Port: p})
	}
	return ports
}

func clusterloadassignment(name string, lbendpoints []*v2.LbEndpoint) *v2.ClusterLoadAssignment {
	return &v2.ClusterLoadAssignment{
		ClusterName: name,
		Endpoints: []*v2.LocalityLbEndpoints{{
			Locality: &v2.Locality{
				Region:  "ap-southeast-2", // totally a guess
				Zone:    "2b",
				SubZone: "banana", // yeah, need to think of better values here
			},
			LbEndpoints: lbendpoints,
		}},
		Policy: &v2.ClusterLoadAssignment_Policy{
			DropOverload: 0.0,
		},
	}
}

func endpoint(addr string, port uint32) *v2.Endpoint {
	return &v2.Endpoint{
		Address: &v2.Address{
			Address: &v2.Address_SocketAddress{
				SocketAddress: &v2.SocketAddress{
					Protocol: v2.SocketAddress_TCP,
					Address:  addr,
					PortSpecifier: &v2.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
	}
}

func lbendpoints(eps ...*v2.Endpoint) []*v2.LbEndpoint {
	var lbep []*v2.LbEndpoint
	for _, ep := range eps {
		lbep = append(lbep, &v2.LbEndpoint{
			Endpoint: ep,
		})
	}
	return lbep
}

func backend(name string, port intstr.IntOrString) *v1beta1.IngressBackend {
	return &v1beta1.IngressBackend{
		ServiceName: name,
		ServicePort: port,
	}
}

func ingressrulevalue(backend *v1beta1.IngressBackend) v1beta1.IngressRuleValue {
	return v1beta1.IngressRuleValue{
		HTTP: &v1beta1.HTTPIngressRuleValue{
			Paths: []v1beta1.HTTPIngressPath{{
				Backend: *backend,
			}},
		},
	}
}
