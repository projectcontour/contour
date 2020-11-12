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

package v2

import (
	"path"
	"testing"
	"time"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/golang/protobuf/proto"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v2 "github.com/projectcontour/contour/internal/envoy/v2"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestListenerCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_api_v2.Listener
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
			want: []proto.Message{
				&envoy_api_v2.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
					SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var lc ListenerCache
			lc.Update(tc.contents)
			got := lc.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestListenerCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_api_v2.Listener
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
			query: []string{ENVOY_HTTP_LISTENER},
			want: []proto.Message{
				&envoy_api_v2.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
					SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
				},
			},
		},
		"partial match": {
			contents: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
			query: []string{ENVOY_HTTP_LISTENER, "stats-listener"},
			want: []proto.Message{
				&envoy_api_v2.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
					SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
				},
			},
		},
		"no match": {
			contents: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
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
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestListenerVisit(t *testing.T) {
	httpsFilterFor := func(vhost string) *envoy_api_v2_listener.Filter {
		return envoy_v2.HTTPConnectionManagerBuilder().
			AddFilter(envoy_v2.FilterMisdirectedRequests(vhost)).
			DefaultFilters().
			MetricsPrefix(ENVOY_HTTPS_LISTENER).
			RouteConfigName(path.Join("https", vhost)).
			AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
			Get()
	}

	fallbackCertFilter := envoy_v2.HTTPConnectionManagerBuilder().
		DefaultFilters().
		MetricsPrefix(ENVOY_HTTPS_LISTENER).
		RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
		AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
		Get()

	tests := map[string]struct {
		ListenerConfig
		fallbackCertificate *types.NamespacedName
		objs                []interface{}
		want                map[string]*envoy_api_v2.Listener
	}{
		"nothing": {
			objs: nil,
			want: map[string]*envoy_api_v2.Listener{},
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
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
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"sortedfirst.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("sortedfirst.example.com")),
				}, {
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"sortedsecond.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("sortedsecond.example.com")),
				}},
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
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
			want: map[string]*envoy_api_v2.Listener{},
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"http listener on non default port": { // issue 72
			ListenerConfig: ListenerConfig{
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("127.0.0.100", 9100),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("127.0.0.200", 9200),
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"use proxy proto": {
			ListenerConfig: ListenerConfig{
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.ProxyProtocol(),
				),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.ProxyProtocol(),
					envoy_v2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"--envoy-http-access-log": {
			ListenerConfig: ListenerConfig{
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress(DEFAULT_HTTP_LISTENER_ADDRESS, DEFAULT_HTTP_LISTENER_PORT),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy("/tmp/http_access.log"), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress(DEFAULT_HTTPS_LISTENER_ADDRESS, DEFAULT_HTTPS_LISTENER_PORT),
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters: envoy_v2.Filters(envoy_v2.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v2.FilterMisdirectedRequests("whatever.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "whatever.example.com")).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy("/tmp/https_access.log")).
						Get()),
				}},
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.3",
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by annotation": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.3",
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v2.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by legacy annotation": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.3",
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v2.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by httpproxy": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.3",
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
								SecretName:             "secret",
								MinimumProtocolVersion: "1.2",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_3, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v2.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
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
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("www.example.com")),
				}, {
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(fallbackCertFilter),
					Name:            "fallback-certificate",
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"multiple httpproxies with fallback certificate": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple2",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.another.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
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
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{
					{
						FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
							ServerNames: []string{"www.another.com"},
						},
						TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
						Filters:         envoy_v2.Filters(httpsFilterFor("www.another.com")),
					},
					{
						FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
							ServerNames: []string{"www.example.com"},
						},
						TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
						Filters:         envoy_v2.Filters(httpsFilterFor("www.example.com")),
					},
					{
						FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
							TransportProtocol: "tls",
						},
						TransportSocket: transportSocket("fallbacksecret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
						Filters:         envoy_v2.Filters(fallbackCertFilter),
						Name:            "fallback-certificate",
					}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate - no cert passed": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "",
				Namespace: "",
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
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
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
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbackcert",
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
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
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
			want: listenermap(&envoy_api_v2.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v2.FilterChains(envoy_v2.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG), 0)),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters:         envoy_v2.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with connection idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionIdleTimeout: timeout.DurationSetting(90 * time.Second),
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
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(
					envoy_v2.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with stream idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				StreamIdleTimeout: timeout.DurationSetting(90 * time.Second),
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
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(
					envoy_v2.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with max connection duration set in visitor config": {
			ListenerConfig: ListenerConfig{
				MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
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
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(
					envoy_v2.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with connection shutdown grace period set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
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
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(
					envoy_v2.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						DefaultFilters().
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with connection idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionIdleTimeout: timeout.DurationSetting(90 * time.Second),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(envoy_v2.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters: envoy_v2.Filters(envoy_v2.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v2.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with stream idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				StreamIdleTimeout: timeout.DurationSetting(90 * time.Second),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(envoy_v2.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters: envoy_v2.Filters(envoy_v2.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v2.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with max connection duration set in visitor config": {
			ListenerConfig: ListenerConfig{
				MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(envoy_v2.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters: envoy_v2.Filters(envoy_v2.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v2.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with connection shutdown grace period set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
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
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_api_v2.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v2.FilterChains(envoy_v2.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}, &envoy_api_v2.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_api_v2_listener.FilterChain{{
					FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2, "h2", "http/1.1"),
					Filters: envoy_v2.Filters(envoy_v2.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v2.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG)).
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v2.ListenerFilters(
					envoy_v2.TLSInspector(),
				),
				SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := buildDAGFallback(t, tc.fallbackCertificate, tc.objs...)
			got := visitListeners(root, &tc.ListenerConfig)
			protobuf.ExpectEqual(t, tc.want, got)
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
	return envoy_v2.DownstreamTLSTransportSocket(
		envoy_v2.DownstreamTLSContext(secret, tlsMinProtoVersion, nil, alpnprotos...),
	)
}

func listenermap(listeners ...*envoy_api_v2.Listener) map[string]*envoy_api_v2.Listener {
	m := make(map[string]*envoy_api_v2.Listener)
	for _, l := range listeners {
		m[l.Name] = l
	}
	return m
}
