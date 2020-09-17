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

package featuretests

import (
	"path"
	"testing"
	"time"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_config_filter_http_ext_authz_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	envoyv2 "github.com/projectcontour/contour/internal/envoy/v2"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

func grpcCluster(name string) *envoy_config_filter_http_ext_authz_v2.ExtAuthz_GrpcService {
	return &envoy_config_filter_http_ext_authz_v2.ExtAuthz_GrpcService{
		GrpcService: &envoy_api_v2_core.GrpcService{
			TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
					ClusterName: name,
				},
			},
		},
	}
}

func authzResponseTimeout(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(projcontour.AuthorizationServer{
			ExtensionServiceRef: projcontour.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			ResponseTimeout: "10m",
		}).
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	cluster := grpcCluster("extension/auth/extension")
	cluster.GrpcService.Timeout = protobuf.Duration(10 * time.Minute)

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_api_v2.Listener{
				Name:    "ingress_https",
				Address: envoyv2.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoyv2.ListenerFilters(
					envoyv2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
						},
						authzFilterFor(
							fqdn,
							&envoy_config_filter_http_ext_authz_v2.ExtAuthz{
								Services:               cluster,
								ClearRouteCache:        true,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type.HttpStatus{
									Code: envoy_type.StatusCode_Forbidden,
								},
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoyv2.TCPKeepaliveSocketOptions(),
			},
			staticListener()),
	}).Status(p).Like(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusValid,
	})
}

func authzInvalidResponseTimeout(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(projcontour.AuthorizationServer{
			ExtensionServiceRef: projcontour.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			ResponseTimeout: "invalid-timeout",
		}).
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	cluster := grpcCluster("extension/auth/extension")
	cluster.GrpcService.Timeout = protobuf.Duration(10 * time.Minute)

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, staticListener()),
	}).Status(p).Equals(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusInvalid,
		Description:   `Spec.Virtualhost.Authorization.ResponseTimeout is invalid: unable to parse timeout string "invalid-timeout": time: invalid duration "invalid-timeout"`,
	})
}

func authzFailOpen(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const fqdn = "failopen.projectcontour.io"

	p := fixture.NewProxy("proxy").
		WithFQDN(fqdn).
		WithCertificate("certificate").
		WithAuthServer(projcontour.AuthorizationServer{
			ExtensionServiceRef: projcontour.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			FailOpen: true,
		}).
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{Name: "app-server", Port: 80}},
			}},
		})

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_api_v2.Listener{
				Name:    "ingress_https",
				Address: envoyv2.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoyv2.ListenerFilters(
					envoyv2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
						},
						authzFilterFor(
							fqdn,
							&envoy_config_filter_http_ext_authz_v2.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       true,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type.HttpStatus{
									Code: envoy_type.StatusCode_Forbidden,
								},
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoyv2.TCPKeepaliveSocketOptions(),
			},
			staticListener()),
	}).Status(p).Like(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusValid,
	})
}

func authzFallbackIncompat(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := fixture.NewProxy("proxy").
		WithFQDN("echo.projectcontour.io").
		WithCertificate("certificate").
		WithAuthServer(projcontour.AuthorizationServer{
			ExtensionServiceRef: projcontour.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
		}).
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{Name: "app-server", Port: 80}},
			}},
		})

	p.Spec.VirtualHost.TLS.EnableFallbackCertificate = true

	rh.OnAdd(p)

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, staticListener()),
	}).Status(p).Equals(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusInvalid,
		Description:   `Spec.Virtualhost.TLS fallback & client authorization are incompatible`,
	})
}

