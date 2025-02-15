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
	"net/url"
	"path"
	"testing"
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoy_filter_http_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/contourconfig"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

func TestListenerCacheContents(t *testing.T) {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	tests := map[string]struct {
		contents map[string]*envoy_config_listener_v3.Listener
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
			want: []proto.Message{
				&envoy_config_listener_v3.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
					SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
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

func TestListenerVisit(t *testing.T) {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	httpsFilterFor := func(vhost string) *envoy_config_listener_v3.Filter {
		return envoyGen.HTTPConnectionManagerBuilder().
			AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
			DefaultFilters().
			MetricsPrefix(ENVOY_HTTPS_LISTENER).
			RouteConfigName(path.Join("https", vhost)).
			AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
			Get()
	}

	fallbackCertFilter := envoyGen.HTTPConnectionManagerBuilder().
		DefaultFilters().
		MetricsPrefix(ENVOY_HTTPS_LISTENER).
		RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
		Get()

	jwksTimeout := "10s"
	jwksTimeoutDuration, _ := time.ParseDuration(jwksTimeout)
	jwtProvider := contour_v1.JWTProvider{
		Name:   "provider-1",
		Issuer: "issuer.jwt.example.com",
		RemoteJWKS: contour_v1.RemoteJWKS{
			URI:     "https://jwt.example.com/jwks.json",
			Timeout: jwksTimeout,
		},
	}
	jwksURL, _ := url.Parse(jwtProvider.RemoteJWKS.URI)

	secret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	fallbackSecret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	service := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}
	tests := map[string]struct {
		ListenerConfig
		fallbackCertificate *types.NamespacedName
		objs                []any
		want                map[string]*envoy_config_listener_v3.Listener
	}{
		"nothing": {
			objs: nil,
			want: map[string]*envoy_config_listener_v3.Listener{},
		},
		"one http only ingress": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"one http only httpproxy": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"simple ingress with secret": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"multiple tls ingress with secrets should be sorted": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "sortedsecond",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"sortedsecond.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "sortedsecond.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "sortedfirst",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"sortedfirst.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "sortedfirst.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"sortedfirst.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("sortedfirst.example.com")),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"sortedsecond.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("sortedsecond.example.com")),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"simple ingress with missing secret": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "missing",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"simple httpproxy with secret": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"ingress with allow-http: false": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.allow-http": "false",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
					},
				},
			},
			want: map[string]*envoy_config_listener_v3.Listener{},
		},
		"simple tls ingress with allow-http:false": {
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
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
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"use proxy proto": {
			ListenerConfig: ListenerConfig{
				UseProxyProto: true,
			},
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.ProxyProtocol(),
				),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.ProxyProtocol(),
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"--envoy-http-access-log": {
			ListenerConfig: ListenerConfig{
				HTTPAccessLog:  "/tmp/http_access.log",
				HTTPSAccessLog: "/tmp/https_access.log",
			},
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy("/tmp/http_access.log", "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("whatever.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "whatever.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy("/tmp/https_access.log", "", nil, contour_v1alpha1.LogLevelInfo)).
						Get()),
				}},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"tls-protocol-version from config": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.2",
				MaximumTLSVersion: "1.3",
			},
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"tls-protocol-version from config overridden by annotation": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.2",
				MaximumTLSVersion: "1.3",
			},
			objs: []any{
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/tls-minimum-protocol-version": "1.3",
							"projectcontour.io/tls-maximum-protocol-version": "1.3",
						},
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"tls-protocol-version from config overridden by httpproxy": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.2",
				MaximumTLSVersion: "1.3",
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:             "secret",
								MinimumProtocolVersion: "1.3",
								MaximumProtocolVersion: "1.3",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"tls-maximum-protocol-version from config not overridden by httpproxy": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.2",
				MaximumTLSVersion: "1.2",
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:             "secret",
								MinimumProtocolVersion: "1.2",
								MaximumProtocolVersion: "1.3",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"tls-cipher-suites from config": {
			ListenerConfig: ListenerConfig{
				CipherSuites: []string{
					"ECDHE-ECDSA-AES256-GCM-SHA384",
					"ECDHE-RSA-AES256-GCM-SHA384",
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, []string{"ECDHE-ECDSA-AES256-GCM-SHA384", "ECDHE-RSA-AES256-GCM-SHA384"}, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpproxy with fallback certificate and with request timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					Request: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						RequestTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						RequestTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						RequestTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpproxy with fallback certificate and with connection idle timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					ConnectionIdle: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpproxy with fallback certificate and with stream idle timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					StreamIdle: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpproxy with fallback certificate and with delayed close timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					DelayedClose: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpproxy with fallback certificate and with max connection duration set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpproxy with fallback certificate and with connection shutdown grace period set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with fallback certificate": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(fallbackCertFilter),
					Name:            "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"multiple httpproxies with fallback certificate": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple2",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.another.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
							ServerNames: []string{"www.another.com"},
						},
						TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
						Filters:         envoy_v3.Filters(httpsFilterFor("www.another.com")),
					},
					{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
							ServerNames: []string{"www.example.com"},
						},
						TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
						Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
					},
					{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
							TransportProtocol: "tls",
						},
						TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
						Filters:         envoy_v3.Filters(fallbackCertFilter),
						Name:            "fallback-certificate",
					},
				},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with fallback certificate - no cert passed": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "",
				Namespace: "",
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				service,
			},
			want: listenermap(),
		},

		"httpproxy with fallback certificate - cert passed but vhost not enabled": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbackcert",
				Namespace: "default",
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with connection idle timeout set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					ConnectionIdle: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with stream idle timeout set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					StreamIdle: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with max connection duration set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with delayed close timeout set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					DelayedClose: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with connection shutdown grace period set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpsproxy with secret with connection idle timeout set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					ConnectionIdle: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with allow_chunked_length set in listener config": {
			ListenerConfig: ListenerConfig{
				AllowChunkedLength: true,
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						AllowChunkedLength(true).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with merge_slashes set in listener config": {
			ListenerConfig: ListenerConfig{
				MergeSlashes: true,
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},

			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						MergeSlashes(true).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with server_header_transformation set to pass through in listener config": {
			ListenerConfig: ListenerConfig{
				ServerHeaderTransformation: contour_v1alpha1.PassThroughServerHeader,
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},

			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						ServerHeaderTransformation(contour_v1alpha1.PassThroughServerHeader).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with XffNumTrustedHops set in listener config": {
			ListenerConfig: ListenerConfig{
				XffNumTrustedHops: 1,
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						NumTrustedHops(1).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with StripTrailingHostDot set in listener config": {
			ListenerConfig: ListenerConfig{
				StripTrailingHostDot: true,
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						StripTrailingHostDot(true).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpsproxy with secret with stream idle timeout set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					StreamIdle: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpsproxy with secret with max connection duration set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpsproxy with secret with delayed close timeout set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					DelayedClose: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpsproxy with secret with connection shutdown grace period set in listener config": {
			ListenerConfig: ListenerConfig{
				Timeouts: contourconfig.Timeouts{
					ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"insecure httpproxy with rate limit config": {
			ListenerConfig: ListenerConfig{
				RateLimitConfig: &RateLimitConfig{
					ExtensionServiceConfig: ExtensionServiceConfig{
						ExtensionService: types.NamespacedName{Namespace: "projectcontour", Name: "ratelimit"},
						Timeout:          timeout.DurationSetting(7 * time.Second),
					},
					Domain:                      "contour",
					FailOpen:                    false,
					EnableXRateLimitHeaders:     true,
					EnableResourceExhaustedCode: true,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName("ingress_http").
					MetricsPrefix("ingress_http").
					AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
						Name: wellknown.HTTPRateLimit,
						ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
							TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
								Domain:          "contour",
								FailureModeDeny: true,
								Timeout:         durationpb.New(7 * time.Second),
								RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
									GrpcService: &envoy_config_core_v3.GrpcService{
										TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
												ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
												Authority:   "extension.projectcontour.ratelimit",
											},
										},
									},
									TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
								},
								EnableXRatelimitHeaders:        envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03,
								RateLimitedAsResourceExhausted: true,
							}),
						},
					}).Get()),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"secure httpproxy with rate limit config": {
			ListenerConfig: ListenerConfig{
				RateLimitConfig: &RateLimitConfig{
					ExtensionServiceConfig: ExtensionServiceConfig{
						ExtensionService: types.NamespacedName{Namespace: "projectcontour", Name: "ratelimit"},
						SNI:              "ratelimit-example.com",
						Timeout:          timeout.DurationSetting(7 * time.Second),
					},
					Domain:                      "contour",
					FailOpen:                    false,
					EnableXRateLimitHeaders:     true,
					EnableResourceExhaustedCode: true,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
						Name: wellknown.HTTPRateLimit,
						ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
							TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
								Domain:          "contour",
								FailureModeDeny: true,
								Timeout:         durationpb.New(7 * time.Second),
								RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
									GrpcService: &envoy_config_core_v3.GrpcService{
										TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
												ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
												Authority:   "ratelimit-example.com",
											},
										},
									},
									TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
								},
								EnableXRatelimitHeaders:        envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03,
								RateLimitedAsResourceExhausted: true,
							}),
						},
					}).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         durationpb.New(7 * time.Second),
									RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
										GrpcService: &envoy_config_core_v3.GrpcService{
											TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
													Authority:   "ratelimit-example.com",
												},
											},
										},
										TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders:        envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03,
									RateLimitedAsResourceExhausted: true,
								}),
							},
						}).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"secure httpproxy using fallback certificate with rate limit config": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				RateLimitConfig: &RateLimitConfig{
					ExtensionServiceConfig: ExtensionServiceConfig{
						ExtensionService: types.NamespacedName{Namespace: "projectcontour", Name: "ratelimit"},
						Timeout:          timeout.DurationSetting(7 * time.Second),
					},
					Domain:                      "contour",
					FailOpen:                    false,
					EnableXRateLimitHeaders:     true,
					EnableResourceExhaustedCode: true,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				secret,
				fallbackSecret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         durationpb.New(7 * time.Second),
									RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
										GrpcService: &envoy_config_core_v3.GrpcService{
											TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
													Authority:   "extension.projectcontour.ratelimit",
												},
											},
										},
										TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders:        envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03,
									RateLimitedAsResourceExhausted: true,
								}),
							},
						}).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         durationpb.New(7 * time.Second),
									RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
										GrpcService: &envoy_config_core_v3.GrpcService{
											TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
													Authority:   "extension.projectcontour.ratelimit",
												},
											},
										},
										TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders:        envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03,
									RateLimitedAsResourceExhausted: true,
								}),
							},
						}).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket(envoyGen, "fallbacksecret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         durationpb.New(7 * time.Second),
									RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
										GrpcService: &envoy_config_core_v3.GrpcService{
											TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
													Authority:   "extension.projectcontour.ratelimit",
												},
											},
										},
										TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders:        envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03,
									RateLimitedAsResourceExhausted: true,
								}),
							},
						}).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"DSCP marking with socket options": {
			ListenerConfig: ListenerConfig{
				SocketOptions: &contour_v1alpha1.SocketOptions{
					TOS:          64,
					TrafficClass: 64,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoyGen.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo), 0)),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().TOS(64).TrafficClass(64).Build(),
			}),
		},
		"httpproxy with MaxRequestsPerConnection set in listener config": {
			ListenerConfig: ListenerConfig{
				MaxRequestsPerConnection: ptr.To(uint32(1)),
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						MaxRequestsPerConnection(ptr.To(uint32(1))).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpsproxy with MaxRequestsPerConnection set in listener config": {
			ListenerConfig: ListenerConfig{
				MaxRequestsPerConnection: ptr.To(uint32(1)),
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					MaxRequestsPerConnection(ptr.To(uint32(1))).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						MaxRequestsPerConnection(ptr.To(uint32(1))).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with HTTP2MaxConcurrentStreams set in listener config": {
			ListenerConfig: ListenerConfig{
				HTTP2MaxConcurrentStreams: ptr.To(uint32(100)),
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						HTTP2MaxConcurrentStreams(ptr.To(uint32(100))).
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpsproxy with HTTP2MaxConcurrentStreams set in listener config": {
			ListenerConfig: ListenerConfig{
				HTTP2MaxConcurrentStreams: ptr.To(uint32(101)),
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					HTTP2MaxConcurrentStreams(ptr.To(uint32(101))).
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						HTTP2MaxConcurrentStreams(ptr.To(uint32(101))).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with PerConnectionBufferLimitBytes set in listener config": {
			ListenerConfig: ListenerConfig{
				PerConnectionBufferLimitBytes: ptr.To(uint32(32768)),
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:                          ENVOY_HTTP_LISTENER,
				Address:                       envoy_v3.SocketAddress("0.0.0.0", 8080),
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with disabled compression set in listener config": {
			ListenerConfig: ListenerConfig{
				Compression: &contour_v1alpha1.EnvoyCompression{
					Algorithm: contour_v1alpha1.DisabledCompression,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						Compression(&contour_v1alpha1.EnvoyCompression{
							Algorithm: contour_v1alpha1.DisabledCompression,
						}).
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with gzip compression set in listener config": {
			ListenerConfig: ListenerConfig{
				Compression: &contour_v1alpha1.EnvoyCompression{
					Algorithm: contour_v1alpha1.GzipCompression,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						Compression(&contour_v1alpha1.EnvoyCompression{
							Algorithm: contour_v1alpha1.GzipCompression,
						}).
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with brotli compression set in listener config": {
			ListenerConfig: ListenerConfig{
				Compression: &contour_v1alpha1.EnvoyCompression{
					Algorithm: contour_v1alpha1.BrotliCompression,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						Compression(&contour_v1alpha1.EnvoyCompression{
							Algorithm: contour_v1alpha1.BrotliCompression,
						}).
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with zstd compression set in listener config": {
			ListenerConfig: ListenerConfig{
				Compression: &contour_v1alpha1.EnvoyCompression{
					Algorithm: contour_v1alpha1.ZstdCompression,
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						Compression(&contour_v1alpha1.EnvoyCompression{
							Algorithm: contour_v1alpha1.ZstdCompression,
						}).
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpproxy with invalid compression set in listener config": {
			ListenerConfig: ListenerConfig{
				Compression: &contour_v1alpha1.EnvoyCompression{
					Algorithm: "invalid value",
				},
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoyGen.HTTPConnectionManagerBuilder().
						Compression(&contour_v1alpha1.EnvoyCompression{
							Algorithm: contour_v1alpha1.GzipCompression,
						}).
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						DefaultFilters().
						Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
		"httpsproxy with PerConnectionBufferLimitBytes set in listener config": {
			ListenerConfig: ListenerConfig{
				PerConnectionBufferLimitBytes: ptr.To(uint32(32768)),
			},
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:                          ENVOY_HTTP_LISTENER,
				Address:                       envoy_v3.SocketAddress("0.0.0.0", 8080),
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:                          ENVOY_HTTPS_LISTENER,
				Address:                       envoy_v3.SocketAddress("0.0.0.0", 8443),
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},

		"httpproxy with authZ the authN": {
			ListenerConfig: ListenerConfig{
				PerConnectionBufferLimitBytes: ptr.To(uint32(32768)),
			},
			objs: []any{
				&contour_v1alpha1.ExtensionService{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "auth",
						Namespace: "extension",
					},
				},
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_v1.TLS{
								SecretName: "secret",
							},
							Authorization: &contour_v1.AuthorizationServer{
								ExtensionServiceRef: contour_v1.ExtensionServiceReference{
									Namespace: "extension",
									Name:      "auth",
								},
							},
							JWTProviders: []contour_v1.JWTProvider{jwtProvider},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				secret,
				service,
			},
			want: listenermap(&envoy_config_listener_v3.Listener{
				Name:                          ENVOY_HTTP_LISTENER,
				Address:                       envoy_v3.SocketAddress("0.0.0.0", 8080),
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
				FilterChains: envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
					DefaultFilters().
					Get(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}, &envoy_config_listener_v3.Listener{
				Name:                          ENVOY_HTTPS_LISTENER,
				Address:                       envoy_v3.SocketAddress("0.0.0.0", 8443),
				PerConnectionBufferLimitBytes: wrapperspb.UInt32(32768),
				FilterChains: []*envoy_config_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket(envoyGen, "secret", envoy_transport_socket_tls_v3.TlsParameters_TLSv1_2, envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoyGen.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						AddFilter(envoy_v3.FilterJWTAuthN([]dag.JWTProvider{{
							Name:   jwtProvider.Name,
							Issuer: jwtProvider.Issuer,
							RemoteJWKS: dag.RemoteJWKS{
								URI: jwtProvider.RemoteJWKS.URI,
								Cluster: dag.DNSNameCluster{
									Address: jwksURL.Hostname(),
									Scheme:  jwksURL.Scheme,
									Port:    443,
								},
								Timeout: jwksTimeoutDuration,
							},
						}})).
						AddFilter(envoy_v3.FilterExternalAuthz(&dag.ExternalAuthorization{
							AuthorizationService: &dag.ExtensionCluster{
								Name: "extension/extension/auth",
							},
						})).
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			lc := ListenerCache{
				Config:   tc.ListenerConfig,
				envoyGen: envoyGen,
			}

			lc.OnChange(buildDAGFallback(t, tc.fallbackCertificate, tc.objs...))
			protobuf.ExpectEqual(t, tc.want, lc.values)
		})
	}
}

func transportSocket(envoyGen *envoy_v3.EnvoyGen, secretName string, tlsMinProtoVersion, tlsMaxProtoVersion envoy_transport_socket_tls_v3.TlsParameters_TlsProtocol, cipherSuites []string, alpnprotos ...string) *envoy_config_core_v3.TransportSocket {
	secret := &dag.Secret{
		Object: &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      secretName,
				Namespace: "default",
			},
			Type: core_v1.SecretTypeTLS,
			Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
		},
	}

	return envoy_v3.DownstreamTLSTransportSocket(
		envoyGen.DownstreamTLSContext(secret, tlsMinProtoVersion, tlsMaxProtoVersion, cipherSuites, nil, alpnprotos...),
	)
}

func listenermap(listeners ...*envoy_config_listener_v3.Listener) map[string]*envoy_config_listener_v3.Listener {
	m := make(map[string]*envoy_config_listener_v3.Listener)
	for _, l := range listeners {
		m[l.Name] = l
	}
	return m
}
