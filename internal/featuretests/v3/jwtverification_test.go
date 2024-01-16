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
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_jwt_authn_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	core_v1 "k8s.io/api/core/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestJWTVerification(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	sec1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(sec1)

	s1 := fixture.NewService("s1").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	// Valid HTTPProxy without JWT verification enabled
	proxy1 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})

	rh.OnAdd(proxy1)

	// We should start with a single generic HTTPS service.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1, httpsFilterFor("jwt.example.com"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})

	// Valid HTTPProxy with JWT verification enabled
	proxy2 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
			}},
		})

	rh.OnUpdate(proxy1, proxy2)

	// Now we should have the JWT authentication filter,
	// a cluster for the JWKS URI and the route should have
	// a reference to the requirement.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(clusterType, "dnsname/https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			&envoy_config_cluster_v3.Cluster{
				Name: "dnsname/https/jwt.example.com",
				ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
					Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
				},
				CommonLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type_v3.Percent{Value: 0},
				},
				ConnectTimeout: durationpb.New(2 * time.Second),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/jwt.example.com",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_SocketAddress{
													SocketAddress: &envoy_config_core_v3.SocketAddress{
														Protocol: envoy_config_core_v3.SocketAddress_TCP,
														Address:  "jwt.example.com",
														PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
															PortValue: uint32(443),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: &envoy_config_core_v3.TransportSocket{
					Name: "envoy.transport_sockets.tls",
					ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
							Sni:              "jwt.example.com",
						}),
					},
				},
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// Valid HTTPProxy with JWT verification enabled, with all paths
	// *except* /css opting into verification.
	proxy3 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
				},
				{
					Conditions: []contour_v1.MatchCondition{{Prefix: "/css"}},
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
				},
			},
		})

	rh.OnUpdate(proxy2, proxy3)

	// Verify that the "/css" JWT rule gets sorted before the "/" one.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/css"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// Same as proxy3, except using "opt-out" pattern instead of "opt-in".
	proxy4 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:    "provider-1",
						Default: true,
						Issuer:  "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
				},
				{
					Conditions: []contour_v1.MatchCondition{{Prefix: "/css"}},
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Disabled: true},
				},
			},
		})

	rh.OnUpdate(proxy3, proxy4)

	// Verify that the "/css" JWT rule gets sorted before the "/" one.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/css"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// Route overrides the default provider.
	proxy5 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:    "provider-1",
						Default: true,
						Issuer:  "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
					{
						Name:   "provider-2",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-2"},
				},
			},
		})

	rh.OnUpdate(proxy4, proxy5)

	// Verify that the route requires "provider-2".
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
								"provider-2": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
								"provider-2": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-2",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-2"},
							}),
						},
					},
				),
			),
		),
	})

	// JWKS with a non-standard port
	proxy6 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com:8443/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
			}},
		})

	rh.OnUpdate(proxy5, proxy6)

	// the JWKS cluster should reflect the non-standard port.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com:8443/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(clusterType, "dnsname/https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			&envoy_config_cluster_v3.Cluster{
				Name: "dnsname/https/jwt.example.com",
				ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
					Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
				},
				CommonLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type_v3.Percent{Value: 0},
				},
				ConnectTimeout: durationpb.New(2 * time.Second),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/jwt.example.com",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_SocketAddress{
													SocketAddress: &envoy_config_core_v3.SocketAddress{
														Protocol: envoy_config_core_v3.SocketAddress_TCP,
														Address:  "jwt.example.com",
														PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
															PortValue: uint32(8443),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: &envoy_config_core_v3.TransportSocket{
					Name: "envoy.transport_sockets.tls",
					ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
							Sni:              "jwt.example.com",
						}),
					},
				},
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	rh.OnAdd(featuretests.CASecret(t, "cacert", &featuretests.CACertificate))

	// JWKS with upstream validation
	proxy7 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_v1.UpstreamValidation{
								CACertificate: "cacert",
								SubjectName:   "jwt.example.com",
							},
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
			}},
		})

	rh.OnUpdate(proxy6, proxy7)

	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(clusterType, "dnsname/https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			&envoy_config_cluster_v3.Cluster{
				Name: "dnsname/https/jwt.example.com",
				ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
					Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
				},
				CommonLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type_v3.Percent{Value: 0},
				},
				ConnectTimeout: durationpb.New(2 * time.Second),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/jwt.example.com",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_SocketAddress{
													SocketAddress: &envoy_config_core_v3.SocketAddress{
														Protocol: envoy_config_core_v3.SocketAddress_TCP,
														Address:  "jwt.example.com",
														PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
															PortValue: uint32(443),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: &envoy_config_core_v3.TransportSocket{
					Name: "envoy.transport_sockets.tls",
					ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
								ValidationContextType: &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
									ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
										TrustedCa: &envoy_config_core_v3.DataSource{
											Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
												InlineBytes: featuretests.PEMBytes(t, &featuretests.CACertificate),
											},
										},
										MatchTypedSubjectAltNames: []*envoy_transport_socket_tls_v3.SubjectAltNameMatcher{
											{
												SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
												Matcher: &envoy_matcher_v3.StringMatcher{
													MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
														Exact: "jwt.example.com",
													},
												},
											},
										},
									},
								},
							},
							Sni: "jwt.example.com",
						}),
					},
				},
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// JWKS with a DNS lookup family specified
	proxy8 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:             "https://jwt.example.com:8443/jwks.json",
							Timeout:         "7s",
							CacheDuration:   "30s",
							DNSLookupFamily: "v4",
						},
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
			}},
		})

	rh.OnUpdate(proxy7, proxy8)

	// the JWKS cluster should reflect the non-default DNS lookup family.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com:8443/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(clusterType, "dnsname/https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			&envoy_config_cluster_v3.Cluster{
				Name: "dnsname/https/jwt.example.com",
				ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
					Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
				},
				CommonLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type_v3.Percent{Value: 0},
				},
				ConnectTimeout: durationpb.New(2 * time.Second),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/jwt.example.com",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_SocketAddress{
													SocketAddress: &envoy_config_core_v3.SocketAddress{
														Protocol: envoy_config_core_v3.SocketAddress_TCP,
														Address:  "jwt.example.com",
														PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
															PortValue: uint32(8443),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: &envoy_config_core_v3.TransportSocket{
					Name: "envoy.transport_sockets.tls",
					ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
							Sni:              "jwt.example.com",
						}),
					},
				},
				DnsLookupFamily: envoy_config_cluster_v3.Cluster_V4_ONLY,
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// JWT Provider with ForwardJWT specified.
	proxy9 := fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
						ForwardJWT: true,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
			}},
		})

	rh.OnUpdate(proxy8, proxy9)

	// the JWT Provider should have Forward: true.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
									Forward: true,
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(clusterType, "dnsname/https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			&envoy_config_cluster_v3.Cluster{
				Name: "dnsname/https/jwt.example.com",
				ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
					Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
				},
				CommonLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type_v3.Percent{Value: 0},
				},
				ConnectTimeout: durationpb.New(2 * time.Second),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/jwt.example.com",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_SocketAddress{
													SocketAddress: &envoy_config_core_v3.SocketAddress{
														Protocol: envoy_config_core_v3.SocketAddress_TCP,
														Address:  "jwt.example.com",
														PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
															PortValue: uint32(443),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: &envoy_config_core_v3.TransportSocket{
					Name: "envoy.transport_sockets.tls",
					ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
							Sni:              "jwt.example.com",
						}),
					},
				},
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})
}

