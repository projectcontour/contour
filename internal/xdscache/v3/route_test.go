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

package v3

import (
	"net/http"
	"testing"
	"time"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_cors_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_config_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRouteCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_route_v3.RouteConfiguration
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: map[string]*envoy_route_v3.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
				"ingress_https": {
					Name: "ingress_https",
				},
			},
			want: []proto.Message{
				&envoy_route_v3.RouteConfiguration{
					Name: "ingress_http",
				},
				&envoy_route_v3.RouteConfiguration{
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
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestRouteCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_route_v3.RouteConfiguration
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: map[string]*envoy_route_v3.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
			},
			query: []string{"ingress_http"},
			want: []proto.Message{
				&envoy_route_v3.RouteConfiguration{
					Name: "ingress_http",
				},
			},
		},
		"partial match": {
			contents: map[string]*envoy_route_v3.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
			},
			query: []string{"stats-handler", "ingress_http"},
			want: []proto.Message{
				&envoy_route_v3.RouteConfiguration{
					Name: "ingress_http",
				},
				&envoy_route_v3.RouteConfiguration{
					Name: "stats-handler",
				},
			},
		},
		"no match": {
			contents: map[string]*envoy_route_v3.RouteConfiguration{
				"ingress_http": {
					Name: "ingress_http",
				},
			},
			query: []string{"stats-handler"},
			want: []proto.Message{
				&envoy_route_v3.RouteConfiguration{
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
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestRouteVisit(t *testing.T) {
	tests := map[string]struct {
		objs                []interface{}
		fallbackCertificate *types.NamespacedName
		want                map[string]*envoy_route_v3.RouteConfiguration
	}{
		"nothing": {
			objs: nil,
			want: routeConfigurations(
				envoy_v3.RouteConfiguration("ingress_http"),
			),
		},
		"one http only ingress with service": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"one http only ingress with regex match": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{{
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Path:    "/[^/]+/invoices(/.*|/?)", // issue 1243
										Backend: *backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routeRegex("/[^/]+/invoices(/.*|/?)"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"one http only httpproxy": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					),
				),
			),
		},
		"default backend ingress with secret": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"vhost ingress with secret": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"www.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "kuard",
												Port: networking_v1.ServiceBackendPort{Name: "www"},
											},
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"simple httpproxy with secret": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 8080,
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
							Name:       "www",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
			},
			want: routeConfigurations(
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/backend/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"simple tls ingress with allow-http:false": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.allow-http": "false",
						},
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"www.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "kuard",
												Port: networking_v1.ServiceBackendPort{Name: "www"},
											},
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
				envoy_v3.RouteConfiguration("ingress_http"),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"simple tls ingress with force-ssl-redirect": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"ingress.kubernetes.io/force-ssl-redirect": "true",
						},
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"www.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "kuard",
												Port: networking_v1.ServiceBackendPort{Name: "www"},
											},
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"ingress with websocket annotation": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/websocket-routes": "/ws1 , /ws2",
						},
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Path: "/",
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "kuard",
												Port: networking_v1.ServiceBackendPort{Name: "www"},
											},
										},
									}, {
										Path: "/ws1",
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "kuard",
												Port: networking_v1.ServiceBackendPort{Name: "www"},
											},
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/ws1"),
							Action: websocketroute("default/kuard/8080/da39a3ee5e"),
						},
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"ingress invalid timeout": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour/request-timeout": "contour",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"ingress infinite timeout": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/request-timeout": "infinity",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routetimeout("default/kuard/8080/da39a3ee5e", 0),
						},
					),
				),
			),
		},
		"ingress 90 second timeout": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/request-timeout": "1m30s",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routetimeout("default/kuard/8080/da39a3ee5e", 90*time.Second),
						},
					),
				),
			),
		},
		"ingress different path matches": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{{
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: (*networking_v1.PathType)(ref.To("Prefix")),
											Backend:  *backend("kuard", 8080),
										},
										{
											Path:     "/foo",
											PathType: (*networking_v1.PathType)(ref.To("Prefix")),
											Backend:  *backend("kuard", 8080),
										},
										{
											Path:     "/foo",
											PathType: (*networking_v1.PathType)(ref.To("ImplementationSpecific")),
											Backend:  *backend("kuard", 8080),
										},
										{
											Path:     "/foo2",
											PathType: (*networking_v1.PathType)(ref.To("ImplementationSpecific")),
											Backend:  *backend("kuard", 8080),
										},
										{
											Path:     "/foo3[a|b]?",
											PathType: (*networking_v1.PathType)(ref.To("ImplementationSpecific")),
											Backend:  *backend("kuard", 8080),
										},
										{
											Path:     "/foo4",
											PathType: (*networking_v1.PathType)(ref.To("Exact")),
											Backend:  *backend("kuard", 8080),
										},
									},
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routeExact("/foo4"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
						&envoy_route_v3.Route{
							Match:  routeRegex("/foo3[a|b]?"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
						&envoy_route_v3.Route{
							Match:  routePrefix("/foo2"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
						&envoy_route_v3.Route{
							Match:  routePrefixIngress("/foo"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
						&envoy_route_v3.Route{
							Match:  routePrefix("/foo"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/8080/da39a3ee5e"),
						},
					),
				),
			),
		},
		"vhost name exceeds 60 chars": { // projectcontour/contour#25
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-service-name",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{{
							Host: "my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Path: "/",
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "kuard",
												Port: networking_v1.ServiceBackendPort{Name: "www"},
											},
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/kuard/80/da39a3ee5e"),
						},
					),
				),
			),
		},
		"ingress retry-on": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/retry-on": "5xx,gateway-error",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeretry("default/kuard/8080/da39a3ee5e", "5xx,gateway-error", 1, 0),
						},
					),
				),
			),
		},
		"ingress retry-on, num-retries": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/retry-on":    "5xx,gateway-error",
							"projectcontour.io/num-retries": "7",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeretry("default/kuard/8080/da39a3ee5e", "5xx,gateway-error", 7, 0),
						},
					),
				),
			),
		},

		"ingress num-retries disabled": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/retry-on":    "5xx,gateway-error",
							"projectcontour.io/num-retries": "-1",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeretry("default/kuard/8080/da39a3ee5e", "5xx,gateway-error", 0, 0),
						},
					),
				),
			),
		},

		"ingress retry-on, per-try-timeout": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/retry-on":        "5xx,gateway-error",
							"projectcontour.io/per-try-timeout": "150ms",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("*",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeretry("default/kuard/8080/da39a3ee5e", "5xx,gateway-error", 1, 150*time.Millisecond),
						},
					),
				),
			),
		},

		"httpproxy num-retries disabled": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
							RetryPolicy: &contour_api_v1.RetryPolicy{
								NumRetries: -1,
								RetryOn:    []contour_api_v1.RetryOn{"5xx", "gateway-error"},
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeretry("default/backend/80/da39a3ee5e", "5xx,gateway-error", 0, 0),
						},
					),
				),
			),
		},

		"httpproxy no weights defined": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					),
				),
			),
		},
		"httpproxy one weight defined": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 0),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 50),
											),
										},
									},
								},
							},
						},
					),
				),
			),
		},
		"httpproxy all weights defined": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 22),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 50),
											),
										},
									},
								},
							},
						},
					),
				),
			),
		},
		"httpproxy w/ missing fqdn": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http"), // should be blank, no fqdn defined.
			),
		},
		"httpproxy with pathPrefix": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					),
				),
			),
		},
		"httpproxy with mirror policy": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: withMirrorPolicy(routecluster("default/backend/80/da39a3ee5e"), "default/backendtwo/80/da39a3ee5e"),
						},
					),
				),
			),
		},
		"httpproxy with pathPrefix with tls": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
			),
		},
		"httpproxy with pathPrefix includes": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Includes: []contour_api_v1.Include{{
							Name:      "child",
							Namespace: "teama",
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/blog",
							}},
						}},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backendtwo",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child",
						Namespace: "teama",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/info",
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/blog/info"),
							Action: routecluster("teama/backend/80/da39a3ee5e"),
						},
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					),
				),
			),
		},
		"httpproxy with corsPolicy": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							CORSPolicy: &contour_api_v1.CORSPolicy{
								AllowOrigin:  []string{"*"},
								AllowMethods: []contour_api_v1.CORSHeaderValue{"GET, PUT, POST"},
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.CORSVirtualHost("www.example.com",
						&envoy_cors_v3.CorsPolicy{
							AllowCredentials:          &wrapperspb.BoolValue{Value: false},
							AllowPrivateNetworkAccess: &wrapperspb.BoolValue{Value: false},
							AllowOriginStringMatch: []*matcher.StringMatcher{{
								MatchPattern: &matcher.StringMatcher_Exact{
									Exact: "*",
								},
								IgnoreCase: true,
							}},
							AllowMethods: "GET, PUT, POST",
						},
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					),
				),
			),
		},
		"httpproxy with corsPolicy with tls": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							CORSPolicy: &contour_api_v1.CORSPolicy{
								AllowOrigin:  []string{"*"},
								AllowMethods: []contour_api_v1.CORSHeaderValue{"GET, PUT, POST"},
							},
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
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
			},
			want: routeConfigurations(
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.CORSVirtualHost("www.example.com",
						&envoy_cors_v3.CorsPolicy{
							AllowCredentials:          &wrapperspb.BoolValue{Value: false},
							AllowPrivateNetworkAccess: &wrapperspb.BoolValue{Value: false},
							AllowOriginStringMatch: []*matcher.StringMatcher{{
								MatchPattern: &matcher.StringMatcher_Exact{
									Exact: "*",
								},
								IgnoreCase: true,
							}},
							AllowMethods: "GET, PUT, POST",
						},
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.CORSVirtualHost("www.example.com",
						&envoy_cors_v3.CorsPolicy{
							AllowCredentials:          &wrapperspb.BoolValue{Value: false},
							AllowPrivateNetworkAccess: &wrapperspb.BoolValue{Value: false},
							AllowOriginStringMatch: []*matcher.StringMatcher{{
								MatchPattern: &matcher.StringMatcher_Exact{
									Exact: "*",
								},
								IgnoreCase: true,
							}},
							AllowMethods: "GET, PUT, POST",
						},
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with header contains conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}, {
								Header: &contour_api_v1.HeaderMatchCondition{
									Name:     "x-header",
									Contains: "abc",
								},
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithHeaderConditions("/", dag.HeaderMatchCondition{
								Name:      "x-header",
								Value:     "abc",
								MatchType: "contains",
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with header notcontains conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									Header: &contour_api_v1.HeaderMatchCondition{
										Name:        "x-header",
										NotContains: "abc",
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithHeaderConditions("/", dag.HeaderMatchCondition{
								Name:      "x-header",
								Value:     "abc",
								MatchType: "contains",
								Invert:    true,
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with header exact match conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									Header: &contour_api_v1.HeaderMatchCondition{
										Name:  "x-header",
										Exact: "abc",
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithHeaderConditions("/", dag.HeaderMatchCondition{
								Name:      "x-header",
								Value:     "abc",
								MatchType: "exact",
								Invert:    false,
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with header exact not match conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									Header: &contour_api_v1.HeaderMatchCondition{
										Name:     "x-header",
										NotExact: "abc",
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithHeaderConditions("/", dag.HeaderMatchCondition{
								Name:      "x-header",
								Value:     "abc",
								MatchType: "exact",
								Invert:    true,
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with header present conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									Header: &contour_api_v1.HeaderMatchCondition{
										Name:    "x-header",
										Present: true,
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithHeaderConditions("/", dag.HeaderMatchCondition{
								Name:      "x-header",
								MatchType: "present",
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with query parameter contains conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}, {
								QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
									Name:     "param",
									Contains: "abc",
								},
							}},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
								Name:      "param",
								Value:     "abc",
								MatchType: "contains",
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with header prefix conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
										Name:   "param",
										Prefix: "abc",
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
								Name:      "param",
								Value:     "abc",
								MatchType: "prefix",
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with header suffix conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
										Name:   "param",
										Suffix: "abc",
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
								Name:      "param",
								Value:     "abc",
								MatchType: "suffix",
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with query parameter exact match conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
										Name:       "param",
										Exact:      "abc",
										IgnoreCase: true,
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
								Name:       "param",
								Value:      "abc",
								MatchType:  "exact",
								IgnoreCase: true,
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with query parameter regex match conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
										Name:  "param",
										Regex: "^abc.*",
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
								Name:      "param",
								Value:     "^abc.*",
								MatchType: "regex",
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with query parameter present conditions": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{
								{
									Prefix: "/",
								},
								{
									QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
										Name:    "param",
										Present: true,
									},
								},
							},
							Services: []contour_api_v1.Service{{
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefixWithQueryParameterConditions("/", dag.QueryParamMatchCondition{
								Name:      "param",
								MatchType: "present",
							}),
							Action: routecluster("default/backend/80/da39a3ee5e"),
						},
					)),
			),
		},
		"httpproxy with route-level header manipulation": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
							RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
								Set: []contour_api_v1.HeaderValue{{
									Name:  "In-Foo",
									Value: "bar",
								}},
								Remove: []string{
									"In-Baz",
								},
							},
							ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
								Set: []contour_api_v1.HeaderValue{{
									Name:  "Out-Foo",
									Value: "bar",
								}},
								Remove: []string{
									"Out-Baz",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
										Cluster: "default/backend/80/da39a3ee5e",
									},
								},
							},
							RequestHeadersToAdd: []*envoy_core_v3.HeaderValueOption{{
								Header: &envoy_core_v3.HeaderValue{
									Key:   "In-Foo",
									Value: "bar",
								},
								AppendAction: envoy_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
							}},
							RequestHeadersToRemove: []string{"In-Baz"},
							ResponseHeadersToAdd: []*envoy_core_v3.HeaderValueOption{{
								Header: &envoy_core_v3.HeaderValue{
									Key:   "Out-Foo",
									Value: "bar",
								},
								AppendAction: envoy_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
							}},
							ResponseHeadersToRemove: []string{"Out-Baz"},
						},
					),
				),
			),
		},
		"httpproxy with fallback certificate": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
				envoy_v3.RouteConfiguration(ENVOY_FALLBACK_ROUTECONFIG,
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
			),
		},
		"httpproxy with fallback certificate - one enabled": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backendtwo",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-enabled",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "projectcontour.io",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("projectcontour.io",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/projectcontour.io",
					envoy_v3.VirtualHost("projectcontour.io",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
				envoy_v3.RouteConfiguration(ENVOY_FALLBACK_ROUTECONFIG,
					envoy_v3.VirtualHost("projectcontour.io",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
			),
		},
		"httpproxy with fallback certificate - two enabled": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}, {
								Name: "backendtwo",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-enabled",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "projectcontour.io",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("projectcontour.io",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/projectcontour.io",
					envoy_v3.VirtualHost("projectcontour.io",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
				envoy_v3.RouteConfiguration(ENVOY_FALLBACK_ROUTECONFIG,
					envoy_v3.VirtualHost("projectcontour.io",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					), envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
			),
		},
		"httpproxy with fallback certificate - bad global cert": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "badnamespace",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
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
			want: routeConfigurations(envoy_v3.RouteConfiguration("ingress_http")),
		},
		"httpproxy with fallback certificate - no fqdn enabled": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Route{
								Route: &envoy_route_v3.RouteAction{
									ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
										WeightedClusters: &envoy_route_v3.WeightedCluster{
											Clusters: weightedClusters(
												weightedCluster("default/backend/80/da39a3ee5e", 1),
												weightedCluster("default/backendtwo/80/da39a3ee5e", 1),
											),
										},
									},
								},
							},
						},
					)),
			),
		},
		"direct response on configuration error": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "missing-backend-service",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: routeConfigurations(
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_DirectResponse{
								DirectResponse: &envoy_route_v3.DirectResponseAction{
									Status: http.StatusServiceUnavailable,
								},
							},
						},
					),
				),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var rc RouteCache
			rc.OnChange(buildDAGFallback(t, tc.fallbackCertificate, tc.objs...))
			protobuf.ExpectEqual(t, tc.want, rc.values)
		})
	}
}

