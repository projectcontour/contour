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
	"sort"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTranslatorAddService(t *testing.T) {
	tests := []struct {
		name string
		svc  *v1.Service
		want []proto.Message
	}{{
		name: "single service port",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []proto.Message{
			cluster("default/simple/80", "default/simple"),
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
		want: []proto.Message{
			cluster(
				"beurocratic-company-test-domain-1/tiny-cog-depa-52e801/80",
				"beurocratic-company-test-domain-1/tiny-cog-department-test-instance", // ServiceName is not subject to the 60 char limit
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
		want: []proto.Message{
			cluster("default/simple/80", "default/simple/http"),
			cluster("default/simple/http", "default/simple/http"),
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
		want: []proto.Message{
			cluster("default/simple/80", "default/simple/http"),
			cluster("default/simple/8080", "default/simple/alt"),
			cluster("default/simple/alt", "default/simple/alt"),
			cluster("default/simple/http", "default/simple/http"),
		},
	}, {
		// TODO(dfc) I think this is impossible, the apiserver would require
		// these ports to be named.
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
		want: []proto.Message{
			cluster("default/simple/8080", "default/simple"),
		},
	}, {
		name: "one udp service",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "UDP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []proto.Message{},
	}}

	log := testLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tr.OnAdd(tc.svc)
			got := contents(&tr.ClusterCache)
			sort.Stable(clusterByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
		})
	}
}

func TestTranslatorUpdateService(t *testing.T) {
	tests := map[string]struct {
		oldObj *v1.Service
		newObj *v1.Service
		want   []proto.Message
	}{
		"remove named service port": {
			oldObj: service("default", "kuard",
				v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
				v1.ServicePort{
					Name:       "https",
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			),
			newObj: service("default", "kuard",
				v1.ServicePort{
					Name:       "https",
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			),
			want: []proto.Message{
				&v2.Cluster{
					Name: "default/kuard/443",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard/https",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				}, &v2.Cluster{
					Name: "default/kuard/https",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard/https",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				},
			},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tr.OnAdd(tc.oldObj)
			tr.OnUpdate(tc.oldObj, tc.newObj)
			got := contents(&tr.ClusterCache)
			sort.Stable(clusterByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
		})
	}
}

func TestTranslatorRemoveService(t *testing.T) {
	tests := map[string]struct {
		setup func(*Translator)
		svc   *v1.Service
		want  []proto.Message
	}{
		"remove existing": {
			setup: func(tr *Translator) {
				tr.OnAdd(service("default", "simple", v1.ServicePort{
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
			want: []proto.Message{},
		},
		"remove named": {
			setup: func(tr *Translator) {
				tr.OnAdd(service("default", "simple", v1.ServicePort{
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
			want: []proto.Message{},
		},
		"remove different": {
			setup: func(tr *Translator) {
				tr.OnAdd(service("default", "simple", v1.ServicePort{
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
			want: []proto.Message{
				cluster("default/simple/80", "default/simple"),
			},
		},
		"remove non existant": {
			setup: func(*Translator) {},
			svc: service("default", "simple", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []proto.Message{},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tc.setup(tr)
			tr.OnDelete(tc.svc)
			got := contents(&tr.ClusterCache)
			sort.Stable(clusterByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
		})
	}
}

func TestTranslatorAddIngress(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*Translator)
		ing           *v1beta1.Ingress
		ingress_http  []proto.Message
		ingress_https []proto.Message
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/backend/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
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
		ingress_http:  []proto.Message{}, // expected to be empty, the ingress class is ingnored
		ingress_https: []proto.Message{}, // expected to be empty, the ingress class is ingnored
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/backend/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		name: "explicit custom ingress class",
		setup: func(tr *Translator) {
			tr.IngressClass = "testingress"
		},
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "testingress",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/backend/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		name: "explicit incorrect custom ingress class",
		setup: func(tr *Translator) {
			tr.IngressClass = "badingress"
		},
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "goodingress",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		ingress_http:  []proto.Message{}, // expected to be empty, the ingress class is ingnored
		ingress_https: []proto.Message{},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org", "httpbin.org:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org", "httpbin.org:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/ip"), // if the field does not contact any regex characters, we treat it as a prefix
					Action: clusteraction("default/httpbin-org/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org", "httpbin.org:80"},
				Routes: []route.Route{{
					Match:  regexmatch("/get.*"),
					Action: clusteraction("default/httpbin-org/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org", "httpbin.org:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/httpbin-org/http"),
				}},
			},
		},
		ingress_https: []proto.Message{},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org", "httpbin.org:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/peter"),
					Action: clusteraction("default/peter/80"),
				}, {
					Match:  prefixmatch("/paul"),
					Action: clusteraction("default/paul/paul"),
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		name: "multiple rules, tls admin",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				TLS: []v1beta1.IngressTLS{{
					Hosts:      []string{"admin.httpbin.org"},
					SecretName: "adminsecret",
				}},
				Rules: []v1beta1.IngressRule{{
					Host:             "httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
				}, {
					Host:             "admin.httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("paul", intstr.FromString("paul"))),
				}},
			},
		},
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "admin.httpbin.org",
				Domains: []string{"admin.httpbin.org", "admin.httpbin.org:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/paul/paul"),
				}},
			}, &route.VirtualHost{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org", "httpbin.org:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/peter/80"),
				}},
			},
		},
		ingress_https: []proto.Message{
			&route.VirtualHost{
				Name:    "admin.httpbin.org",
				Domains: []string{"admin.httpbin.org", "admin.httpbin.org:443"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/paul/paul"),
				}},
			},
		},
	}, {
		name: "multiple rules, tls admin, no http",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.allow-http": "false",
				},
			},
			Spec: v1beta1.IngressSpec{
				TLS: []v1beta1.IngressTLS{{
					Hosts:      []string{"admin.httpbin.org"},
					SecretName: "adminsecret",
				}},
				Rules: []v1beta1.IngressRule{{
					Host:             "httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
				}, {
					Host:             "admin.httpbin.org",
					IngressRuleValue: ingressrulevalue(backend("paul", intstr.FromString("paul"))),
				}},
			},
		},
		ingress_http: []proto.Message{}, //  allow-http: false disables the http route entirely.
		ingress_https: []proto.Message{
			&route.VirtualHost{
				Name:    "admin.httpbin.org",
				Domains: []string{"admin.httpbin.org", "admin.httpbin.org:443"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/paul/paul"),
				}},
			},
		},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "d31bb322ca62bb395acad00b3cbf45a3aa1010ca28dca7cddb4f7db786fa",
				Domains: []string{"my-very-very-long-service-host-name.subdomain.boring-dept.my.company", "my-very-very-long-service-host-name.subdomain.boring-dept.my.company:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/my-service-name/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org", "httpbin.org:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/admin"),
					Action: clusteraction("kube-system/admin/admin"),
				}, {
					Match:  prefixmatch("/"),
					Action: clusteraction("default/default/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		// kube-lego uses a single vhost in its own namespace to insert its
		// callback route for let's encrypt support.
		name: "kube-lego style extend vhost definitions",
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "httpbin.davecheney.com",
				Domains: []string{"httpbin.davecheney.com", "httpbin.davecheney.com:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/.well-known/acme-challenge"),
					Action: clusteraction("kube-lego/kube-lego-nginx/8080"),
				}, {
					Match:  prefixmatch("/"),
					Action: clusteraction("default/httpbin/80"),
				}},
			}, &route.VirtualHost{
				Name:    "httpbin2.davecheney.com",
				Domains: []string{"httpbin2.davecheney.com", "httpbin2.davecheney.com:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/.well-known/acme-challenge"),
					Action: clusteraction("kube-lego/kube-lego-nginx/8080"),
				}, {
					Match:  prefixmatch("/"),
					Action: clusteraction("default/httpbin/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
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
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/hello"),
					Action: clusteraction("default/hello/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		name: "explicitly set upstream timeout to seconds",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"contour.heptio.com/request-timeout": "20s",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteractiontimeout("default/backend/80", 20*time.Second),
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		name: "explicitly set upstream timeout to infinite",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"contour.heptio.com/request-timeout": "infinity",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),                                          // match all
					Action: clusteractiontimeout("default/backend/80", 0*time.Second), // infinity
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		name: "explicitly set upstream timeout to an invalid duration",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"contour.heptio.com/request-timeout": "300jiosadf",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),                                          // match all
					Action: clusteractiontimeout("default/backend/80", 0*time.Second), // infinity
				}},
			},
		},
		ingress_https: []proto.Message{},
	}, {
		name: "default vhost should not overwrite named vhost",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hello",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("kuard", intstr.FromInt(80)),
				Rules: []v1beta1.IngressRule{{
					Host: "test-gui",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path: "/",
								Backend: v1beta1.IngressBackend{
									ServiceName: "test-gui",
									ServicePort: intstr.FromInt(80),
								},
							}},
						},
					},
				}},
			},
		},
		ingress_http: []proto.Message{
			&route.VirtualHost{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/kuard/80"),
				}},
			},
			&route.VirtualHost{
				Name:    "test-gui",
				Domains: []string{"test-gui", "test-gui:80"},
				Routes: []route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/test-gui/80"),
				}},
			},
		},
		ingress_https: []proto.Message{},
	}}

	log := testLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			if tc.setup != nil {
				tc.setup(tr)
			}
			tr.OnAdd(tc.ing)
			got := contents(&tr.VirtualHostCache.HTTP)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_http, got) {
				t.Fatalf("(ingress_http) want:\n%v\n got:\n%v", tc.ingress_http, got)
			}

			got = contents(&tr.VirtualHostCache.HTTPS)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_https, got) {
				t.Fatalf("(ingress_https) want:\n%v\n got:\n%v", tc.ingress_https, got)
			}
		})
	}
}

