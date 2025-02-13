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
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
)

var (
	normalGlobalExtAuthConfig contour_v1.AuthorizationServer = contour_v1.AuthorizationServer{
		ExtensionServiceRef: contour_v1.ExtensionServiceReference{
			Name:      "extension",
			Namespace: "auth",
		},
		FailOpen:        false,
		ResponseTimeout: defaultResponseTimeout.String(),
		AuthPolicy: &contour_v1.AuthorizationPolicy{
			Context: map[string]string{
				"header_type": "root_config",
				"header_1":    "message_1",
			},
		},
	}

	disabledGlobalExtAuthConfig contour_v1.AuthorizationServer = contour_v1.AuthorizationServer{
		ExtensionServiceRef: contour_v1.ExtensionServiceReference{
			Name:      "extension",
			Namespace: "auth",
		},
		FailOpen:        false,
		ResponseTimeout: defaultResponseTimeout.String(),
		AuthPolicy: &contour_v1.AuthorizationPolicy{
			Disabled: true,
			Context: map[string]string{
				"header_type": "root_config",
				"header_1":    "message_1",
			},
		},
	}
)

func globalExternalAuthorizationFilterExists(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			statsListener()),
	}).Status(p).IsValid()
}

func globalExternalAuthorizationFilterExistsTLS(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := fixture.NewProxy("TLSProxy").
		WithFQDN("foo.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName: "certificate",
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
				},
			},
		})

	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("foo.com",
				featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
				authzFilterFor(
					"foo.com",
					&envoy_filter_http_ext_authz_v3.ExtAuthz{
						Services:               grpcCluster("extension/auth/extension"),
						ClearRouteCache:        true,
						IncludePeerCertificate: true,
						StatusOnError: &envoy_type_v3.HttpStatus{
							Code: envoy_type_v3.StatusCode_Forbidden,
						},
						TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
					},
				),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener()),
	}).Status(p).IsValid()
}

func globalExternalAuthorizationWithTLSGlobalAuthDisabled(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := fixture.NewProxy("TLSProxy").
		WithFQDN("foo.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName: "certificate",
				},
				Authorization: &contour_v1.AuthorizationServer{
					AuthPolicy: &contour_v1.AuthorizationPolicy{
						Disabled: true,
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
				},
			},
		})

	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("foo.com",
				featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
				authzFilterFor(
					"foo.com",
					&envoy_filter_http_ext_authz_v3.ExtAuthz{
						Services:               grpcCluster("extension/auth/extension"),
						ClearRouteCache:        true,
						IncludePeerCertificate: true,
						StatusOnError: &envoy_type_v3.HttpStatus{
							Code: envoy_type_v3.StatusCode_Forbidden,
						},
						TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
					},
				),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener()),
	}).Status(p).IsValid()
}

func globalExternalAuthorizationWithMergedAuthPolicy(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
					AuthPolicy: &contour_v1.AuthorizationPolicy{
						Context: map[string]string{
							"header_type": "proxy_config",
							"header_2":    "message_2",
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			statsListener()),
	}).Status(p).IsValid()

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost("foo.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.ExtAuthzFilterName: protobuf.MustMarshalAny(
								&envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute{
									Override: &envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
										CheckSettings: &envoy_filter_http_ext_authz_v3.CheckSettings{
											ContextExtensions: map[string]string{
												"header_type": "proxy_config",
												"header_1":    "message_1",
												"header_2":    "message_2",
											},
										},
									},
								},
							),
						},
					},
				),
			),
		),
	})
}

func globalExternalAuthorizationDisabledByDefault(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			statsListener()),
	}).Status(p).IsValid()

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost("foo.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.ExtAuthzFilterName: protobuf.MustMarshalAny(
								&envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute{
									Override: &envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute_Disabled{
										Disabled: true,
									},
								},
							),
						},
					},
				),
			),
		),
	})
}