func TestRouteVisit_GlobalExternalAuthorization(t *testing.T) {
	tests := map[string]struct {
		objs                []interface{}
		fallbackCertificate *types.NamespacedName
		want                map[string]*envoy_route_v3.RouteConfiguration
	}{
		"HTTP virtual host, authcontext override": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
							AuthPolicy: &contour_api_v1.AuthorizationPolicy{
								Context: map[string]string{
									"header_2": "new_message_2",
									"header_3": "message_3",
								},
							},
						}},
					},
				},
				&contour_api_v1alpha1.ExtensionService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "ns",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/backend/80/da39a3ee5e"),
							TypedPerFilterConfig: map[string]*anypb.Any{
								"envoy.filters.http.ext_authz": protobuf.MustMarshalAny(&envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute{
									Override: &envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
										CheckSettings: &envoy_config_filter_http_ext_authz_v3.CheckSettings{
											ContextExtensions: map[string]string{
												"header_1": "message_1",
												"header_2": "new_message_2",
												"header_3": "message_3",
											},
										},
									},
								}),
							},
						},
					),
				),
			),
		},
		"HTTP virtual host, auth disabled for a route": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
							AuthPolicy: &contour_api_v1.AuthorizationPolicy{
								Disabled: true,
							},
						}},
					},
				},
				&contour_api_v1alpha1.ExtensionService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "ns",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/backend/80/da39a3ee5e"),
							TypedPerFilterConfig: map[string]*anypb.Any{
								"envoy.filters.http.ext_authz": protobuf.MustMarshalAny(&envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute{
									Override: &envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute_Disabled{
										Disabled: true,
									},
								}),
							},
						},
					),
				),
			),
		},
		"HTTPs virtual host, authcontext override": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
							AuthPolicy: &contour_api_v1.AuthorizationPolicy{
								Context: map[string]string{
									"header_2": "new_message_2",
									"header_3": "message_3",
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
				&contour_api_v1alpha1.ExtensionService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "ns",
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
				envoy_v3.RouteConfiguration("ingress_http",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match: routePrefix("/"),
							Action: &envoy_route_v3.Route_Redirect{
								Redirect: &envoy_route_v3.RedirectAction{
									SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
										HttpsRedirect: true,
									},
								},
							},
						},
					),
				),
				envoy_v3.RouteConfiguration("https/www.example.com",
					envoy_v3.VirtualHost("www.example.com",
						&envoy_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routecluster("default/backend/80/da39a3ee5e"),
							TypedPerFilterConfig: map[string]*anypb.Any{
								"envoy.filters.http.ext_authz": protobuf.MustMarshalAny(&envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute{
									Override: &envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
										CheckSettings: &envoy_config_filter_http_ext_authz_v3.CheckSettings{
											ContextExtensions: map[string]string{
												"header_1": "message_1",
												"header_2": "new_message_2",
												"header_3": "message_3",
											},
										},
									},
								}),
							},
						},
					),
				)),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var rc RouteCache
			rc.OnChange(buildDAGGlobalExtAuth(t, tc.fallbackCertificate, tc.objs...))
			protobuf.ExpectEqual(t, tc.want, rc.values)
		})
	}
}

