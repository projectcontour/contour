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

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func filterExists(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
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

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_http",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				// TODO since this uses the same helper as the actual Contour code, this is not a very good test.
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager("ingress_http", envoy_v3.FileAccessLogEnvoy("/dev/stdout"), 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
			staticListener()),
	}).Status(p).IsValid()
}

func noRateLimitsDefined(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
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

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration(
				"ingress_http",
				envoy_v3.VirtualHost("foo.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/s1/80/da39a3ee5e"),
					},
				),
			),
		),
	}).Status(p).IsValid()
}

func vhostRateLimitDefined(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
				RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
					Local: &contour_api_v1.LocalRateLimitPolicy{
						Requests: 100,
						Unit:     "minute",
						Burst:    50,
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
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

	vhost := envoy_v3.VirtualHost("foo.com",
		&envoy_route_v3.Route{
			Match:  routePrefix("/"),
			Action: routeCluster("default/s1/80/da39a3ee5e"),
		})
	vhost.TypedPerFilterConfig = withFilterConfig("envoy.filters.http.local_ratelimit",
		&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
			StatPrefix: "vhost.foo.com",
			TokenBucket: &envoy_type_v3.TokenBucket{
				MaxTokens:     150,
				TokensPerFill: protobuf.UInt32(100),
				FillInterval:  protobuf.Duration(time.Minute),
			},
			FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
				DefaultValue: &envoy_type_v3.FractionalPercent{
					Numerator:   100,
					Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
				},
			},
			FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
				DefaultValue: &envoy_type_v3.FractionalPercent{
					Numerator:   100,
					Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
				},
			},
		})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", vhost)),
	}).Status(p).IsValid()
}

func routeRateLimitsDefined(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Prefix: "/s1",
						},
					},
					Services: []contour_api_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
					RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
						Local: &contour_api_v1.LocalRateLimitPolicy{
							Requests: 100,
							Unit:     "minute",
							Burst:    50,
						},
					},
				},
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Prefix: "/s2",
						},
					},
					Services: []contour_api_v1.Service{
						{
							Name: "s2",
							Port: 80,
						},
					},
					RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
						Local: &contour_api_v1.LocalRateLimitPolicy{
							Requests: 5,
							Unit:     "second",
							Burst:    1,
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	vhost := envoy_v3.VirtualHost("foo.com",
		// note, order of routes is reversed here because route sorting of prefixes
		// is reverse alphabetic.
		&envoy_route_v3.Route{
			Match:  routePrefix("/s2"),
			Action: routeCluster("default/s2/80/da39a3ee5e"),
			TypedPerFilterConfig: withFilterConfig("envoy.filters.http.local_ratelimit",
				&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "vhost.foo.com",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     6,
						TokensPerFill: protobuf.UInt32(5),
						FillInterval:  protobuf.Duration(time.Second),
					},
					FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
				}),
		},
		&envoy_route_v3.Route{
			Match:  routePrefix("/s1"),
			Action: routeCluster("default/s1/80/da39a3ee5e"),
			TypedPerFilterConfig: withFilterConfig("envoy.filters.http.local_ratelimit",
				&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "vhost.foo.com",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     150,
						TokensPerFill: protobuf.UInt32(100),
						FillInterval:  protobuf.Duration(time.Minute),
					},
					FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
				}),
		},
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", vhost)),
	}).Status(p).IsValid()
}

func vhostAndRouteRateLimitsDefined(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
				RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
					Local: &contour_api_v1.LocalRateLimitPolicy{
						Requests: 100,
						Unit:     "minute",
						Burst:    50,
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Prefix: "/s1",
						},
					},
					Services: []contour_api_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
					RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
						Local: &contour_api_v1.LocalRateLimitPolicy{
							Requests: 100,
							Unit:     "minute",
							Burst:    50,
						},
					},
				},
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Prefix: "/s2",
						},
					},
					Services: []contour_api_v1.Service{
						{
							Name: "s2",
							Port: 80,
						},
					},
					RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
						Local: &contour_api_v1.LocalRateLimitPolicy{
							Requests: 5,
							Unit:     "second",
							Burst:    1,
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	vhost := envoy_v3.VirtualHost("foo.com",
		// note, order of routes is reversed here because route sorting of prefixes
		// is reverse alphabetic.
		&envoy_route_v3.Route{
			Match:  routePrefix("/s2"),
			Action: routeCluster("default/s2/80/da39a3ee5e"),
			TypedPerFilterConfig: withFilterConfig("envoy.filters.http.local_ratelimit",
				&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "vhost.foo.com",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     6,
						TokensPerFill: protobuf.UInt32(5),
						FillInterval:  protobuf.Duration(time.Second),
					},
					FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
				}),
		},
		&envoy_route_v3.Route{
			Match:  routePrefix("/s1"),
			Action: routeCluster("default/s1/80/da39a3ee5e"),
			TypedPerFilterConfig: withFilterConfig("envoy.filters.http.local_ratelimit",
				&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "vhost.foo.com",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     150,
						TokensPerFill: protobuf.UInt32(100),
						FillInterval:  protobuf.Duration(time.Minute),
					},
					FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
				}),
		},
	)

	vhost.TypedPerFilterConfig = withFilterConfig("envoy.filters.http.local_ratelimit",
		&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
			StatPrefix: "vhost.foo.com",
			TokenBucket: &envoy_type_v3.TokenBucket{
				MaxTokens:     150,
				TokensPerFill: protobuf.UInt32(100),
				FillInterval:  protobuf.Duration(time.Minute),
			},
			FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
				DefaultValue: &envoy_type_v3.FractionalPercent{
					Numerator:   100,
					Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
				},
			},
			FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
				DefaultValue: &envoy_type_v3.FractionalPercent{
					Numerator:   100,
					Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
				},
			},
		})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", vhost)),
	}).Status(p).IsValid()
}

