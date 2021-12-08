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
	"path"
	"testing"
	"time"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	ratelimit_config_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	ratelimit_filter_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

const defaultResponseTimeout = time.Minute * 60

func grpcCluster(name string) *envoy_config_filter_http_ext_authz_v3.ExtAuthz_GrpcService {
	return &envoy_config_filter_http_ext_authz_v3.ExtAuthz_GrpcService{
		GrpcService: &envoy_core_v3.GrpcService{
			TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
					ClusterName: name,
				},
			},
			Timeout: protobuf.Duration(defaultResponseTimeout),
		},
	}
}

func authzResponseTimeout(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			ResponseTimeout: "10m",
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	cluster := grpcCluster("extension/auth/extension")
	cluster.GrpcService.Timeout = protobuf.Duration(10 * time.Minute)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),

			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
						},
						authzFilterFor(
							fqdn,
							&envoy_config_filter_http_ext_authz_v3.ExtAuthz{
								Services:               cluster,
								ClearRouteCache:        true,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type.HttpStatus{
									Code: envoy_type.StatusCode_Forbidden,
								},
								TransportApiVersion: envoy_core_v3.ApiVersion_V3,
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},

			statsListener()),
	}).Status(p).IsValid()
}

func authzInvalidResponseTimeout(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			ResponseTimeout: "invalid-timeout",
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(p).HasError(contour_api_v1.ConditionTypeAuthError, "AuthResponseTimeoutInvalid", `Spec.Virtualhost.Authorization.ResponseTimeout is invalid: unable to parse timeout string "invalid-timeout": time: invalid duration "invalid-timeout"`)
}

func authzFailOpen(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			FailOpen: true,
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
						},
						authzFilterFor(
							fqdn,
							&envoy_config_filter_http_ext_authz_v3.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       true,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type.HttpStatus{
									Code: envoy_type.StatusCode_Forbidden,
								},
								TransportApiVersion: envoy_core_v3.ApiVersion_V3,
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			statsListener()),
	}).Status(p).IsValid()
}

func authzFallbackIncompat(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := fixture.NewProxy("proxy").
		WithFQDN("echo.projectcontour.io").
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	p.Spec.VirtualHost.TLS.EnableFallbackCertificate = true

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(p).HasError(contour_api_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client authorization are incompatible")
}

func authzOverrideDisabled(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const enabled = "enabled.projectcontour.io"
	const disabled = "disabled.projectcontour.io"

	var extensionRef = contour_api_v1.ExtensionServiceReference{
		Namespace: "auth",
		Name:      "extension",
	}

	rh.OnAdd(fixture.NewProxy("enabled").
		WithFQDN(enabled).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: extensionRef,
			AuthPolicy:          &contour_api_v1.AuthorizationPolicy{Disabled: false},
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/disabled")),
				Services:   []contour_api_v1.Service{{Name: "app-server", Port: 80}},
				AuthPolicy: &contour_api_v1.AuthorizationPolicy{Disabled: true},
			}, {
				Conditions: matchconditions(prefixMatchCondition("/default")),
				Services:   []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		}),
	)

	rh.OnAdd(fixture.NewProxy("disabled").
		WithFQDN(disabled).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: extensionRef,
			AuthPolicy:          &contour_api_v1.AuthorizationPolicy{Disabled: true},
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/enabled")),
				Services:   []contour_api_v1.Service{{Name: "app-server", Port: 80}},
				AuthPolicy: &contour_api_v1.AuthorizationPolicy{},
			}, {
				Conditions: matchconditions(prefixMatchCondition("/default")),
				Services:   []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		}),
	)

	// For each proxy, the `/default` route should have the
	// same authorization enablement as the root proxy, and
	// the other path should have the opposite enablement.

	disabledConfig := withFilterConfig("envoy.filters.http.ext_authz",
		&envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute_Disabled{
				Disabled: true,
			},
		})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				path.Join("https", disabled),
				envoy_v3.VirtualHost(disabled,
					&envoy_route_v3.Route{
						Match:  routePrefix("/enabled"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:                routePrefix("/default"),
						Action:               routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: disabledConfig,
					},
				),
			),
			envoy_v3.RouteConfiguration(
				path.Join("https", enabled),
				envoy_v3.VirtualHost(enabled,
					&envoy_route_v3.Route{
						Match:                routePrefix("/disabled"),
						Action:               routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: disabledConfig,
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/default"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost(disabled,
					&envoy_route_v3.Route{
						Match:  routePrefix("/enabled"),
						Action: withRedirect(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/default"),
						Action: withRedirect(),
					},
				),
				envoy_v3.VirtualHost(enabled,
					&envoy_route_v3.Route{
						Match:  routePrefix("/disabled"),
						Action: withRedirect(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/default"),
						Action: withRedirect(),
					},
				),
			),
		),
	})
}