func TestSortLongestRouteFirst(t *testing.T) {
	tests := map[string]struct {
		routes []*dag.Route
		want   []*dag.Route
	}{
		"two prefixes": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/longer"},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/longer"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}},
		},
		"two regexes": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.RegexMatchCondition{Regex: "/v2"},
			}, {
				PathMatchCondition: &dag.RegexMatchCondition{Regex: "/v1/.+"},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.RegexMatchCondition{Regex: "/v2"},
			}, {
				PathMatchCondition: &dag.RegexMatchCondition{Regex: "/v1/.+"},
			}},
		},
		"two exact matches": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.ExactMatchCondition{Path: "/foo"},
			}, {
				PathMatchCondition: &dag.ExactMatchCondition{Path: "/foo/"},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.ExactMatchCondition{Path: "/foo/"},
			}, {
				PathMatchCondition: &dag.ExactMatchCondition{Path: "/foo"},
			}},
		},
		"exact sorts before regex sorts before prefix": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.RegexMatchCondition{Regex: "/api/v?"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}, {
				PathMatchCondition: &dag.RegexMatchCondition{Regex: ".*"},
			}, {
				PathMatchCondition: &dag.ExactMatchCondition{Path: "/api/"},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.ExactMatchCondition{Path: "/api/"},
			}, {
				PathMatchCondition: &dag.RegexMatchCondition{Regex: "/api/v?"},
			}, {
				PathMatchCondition: &dag.RegexMatchCondition{Regex: ".*"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}},
		},
		"more headers sort before less": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-id", MatchType: "present"},
				},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-id", MatchType: "present"},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}},
		},

		// Verify that longest path sorts before longest
		// headers. We used to sort by longest header list
		// first, which does end up with the same net result,
		// so this isn't strictly necessary.  However, ordering
		// the path first is arguably more intuitive, and
		// allows us to avoid comparing the header matches
		// unless necessary.
		"longest path before longest headers": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-id", MatchType: "present"},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/longest/path/match"},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/longest/path/match"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-id", MatchType: "present"},
				},
			}},
		},

		// The path and the length of header condition list are equal,
		// so we should order lexicographically by header name.
		"headers sort stably by name": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "zzz-2", MatchType: "present"},
					{Name: "zzz-1", MatchType: "present"},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "aaa-2", MatchType: "present"},
					{Name: "aaa-1", MatchType: "present"},
				},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "aaa-1", MatchType: "present"},
					{Name: "aaa-2", MatchType: "present"},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "zzz-1", MatchType: "present"},
					{Name: "zzz-2", MatchType: "present"},
				},
			}},
		},

		// The path and the length of query param condition list are equal,
		// so we should order lexicographically by query param name.
		"query params sort stably by name": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				QueryParamMatchConditions: []dag.QueryParamMatchCondition{
					{Name: "zzz-2", MatchType: "present"},
					{Name: "zzz-1", MatchType: "present"},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				QueryParamMatchConditions: []dag.QueryParamMatchCondition{
					{Name: "aaa-2", MatchType: "present"},
					{Name: "aaa-1", MatchType: "present"},
				},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				QueryParamMatchConditions: []dag.QueryParamMatchCondition{
					{Name: "aaa-1", MatchType: "present"},
					{Name: "aaa-2", MatchType: "present"},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				QueryParamMatchConditions: []dag.QueryParamMatchCondition{
					{Name: "zzz-1", MatchType: "present"},
					{Name: "zzz-2", MatchType: "present"},
				},
			}},
		},

		// If we have multiple conditions on the same header, ensure
		// that we order on the match type too.
		"headers order by match type": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-1", MatchType: "present"},
					{Name: "x-request-2", MatchType: "present", Invert: true},
					{Name: "x-request-1", MatchType: "exact", Value: "foo"},
				},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-1", MatchType: "exact", Value: "foo"},
					{Name: "x-request-1", MatchType: "present"},
					{Name: "x-request-2", MatchType: "present", Invert: true},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}},
		},

		// If we have multiple conditions on the same query param, ensure
		// that we order on the match type too.
		"query params order by match type": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "param-1", MatchType: "present"},
					{Name: "param-2", MatchType: "present", Invert: true},
					{Name: "param-1", MatchType: "exact", Value: "foo"},
				},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "param-1", MatchType: "exact", Value: "foo"},
					{Name: "param-1", MatchType: "present"},
					{Name: "param-2", MatchType: "present", Invert: true},
				},
			}, {
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
			}},
		},

		// Verify that we always order the headers, even if
		// we don't need to compare the header conditions to
		// order multiple routes with the same prefix.
		"headers order in single route": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-1", MatchType: "present"},
					{Name: "x-request-2", MatchType: "present", Invert: true},
					{Name: "x-request-1", MatchType: "exact", Value: "foo"},
				},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "x-request-1", MatchType: "exact", Value: "foo"},
					{Name: "x-request-1", MatchType: "present"},
					{Name: "x-request-2", MatchType: "present", Invert: true},
				},
			}},
		},

		// Verify that we always order the query params, even if
		// we don't need to compare the conditions to
		// order multiple routes with the same prefix.
		"query params order in single route": {
			routes: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "param-1", MatchType: "present"},
					{Name: "param-2", MatchType: "present", Invert: true},
					{Name: "param-1", MatchType: "exact", Value: "foo"},
				},
			}},
			want: []*dag.Route{{
				PathMatchCondition: &dag.PrefixMatchCondition{Prefix: "/"},
				HeaderMatchConditions: []dag.HeaderMatchCondition{
					{Name: "param-1", MatchType: "exact", Value: "foo"},
					{Name: "param-1", MatchType: "present"},
					{Name: "param-2", MatchType: "present", Invert: true},
				},
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := append([]*dag.Route{}, tc.routes...) // shallow copy
			sortRoutes(got)
			assert.Equal(t, tc.want, got)
		})
	}
}