func authzOverrideDisabled(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	const enabled = "enabled.projectcontour.io"
	const disabled = "disabled.projectcontour.io"

	var extensionRef = projcontour.ExtensionServiceReference{
		Namespace: "auth",
		Name:      "extension",
	}

	rh.OnAdd(fixture.NewProxy("enabled").
		WithFQDN(enabled).
		WithCertificate("certificate").
		WithAuthServer(projcontour.AuthorizationServer{
			ExtensionServiceRef: extensionRef,
			AuthPolicy:          &projcontour.AuthorizationPolicy{Disabled: false},
		}).
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/disabled")),
				Services:   []projcontour.Service{{Name: "app-server", Port: 80}},
				AuthPolicy: &projcontour.AuthorizationPolicy{Disabled: true},
			}, {
				Conditions: matchconditions(prefixMatchCondition("/default")),
				Services:   []projcontour.Service{{Name: "app-server", Port: 80}},
			}},
		}),
	)

	rh.OnAdd(fixture.NewProxy("disabled").
		WithFQDN(disabled).
		WithCertificate("certificate").
		WithAuthServer(projcontour.AuthorizationServer{
			ExtensionServiceRef: extensionRef,
			AuthPolicy:          &projcontour.AuthorizationPolicy{Disabled: true},
		}).
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/enabled")),
				Services:   []projcontour.Service{{Name: "app-server", Port: 80}},
				AuthPolicy: &projcontour.AuthorizationPolicy{},
			}, {
				Conditions: matchconditions(prefixMatchCondition("/default")),
				Services:   []projcontour.Service{{Name: "app-server", Port: 80}},
			}},
		}),
	)

	// For each proxy, the `/default` route should have the
	//' same authorization enablement as the root proxy, and
	// the ' other path should have the opposite enablement.

	disabledConfig := withFilterConfig("envoy.filters.http.ext_authz",
		&envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute{
			Override: &envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute_Disabled{
				Disabled: true,
			},
		})

	c.Request(routeType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoyv2.RouteConfiguration(
				path.Join("https", disabled),
				envoyv2.VirtualHost(disabled,
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/enabled"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:                routePrefix("/default"),
						Action:               routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: disabledConfig,
					},
				),
			),
			envoyv2.RouteConfiguration(
				path.Join("https", enabled),
				envoyv2.VirtualHost(enabled,
					&envoy_api_v2_route.Route{
						Match:                routePrefix("/disabled"),
						Action:               routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: disabledConfig,
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/default"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
					},
				),
			),
			envoyv2.RouteConfiguration(
				"ingress_http",
				envoyv2.VirtualHost(disabled,
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/enabled"),
						Action: withRedirect(),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/default"),
						Action: withRedirect(),
					},
				),
				envoyv2.VirtualHost(enabled,
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/disabled"),
						Action: withRedirect(),
					},
					&envoy_api_v2_route.Route{
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
		WithAuthServer(projcontour.AuthorizationServer{
			ExtensionServiceRef: projcontour.ExtensionServiceReference{
				Namespace: "auth",
				Name:      "extension",
			},
			AuthPolicy: &projcontour.AuthorizationPolicy{
				Context: map[string]string{
					"root-element":   "root",
					"common-element": "root",
				},
			},
		}).
		WithSpec(projcontour.HTTPProxySpec{
			Includes: []projcontour.Include{{
				Name: "proxy-leaf",
			}},
		}),
	)

	rh.OnAdd(fixture.NewProxy("proxy-leaf").
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "app-server",
					Port: 80,
				}},
				AuthPolicy: &projcontour.AuthorizationPolicy{
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

	c.Request(routeType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoyv2.RouteConfiguration(
				path.Join("https", fqdn),
				envoyv2.VirtualHost(fqdn,
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/app-server/80/da39a3ee5e"),
						TypedPerFilterConfig: withFilterConfig("envoy.filters.http.ext_authz",
							&envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute{
								Override: &envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute_CheckSettings{
									CheckSettings: &envoy_config_filter_http_ext_authz_v2.CheckSettings{
										ContextExtensions: context,
									},
								},
							}),
					},
				),
			),
			envoyv2.RouteConfiguration(
				"ingress_http",
				envoyv2.VirtualHost(fqdn,
					&envoy_api_v2_route.Route{
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
		WithAuthServer(projcontour.AuthorizationServer{}).
		WithSpec(projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "app-server",
					Port: 80,
				}},
			}},
		})

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = projcontour.ExtensionServiceReference{
		APIVersion: "foo/bar",
		Namespace:  "",
		Name:       "",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, staticListener()),
	}).Status(invalid).Equals(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusInvalid,
		Description:   `Spec.Virtualhost.Authorization.ServiceRef specifies an unsupported resource version "foo/bar"`,
	})

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = projcontour.ExtensionServiceReference{
		APIVersion: "projectcontour.io/v1alpha1",
		Namespace:  "missing",
		Name:       "extension",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, staticListener()),
	}).Status(invalid).Equals(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusInvalid,
		Description:   `Spec.Virtualhost.Authorization.ServiceRef extension service "missing/extension" not found`,
	})

	invalid.Spec.VirtualHost.Authorization.ExtensionServiceRef = projcontour.ExtensionServiceReference{
		Namespace: "auth",
		Name:      "extension",
	}

	rh.OnDelete(invalid)
	rh.OnAdd(invalid)

	c.Request(listenerType).Equals(&envoy_api_v2.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_api_v2.Listener{
				Name:    "ingress_https",
				Address: envoyv2.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoyv2.ListenerFilters(
					envoyv2.TLSInspector(),
				),
				FilterChains: []*envoy_api_v2_listener.FilterChain{
					filterchaintls(fqdn,
						&corev1.Secret{
							ObjectMeta: fixture.ObjectMeta("certificate"),
							Type:       "kubernetes.io/tls",
							Data:       secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
						},
						authzFilterFor(
							fqdn,
							&envoy_config_filter_http_ext_authz_v2.ExtAuthz{
								Services:               grpcCluster("extension/auth/extension"),
								ClearRouteCache:        true,
								FailureModeAllow:       false,
								IncludePeerCertificate: true,
								StatusOnError: &envoy_type.HttpStatus{
									Code: envoy_type.StatusCode_Forbidden,
								},
							},
						),
						nil, "h2", "http/1.1"),
				},
				SocketOptions: envoyv2.TCPKeepaliveSocketOptions(),
			},
			staticListener()),
	}).Status(invalid).Like(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusValid,
	})
}

