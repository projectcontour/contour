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
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestHeaderPolicy_ReplaceHeader_HTTProxy(t *testing.T) {
	// Enable ExternalName processing here because
	// we need to check that host rewrites work in combination
	// with ExternalName.
	rh, c, done := setup(t, enableExternalNameService(t))
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "goodbye.planet",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/svc1/80/3eb3d00648", "goodbye.planet"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Non-Host header
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "x-header",
						Value: "goodbye.planet",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
						RequestHeadersToAdd: []*envoy_config_core_v3.HeaderValueOption{{
							Header: &envoy_config_core_v3.HeaderValue{
								Key:   "X-Header",
								Value: "goodbye.planet",
							},
							AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
						}},
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Empty value for replaceHeader in HeadersPolicy
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name: "Host",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	rh.OnAdd(fixture.NewService("externalname").
		Annotate("projectcontour.io/upstream-protocol.tls", "https,443").
		WithSpec(core_v1.ServiceSpec{
			ExternalName: "goodbye.planet",
			Type:         core_v1.ServiceTypeExternalName,
			Ports: []core_v1.ServicePort{{
				Port: 443,
				Name: "https",
			}},
		}),
	)

	rh.OnAdd(featuretests.TLSSecret(t, "foo", &featuretests.ServerCertificate))

	// Proxy with SNI
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "hello.world",
				TLS:  &contour_v1.TLS{SecretName: "foo"},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "externalname",
					Port: 443,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "goodbye.planet",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/"),
						Action:               envoy_v3.UpgradeHTTPS(),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					}),
			),
			envoy_v3.RouteConfiguration("https/hello.world",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/externalname/443/9ebffe8f28", "goodbye.planet"),
					},
				)),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(externalNameCluster("default/externalname/443/9ebffe8f28", "default/externalname/https", "default_externalname_443", "goodbye.planet", 443), nil, "goodbye.planet", "goodbye.planet", nil, nil),
		),
		TypeUrl: clusterType,
	})
}

func TestHeaderPolicy_ReplaceHostHeader_HTTProxy(t *testing.T) {
	// Enable ExternalName processing here because
	// we need to check that host rewrites work in combination
	// with ExternalName.
	rh, c, done := setup(t, enableExternalNameService(t))
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "%REQ(x-goodbye-planet)%",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewriteHeader("default/svc1/80/da39a3ee5e", "X-Goodbye-Planet"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	rh.OnAdd(fixture.NewService("externalname").
		Annotate("projectcontour.io/upstream-protocol.tls", "https,443").
		WithSpec(core_v1.ServiceSpec{
			ExternalName: "goodbye.planet",
			Type:         core_v1.ServiceTypeExternalName,
			Ports: []core_v1.ServicePort{{
				Port: 443,
				Name: "https",
			}},
		}),
	)

	rh.OnAdd(featuretests.TLSSecret(t, "foo", &featuretests.ServerCertificate))

	// Proxy with SNI
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "hello.world",
				TLS:  &contour_v1.TLS{SecretName: "foo"},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "externalname",
					Port: 443,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "%REQ(x-goodbye-planet)%",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:                routePrefix("/"),
						Action:               envoy_v3.UpgradeHTTPS(),
						TypedPerFilterConfig: envoy_v3.DisabledExtAuthConfig(),
					}),
			),
			envoy_v3.RouteConfiguration("https/hello.world",
				envoy_v3.VirtualHost("hello.world",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewriteHeader("default/externalname/443/9ebffe8f28", "X-Goodbye-Planet"),
					},
				)),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(externalNameCluster("default/externalname/443/9ebffe8f28", "default/externalname/https", "default_externalname_443", "goodbye.planet", 443), nil, "goodbye.planet", "goodbye.planet", nil, nil),
		),
		TypeUrl: clusterType,
	})
}