func routecluster(cluster string) *envoy_route_v3.Route_Route {
	return &envoy_route_v3.Route_Route{
		Route: &envoy_route_v3.RouteAction{
			ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}

}

func websocketroute(c string) *envoy_route_v3.Route_Route {
	r := routecluster(c)
	r.Route.UpgradeConfigs = append(r.Route.UpgradeConfigs,
		&envoy_route_v3.RouteAction_UpgradeConfig{
			UpgradeType: "websocket",
		},
	)
	return r
}

func routetimeout(cluster string, timeout time.Duration) *envoy_route_v3.Route_Route {
	r := routecluster(cluster)
	r.Route.Timeout = durationpb.New(timeout)
	return r
}

func routeretry(cluster string, retryOn string, numRetries uint32, perTryTimeout time.Duration) *envoy_route_v3.Route_Route {
	r := routecluster(cluster)
	r.Route.RetryPolicy = &envoy_route_v3.RetryPolicy{
		RetryOn: retryOn,
	}
	if numRetries > 0 {
		r.Route.RetryPolicy.NumRetries = wrapperspb.UInt32(numRetries)
	}
	if perTryTimeout > 0 {
		r.Route.RetryPolicy.PerTryTimeout = durationpb.New(perTryTimeout)
	}
	return r
}

func routeRegex(regex string, headers ...dag.HeaderMatchCondition) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.RegexMatchCondition{
			Regex: regex,
		},
		HeaderMatchConditions: headers,
	})
}