func TestAuthorization(t *testing.T) {
	subtests := map[string]func(*testing.T, cache.ResourceEventHandler, *Contour){
		"MissingExtension":       authzInvalidReference,
		"MergeRouteContext":      authzMergeRouteContext,
		"OverrideDisabled":       authzOverrideDisabled,
		"FallbackIncompat":       authzFallbackIncompat,
		"FailOpen":               authzFailOpen,
		"ResponseTimeout":        authzResponseTimeout,
		"InvalidResponseTimeout": authzInvalidResponseTimeout,
	}

	for n, f := range subtests {
		f := f
		t.Run(n, func(t *testing.T) {
			rh, c, done := setup(t)
			defer done()

			// Add common test fixtures.

			rh.OnAdd(fixture.NewService("auth/oidc-server").
				WithPorts(corev1.ServicePort{Port: 8081}))

			rh.OnAdd(endpoints("auth", "oidc-server", corev1.EndpointSubset{
				Addresses: addresses("192.168.183.21"),
				Ports:     ports(port("", 8081)),
			}))

			rh.OnAdd(&v1alpha1.ExtensionService{
				ObjectMeta: fixture.ObjectMeta("auth/extension"),
				Spec: v1alpha1.ExtensionServiceSpec{
					Services: []v1alpha1.ExtensionServiceTarget{
						{Name: "oidc-server", Port: 8081},
					},
				},
			})

			rh.OnAdd(fixture.NewService("app-server").
				WithPorts(corev1.ServicePort{Port: 80}))

			rh.OnAdd(endpoints("auth", "app-server", corev1.EndpointSubset{
				Addresses: addresses("192.168.183.21"),
				Ports:     ports(port("", 80)),
			}))

			rh.OnAdd(&corev1.Secret{
				ObjectMeta: fixture.ObjectMeta("certificate"),
				Type:       "kubernetes.io/tls",
				Data:       secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
			})

			f(t, rh, c)
		})
	}
}