func authzMergeRouteContext(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "echo.projectcontour.io"

	rh.OnAdd(fixture.NewProxy("proxy-root").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			AuthPolicy: &contour_api_v1.AuthorizationPolicy{
				Context: map[string]string{
					"root-element":   "root",
					"common-element": "root",
				},
			},
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Includes: []contour_api_v1.Include{{
				Name: "proxy-leaf",
			}},
		}),
	)

	rh.OnAdd(fixture.NewProxy("proxy-leaf").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "app-server",
					Port: 80,
				}},
				AuthPolicy: &contour_api_v1.AuthorizationPolicy{
					Context: map[string]string{
						"common-element": "leaf",
						"leaf-element":   "leaf",
					},
				},
			}},
		}),
	)

	// Ensure the final route context is merged with leaf entries
	// overwriting root entries.
	context := map[string]string{
		"root-element":   "root",
		"common-element": "leaf",
		"leaf-element":   "leaf",
	}

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				path.Join("https", fqdn),
				envoy_v3.VirtualHost(fqdn,
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: withFilterConfig("envoy.filters.http.ext_authz",
							&envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute{
								Override: &envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
									CheckSettings: &envoy_config_filter_http_ext_authz_v3.CheckSettings{
										ContextExtensions: context,
									},
								},
							}),
					},
				),
			),
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost(fqdn,
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: withRedirect(),
					},
				),
			),
		),
	})
}

func authzInvalidReference(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "echo.projectcontour.io"

	invalid := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "app-server",
					Port: 80,
				}},
			}},
		})

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = contour_api_v1.ExtensionServiceReference{
		APIVersion: "foo/bar",
		Namespace:  "",
		Name:       "",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(invalid).HasError(contour_api_v1.ConditionTypeAuthError, "AuthBadResourceVersion", `Spec.Virtualhost.Authorization.extensionRef specifies an unsupported resource version "foo/bar"`)

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = contour_api_v1.ExtensionServiceReference{
		APIVersion: "projectcontour.io/v1alpha1",
		Namespace:  "missing",
		Name:       "extension",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(invalid).HasError(contour_api_v1.ConditionTypeAuthError, "ExtensionServiceNotFound", `Spec.Virtualhost.Authorization.ServiceRef extension service "missing/extension" not found`)

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = contour_api_v1.ExtensionServiceReference{
		Namespace: "auth",
		Name:      "extension",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
						},
						authzFilterFor(
							fqdn,
							&envoy_config_filter_http_ext_authz_v3.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       false,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type.HttpStatus{
									Code: envoy_type.StatusCode_Forbidden,
								},
								TransportApiVersion: envoy_core_v3.ApiVersion_V3,
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			statsListener()),
	}).Status(invalid).IsValid()
}

