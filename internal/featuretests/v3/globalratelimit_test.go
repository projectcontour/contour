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
	envoy_config_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
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
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
)

func globalRateLimitFilterExists(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
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

	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})

	// replace the default filter chains with an HCM that includes the global
	// rate limit filter.
	hcm := envoyGen.HTTPConnectionManagerBuilder().
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
				}),
			},
		}).
		Get()

	httpListener.FilterChains = envoy_v3.FilterChains(hcm)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			httpListener,
			statsListener()),
	}).Status(p).IsValid()
}

func globalRateLimitNoRateLimitsDefined(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour, tls tlsConfig) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				RateLimitPolicy: &contour_v1.RateLimitPolicy{
					Global: &contour_v1.GlobalRateLimitPolicy{
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
		},
	}

	if tls.enabled {
		p.Spec.VirtualHost.TLS = &contour_v1.TLS{
			SecretName:                "tls-cert",
			EnableFallbackCertificate: tls.fallbackEnabled,
		}
	}

	rh.OnAdd(p)
	c.Status(p).IsValid()

	switch tls.enabled {
	case true:
		c.Request(routeType, "https/foo.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl: routeType,
			Resources: resources(t,
				envoy_v3.RouteConfiguration(
					"https/foo.com",
					envoy_v3.VirtualHost("foo.com",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/s1/80/da39a3ee5e"),
						},
					),
				),
			),
		})
		if tls.fallbackEnabled {
			c.Request(routeType, "ingress_fallbackcert").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
				TypeUrl: routeType,
				Resources: resources(t,
					envoy_v3.RouteConfiguration(
						"ingress_fallbackcert",
						envoy_v3.VirtualHost("foo.com",
							&envoy_config_route_v3.Route{
								Match:  routePrefix("/"),
								Action: routeCluster("default/s1/80/da39a3ee5e"),
							},
						),
					),
				),
			})
		}
	default:
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl: routeType,
			Resources: resources(t,
				envoy_v3.RouteConfiguration(
					"ingress_http",
					envoy_v3.VirtualHost("foo.com",
						&envoy_config_route_v3.Route{
							Match:  routePrefix("/"),
							Action: routeCluster("default/s1/80/da39a3ee5e"),
						},
					),
				),
			),
		})
	}
}

func globalRateLimitVhostRateLimitDefined(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour, tls tlsConfig) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				RateLimitPolicy: &contour_v1.RateLimitPolicy{
					Global: &contour_v1.GlobalRateLimitPolicy{
						Descriptors: []contour_v1.RateLimitDescriptor{
							{
								Entries: []contour_v1.RateLimitDescriptorEntry{
									{
										RemoteAddress: &contour_v1.RemoteAddressDescriptor{},
									},
									{
										GenericKey: &contour_v1.GenericKeyDescriptor{Value: "generic-key-value"},
									},
								},
							},
						},
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
		},
	}

	if tls.enabled {
		p.Spec.VirtualHost.TLS = &contour_v1.TLS{
			SecretName:                "tls-cert",
			EnableFallbackCertificate: tls.fallbackEnabled,
		}
	}

	rh.OnAdd(p)
	c.Status(p).IsValid()

	route := &envoy_config_route_v3.Route{
		Match:  routePrefix("/"),
		Action: routeCluster("default/s1/80/da39a3ee5e"),
	}

	vhost := envoy_v3.VirtualHost("foo.com", route)
	vhost.RateLimits = []*envoy_config_route_v3.RateLimit{
		{
			Actions: []*envoy_config_route_v3.RateLimit_Action{
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
						RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
					},
				},
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
						GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{DescriptorValue: "generic-key-value"},
					},
				},
			},
		},
	}

	switch tls.enabled {
	case true:
		c.Request(routeType, "https/foo.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("https/foo.com", vhost)),
		})
		if tls.fallbackEnabled {
			c.Request(routeType, "ingress_fallbackcert").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
				TypeUrl:   routeType,
				Resources: resources(t, envoy_v3.RouteConfiguration("ingress_fallbackcert", vhost)),
			})
		}
	default:
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("ingress_http", vhost)),
		})
	}
}