func TestTranslatorUpdateIngress(t *testing.T) {
	tests := map[string]struct {
		before, after               *v1beta1.Ingress
		ingress_http, ingress_https []proto.Message
	}{
		"move rename default ingress to named vhost without renaming object": { // issue 257
			before: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard-ing",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "contour",
					},
				},
				Spec: v1beta1.IngressSpec{
					Backend: &v1beta1.IngressBackend{
						ServiceName: "kuard",
						ServicePort: intstr.FromInt(80),
					},
				},
			},
			after: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard-ing",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "contour",
					},
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "kuard.db.gd-ms.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path: "/",
									Backend: v1beta1.IngressBackend{
										ServiceName: "kuard",
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			},
			ingress_http: []proto.Message{
				&route.VirtualHost{
					Name:    "kuard.db.gd-ms.com",
					Domains: []string{"kuard.db.gd-ms.com", "kuard.db.gd-ms.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/"), // match all
						Action: clusteraction("default/kuard/80"),
					}},
				},
			},
			ingress_https: []proto.Message{},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tr.OnAdd(tc.before)
			tr.OnUpdate(tc.before, tc.after)
			got := contents(&tr.VirtualHostCache.HTTP)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_http, got) {
				t.Fatalf("(ingress_http): got: %v, want: %v", got, tc.ingress_http)
			}

			got = contents(&tr.VirtualHostCache.HTTPS)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_https, got) {
				t.Fatalf("(ingress_https): got: %v, want: %v", got, tc.ingress_https)
			}
		})
	}
}