func authzWithRequestBodyBufferSettings(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "buffersettings.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			FailOpen: true,
			WithRequestBody: &contour_api_v1.AuthorizationServerBufferSettings{
				MaxRequestBytes:     100,
				AllowPartialMessage: true,
				PackAsBytes:         true,
			},
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
						},
						authzFilterFor(
							fqdn,
							&envoy_config_filter_http_ext_authz_v3.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       true,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type.HttpStatus{
									Code: envoy_type.StatusCode_Forbidden,
								},
								TransportApiVersion: envoy_core_v3.ApiVersion_V3,
								WithRequestBody: &envoy_config_filter_http_ext_authz_v3.BufferSettings{
									MaxRequestBytes:     100,
									AllowPartialMessage: true,
									PackAsBytes:         true,
								},
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			statsListener()),
	}).Status(p).IsValid()
}

func TestAuthorization(t *testing.T) {
	subtests := map[string]func(*testing.T, cache.ResourceEventHandler, *Contour){
		"MissingExtension":                   authzInvalidReference,
		"MergeRouteContext":                  authzMergeRouteContext,
		"OverrideDisabled":                   authzOverrideDisabled,
		"FallbackIncompat":                   authzFallbackIncompat,
		"FailOpen":                           authzFailOpen,
		"ResponseTimeout":                    authzResponseTimeout,
		"InvalidResponseTimeout":             authzInvalidResponseTimeout,
		"AuthzWithRequestBodyBufferSettings": authzWithRequestBodyBufferSettings,
	}

	for n, f := range subtests {
		f := f
		t.Run(n, func(t *testing.T) {
			rh, c, done := setup(t)
			defer done()

			// Add common test fixtures.

			rh.OnAdd(fixture.NewService("auth/oidc-server").
				WithPorts(corev1.ServicePort{Port: 8081}))

			rh.OnAdd(featuretests.Endpoints("auth", "oidc-server", corev1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.21"),
				Ports:     featuretests.Ports(featuretests.Port("", 8081)),
			}))

			rh.OnAdd(&v1alpha1.ExtensionService{
				ObjectMeta: fixture.ObjectMeta("auth/extension"),
				Spec: v1alpha1.ExtensionServiceSpec{
					Services: []v1alpha1.ExtensionServiceTarget{
						{Name: "oidc-server", Port: 8081},
					},
					TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
						Response: defaultResponseTimeout.String(),
					},
				},
			})

			rh.OnAdd(fixture.NewService("app-server").
				WithPorts(corev1.ServicePort{Port: 80}))

			rh.OnAdd(featuretests.Endpoints("auth", "app-server", corev1.EndpointSubset{
				Addresses: featuretests.Addresses("192.168.183.21"),
				Ports:     featuretests.Ports(featuretests.Port("", 80)),
			}))

			rh.OnAdd(&corev1.Secret{
				ObjectMeta: fixture.ObjectMeta("certificate"),
				Type:       "kubernetes.io/tls",
				Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
			})

			f(t, rh, c)
		})
	}
}

