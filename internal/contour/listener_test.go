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

package contour

import (
	"path"
	"testing"
	"time"

	"github.com/projectcontour/contour/internal/k8s"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/golang/protobuf/proto"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestListenerCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*v2.Listener
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
			want: []proto.Message{
				&v2.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
					SocketOptions: envoy.TCPKeepaliveSocketOptions(),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var lc ListenerCache
			lc.Update(tc.contents)
			got := lc.Contents()
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestListenerCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*v2.Listener
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
			query: []string{ENVOY_HTTP_LISTENER},
			want: []proto.Message{
				&v2.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
					SocketOptions: envoy.TCPKeepaliveSocketOptions(),
				},
			},
		},
		"partial match": {
			contents: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
			query: []string{ENVOY_HTTP_LISTENER, "stats-listener"},
			want: []proto.Message{
				&v2.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
					SocketOptions: envoy.TCPKeepaliveSocketOptions(),
				},
			},
		},
		"no match": {
			contents: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
			query: []string{"stats-listener"},
			want:  nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var lc ListenerCache
			lc.Update(tc.contents)
			got := lc.Query(tc.query)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestListenerVisit(t *testing.T) {
	httpsFilterFor := func(vhost string) *envoy_api_v2_listener.Filter {
		return envoy.HTTPConnectionManagerBuilder().
			AddFilter(envoy.FilterMisdirectedRequests(vhost)).
			DefaultFilters().
			MetricsPrefix(ENVOY_HTTPS_LISTENER).
			RouteConfigName(path.Join("https", vhost)).
			AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
			Get()
	}

	fallbackCertFilter := envoy.HTTPConnectionManagerBuilder().
		MetricsPrefix(ENVOY_HTTPS_LISTENER).
		RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
		AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
		Get()

	tests := map[string]struct {
		ListenerVisitorConfig
		fallbackCertificate *k8s.FullName
		objs                []interface{}
		want                map[string]*v2.Listener
	}{
		"nothing": {
			objs: nil,
			want: map[string]*v2.Listener{},
		},
		"one http only ingress": {
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"one http only httpproxy": {
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
							Conditions: []projcontour.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"simple ingress with secret": {
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
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"multiple tls ingress with secrets should be sorted": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sortedsecond",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"sortedsecond.example.com"},
							SecretName: "secret",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "sortedsecond.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sortedfirst",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"sortedfirst.example.com"},
							SecretName: "secret",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "sortedfirst.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"sortedfirst.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("sortedfirst.example.com")),
				}, {
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"sortedsecond.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("sortedsecond.example.com")),
				}},
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"simple ingress with missing secret": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "missing",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"simple httpproxy with secret": {
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
							Services: []projcontour.Service{{
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"ingress with allow-http: false": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.allow-http": "false",
						},
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("kuard", 8080),
					},
				},
			},
			want: map[string]*v2.Listener{},
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
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"http listener on non default port": { // issue 72
			ListenerVisitorConfig: ListenerVisitorConfig{
				HTTPAddress:  "127.0.0.100",
				HTTPPort:     9100,
				HTTPSAddress: "127.0.0.200",
				HTTPSPort:    9200,
			},
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
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("127.0.0.100", 9100),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("127.0.0.200", 9200),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"use proxy proto": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				UseProxyProto: true,
			},
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
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: envoy.ListenerFilters(
					envoy.ProxyProtocol(),
				),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy.ListenerFilters(
					envoy.ProxyProtocol(),
					envoy.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"--envoy-http-access-log": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				HTTPAccessLog:  "/tmp/http_access.log",
				HTTPSAccessLog: "/tmp/https_access.log",
			},
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
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress(DEFAULT_HTTP_LISTENER_ADDRESS, DEFAULT_HTTP_LISTENER_PORT),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy("/tmp/http_access.log"), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress(DEFAULT_HTTPS_LISTENER_ADDRESS, DEFAULT_HTTPS_LISTENER_PORT),
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters: envoy.Filters(envoy.HTTPConnectionManagerBuilder().
						AddFilter(envoy.FilterMisdirectedRequests("whatever.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "whatever.example.com")).
						AccessLoggers(envoy.FileAccessLogEnvoy("/tmp/https_access.log")).
						Get()),
				}},
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				MinimumTLSVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
			},
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
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by annotation": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				MinimumTLSVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
			},
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/tls-minimum-protocol-version": "1.2",
						},
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by legacy annotation": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				MinimumTLSVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
			},
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"contour.heptio.com/tls-minimum-protocol-version": "1.2",
						},
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []v1beta1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by httpproxy": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				MinimumTLSVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
			},
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
								SecretName:             "secret",
								MinimumProtocolVersion: "1.2",
							},
						},
						Routes: []projcontour.Route{{
							Services: []projcontour.Service{{
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate": {
			fallbackCertificate: &k8s.FullName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
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
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []projcontour.Route{
							{
								Services: []projcontour.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("www.example.com")),
				}, {
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(fallbackCertFilter),
					Name:            "fallback-certificate",
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"multiple httpproxies with fallback certificate": {
			fallbackCertificate: &k8s.FullName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple2",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.another.com",
							TLS: &projcontour.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []projcontour.Route{
							{
								Services: []projcontour.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&projcontour.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: projcontour.HTTPProxySpec{
						VirtualHost: &projcontour.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &projcontour.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []projcontour.Route{
							{
								Services: []projcontour.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{
					{
						FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
							ServerNames: []string{"www.another.com"},
						},
						TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
						Filters:         envoy.Filters(httpsFilterFor("www.another.com")),
					},
					{
						FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
							ServerNames: []string{"www.example.com"},
						},
						TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
						Filters:         envoy.Filters(httpsFilterFor("www.example.com")),
					},
					{
						FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
							TransportProtocol: "tls",
						},
						TransportSocket: transportSocket("fallbacksecret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
						Filters:         envoy.Filters(fallbackCertFilter),
						Name:            "fallback-certificate",
					}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate - no cert passed": {
			fallbackCertificate: &k8s.FullName{
				Name:      "",
				Namespace: "",
			},
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
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []projcontour.Route{
							{
								Services: []projcontour.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(),
		},
		"httpproxy with fallback certificate - cert passed but vhost not enabled": {
			fallbackCertificate: &k8s.FullName{
				Name:      "fallbackcert",
				Namespace: "default",
			},
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
								SecretName:                "secret",
								EnableFallbackCertificate: false,
							},
						},
						Routes: []projcontour.Route{
							{
								Services: []projcontour.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy.FilterChains(envoy.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters:         envoy.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with connection idle timeout set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				ConnectionIdleTimeout: 90 * time.Second,
			},
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
							Conditions: []projcontour.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						ConnectionIdleTimeout(90 * time.Second).
						Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with stream idle timeout set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				StreamIdleTimeout: 90 * time.Second,
			},
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
							Conditions: []projcontour.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						StreamIdleTimeout(90 * time.Second).
						Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with max connection duration set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				MaxConnectionDuration: 90 * time.Second,
			},
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
							Conditions: []projcontour.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						MaxConnectionDuration(90 * time.Second).
						Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with drain timeout set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				DrainTimeout: 90 * time.Second,
			},
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
							Conditions: []projcontour.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						DrainTimeout(90 * time.Second).
						Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with connection idle timeout set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				ConnectionIdleTimeout: 90 * time.Second,
			},
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
							Services: []projcontour.Service{{
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					ConnectionIdleTimeout(90 * time.Second).
					Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters: envoy.Filters(envoy.HTTPConnectionManagerBuilder().
						AddFilter(envoy.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						ConnectionIdleTimeout(90 * time.Second).
						Get()),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with stream idle timeout set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				StreamIdleTimeout: 90 * time.Second,
			},
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
							Services: []projcontour.Service{{
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					StreamIdleTimeout(90 * time.Second).
					Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters: envoy.Filters(envoy.HTTPConnectionManagerBuilder().
						AddFilter(envoy.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						StreamIdleTimeout(90 * time.Second).
						Get()),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with max connection duration set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				MaxConnectionDuration: 90 * time.Second,
			},
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
							Services: []projcontour.Service{{
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					MaxConnectionDuration(90 * time.Second).
					Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters: envoy.Filters(envoy.HTTPConnectionManagerBuilder().
						AddFilter(envoy.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						MaxConnectionDuration(90 * time.Second).
						Get()),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with drain timeout set in visitor config": {
			ListenerVisitorConfig: ListenerVisitorConfig{
				DrainTimeout: 90 * time.Second,
			},
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
							Services: []projcontour.Service{{
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					DrainTimeout(90 * time.Second).
					Get(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}, &v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
					Filters: envoy.Filters(envoy.HTTPConnectionManagerBuilder().
						AddFilter(envoy.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DrainTimeout(90 * time.Second).
						Get()),
				}},
				ListenerFilters: envoy.ListenerFilters(
					envoy.TLSInspector(),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := buildDAGFallback(t, tc.fallbackCertificate, tc.objs...)
			got := visitListeners(root, &tc.ListenerVisitorConfig)
			assert.Equal(t, tc.want, got)
		})
	}
}

func transportSocket(secretname string, tlsMinProtoVersion envoy_api_v2_auth.TlsParameters_TlsProtocol, alpnprotos ...string) *envoy_api_v2_core.TransportSocket {
	secret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretname,
				Namespace: "default",
			},
			Type: v1.SecretTypeTLS,
			Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
		},
	}
	return envoy.DownstreamTLSTransportSocket(
		envoy.DownstreamTLSContext(secret, tlsMinProtoVersion, nil, alpnprotos...),
	)
}

func listenermap(listeners ...*v2.Listener) map[string]*v2.Listener {
	m := make(map[string]*v2.Listener)
	for _, l := range listeners {
		m[l.Name] = l
	}
	return m
}
