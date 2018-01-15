// Copyright © 2018 Heptio
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

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/heptio/contour/internal/log/stdlog"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestVirtualHostCacheRecomputevhost(t *testing.T) {
	tests := map[string]struct {
		vhost         string
		ingresses     []*v1beta1.Ingress
		ingress_http  []*v2.VirtualHost
		ingress_https []*v2.VirtualHost
	}{
		/*		"default backend": {
					ingresses: []*v1beta1.Ingress{{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Backend: backend("backend", intstr.FromInt(80)),
						},
					}},
					ingress_http: []*v2.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []*v2.Route{{
							Match:  prefixmatch("/"),
							Action: clusteraction("default/backend/80"),
						}},
					}},
					ingress_https: []*v2.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []*v2.Route{{
							Match:  prefixmatch("/"),
							Action: clusteraction("default/backend/80"),
						}},
					}},
				},
				"incorrect ingress class": {
					ingresses: []*v1beta1.Ingress{{
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
					}},
					ingress_http:  []*v2.VirtualHost{}, // expected to be empty, the ingress class is ingnored
					ingress_https: []*v2.VirtualHost{}, // expected to be empty, the ingress class is ingnored
				},
				"explicit ingress class": {
					ingresses: []*v1beta1.Ingress{{
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
					}},
					ingress_http: []*v2.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []*v2.Route{{
							Match:  prefixmatch("/"), // match all
							Action: clusteraction("default/backend/80"),
						}},
					}},
					ingress_https: []*v2.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []*v2.Route{{
							Match:  prefixmatch("/"), // match all
							Action: clusteraction("default/backend/80"),
						}},
					}},
				}, */
		"name based vhost": {
			vhost: "httpbin.org",
			ingresses: []*v1beta1.Ingress{{
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
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []*v2.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		"regex vhost without match characters": {
			vhost: "httpbin.org",
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
									Path:    "/ip", // this field _is_ a regex
									Backend: *backend("httpbin-org", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []*v2.Route{{
					Match:  prefixmatch("/ip"), // if the field does not contact any regex characters, we treat it as a prefix
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		"regex vhost with match characters": {
			vhost: "httpbin.org",
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
									Path:    "/get.*", // this field _is_ a regex
									Backend: *backend("httpbin-org", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []*v2.Route{{
					Match:  regexmatch("/get.*"),
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		"named service port": {
			vhost: "httpbin.org",
			ingresses: []*v1beta1.Ingress{{
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
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []*v2.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/httpbin-org/http"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		"multiple routes": {
			vhost: "httpbin.org",
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
			}},
			ingress_http: []*v2.VirtualHost{{
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
			ingress_https: []*v2.VirtualHost{},
		},
		"multiple rules (httpbin.org)": {
			vhost: "httpbin.org",
			ingresses: []*v1beta1.Ingress{{
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
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []*v2.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/peter/80"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		"multiple rules (admin.httpbin.org)": {
			vhost: "admin.httpbin.org",
			ingresses: []*v1beta1.Ingress{{
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
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "admin.httpbin.org",
				Domains: []string{"admin.httpbin.org"},
				Routes: []*v2.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/paul/paul"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		"vhost name exceeds 60 chars": { // heptio/contour#25
			vhost: "my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
			ingresses: []*v1beta1.Ingress{{
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
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "d31bb322ca62bb395acad00b3cbf45a3aa1010ca28dca7cddb4f7db786fa",
				Domains: []string{"my-very-very-long-service-host-name.subdomain.boring-dept.my.company"},
				Routes: []*v2.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/my-service-name/80"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		"second ingress object extends an existing vhost": {
			vhost: "httpbin.org",
			ingresses: []*v1beta1.Ingress{{
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
			}, {
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
			}},
			ingress_http: []*v2.VirtualHost{{
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
			ingress_https: []*v2.VirtualHost{},
		},
		// kube-lego uses a single vhost in its own namespace to insert its
		// callback route for let's encrypt support.
		"kube-lego style extend vhost definitions": {
			vhost: "httpbin.davecheney.com",
			ingresses: []*v1beta1.Ingress{{
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
			}, {
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
			}},
			ingress_http: []*v2.VirtualHost{{
				Name:    "httpbin.davecheney.com",
				Domains: []string{"httpbin.davecheney.com"},
				Routes: []*v2.Route{{
					Match:  prefixmatch("/.well-known/acme-challenge"),
					Action: clusteraction("kube-lego/kube-lego-nginx/8080"),
				}, {
					Match:  prefixmatch("/"),
					Action: clusteraction("default/httpbin/80"),
				}},
			}},
			ingress_https: []*v2.VirtualHost{},
		},
		/*
			"IngressRuleValue without host should become the default vhost": { // heptio/contour#101
				ingresses: []*v1beta1.Ingress{{
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
				}},
				ingress_http: []*v2.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*v2.Route{{
						Match:  prefixmatch("/hello"),
						Action: clusteraction("default/hello/80"),
					}},
				}},
				ingress_https: []*v2.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*v2.Route{{
						Match:  prefixmatch("/hello"),
						Action: clusteraction("default/hello/80"),
					}},
				}},
			},*/
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := &Translator{
				Logger: stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS),
			}
			tr.recomputevhost(tc.vhost, tc.ingresses)
			got := tr.VirtualHostCache.HTTP.Values()
			if !reflect.DeepEqual(tc.ingress_http, got) {
				t.Fatalf("addIngress(%v):\n (ingress_http) got: %v\nwant: %v", tc.vhost, got, tc.ingress_http)
			}

			got = tr.VirtualHostCache.HTTPS.Values()
			if !reflect.DeepEqual(tc.ingress_https, got) {
				t.Fatalf("addIngress(%v):\n (ingress_https) got: %v\nwant: %v", tc.vhost, got, tc.ingress_https)
			}
		})
	}
}