func TestTranslatorRemoveIngress(t *testing.T) {
	tests := map[string]struct {
		setup         func(*Translator)
		ing           *v1beta1.Ingress
		ingress_http  []proto.Message
		ingress_https []proto.Message
	}{
		"remove existing": {
			setup: func(tr *Translator) {
				tr.OnAdd(&v1beta1.Ingress{
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
			ingress_http:  []proto.Message{},
			ingress_https: []proto.Message{},
		},
		"remove different": {
			setup: func(tr *Translator) {
				tr.OnAdd(&v1beta1.Ingress{
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
			ingress_http: []proto.Message{
				&route.VirtualHost{
					Name:    "httpbin.org",
					Domains: []string{"httpbin.org", "httpbin.org:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/"),
						Action: clusteraction("default/peter/80"),
					}},
				},
			},
			ingress_https: []proto.Message{},
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
			ingress_http:  []proto.Message{},
			ingress_https: []proto.Message{},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tc.setup(tr)
			tr.OnDelete(tc.ing)
			got := contents(&tr.VirtualHostCache.HTTP)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_http, got) {
				t.Fatalf("(ingress_http): got: %v, want: %v", got, tc.ingress_http)
			}

			got = contents(&tr.VirtualHostCache.HTTPS)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_https, got) {
				t.Fatalf("(ingress_https): got: %v, want: %v", got, tc.ingress_https)
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
	return serviceWithAnnotations(ns, name, nil, ports...)
}

func serviceWithAnnotations(ns, name string, annotations map[string]string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: annotations,
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
			EdsConfig:   apiconfigsource("contour"),
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

func testLogger(t *testing.T) logrus.FieldLogger {
	log := logrus.New()
	log.Out = &testWriter{t}
	return log
}

type testWriter struct {
	*testing.T
}

func (t *testWriter) Write(buf []byte) (int, error) {
	t.Logf("%s", buf)
	return len(buf), nil
}

func contents(v interface {
	Values(func(string) bool) []proto.Message
}) []proto.Message {
	return v.Values(func(string) bool { return true })
}

// prefixmatch returns a RouteMatch for the supplied prefix.
func prefixmatch(prefix string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

// clusteraction returns a Route_Route action for the supplied cluster.
func clusteraction(cluster string) *route.Route_Route {
	return &route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

// clusteractiontimeout returns a cluster action with the specified timeout.
// A timeout of 0 means infinity. If you do not want to specify a timeout, use
// clusteraction instead.
func clusteractiontimeout(name string, timeout time.Duration) *route.Route_Route {
	// TODO(cmaloney): Pull timeout off of the backend cluster annotation
	// and use it over the value retrieved from the ingress annotation if
	// specified.
	c := clusteraction(name)
	c.Route.Timeout = &timeout
	return c
}

type virtualHostsByName []proto.Message

func (v virtualHostsByName) Len() int      { return len(v) }
func (v virtualHostsByName) Swap(i, j int) { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool {
	return v[i].(*route.VirtualHost).Name < v[j].(*route.VirtualHost).Name
}

// regexmatch returns a RouteMatch for the supplied regex.
func regexmatch(regex string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Regex{
			Regex: regex,
		},
	}
}