func routePrefixIngress(prefix string, headers ...dag.HeaderMatchCondition) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix:          prefix,
			PrefixMatchType: dag.PrefixMatchSegment,
		},
		HeaderMatchConditions: headers,
	})
}

func routePrefix(prefix string) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
	})
}

func routePrefixWithHeaderConditions(prefix string, headers ...dag.HeaderMatchCondition) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
		HeaderMatchConditions: headers,
	})
}

func routePrefixWithQueryParameterConditions(prefix string, queryParams ...dag.QueryParamMatchCondition) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
		QueryParamMatchConditions: queryParams,
	})
}

func routeExact(path string, headers ...dag.HeaderMatchCondition) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.ExactMatchCondition{
			Path: path,
		},
		HeaderMatchConditions: headers,
	})
}

func weightedClusters(first, second *envoy_route_v3.WeightedCluster_ClusterWeight, rest ...*envoy_route_v3.WeightedCluster_ClusterWeight) []*envoy_route_v3.WeightedCluster_ClusterWeight {
	return append([]*envoy_route_v3.WeightedCluster_ClusterWeight{first, second}, rest...)
}

func weightedCluster(name string, weight uint32) *envoy_route_v3.WeightedCluster_ClusterWeight {
	return &envoy_route_v3.WeightedCluster_ClusterWeight{
		Name:   name,
		Weight: wrapperspb.UInt32(weight),
	}
}