func GlobalExternalAuthorizationDisabledByDefaultAndEnabledOnRoute(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
					AuthPolicy: &contour_v1.AuthorizationPolicy{
						Disabled: false,
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			statsListener()),
	}).Status(p).IsValid()

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost("foo.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.ExtAuthzFilterName: protobuf.MustMarshalAny(
								&envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute{
									Override: &envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
										CheckSettings: &envoy_filter_http_ext_authz_v3.CheckSettings{
											ContextExtensions: map[string]string{
												"header_type": "root_config",
												"header_1":    "message_1",
											},
										},
									},
								},
							),
						},
					},
				),
			),
		),
	})
}

func globalExternalAuthorizationWithMergedAuthPolicyTLS(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := fixture.NewProxy("TLSProxy").
		WithFQDN("foo.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName: "certificate",
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
					AuthPolicy: &contour_v1.AuthorizationPolicy{
						Context: map[string]string{
							"header_type": "proxy_config",
							"header_2":    "message_2",
						},
					},
				},
			},
		})

	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("foo.com",
				featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
				authzFilterFor(
					"foo.com",
					&envoy_filter_http_ext_authz_v3.ExtAuthz{
						Services:               grpcCluster("extension/auth/extension"),
						ClearRouteCache:        true,
						IncludePeerCertificate: true,
						StatusOnError: &envoy_type_v3.HttpStatus{
							Code: envoy_type_v3.StatusCode_Forbidden,
						},
						TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
					},
				),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener()),
	}).Status(p).IsValid()

	expectedAuthContext := map[string]*anypb.Any{
		envoy_v3.ExtAuthzFilterName: protobuf.MustMarshalAny(
			&envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute{
				Override: &envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
					CheckSettings: &envoy_filter_http_ext_authz_v3.CheckSettings{
						ContextExtensions: map[string]string{
							"header_type": "proxy_config",
							"header_1":    "message_1",
							"header_2":    "message_2",
						},
					},
				},
			},
		),
	}

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/foo.com",
				envoy_v3.VirtualHost("foo.com",
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/"),
						Action:               routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: expectedAuthContext,
					},
				),
			),
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost("foo.com",
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/"),
						Action:               withRedirect(),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					},
				),
			),
		),
	})
}

func globalExternalAuthorizationWithTLSAuthOverride(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := fixture.NewProxy("TLSProxy").
		WithFQDN("foo.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName: "certificate",
				},
				Authorization: &contour_v1.AuthorizationServer{
					ExtensionServiceRef: contour_v1.ExtensionServiceReference{
						Namespace: "auth",
						Name:      "extension",
					},
					ResponseTimeout: defaultResponseTimeout.String(),
					FailOpen:        true,
					WithRequestBody: &contour_v1.AuthorizationServerBufferSettings{
						MaxRequestBytes:     512,
						PackAsBytes:         true,
						AllowPartialMessage: true,
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
				},
			},
		})

	rh.OnAdd(p)

	httpListener := defaultHTTPListener()

	// replace the default filter chains with an HCM that includes the global
	// extAuthz filter.
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("foo.com",
				featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
				authzFilterFor(
					"foo.com",
					&envoy_filter_http_ext_authz_v3.ExtAuthz{
						Services:               grpcCluster("extension/auth/extension"),
						ClearRouteCache:        true,
						IncludePeerCertificate: true,
						FailureModeAllow:       true,
						StatusOnError: &envoy_type_v3.HttpStatus{
							Code: envoy_type_v3.StatusCode_Forbidden,
						},
						WithRequestBody: &envoy_filter_http_ext_authz_v3.BufferSettings{
							MaxRequestBytes:     512,
							PackAsBytes:         true,
							AllowPartialMessage: true,
						},
						TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
					},
				),
				nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener()),
	}).Status(p).IsValid()
}

