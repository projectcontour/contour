// Copyright © 2019 VMware
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
	"sort"
	"testing"
	"time"

	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/dag"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/proto"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRouteCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*v2.RouteConfiguration
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
			want: []proto.Message{
				&v2.RouteConfiguration{
					Name: "ingress_http",
				},
				&v2.RouteConfiguration{
					Name: "ingress_https",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var rc RouteCache
			rc.Update(tc.contents)
			got := rc.Contents()
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRouteCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*v2.RouteConfiguration
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
			},
			query: []string{"ingress_http"},
			want: []proto.Message{
				&v2.RouteConfiguration{
					Name: "ingress_http",
				},
			},
		},
		"partial match": {
			contents: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
			},
			query: []string{"stats-handler", "ingress_http"},
			want: []proto.Message{
				&v2.RouteConfiguration{
					Name: "ingress_http",
				},
				&v2.RouteConfiguration{
					Name: "stats-handler",
				},
			},
		},
		"no match": {
			contents: map[string]*v2.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
			},
			query: []string{"stats-handler"},
			want: []proto.Message{
				&v2.RouteConfiguration{
					Name: "stats-handler",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var rc RouteCache
			rc.Update(tc.contents)
			got := rc.Query(tc.query)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRouteVisit(t *testing.T) {
	tests := map[string]struct {
		objs []interface{}
		want map[string]*v2.RouteConfiguration
	}{
		"nothing": {
			objs: nil,
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"one http only ingress with service": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("kuard", 8080),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"one http only ingress with regex match": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Path: "/[^/]+/invoices(/.*|/?)", // issue 1243
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromInt(8080),
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
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RouteRegex("/[^/]+/invoices(/.*|/?)"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"one http only ingressroute": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/backend/80/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
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
						Backend: backend("kuard", 8080),
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
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
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
			),
		},
		"simple ingressroute with secret": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &projcontour.TLS{
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
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match: envoy.RoutePrefix("/"),
							Action: &envoy_api_v2_route.Route_Redirect{
								Redirect: &envoy_api_v2_route.RedirectAction{
									SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy.RouteConfiguration("ingress_https",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/backend/8080/da39a3ee5e")),
					),
				),
			),
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
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http"),
				envoy.RouteConfiguration("ingress_https",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
			),
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
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match: envoy.RoutePrefix("/"),
							Action: &envoy_api_v2_route.Route_Redirect{
								Redirect: &envoy_api_v2_route.RedirectAction{
									SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy.RouteConfiguration("ingress_https",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
			),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/ws1"), websocketroute("default/kuard/8080/da39a3ee5e")),
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/8080/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
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
						Backend: backend("kuard", 8080),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routetimeout("default/kuard/8080/da39a3ee5e", 0)),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
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
						Backend: backend("kuard", 8080),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routetimeout("default/kuard/8080/da39a3ee5e", 0)),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
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
						Backend: backend("kuard", 8080),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routetimeout("default/kuard/8080/da39a3ee5e", 90*time.Second)),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
						envoy.Route(envoy.RoutePrefix("/"), routecluster("default/kuard/80/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"ingress retry-on": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/retry-on": "5xx,gateway-error",
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("kuard", 8080),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routeretry("default/kuard/8080/da39a3ee5e", "5xx,gateway-error", 0, 0)),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"ingress retry-on, num-retries": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/retry-on":    "5xx,gateway-error",
							"contour.heptio.com/num-retries": "7", // not five or six or eight, but seven.
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("kuard", 8080),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routeretry("default/kuard/8080/da39a3ee5e", "5xx,gateway-error", 7, 0)),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"ingress retry-on, per-try-timeout": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/retry-on":        "5xx,gateway-error",
							"contour.heptio.com/per-try-timeout": "150ms",
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("kuard", 8080),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("*",
						envoy.Route(envoy.RoutePrefix("/"), routeretry("default/kuard/8080/da39a3ee5e", "5xx,gateway-error", 0, 150*time.Millisecond)),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"ingressroute no weights defined": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backendtwo",
								Port: 80,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), &envoy_api_v2_route.Route_Route{
							Route: &envoy_api_v2_route.RouteAction{
								ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
									WeightedClusters: &envoy_api_v2_route.WeightedCluster{
										Clusters: weightedClusters(
											weightedCluster("default/backend/80/da39a3ee5e", 1),
											weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
										),
										TotalWeight: protobuf.UInt32(2),
									},
								},
							},
						}),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"ingressroute one weight defined": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name:   "backendtwo",
								Port:   80,
								Weight: 50,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), &envoy_api_v2_route.Route_Route{
							Route: &envoy_api_v2_route.RouteAction{
								ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
									WeightedClusters: &envoy_api_v2_route.WeightedCluster{
										Clusters: weightedClusters(
											weightedCluster("default/backend/80/da39a3ee5e", 0),
											weightedCluster("default/backendtwo/80/da39a3ee5e", 50),
										),
										TotalWeight: protobuf.UInt32(50),
									},
								},
							},
						}),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"ingressroute all weights defined": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{{
							Match: "/",
							Services: []ingressroutev1.Service{{
								Name:   "backend",
								Port:   80,
								Weight: 22,
							}, {
								Name:   "backendtwo",
								Port:   80,
								Weight: 50,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), &envoy_api_v2_route.Route_Route{
							Route: &envoy_api_v2_route.RouteAction{
								ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
									WeightedClusters: &envoy_api_v2_route.WeightedCluster{
										Clusters: weightedClusters(
											weightedCluster("default/backend/80/da39a3ee5e", 22),
											weightedCluster("default/backendtwo/80/da39a3ee5e", 50),
										),
										TotalWeight: protobuf.UInt32(72),
									},
								},
							},
						}),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"ingressroute w/ missing fqdn": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &projcontour.VirtualHost{},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http"), // should be blank, no fqdn defined.
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with pathPrefix": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{{
								Prefix: "/",
							}},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backendtwo",
								Port: 80,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), &envoy_api_v2_route.Route_Route{
							Route: &envoy_api_v2_route.RouteAction{
								ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
									WeightedClusters: &envoy_api_v2_route.WeightedCluster{
										Clusters: weightedClusters(
											weightedCluster("default/backend/80/da39a3ee5e", 1),
											weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
										),
										TotalWeight: protobuf.UInt32(2),
									},
								},
							},
						}),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with mirror policy": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{{
								Prefix: "/",
							}},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name:   "backendtwo",
								Port:   80,
								Mirror: true,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), withMirrorPolicy(routecluster("default/backend/80/da39a3ee5e"), "default/backendtwo/80/da39a3ee5e")),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with pathPrefix with tls": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &projcontour.TLS{
								SecretName: "secret",
							},
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{{
								Prefix: "/",
							}},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backendtwo",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						&envoy_api_v2_route.Route{
							Match: envoy.RoutePrefix("/"),
							Action: &envoy_api_v2_route.Route_Redirect{
								Redirect: &envoy_api_v2_route.RedirectAction{
									SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy.RouteConfiguration("ingress_https",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/"), &envoy_api_v2_route.Route_Route{
							Route: &envoy_api_v2_route.RouteAction{
								ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
									WeightedClusters: &envoy_api_v2_route.WeightedCluster{
										Clusters: weightedClusters(
											weightedCluster("default/backend/80/da39a3ee5e", 1),
											weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
										),
										TotalWeight: protobuf.UInt32(2),
									},
								},
							},
						}),
					)),
			),
		},
		"httpproxy with pathPrefix includes": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Includes: []projcontour.Include{{
							Name:      "child",
							Namespace: "teama",
							Conditions: []projcontour.Condition{{
								Prefix: "/blog",
							}},
						}},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{{
								Prefix: "/",
							}},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backendtwo",
								Port: 80,
							}},
						}},
					},
				},
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child",
						Namespace: "teama",
					},
					Spec: projcontour.HTTPProxySpec{
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{{
								Prefix: "/info",
							}},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}},
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
						Name:      "backend",
						Namespace: "teama",
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/blog/info"), routecluster("teama/backend/80/da39a3ee5e")),
						envoy.Route(envoy.RoutePrefix("/"), &envoy_api_v2_route.Route_Route{
							Route: &envoy_api_v2_route.RouteAction{
								ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
									WeightedClusters: &envoy_api_v2_route.WeightedCluster{
										Clusters: weightedClusters(
											weightedCluster("default/backend/80/da39a3ee5e", 1),
											weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
										),
										TotalWeight: protobuf.UInt32(2),
									},
								},
							},
						}),
					),
				),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with header contains conditions": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{
								{
									Prefix: "/",
								},
								{
									Header: &projcontour.HeaderCondition{
										Name:     "x-header",
										Contains: "abc",
									},
								},
							},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "contains",
						}), routecluster("default/backend/80/da39a3ee5e")),
					)),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with header notcontains conditions": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{
								{
									Prefix: "/",
								},
								{
									Header: &projcontour.HeaderCondition{
										Name:        "x-header",
										NotContains: "abc",
									},
								},
							},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "contains",
							Invert:    true,
						}), routecluster("default/backend/80/da39a3ee5e")),
					)),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with header exact match conditions": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{
								{
									Prefix: "/",
								},
								{
									Header: &projcontour.HeaderCondition{
										Name:  "x-header",
										Exact: "abc",
									},
								},
							},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "exact",
							Invert:    false,
						}), routecluster("default/backend/80/da39a3ee5e")),
					)),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with header exact not match conditions": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{
								{
									Prefix: "/",
								},
								{
									Header: &projcontour.HeaderCondition{
										Name:     "x-header",
										NotExact: "abc",
									},
								},
							},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "exact",
							Invert:    true,
						}), routecluster("default/backend/80/da39a3ee5e")),
					)),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
		"httpproxy with header header present conditions": {
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []projcontour.Route{{
							Conditions: []projcontour.Condition{
								{
									Prefix: "/",
								},
								{
									Header: &projcontour.HeaderCondition{
										Name:    "x-header",
										Present: true,
									},
								},
							},
							Services: []projcontour.Service{{
								Name: "backend",
								Port: 80,
							}},
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
			want: routeConfigurations(
				envoy.RouteConfiguration("ingress_http",
					envoy.VirtualHost("www.example.com",
						envoy.Route(envoy.RoutePrefix("/", dag.HeaderCondition{
							Name:      "x-header",
							MatchType: "present",
						}), routecluster("default/backend/80/da39a3ee5e")),
					)),
				envoy.RouteConfiguration("ingress_https"),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := buildDAG(t, tc.objs...)
			got := visitRoutes(root)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSortLongestRouteFirst(t *testing.T) {
	tests := map[string]struct {
		routes []*envoy_api_v2_route.Route
		want   []*envoy_api_v2_route.Route
	}{
		"two prefixes": {
			routes: []*envoy_api_v2_route.Route{{
				Match: envoy.RoutePrefix("/"),
			}, {
				Match: envoy.RoutePrefix("/longer"),
			}},
			want: []*envoy_api_v2_route.Route{{
				Match: envoy.RoutePrefix("/longer"),
			}, {
				Match: envoy.RoutePrefix("/"),
			}},
		},
		"two regexes": {
			routes: []*envoy_api_v2_route.Route{{
				Match: envoy.RouteRegex("/v2"),
			}, {
				Match: envoy.RouteRegex("/v1/.+"),
			}},
			want: []*envoy_api_v2_route.Route{{
				Match: envoy.RouteRegex("/v2"),
			}, {
				Match: envoy.RouteRegex("/v1/.+"),
			}},
		},
		"regex sorts before prefix": {
			routes: []*envoy_api_v2_route.Route{{
				Match: envoy.RouteRegex("/api/v?"),
			}, {
				Match: envoy.RoutePrefix("/"),
			}, {
				Match: envoy.RouteRegex(".*"),
			}},
			want: []*envoy_api_v2_route.Route{{
				Match: envoy.RouteRegex("/api/v?"),
			}, {
				Match: envoy.RouteRegex(".*"),
			}, {
				Match: envoy.RoutePrefix("/"),
			}},
		},
		"more headers sort before less": {
			routes: []*envoy_api_v2_route.Route{{
				Match: envoy.RoutePrefix("/"),
			}, {
				Match: envoy.RoutePrefix("/", dag.HeaderCondition{
					Name:      "x-request-id",
					MatchType: "present",
				}),
			}},
			want: []*envoy_api_v2_route.Route{{
				Match: envoy.RoutePrefix("/", dag.HeaderCondition{
					Name:      "x-request-id",
					MatchType: "present",
				}),
			}, {
				Match: envoy.RoutePrefix("/"),
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := append([]*envoy_api_v2_route.Route{}, tc.routes...) // shallow copy
			sort.Stable(longestRouteFirst(got))
			assert.Equal(t, tc.want, got)
		})
	}
}

func routecluster(cluster string) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}

}

func websocketroute(c string) *envoy_api_v2_route.Route_Route {
	r := routecluster(c)
	r.Route.UpgradeConfigs = append(r.Route.UpgradeConfigs,
		&envoy_api_v2_route.RouteAction_UpgradeConfig{
			UpgradeType: "websocket",
		},
	)
	return r
}

func routetimeout(cluster string, timeout time.Duration) *envoy_api_v2_route.Route_Route {
	r := routecluster(cluster)
	r.Route.Timeout = protobuf.Duration(timeout)
	return r
}

func routeretry(cluster string, retryOn string, numRetries uint32, perTryTimeout time.Duration) *envoy_api_v2_route.Route_Route {
	r := routecluster(cluster)
	r.Route.RetryPolicy = &envoy_api_v2_route.RetryPolicy{
		RetryOn: retryOn,
	}
	if numRetries > 0 {
		r.Route.RetryPolicy.NumRetries = protobuf.UInt32(numRetries)
	}
	if perTryTimeout > 0 {
		r.Route.RetryPolicy.PerTryTimeout = protobuf.Duration(perTryTimeout)
	}
	return r
}

func weightedClusters(first, second *envoy_api_v2_route.WeightedCluster_ClusterWeight, rest ...*envoy_api_v2_route.WeightedCluster_ClusterWeight) []*envoy_api_v2_route.WeightedCluster_ClusterWeight {
	return append([]*envoy_api_v2_route.WeightedCluster_ClusterWeight{first, second}, rest...)
}

func weightedCluster(name string, weight uint32) *envoy_api_v2_route.WeightedCluster_ClusterWeight {
	return &envoy_api_v2_route.WeightedCluster_ClusterWeight{
		Name:   name,
		Weight: protobuf.UInt32(weight),
	}
}

func routeConfigurations(rcs ...*v2.RouteConfiguration) map[string]*v2.RouteConfiguration {
	m := make(map[string]*v2.RouteConfiguration)
	for _, rc := range rcs {
		m[rc.Name] = rc
	}
	return m
}

func withMirrorPolicy(route *envoy_api_v2_route.Route_Route, mirror string) *envoy_api_v2_route.Route_Route {
	route.Route.RequestMirrorPolicy = &envoy_api_v2_route.RouteAction_RequestMirrorPolicy{
		Cluster: mirror,
	}
	return route
}