func TestAuthzBeforeRateLimiting(t *testing.T) {
	rh, c, done := setup(
		t,
		func(cfg *xdscache_v3.ListenerConfig) {
			cfg.RateLimitConfig = &xdscache_v3.RateLimitConfig{
				ExtensionService: k8s.NamespacedNameFrom("projectcontour/ratelimit"),
				Domain:           "contour",
			}
		},
	)
	defer done()

	// Add ext auth service
	rh.OnAdd(fixture.NewService("auth/oidc-server").
		WithPorts(corev1.ServicePort{Port: 8081}))

	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("auth/extension"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "oidc-server", Port: 8081},
			},
			TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
				Response: defaultResponseTimeout.String(),
			},
		},
	})

	// Add rate limit service
	rh.OnAdd(fixture.NewService("projectcontour/ratelimit").
		WithPorts(corev1.ServicePort{Port: 8081}))

	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("projectcontour/ratelimit"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "ratelimit", Port: 8081},
			},
		},
	})

	// Add app service
	rh.OnAdd(fixture.NewService("app-server").
		WithPorts(corev1.ServicePort{Port: 80}))

	rh.OnAdd(&corev1.Secret{
		ObjectMeta: fixture.ObjectMeta("certificate"),
		Type:       "kubernetes.io/tls",
		Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	const fqdn = "authbefore.ratelimiting.projectcontour.io"

	// HTTPProxy.Spec.AuthorizationService.AuthBeforeRateLimiting == false
	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_api_v1.AuthorizationServer{
			ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			AuthBeforeRateLimiting: false,
		}).
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	// Get an expected HTTP listener that replaces the default filter chains
	// with an HCM that includes the global rate limit filter.
	httpListener := defaultHTTPListener()
	hcm := envoy_v3.HTTPConnectionManagerBuilder().
		RouteConfigName("ingress_http").
		MetricsPrefix("ingress_http").
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil)).
		DefaultFilters().
		AddFilter(&http.HttpFilter{
			Name: wellknown.HTTPRateLimit,
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
					Domain:          "contour",
					FailureModeDeny: true,
					RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
						GrpcService: &envoy_core_v3.GrpcService{
							TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
								EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
									ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
								},
							},
						},
						TransportApiVersion: envoy_core_v3.ApiVersion_V3,
					},
				}),
			},
		}).
		Get()
	httpListener.FilterChains = envoy_v3.FilterChains(hcm)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
						},
						envoy_v3.HTTPConnectionManagerBuilder().
							AddFilter(envoy_v3.FilterMisdirectedRequests(fqdn)).
							DefaultFilters().
							AddFilter(&http.HttpFilter{
								Name: "envoy.filters.http.ratelimit",
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
										Domain:          "contour",
										FailureModeDeny: true,
										RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
											GrpcService: &envoy_core_v3.GrpcService{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
													},
												},
											},
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
										},
									}),
								},
							}).
							// Auth filter is expected to come *after* global rate limit filter.
							AddFilter(&http.HttpFilter{
								Name: "envoy.filters.http.ext_authz",
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_config_filter_http_ext_authz_v3.ExtAuthz{
										Services:               grpcCluster("extension/auth/extension"),
										ClearRouteCache:        true,
										IncludePeerCertificate: true,
										StatusOnError: &envoy_type.HttpStatus{
											Code: envoy_type.StatusCode_Forbidden,
										},
										TransportApiVersion: envoy_core_v3.ApiVersion_V3,
									}),
								},
							}).
							RouteConfigName(path.Join("https", fqdn)).
							MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
							AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil)).
							Get(),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			statsListener()),
	}).Status(p).IsValid()

	// HTTPProxy.Spec.AuthorizationService.AuthBeforeRateLimiting == true
	rh.OnDelete(p)
	p.Spec.VirtualHost.Authorization.AuthBeforeRateLimiting = true
	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
						},
						envoy_v3.HTTPConnectionManagerBuilder().
							AddFilter(envoy_v3.FilterMisdirectedRequests(fqdn)).
							DefaultFilters().
							// Auth filter is expected to come *before* local/global rate limit filters.
							AddFilterBefore(&http.HttpFilter{
								Name: "envoy.filters.http.ext_authz",
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&envoy_config_filter_http_ext_authz_v3.ExtAuthz{
										Services:               grpcCluster("extension/auth/extension"),
										ClearRouteCache:        true,
										IncludePeerCertificate: true,
										StatusOnError: &envoy_type.HttpStatus{
											Code: envoy_type.StatusCode_Forbidden,
										},
										TransportApiVersion: envoy_core_v3.ApiVersion_V3,
									}),
								},
							}, "local_ratelimit").
							AddFilter(&http.HttpFilter{
								Name: "envoy.filters.http.ratelimit",
								ConfigType: &http.HttpFilter_TypedConfig{
									TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
										Domain:          "contour",
										FailureModeDeny: true,
										RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
											GrpcService: &envoy_core_v3.GrpcService{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
													},
												},
											},
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
										},
									}),
								},
							}).
							RouteConfigName(path.Join("https", fqdn)).
							MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
							AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil)).
							Get(),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			statsListener()),
	}).Status(p).IsValid()
}