func customResponseCode(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Prefix: "/s1",
						},
					},
					Services: []contour_api_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
					RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
						Local: &contour_api_v1.LocalRateLimitPolicy{
							Requests:           100,
							Unit:               "minute",
							Burst:              50,
							ResponseStatusCode: 500,
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	vhost := envoy_v3.VirtualHost("foo.com",
		&envoy_route_v3.Route{
			Match:  routePrefix("/s1"),
			Action: routeCluster("default/s1/80/da39a3ee5e"),
			TypedPerFilterConfig: withFilterConfig("envoy.filters.http.local_ratelimit",
				&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "vhost.foo.com",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     150,
						TokensPerFill: protobuf.UInt32(100),
						FillInterval:  protobuf.Duration(time.Minute),
					},
					FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					Status: &envoy_type_v3.HttpStatus{Code: envoy_type_v3.StatusCode(500)},
				}),
		},
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", vhost)),
	}).Status(p).IsValid()
}

func customResponseHeaders(t *testing.T, rh cache.ResourceEventHandler, c *Contour) {
	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "proxy1",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Prefix: "/s1",
						},
					},
					Services: []contour_api_v1.Service{
						{
							Name: "s1",
							Port: 80,
						},
					},
					RateLimitPolicy: &contour_api_v1.RateLimitPolicy{
						Local: &contour_api_v1.LocalRateLimitPolicy{
							Requests: 100,
							Unit:     "minute",
							Burst:    50,
							ResponseHeadersToAdd: []contour_api_v1.HeaderValue{
								{
									Name:  "header-name-1",
									Value: "header-value-1",
								},
								{
									Name:  "header-name-2",
									Value: "%HOSTNAME%",
								},
								{
									Name:  "header-name-3",
									Value: "%NON-ENVOY-VAR%",
								},
							},
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	vhost := envoy_v3.VirtualHost("foo.com",
		&envoy_route_v3.Route{
			Match:  routePrefix("/s1"),
			Action: routeCluster("default/s1/80/da39a3ee5e"),
			TypedPerFilterConfig: withFilterConfig("envoy.filters.http.local_ratelimit",
				&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "vhost.foo.com",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     150,
						TokensPerFill: protobuf.UInt32(100),
						FillInterval:  protobuf.Duration(time.Minute),
					},
					FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   100,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
					},
					ResponseHeadersToAdd: []*envoy_core_v3.HeaderValueOption{
						{
							Header: &envoy_core_v3.HeaderValue{
								Key:   "Header-Name-1",
								Value: "header-value-1",
							},
							Append: wrapperspb.Bool(false),
						},
						// a valid Envoy var (%VARNAME%) should
						// pass through as-is
						{
							Header: &envoy_core_v3.HeaderValue{
								Key:   "Header-Name-2",
								Value: "%HOSTNAME%",
							},
							Append: wrapperspb.Bool(false),
						},
						// a non-valid Envoy var should have its '%'
						// symbols escaped
						{
							Header: &envoy_core_v3.HeaderValue{
								Key:   "Header-Name-3",
								Value: "%%NON-ENVOY-VAR%%",
							},
							Append: wrapperspb.Bool(false),
						},
					},
				}),
		},
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", vhost)),
	}).Status(p).IsValid()
}

func TestLocalRateLimiting(t *testing.T) {
	subtests := map[string]func(*testing.T, cache.ResourceEventHandler, *Contour){
		"LocalRateLimitFilterExists":           filterExists,
		"NoRateLimitsDefined":                  noRateLimitsDefined,
		"VirtualHostRateLimitDefined":          vhostRateLimitDefined,
		"RouteRateLimitsDefined":               routeRateLimitsDefined,
		"VirtualHostAndRouteRateLimitsDefined": vhostAndRouteRateLimitsDefined,
		"CustomResponseCode":                   customResponseCode,
		"CustomResponseHeaders":                customResponseHeaders,
	}

	for n, f := range subtests {
		f := f
		t.Run(n, func(t *testing.T) {
			rh, c, done := setup(t)
			defer done()

			// Add common test fixtures.
			rh.OnAdd(fixture.NewService("s1").WithPorts(corev1.ServicePort{Port: 80}))
			rh.OnAdd(fixture.NewService("s2").WithPorts(corev1.ServicePort{Port: 80}))

			f(t, rh, c)
		})
	}
}
