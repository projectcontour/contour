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
	"reflect"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRouteVisit(t *testing.T) {
	var (
		infinity     = time.Duration(0)
		nintyseconds = time.Duration(90 * time.Second)
	)

	tests := map[string]struct {
		*RouteCache
		objs []interface{}
		want map[string]*v2.RouteConfiguration
	}{
		"nothing": {
			objs: nil,
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"one http only ingress with service": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"one http only ingressroute": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{
								{
									Name: "backend",
									Port: 80,
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/backend/80"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"default backend ingress with secret": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "*", // default backend
						Domains: []string{"*"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https", // no https for default backend
				},
			},
		},
		"vhost ingress with secret": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"www.example.com"},
							SecretName: "secret",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromString("www"),
										},
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "www",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:443"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
			},
		},
		"simple ingressroute with secret": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &ingressroutev1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 8080,
							},
							}},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "www",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/backend/8080"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:443"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/backend/8080"),
						}},
					}},
				},
			},
		},
		"simple tls ingress with allow-http:false": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.allow-http": "false",
						},
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"www.example.com"},
							SecretName: "secret",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromString("www"),
										},
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "www",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
				"ingress_https": {
					Name: "ingress_https",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:443"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
			},
		},
		"simple tls ingress with force-ssl-redirect": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"ingress.kubernetes.io/force-ssl-redirect": "true",
						},
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"www.example.com"},
							SecretName: "secret",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromString("www"),
										},
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "www",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match: prefixmatch("/"),
							Action: &route.Route_Redirect{
								Redirect: &route.RedirectAction{
									HttpsRedirect: true,
								},
							},
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:443"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
			},
		},
		"ingress with websocket annotation": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/websocket-routes": "/ws1 , /ws2",
						},
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Path: "/",
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromString("www"),
										},
									}, {
										Path: "/ws1",
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromString("www"),
										},
									}},
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "www",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match:  prefixmatch("/ws1"),
							Action: websocketroute("default/kuard/8080"),
						}, {
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingress invalid timeout": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/request-timeout": "heptio",
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routetimeout("default/kuard/8080", &infinity),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingress infinite timeout": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/request-timeout": "infinity",
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routetimeout("default/kuard/8080", &infinity),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingress 90 second timeout": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/request-timeout": "1m30s",
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routetimeout("default/kuard/8080", &nintyseconds),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"vhost name exceeds 60 chars": { // heptio/contour#25
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-service-name",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							Host: "my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Path: "/",
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromString("www"),
										},
									}},
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "www",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "d31bb322ca62bb395acad00b3cbf45a3aa1010ca28dca7cddb4f7db786fa",
						Domains: []string{"my-very-very-long-service-host-name.subdomain.boring-dept.my.company", "my-very-very-long-service-host-name.subdomain.boring-dept.my.company:80"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/80"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"incorrect ingress class": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "incorrect",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http", // expected to be empty, the ingress class is ignored
				},
				"ingress_https": {
					Name: "ingress_https", // expected to be empty, the ingress class is ignored
				},
			},
		},
		"explicit ingress class": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "incorrect",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": new(ResourceEventHandler).ingressClass(),
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "*",
						Domains: []string{"*"},
						Routes: []route.Route{{
							Match:  prefixmatch("/"),
							Action: routeroute("default/kuard/8080"),
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingressroute no weights defined": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{
								{
									Name: "backend",
									Port: 80,
								},
								{
									Name: "backendtwo",
									Port: 80,
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backendtwo",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match: prefixmatch("/"),
							Action: &route.Route_Route{
								Route: &route.RouteAction{
									ClusterSpecifier: &route.RouteAction_WeightedClusters{
										WeightedClusters: &route.WeightedCluster{
											Clusters: []*route.WeightedCluster_ClusterWeight{{
												Name:   "default/backend/80",
												Weight: &types.UInt32Value{Value: uint32(1)},
											}, {
												Name:   "default/backendtwo/80",
												Weight: &types.UInt32Value{Value: uint32(1)},
											}},
											TotalWeight: &types.UInt32Value{
												Value: uint32(2),
											},
										},
									},
								},
							},
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingressroute one weight defined": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{
								{
									Name: "backend",
									Port: 80,
								},
								{
									Name:   "backendtwo",
									Port:   80,
									Weight: 50,
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backendtwo",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match: prefixmatch("/"),
							Action: &route.Route_Route{
								Route: &route.RouteAction{
									ClusterSpecifier: &route.RouteAction_WeightedClusters{
										WeightedClusters: &route.WeightedCluster{
											Clusters: []*route.WeightedCluster_ClusterWeight{{
												Name:   "default/backend/80",
												Weight: &types.UInt32Value{Value: uint32(0)},
											}, {
												Name:   "default/backendtwo/80",
												Weight: &types.UInt32Value{Value: uint32(50)},
											}},
											TotalWeight: &types.UInt32Value{
												Value: uint32(50),
											},
										},
									},
								},
							},
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingressroute aliases defined": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
							Aliases: []string{
								"foo.com",
								"bar.com",
							},
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{
								{
									Name: "backend",
									Port: 80,
								},
								{
									Name:   "backendtwo",
									Port:   80,
									Weight: 50,
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backendtwo",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"foo.com", "bar.com", "www.example.com", "www.example.com:80", "foo.com:80", "bar.com:80"},
						Routes: []route.Route{{
							Match: prefixmatch("/"),
							Action: &route.Route_Route{
								Route: &route.RouteAction{
									ClusterSpecifier: &route.RouteAction_WeightedClusters{
										WeightedClusters: &route.WeightedCluster{
											Clusters: []*route.WeightedCluster_ClusterWeight{{
												Name:   "default/backend/80",
												Weight: &types.UInt32Value{Value: uint32(0)},
											}, {
												Name:   "default/backendtwo/80",
												Weight: &types.UInt32Value{Value: uint32(50)},
											}},
											TotalWeight: &types.UInt32Value{
												Value: uint32(50),
											},
										},
									},
								},
							},
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingressroute all weights defined": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{
								{
									Name:   "backend",
									Port:   80,
									Weight: 22,
								},
								{
									Name:   "backendtwo",
									Port:   80,
									Weight: 50,
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backendtwo",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
					VirtualHosts: []route.VirtualHost{{
						Name:    "www.example.com",
						Domains: []string{"www.example.com", "www.example.com:80"},
						Routes: []route.Route{{
							Match: prefixmatch("/"),
							Action: &route.Route_Route{
								Route: &route.RouteAction{
									ClusterSpecifier: &route.RouteAction_WeightedClusters{
										WeightedClusters: &route.WeightedCluster{
											Clusters: []*route.WeightedCluster_ClusterWeight{{
												Name:   "default/backend/80",
												Weight: &types.UInt32Value{Value: uint32(22)},
											}, {
												Name:   "default/backendtwo/80",
												Weight: &types.UInt32Value{Value: uint32(50)},
											}},
											TotalWeight: &types.UInt32Value{
												Value: uint32(72),
											},
										},
									},
								},
							},
						}},
					}},
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
		"ingressroute w/ missing fqdn": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{
								{
									Name: "backend",
									Port: 80,
								},
							},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http", // should be blank, no fqdn defined.
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reh := ResourceEventHandler{
				Notifier: new(nullNotifier),
				Metrics:  metrics.NewMetrics(prometheus.NewRegistry()),
			}
			for _, o := range tc.objs {
				reh.OnAdd(o)
			}
			rc := tc.RouteCache
			if rc == nil {
				rc = new(RouteCache)
			}
			v := routeVisitor{
				RouteCache: rc,
				Visitable:  reh.Build(),
			}
			got := v.Visit()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%+v\ngot:\n%+v", tc.want, got)
			}
		})
	}
}

func routeroute(cluster string) *route.Route_Route {
	return &route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: &route.WeightedCluster{
					Clusters: []*route.WeightedCluster_ClusterWeight{{
						Name:   cluster,
						Weight: &types.UInt32Value{Value: uint32(1)},
					}},
					TotalWeight: &types.UInt32Value{
						Value: uint32(1),
					},
				},
			},
		},
	}
}

func websocketroute(c string) *route.Route_Route {
	cl := routeroute(c)
	cl.Route.UseWebsocket = &types.BoolValue{Value: true}
	return cl
}

func routetimeout(cluster string, timeout *time.Duration) *route.Route_Route {
	cl := routeroute(cluster)
	cl.Route.Timeout = timeout
	return cl
}

func TestActionRoute(t *testing.T) {
	tests := map[string]struct {
		services  []*dag.Service
		websocket bool
		timeout   time.Duration
		want      *route.Route_Route
	}{
		"single service": {
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(1),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(1),
							},
						},
					},
				},
			},
		},
		"single service with timeout": {
			timeout: 30 * time.Second,
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(1),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(1),
							},
						},
					},
					Timeout: pduration(30 * time.Second),
				},
			},
		},
		"single service with infinite timeout": {
			timeout: time.Duration(-1),
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(1),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(1),
							},
						},
					},
					Timeout: pduration(time.Duration(0)),
				},
			},
		},
		"single service with websockets": {
			websocket: true,
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(1),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(1),
							},
						},
					},
					UseWebsocket: &types.BoolValue{Value: true},
				},
			},
		},
		"single service with websockets and timeout": {
			websocket: true,
			timeout:   5 * time.Second,
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(1),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(1),
							},
						},
					},
					Timeout:      pduration(5 * time.Second),
					UseWebsocket: &types.BoolValue{Value: true},
				},
			},
		},
		"multiple services": {
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "nginx",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(1),
								}}, {
								Name: "default/nginx/8080",
								Weight: &types.UInt32Value{
									Value: uint32(1),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(2),
							},
						},
					},
				},
			},
		},
		"multiple weighted services": {
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
					Weight: 80,
				},
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "nginx",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
					Weight: 20,
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(80),
								}}, {
								Name: "default/nginx/8080",
								Weight: &types.UInt32Value{
									Value: uint32(20),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(100),
							},
						},
					},
				},
			},
		},
		"multiple weighted services and one with no weight specified": {
			services: []*dag.Service{
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
					Weight: 80,
				},
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "nginx",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
					Weight: 20,
				},
				{
					Object: &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "notraffic",
							Namespace: "default",
						},
					},
					ServicePort: &v1.ServicePort{
						Port: 8080,
					},
				},
			},
			want: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{{
								Name: "default/kuard/8080",
								Weight: &types.UInt32Value{
									Value: uint32(80),
								}}, {
								Name: "default/nginx/8080",
								Weight: &types.UInt32Value{
									Value: uint32(20),
								}}, {
								Name: "default/notraffic/8080",
								Weight: &types.UInt32Value{
									Value: uint32(0),
								}},
							},
							TotalWeight: &types.UInt32Value{
								Value: uint32(100),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := actionroute(tc.services, tc.websocket, tc.timeout)
			if !reflect.DeepEqual(tc.want, got) {
				t.Errorf("wanted:\n%v\ngot:\n%v\n", tc.want, got)
			}
		})
	}
}

func pduration(d time.Duration) *time.Duration {
	return &d
}