func globalRateLimitRouteRateLimitDefined(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour, tls tlsConfig) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				RateLimitPolicy: &contour_v1.RateLimitPolicy{
					Global: &contour_v1.GlobalRateLimitPolicy{
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
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Descriptors: []contour_v1.RateLimitDescriptor{
								{
									Entries: []contour_v1.RateLimitDescriptorEntry{
										{
											RemoteAddress: &contour_v1.RemoteAddressDescriptor{},
										},
										{
											GenericKey: &contour_v1.GenericKeyDescriptor{Value: "generic-key-value"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if tls.enabled {
		p.Spec.VirtualHost.TLS = &contour_v1.TLS{
			SecretName:                "tls-cert",
			EnableFallbackCertificate: tls.fallbackEnabled,
		}
	}

	rh.OnAdd(p)
	c.Status(p).IsValid()

	route := &envoy_config_route_v3.Route{
		Match: routePrefix("/"),
		Action: routeCluster("default/s1/80/da39a3ee5e", func(r *envoy_config_route_v3.Route_Route) {
			r.Route.RateLimits = []*envoy_config_route_v3.RateLimit{
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{DescriptorValue: "generic-key-value"},
							},
						},
					},
				},
			}
		}),
	}

	vhost := envoy_v3.VirtualHost("foo.com", route)

	switch tls.enabled {
	case true:
		c.Request(routeType, "https/foo.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("https/foo.com", vhost)),
		})
		if tls.fallbackEnabled {
			c.Request(routeType, "ingress_fallbackcert").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
				TypeUrl:   routeType,
				Resources: resources(t, envoy_v3.RouteConfiguration("ingress_fallbackcert", vhost)),
			})
		}
	default:
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("ingress_http", vhost)),
		})
	}
}

func globalRateLimitVhostAndRouteRateLimitDefined(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour, tls tlsConfig) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				RateLimitPolicy: &contour_v1.RateLimitPolicy{
					Global: &contour_v1.GlobalRateLimitPolicy{
						Descriptors: []contour_v1.RateLimitDescriptor{
							{
								Entries: []contour_v1.RateLimitDescriptorEntry{
									{
										RemoteAddress: &contour_v1.RemoteAddressDescriptor{},
									},
									{
										GenericKey: &contour_v1.GenericKeyDescriptor{Value: "generic-key-value-vhost"},
									},
								},
							},
						},
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
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Descriptors: []contour_v1.RateLimitDescriptor{
								{
									Entries: []contour_v1.RateLimitDescriptorEntry{
										{
											RemoteAddress: &contour_v1.RemoteAddressDescriptor{},
										},
										{
											GenericKey: &contour_v1.GenericKeyDescriptor{Value: "generic-key-value"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if tls.enabled {
		p.Spec.VirtualHost.TLS = &contour_v1.TLS{
			SecretName:                "tls-cert",
			EnableFallbackCertificate: tls.fallbackEnabled,
		}
	}

	rh.OnAdd(p)
	c.Status(p).IsValid()

	route := &envoy_config_route_v3.Route{
		Match: routePrefix("/"),
		Action: routeCluster("default/s1/80/da39a3ee5e", func(r *envoy_config_route_v3.Route_Route) {
			r.Route.RateLimits = []*envoy_config_route_v3.RateLimit{
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{DescriptorValue: "generic-key-value"},
							},
						},
					},
				},
			}
		}),
	}

	vhost := envoy_v3.VirtualHost("foo.com", route)
	vhost.RateLimits = []*envoy_config_route_v3.RateLimit{
		{
			Actions: []*envoy_config_route_v3.RateLimit_Action{
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
						RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
					},
				},
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
						GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{DescriptorValue: "generic-key-value-vhost"},
					},
				},
			},
		},
	}

	switch tls.enabled {
	case true:
		c.Request(routeType, "https/foo.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("https/foo.com", vhost)),
		})
		if tls.fallbackEnabled {
			c.Request(routeType, "ingress_fallbackcert").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
				TypeUrl:   routeType,
				Resources: resources(t, envoy_v3.RouteConfiguration("ingress_fallbackcert", vhost)),
			})
		}
	default:
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("ingress_http", vhost)),
		})
	}
}