func TestJWTVerification_Inclusion(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	sec1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(sec1)

	s1 := fixture.NewService("s1").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	// Valid HTTPProxy with an include without JWT verification enabled
	proxy1p := fixture.NewProxy("simple-parent").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
			},
			Includes: []contour_v1.Include{
				{
					Name: "simple-child",
				},
			},
		})
	proxy1c := fixture.NewProxy("simple-child").WithSpec(
		contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		})

	rh.OnAdd(proxy1p)
	rh.OnAdd(proxy1c)

	// We should start with a single generic HTTPS service.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1, httpsFilterFor("jwt.example.com"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	})

	// Valid HTTPProxy with JWT verification enabled
	proxy2p := fixture.NewProxy("simple-parent").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Includes: []contour_v1.Include{
				{
					Name: "simple-child",
				},
			},
		})

	proxy2c := fixture.NewProxy("simple-child").WithSpec(
		contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
			}},
		})

	rh.OnUpdate(proxy1p, proxy2p)
	rh.OnUpdate(proxy1c, proxy2c)

	// Now we should have the JWT authentication filter,
	// a cluster for the JWKS URI and the route should have
	// a reference to the requirement.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(clusterType, "dnsname/https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			&envoy_config_cluster_v3.Cluster{
				Name: "dnsname/https/jwt.example.com",
				ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
					Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
				},
				CommonLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type_v3.Percent{Value: 0},
				},
				ConnectTimeout: durationpb.New(2 * time.Second),
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "dnsname/https/jwt.example.com",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_SocketAddress{
													SocketAddress: &envoy_config_core_v3.SocketAddress{
														Protocol: envoy_config_core_v3.SocketAddress_TCP,
														Address:  "jwt.example.com",
														PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
															PortValue: uint32(443),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TransportSocket: &envoy_config_core_v3.TransportSocket{
					Name: "envoy.transport_sockets.tls",
					ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_transport_socket_tls_v3.UpstreamTlsContext{
							CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{},
							Sni:              "jwt.example.com",
						}),
					},
				},
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// Valid HTTPProxy with JWT verification enabled, with all paths
	// *except* /css opting into verification.
	proxy3p := fixture.NewProxy("simple-parent").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:   "provider-1",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Includes: []contour_v1.Include{
				{
					Name: "simple-child",
				},
			},
		})

	proxy3c := fixture.NewProxy("simple-child").WithSpec(
		contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
				},
				{
					Conditions: []contour_v1.MatchCondition{{Prefix: "/css"}},
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
				},
			},
		})

	rh.OnUpdate(proxy2p, proxy3p)
	rh.OnUpdate(proxy2c, proxy3c)

	// Verify that the "/css" JWT rule gets sorted before the "/" one.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/css"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// Same as proxy3, except using "opt-out" pattern instead of "opt-in".
	proxy4p := fixture.NewProxy("simple-parent").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:    "provider-1",
						Default: true,
						Issuer:  "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Includes: []contour_v1.Include{
				{
					Name: "simple-child",
				},
			},
		})

	proxy4c := fixture.NewProxy("simple-child").WithSpec(
		contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
				},
				{
					Conditions: []contour_v1.MatchCondition{{Prefix: "/css"}},
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Disabled: true},
				},
			},
		})

	rh.OnUpdate(proxy3p, proxy4p)
	rh.OnUpdate(proxy3c, proxy4c)

	// Verify that the "/css" JWT rule gets sorted before the "/" one.
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/css"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-1"},
							}),
						},
					},
				),
			),
		),
	})

	// Route overrides the default provider.
	proxy5p := fixture.NewProxy("simple-parent").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "jwt.example.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:    "provider-1",
						Default: true,
						Issuer:  "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
					{
						Name:   "provider-2",
						Issuer: "issuer.jwt.example.com",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "7s",
							CacheDuration: "30s",
						},
					},
				},
			},
			Includes: []contour_v1.Include{
				{
					Name: "simple-child",
				},
			},
		})

	proxy5c := fixture.NewProxy("simple-child").WithSpec(
		contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-2"},
				},
			},
		})

	rh.OnUpdate(proxy4p, proxy5p)
	rh.OnUpdate(proxy4c, proxy5c)

	// Verify that the route requires "provider-2".
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("jwt.example.com", sec1,
						jwtAuthnFilterFor("jwt.example.com", &envoy_filter_http_jwt_authn_v3.JwtAuthentication{
							Providers: map[string]*envoy_filter_http_jwt_authn_v3.JwtProvider{
								"provider-1": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
								"provider-2": {
									Issuer: "issuer.jwt.example.com",
									JwksSourceSpecifier: &envoy_filter_http_jwt_authn_v3.JwtProvider_RemoteJwks{
										RemoteJwks: &envoy_filter_http_jwt_authn_v3.RemoteJwks{
											HttpUri: &envoy_config_core_v3.HttpUri{
												Uri: "https://jwt.example.com/jwks.json",
												HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
													Cluster: "dnsname/https/jwt.example.com",
												},
												Timeout: durationpb.New(7 * time.Second),
											},
											CacheDuration: durationpb.New(30 * time.Second),
										},
									},
								},
							},
							RequirementMap: map[string]*envoy_filter_http_jwt_authn_v3.JwtRequirement{
								"provider-1": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-1",
									},
								},
								"provider-2": {
									RequiresType: &envoy_filter_http_jwt_authn_v3.JwtRequirement_ProviderName{
										ProviderName: "provider-2",
									},
								},
							},
						}),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
	}).Request(routeType, "https/jwt.example.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"https/jwt.example.com",
				envoy_v3.VirtualHost("jwt.example.com",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
						TypedPerFilterConfig: map[string]*anypb.Any{
							envoy_v3.JWTAuthnFilterName: protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
								RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: "provider-2"},
							}),
						},
					},
				),
			),
		),
	})
}
