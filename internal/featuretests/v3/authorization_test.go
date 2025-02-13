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
	"strings"
	"testing"
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	core_v1 "k8s.io/api/core/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

const defaultResponseTimeout = time.Minute * 60

func grpcCluster(name string) *envoy_filter_http_ext_authz_v3.ExtAuthz_GrpcService {
	return &envoy_filter_http_ext_authz_v3.ExtAuthz_GrpcService{
		GrpcService: &envoy_config_core_v3.GrpcService{
			TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
					ClusterName: name,
					Authority:   strings.ReplaceAll(name, "/", "."),
				},
			},
			Timeout: durationpb.New(defaultResponseTimeout),
		},
	}
}

func authzResponseTimeout(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: contour_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			ResponseTimeout: "10m",
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	cluster := grpcCluster("extension/auth/extension")
	cluster.GrpcService.Timeout = durationpb.New(10 * time.Minute)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),

			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					filterchaintls(fqdn, featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
						authzFilterFor(
							fqdn,
							&envoy_filter_http_ext_authz_v3.ExtAuthz{
								Services:               cluster,
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
			},

			statsListener()),
	}).Status(p).IsValid()
}

func authzInvalidResponseTimeout(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: contour_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			ResponseTimeout: "invalid-timeout",
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(p).HasError(contour_v1.ConditionTypeAuthError, "AuthResponseTimeoutInvalid", `Spec.Virtualhost.Authorization.ResponseTimeout is invalid: unable to parse timeout string "invalid-timeout": time: invalid duration "invalid-timeout"`)
}

func authzFailOpen(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: contour_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			FailOpen: true,
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					filterchaintls(fqdn, featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
						authzFilterFor(
							fqdn,
							&envoy_filter_http_ext_authz_v3.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       true,
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
			},
			statsListener()),
	}).Status(p).IsValid()
}

func authzFallbackIncompat(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := fixture.NewProxy("proxy").
		WithFQDN("echo.projectcontour.io").
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: contour_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	p.Spec.VirtualHost.TLS.EnableFallbackCertificate = true

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(p).HasError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client authorization are incompatible")
}

func authzOverrideDisabled(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	const enabled = "enabled.projectcontour.io"
	const disabled = "disabled.projectcontour.io"

	extensionRef := contour_v1.ExtensionServiceReference{
		Namespace: "auth",
		Name:      "extension",
	}

	rh.OnAdd(fixture.NewProxy("enabled").
		WithFQDN(enabled).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: extensionRef,
			AuthPolicy:          &contour_v1.AuthorizationPolicy{Disabled: false},
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/disabled")),
				Services:   []contour_v1.Service{{Name: "app-server", Port: 80}},
				AuthPolicy: &contour_v1.AuthorizationPolicy{Disabled: true},
			}, {
				Conditions: matchconditions(prefixMatchCondition("/default")),
				Services:   []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		}),
	)

	rh.OnAdd(fixture.NewProxy("disabled").
		WithFQDN(disabled).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: extensionRef,
			AuthPolicy:          &contour_v1.AuthorizationPolicy{Disabled: true},
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/enabled")),
				Services:   []contour_v1.Service{{Name: "app-server", Port: 80}},
				AuthPolicy: &contour_v1.AuthorizationPolicy{},
			}, {
				Conditions: matchconditions(prefixMatchCondition("/default")),
				Services:   []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		}),
	)

	// For each proxy, the `/default` route should have the
	// same authorization enablement as the root proxy, and
	// the other path should have the opposite enablement.

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				path.Join("https", disabled),
				envoy_v3.VirtualHost(disabled,
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/enabled"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
					},
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/default"),
						Action:               routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					},
				),
			),
			envoy_v3.RouteConfiguration(
				path.Join("https", enabled),
				envoy_v3.VirtualHost(enabled,
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/disabled"),
						Action:               routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					},
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/default"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost(disabled,
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/enabled"),
						Action:               withRedirect(),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					},
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/default"),
						Action:               withRedirect(),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					},
				),
				envoy_v3.VirtualHost(enabled,
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/disabled"),
						Action:               withRedirect(),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					},
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/default"),
						Action:               withRedirect(),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					},
				),
			),
		),
	})
}

func authzMergeRouteContext(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	const fqdn = "echo.projectcontour.io"

	rh.OnAdd(fixture.NewProxy("proxy-root").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: contour_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			AuthPolicy: &contour_v1.AuthorizationPolicy{
				Context: map[string]string{
					"root-element":   "root",
					"common-element": "root",
				},
			},
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Includes: []contour_v1.Include{{
				Name: "proxy-leaf",
			}},
		}),
	)

	rh.OnAdd(fixture.NewProxy("proxy-leaf").
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "app-server",
					Port: 80,
				}},
				AuthPolicy: &contour_v1.AuthorizationPolicy{
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

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				path.Join("https", fqdn),
				envoy_v3.VirtualHost(fqdn,
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: withFilterConfig(envoy_v3.ExtAuthzFilterName,
							&envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute{
								Override: &envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
									CheckSettings: &envoy_filter_http_ext_authz_v3.CheckSettings{
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

func authzInvalidReference(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	const fqdn = "echo.projectcontour.io"

	invalid := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "app-server",
					Port: 80,
				}},
			}},
		})

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = contour_v1.ExtensionServiceReference{
		APIVersion: "foo/bar",
		Namespace:  "missing",
		Name:       "extension",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(invalid).HasError(contour_v1.ConditionTypeAuthError, "AuthBadResourceVersion", `Spec.Virtualhost.Authorization.extensionRef specifies an unsupported resource version "foo/bar"`)

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = contour_v1.ExtensionServiceReference{
		APIVersion: "projectcontour.io/v1alpha1",
		Namespace:  "missing",
		Name:       "extension",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, statsListener()),
	}).Status(invalid).HasError(contour_v1.ConditionTypeAuthError, "ExtensionServiceNotFound", `Spec.Virtualhost.Authorization.ServiceRef extension service "missing/extension" not found`)

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = contour_v1.ExtensionServiceReference{
		Namespace: "auth",
		Name:      "extension",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					filterchaintls(fqdn, featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
						authzFilterFor(
							fqdn,
							&envoy_filter_http_ext_authz_v3.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       false,
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
			},
			statsListener()),
	}).Status(invalid).IsValid()
}

func authzWithRequestBodyBufferSettings(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	const fqdn = "buffersettings.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(contour_v1.AuthorizationServer{
			ExtensionServiceRef: contour_v1.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			FailOpen: true,
			WithRequestBody: &contour_v1.AuthorizationServerBufferSettings{
				MaxRequestBytes:     100,
				AllowPartialMessage: true,
				PackAsBytes:         true,
			},
		}).
		WithSpec(contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					filterchaintls(fqdn, featuretests.TLSSecret(t, "certificate", &featuretests.ServerCertificate),
						authzFilterFor(
							fqdn,
							&envoy_filter_http_ext_authz_v3.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       true,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type_v3.HttpStatus{
									Code: envoy_type_v3.StatusCode_Forbidden,
								},
								TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
								WithRequestBody: &envoy_filter_http_ext_authz_v3.BufferSettings{
									MaxRequestBytes:     100,
									AllowPartialMessage: true,
									PackAsBytes:         true,
								},
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener()),
	}).Status(p).IsValid()
}

func TestAuthorization(t *testing.T) {
	subtests := map[string]func(*testing.T, ResourceEventHandlerWrapper, *Contour){
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