func defaultGlobalRateLimitVhostRateLimitDefined(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour, tls tlsConfig) {
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
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Descriptors: []contour_v1.RateLimitDescriptor{
								{
									Entries: []contour_v1.RateLimitDescriptorEntry{
										{
											RemoteAddress: &contour_v1.RemoteAddressDescriptor{},
										},
										{
											GenericKey: &contour_v1.GenericKeyDescriptor{Value: "generic-key-value"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if tls.enabled {
		p.Spec.VirtualHost.TLS = &contour_v1.TLS{
			SecretName:                "tls-cert",
			EnableFallbackCertificate: tls.fallbackEnabled,
		}
	}

	rh.OnAdd(p)
	c.Status(p).IsValid()

	route := &envoy_config_route_v3.Route{
		Match: routePrefix("/"),
		Action: routeCluster("default/s1/80/da39a3ee5e", func(r *envoy_config_route_v3.Route_Route) {
			r.Route.RateLimits = []*envoy_config_route_v3.RateLimit{
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{DescriptorValue: "generic-key-value"},
							},
						},
					},
				},
			}
		}),
	}

	vhost := envoy_v3.VirtualHost("foo.com", route)
	vhost.RateLimits = []*envoy_config_route_v3.RateLimit{
		{
			Actions: []*envoy_config_route_v3.RateLimit_Action{
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
						GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
							DescriptorKey:   "generic-key-vhost",
							DescriptorValue: "generic-key-vhost",
						},
					},
				},
			},
		},
	}

	switch tls.enabled {
	case true:
		c.Request(routeType, "https/foo.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("https/foo.com", vhost)),
		})
		if tls.fallbackEnabled {
			c.Request(routeType, "ingress_fallbackcert").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
				TypeUrl:   routeType,
				Resources: resources(t, envoy_v3.RouteConfiguration("ingress_fallbackcert", vhost)),
			})
		}
	default:
		c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
			TypeUrl:   routeType,
			Resources: resources(t, envoy_v3.RouteConfiguration("ingress_http", vhost)),
		})
	}
}

func globalRateLimitMultipleDescriptorsAndEntries(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
	p := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				RateLimitPolicy: &contour_v1.RateLimitPolicy{
					Global: &contour_v1.GlobalRateLimitPolicy{
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
					RateLimitPolicy: &contour_v1.RateLimitPolicy{
						Global: &contour_v1.GlobalRateLimitPolicy{
							Descriptors: []contour_v1.RateLimitDescriptor{
								// first descriptor
								{
									Entries: []contour_v1.RateLimitDescriptorEntry{
										{
											RemoteAddress: &contour_v1.RemoteAddressDescriptor{},
										},
										{
											GenericKey: &contour_v1.GenericKeyDescriptor{Value: "generic-key-value"},
										},
									},
								},
								// second descriptor
								{
									Entries: []contour_v1.RateLimitDescriptorEntry{
										{
											RequestHeader: &contour_v1.RequestHeaderDescriptor{HeaderName: "X-Contour", DescriptorKey: "header-descriptor"},
										},
										{
											GenericKey: &contour_v1.GenericKeyDescriptor{Key: "generic-key-key", Value: "generic-key-value-2"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	rh.OnAdd(p)
	c.Status(p).IsValid()

	route := &envoy_config_route_v3.Route{
		Match: routePrefix("/"),
		Action: routeCluster("default/s1/80/da39a3ee5e", func(r *envoy_config_route_v3.Route_Route) {
			r.Route.RateLimits = []*envoy_config_route_v3.RateLimit{
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{DescriptorValue: "generic-key-value"},
							},
						},
					},
				},
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
								RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
									HeaderName:    "X-Contour",
									DescriptorKey: "header-descriptor",
								},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
									DescriptorKey:   "generic-key-key",
									DescriptorValue: "generic-key-value-2",
								},
							},
						},
					},
				},
			}
		}),
	}

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   routeType,
		Resources: resources(t, envoy_v3.RouteConfiguration("ingress_http", envoy_v3.VirtualHost("foo.com", route))),
	})
}

type tlsConfig struct {
	enabled         bool
	fallbackEnabled bool
}