func routeConfigurations(rcs ...*envoy_route_v3.RouteConfiguration) map[string]*envoy_route_v3.RouteConfiguration {
	m := make(map[string]*envoy_route_v3.RouteConfiguration)
	for _, rc := range rcs {
		m[rc.Name] = rc
	}
	return m
}

func withMirrorPolicy(route *envoy_route_v3.Route_Route, mirror string) *envoy_route_v3.Route_Route {
	route.Route.RequestMirrorPolicies = []*envoy_route_v3.RouteAction_RequestMirrorPolicy{{
		Cluster: mirror,
	}}
	return route
}

// buildDAGGlobalExtAuth produces a dag.DAG from the supplied objects with global external authorization configured.
func buildDAGGlobalExtAuth(t *testing.T, fallbackCertificate *types.NamespacedName, objs ...interface{}) *dag.DAG {
	builder := dag.Builder{
		Source: dag.KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		},
		Processors: []dag.Processor{
			&dag.ListenerProcessor{},
			&dag.ExtensionServiceProcessor{},
			&dag.IngressProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			},
			&dag.HTTPProxyProcessor{
				FallbackCertificate: fallbackCertificate,
				GlobalExternalAuthorization: &contour_api_v1.AuthorizationServer{
					ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
						Name:      "test",
						Namespace: "ns",
					},
					FailOpen: false,
					AuthPolicy: &contour_api_v1.AuthorizationPolicy{
						Context: map[string]string{
							"header_1": "message_1",
							"header_2": "message_2",
						},
					},
					ResponseTimeout: "10s",
				},
			},
		},
	}

	for _, o := range objs {
		builder.Source.Insert(o)
	}

	return builder.Build()
}