func globalExternalAuthorizationFilterTLSWithFallbackCertificate(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := fixture.NewProxy("TLSProxy").
		WithFQDN("foo.com").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName:                "certificate",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
				},
			},
		})

	rh.OnAdd(p)

	// Add Fallback Certificate Secret
	fallbackSecret := featuretests.TLSSecret(t, "admin/fallbacksecret", &featuretests.ServerCertificate)
	rh.OnAdd(fallbackSecret)

	// Add Fallback Cert Delegation
	certDelegationAll := &contour_v1.TLSCertificateDelegation{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "fallbackcertdelegation",
			Namespace: "admin",
		},
		Spec: contour_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_v1.CertificateDelegation{{
				SecretName:       "fallbacksecret",
				TargetNamespaces: []string{"*"},
			}},
		},
	}

	rh.OnAdd(certDelegationAll)

	httpListener := defaultHTTPListener()
	httpListener.FilterChains = envoy_v3.FilterChains(getGlobalExtAuthHCM())

	httpsListener := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			filterchaintls("foo.com",
				featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
				authzFilterFor(
					"foo.com",
					&envoy_filter_http_ext_authz_v3.ExtAuthz{
						Services:               grpcCluster("extension/auth/extension"),
						ClearRouteCache:        true,
						IncludePeerCertificate: true,
						StatusOnError: &envoy_type_v3.HttpStatus{
							Code: envoy_type_v3.StatusCode_Forbidden,
						},
						TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
					},
				),
				nil, "h2", "http/1.1"),
			filterchaintlsfallbackauthz(fallbackSecret,
				&envoy_filter_http_ext_authz_v3.ExtAuthz{
					Services:               grpcCluster("extension/auth/extension"),
					ClearRouteCache:        true,
					IncludePeerCertificate: true,
					StatusOnError: &envoy_type_v3.HttpStatus{
						Code: envoy_type_v3.StatusCode_Forbidden,
					},
					TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
				}, nil, "h2", "http/1.1"),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			httpsListener,
			statsListener()),
	}).Status(p).IsValid()
}