func TestGlobalRateLimiting(t *testing.T) {
	var (
		tlsDisabled     = tlsConfig{}
		tlsEnabled      = tlsConfig{enabled: true}
		fallbackEnabled = tlsConfig{enabled: true, fallbackEnabled: true}
	)

	subtests := map[string]func(*testing.T, ResourceEventHandlerWrapper, *Contour){
		"GlobalRateLimitFilterExists": globalRateLimitFilterExists,

		// test cases for insecure/non-TLS vhosts
		"NoRateLimitsDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitNoRateLimitsDefined(t, rh, c, tlsDisabled)
		},
		"VirtualHostRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitVhostRateLimitDefined(t, rh, c, tlsDisabled)
		},
		"RouteRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitRouteRateLimitDefined(t, rh, c, tlsDisabled)
		},
		"VirtualHostAndRouteRateLimitsDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitVhostAndRouteRateLimitDefined(t, rh, c, tlsDisabled)
		},
		"VirtualHostDefaultGlobalRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			defaultGlobalRateLimitVhostRateLimitDefined(t, rh, c, tlsDisabled)
		},

		// test cases for secure/TLS vhosts
		"TLSNoRateLimitsDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitNoRateLimitsDefined(t, rh, c, tlsEnabled)
		},
		"TLSVirtualHostRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitVhostRateLimitDefined(t, rh, c, tlsEnabled)
		},
		"TLSRouteRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitRouteRateLimitDefined(t, rh, c, tlsEnabled)
		},
		"TLSVirtualHostAndRouteRateLimitsDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitVhostAndRouteRateLimitDefined(t, rh, c, tlsEnabled)
		},
		"TLSVirtualHostDefaultGlobalRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			defaultGlobalRateLimitVhostRateLimitDefined(t, rh, c, tlsEnabled)
		},

		// test cases for secure/TLS vhosts with fallback cert enabled
		"FallbackNoRateLimitsDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitNoRateLimitsDefined(t, rh, c, fallbackEnabled)
		},
		"FallbackVirtualHostRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitVhostRateLimitDefined(t, rh, c, fallbackEnabled)
		},
		"FallbackRouteRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitRouteRateLimitDefined(t, rh, c, fallbackEnabled)
		},
		"FallbackVirtualHostAndRouteRateLimitsDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			globalRateLimitVhostAndRouteRateLimitDefined(t, rh, c, fallbackEnabled)
		},
		"FallbackVirtualHostDefaultGlobalRateLimitDefined": func(t *testing.T, rh ResourceEventHandlerWrapper, c *Contour) {
			defaultGlobalRateLimitVhostRateLimitDefined(t, rh, c, fallbackEnabled)
		},

		"MultipleDescriptorsAndEntriesDefined": globalRateLimitMultipleDescriptorsAndEntries,
	}

	for n, f := range subtests {
		f := f
		t.Run(n, func(t *testing.T) {
			rh, c, done := setup(t,
				func(cfg *xdscache_v3.ListenerConfig) {
					cfg.RateLimitConfig = &xdscache_v3.RateLimitConfig{
						ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
							ExtensionService: k8s.NamespacedNameFrom("projectcontour/ratelimit"),
						},
						Domain: "contour",
					}
				},
				func(b *dag.Builder) {
					for _, processor := range b.Processors {
						if httpProxyProcessor, ok := processor.(*dag.HTTPProxyProcessor); ok {
							httpProxyProcessor.FallbackCertificate = &types.NamespacedName{
								Name:      "fallback-cert",
								Namespace: "default",
							}

							httpProxyProcessor.GlobalRateLimitService = &contour_v1alpha1.RateLimitServiceConfig{
								ExtensionService: contour_v1alpha1.NamespacedName{
									Name:      "extension",
									Namespace: "ratelimit",
								},
								Domain: "contour",
								DefaultGlobalRateLimitPolicy: &contour_v1.GlobalRateLimitPolicy{
									Descriptors: []contour_v1.RateLimitDescriptor{
										{
											Entries: []contour_v1.RateLimitDescriptorEntry{
												{
													GenericKey: &contour_v1.GenericKeyDescriptor{
														Key:   "generic-key-vhost",
														Value: "generic-key-vhost",
													},
												},
											},
										},
									},
								},
							}
						}
					}
				},
			)

			defer done()

			// Add common test fixtures.
			rh.OnAdd(fixture.NewService("s1").WithPorts(core_v1.ServicePort{Port: 80}))
			rh.OnAdd(fixture.NewService("s2").WithPorts(core_v1.ServicePort{Port: 80}))
			rh.OnAdd(featuretests.TLSSecret(t, "tls-cert", &featuretests.ServerCertificate))
			rh.OnAdd(featuretests.TLSSecret(t, "fallback-cert", &featuretests.ServerCertificate))

			f(t, rh, c)
		})
	}
}
