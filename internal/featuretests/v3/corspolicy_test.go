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

	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCorsPolicy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	// Allow origin
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin: []string{"*"},
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.CORSVirtualHost("hello.world",
					&envoy_route_v3.CorsPolicy{
						AllowCredentials: &wrappers.BoolValue{Value: false},
						AllowOriginStringMatch: []*matcher_v3.StringMatcher{{
							MatchPattern: &matcher_v3.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						}},
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/svc1/80/da39a3ee5e"),
					}),
			),
		),
		TypeUrl: routeType,
	})

	// Allow credentials
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:      []string{"*"},
					AllowCredentials: true,
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.CORSVirtualHost("hello.world",
					&envoy_route_v3.CorsPolicy{
						AllowOriginStringMatch: []*matcher_v3.StringMatcher{{
							MatchPattern: &matcher_v3.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						}},
						AllowCredentials: &wrappers.BoolValue{Value: true},
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/svc1/80/da39a3ee5e"),
					}),
			),
		),
		TypeUrl: routeType,
	})

	// Allow methods
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:      []string{"*"},
					AllowCredentials: true,
					AllowMethods:     []contour_api_v1.CORSHeaderValue{"GET", "POST", "OPTIONS"},
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.CORSVirtualHost("hello.world",
					&envoy_route_v3.CorsPolicy{
						AllowOriginStringMatch: []*matcher_v3.StringMatcher{{
							MatchPattern: &matcher_v3.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						}},
						AllowCredentials: &wrappers.BoolValue{Value: true},
						AllowMethods:     "GET,POST,OPTIONS",
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/svc1/80/da39a3ee5e"),
					}),
			),
		),
		TypeUrl: routeType,
	})

	// Allow headers
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:      []string{"*"},
					AllowCredentials: true,
					AllowHeaders:     []contour_api_v1.CORSHeaderValue{"custom-header-1", "custom-header-2"},
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.CORSVirtualHost("hello.world",
					&envoy_route_v3.CorsPolicy{
						AllowOriginStringMatch: []*matcher_v3.StringMatcher{{
							MatchPattern: &matcher_v3.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						}},
						AllowCredentials: &wrappers.BoolValue{Value: true},
						AllowHeaders:     "custom-header-1,custom-header-2",
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/svc1/80/da39a3ee5e"),
					}),
			),
		),
		TypeUrl: routeType,
	})

	// Expose headers
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:      []string{"*"},
					AllowCredentials: true,
					ExposeHeaders:    []contour_api_v1.CORSHeaderValue{"custom-header-1", "custom-header-2"},
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.CORSVirtualHost("hello.world",
					&envoy_route_v3.CorsPolicy{
						AllowOriginStringMatch: []*matcher_v3.StringMatcher{{
							MatchPattern: &matcher_v3.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						},
						},
						AllowCredentials: &wrappers.BoolValue{Value: true},
						ExposeHeaders:    "custom-header-1,custom-header-2",
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/svc1/80/da39a3ee5e"),
					}),
			),
		),
		TypeUrl: routeType,
	})

	// Max Age
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:      []string{"*"},
					AllowCredentials: true,
					MaxAge:           "10m",
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.CORSVirtualHost("hello.world",
					&envoy_route_v3.CorsPolicy{
						AllowOriginStringMatch: []*matcher_v3.StringMatcher{{
							MatchPattern: &matcher_v3.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						}},
						AllowCredentials: &wrappers.BoolValue{Value: true},
						MaxAge:           "600",
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/svc1/80/da39a3ee5e"),
					}),
			),
		),
		TypeUrl: routeType,
	})

	// Disable preflight request caching
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:      []string{"*"},
					AllowCredentials: true,
					MaxAge:           "0s",
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.CORSVirtualHost("hello.world",
					&envoy_route_v3.CorsPolicy{
						AllowOriginStringMatch: []*matcher_v3.StringMatcher{{
							MatchPattern: &matcher_v3.StringMatcher_Exact{
								Exact: "*",
							},
							IgnoreCase: true,
						}},
						AllowCredentials: &wrappers.BoolValue{Value: true},
						MaxAge:           "0",
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/svc1/80/da39a3ee5e"),
					}),
			),
		),
		TypeUrl: routeType,
	})

	// Virtual hosts with an invalid max age in their policy are not added
	invvhost := &contour_api_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:      []string{"*"},
					AllowCredentials: true,
					MaxAge:           "-10m",
				},
			}, Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(invvhost)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http")),
		TypeUrl: routeType,
	}).Status(invvhost).IsInvalid()

}