func TestGlobalAuthorization(t *testing.T) {
	subtests := map[string]struct {
		globalExtAuthConfig *contour_v1.AuthorizationServer
		testFunction        func(*testing.T, ResourceEventHandlerWrapper, *Contour)
	}{
		// Default extAuthz on non TLS host.
		"GlobalExternalAuthorizationFilterExists": {
			&normalGlobalExtAuthConfig, globalExternalAuthorizationFilterExists,
		},
		// Default extAuthz on non TLS and TLS hosts.
		"GlobalExternalAuthorizationFilterExistsTLS": {
			&normalGlobalExtAuthConfig, globalExternalAuthorizationFilterExistsTLS,
		},
		// extAuthz disabled on TLS host.
		"GlobalExternalAuthorizationWithTLSGlobalAuthDisabled": {
			&normalGlobalExtAuthConfig, globalExternalAuthorizationWithTLSGlobalAuthDisabled,
		},
		// extAuthz override on TLS host.
		"GlobalExternalAuthorizationWithTLSAuthOverride": {
			&normalGlobalExtAuthConfig, globalExternalAuthorizationWithTLSAuthOverride,
		},
		// extAuthz authpolicy merge for non TLS hosts.
		"GlobalExternalAuthorizationWithMergedAuthPolicy": {
			&normalGlobalExtAuthConfig, globalExternalAuthorizationWithMergedAuthPolicy,
		},
		// extAuthz authpolicy merge for TLS hosts.
		"GlobalExternalAuthorizationWithMergedAuthPolicyTLS": {
			&normalGlobalExtAuthConfig, globalExternalAuthorizationWithMergedAuthPolicyTLS,
		},
		// extAuthz on TLS host with Fallback Certificate enabled.
		"GlobalExternalAuthorizationFilterTLSWithFallbackCertificate": {
			&normalGlobalExtAuthConfig, globalExternalAuthorizationFilterTLSWithFallbackCertificate,
		},
		// extAuthz authPolicy.disabled propagation
		"GlobalExternalAuthorizationDisabledByDefault": {
			&disabledGlobalExtAuthConfig, globalExternalAuthorizationDisabledByDefault,
		},
		// extAuthz non-empty vhost authPolicy enables authorization
		"GlobalExternalAuthorizationDisabledByDefaultAndEnabledOnRoute": {
			&disabledGlobalExtAuthConfig, GlobalExternalAuthorizationDisabledByDefaultAndEnabledOnRoute,
		},
		// extAuthz authPolicy.disabled propagation
		"GlobalExternalAuthorizationDisabledByDefaultMergeAuthPolicy": {
			&disabledGlobalExtAuthConfig, globalExternalAuthorizationWithMergedAuthPolicy,
		},
	}

	for n, env := range subtests {
		f := env.testFunction
		t.Run(n, func(t *testing.T) {
			rh, c, done := setup(t,
				func(cfg *xdscache_v3.ListenerConfig) {
					cfg.GlobalExternalAuthConfig = &xdscache_v3.GlobalExternalAuthConfig{
						ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
							ExtensionService: k8s.NamespacedNameFrom("auth/extension"),
							Timeout:          timeout.DurationSetting(defaultResponseTimeout),
						},
						FailOpen: false,
						Context: map[string]string{
							"header_type": "root_config",
							"header_1":    "message_1",
						},
					}
				},
				func(b *dag.Builder) {
					for _, processor := range b.Processors {
						if httpProxyProcessor, ok := processor.(*dag.HTTPProxyProcessor); ok {
							httpProxyProcessor.GlobalExternalAuthorization = env.globalExtAuthConfig
							httpProxyProcessor.FallbackCertificate = &types.NamespacedName{
								Namespace: "admin",
								Name:      "fallbacksecret",
							}
						}
					}
				})
			defer done()

			// Add common test fixtures.
			rh.OnAdd(fixture.NewService("s1").WithPorts(core_v1.ServicePort{Port: 80}))
			rh.OnAdd(fixture.NewService("auth/oidc-server").
				WithPorts(core_v1.ServicePort{Port: 8081}))

			rh.OnAdd(featuretests.Endpoints("auth", "oidc-server", core_v1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.21"),
				Ports:     featuretests.Ports(featuretests.Port("", 8081)),
			}))

			rh.OnAdd(&contour_v1alpha1.ExtensionService{
				ObjectMeta: fixture.ObjectMeta("auth/extension"),
				Spec: contour_v1alpha1.ExtensionServiceSpec{
					Services: []contour_v1alpha1.ExtensionServiceTarget{
						{Name: "oidc-server", Port: 8081},
					},
					TimeoutPolicy: &contour_v1.TimeoutPolicy{
						Response: defaultResponseTimeout.String(),
					},
				},
			})

			rh.OnAdd(fixture.NewService("app-server").
				WithPorts(core_v1.ServicePort{Port: 80}))

			rh.OnAdd(featuretests.Endpoints("auth", "app-server", core_v1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.21"),
				Ports:     featuretests.Ports(featuretests.Port("", 80)),
			}))

			rh.OnAdd(featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate))

			f(t, rh, c)
		})
	}
}

// getGlobalExtAuthHCM returns a HTTP Connection Manager with Global External Authorization configured.
func getGlobalExtAuthHCM() *envoy_config_listener_v3.Filter {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: "contour",
	})
	return envoyGen.HTTPConnectionManagerBuilder().
		RouteConfigName("ingress_http").
		MetricsPrefix("ingress_http").
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		DefaultFilters().
		AddFilter(&envoy_filter_network_http_connection_manager_v3.HttpFilter{
			Name: wellknown.HTTPExternalAuthorization,
			ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ext_authz_v3.ExtAuthz{
					Services:               grpcCluster("extension/auth/extension"),
					ClearRouteCache:        true,
					IncludePeerCertificate: true,
					StatusOnError: &envoy_type_v3.HttpStatus{
						Code: envoy_type_v3.StatusCode_Forbidden,
					},
					TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
				}),
			},
		}).
		Get()
}
