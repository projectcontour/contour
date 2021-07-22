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

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestHeaderPolicy_ReplaceHeader_HTTProxy(t *testing.T) {
	// Enable ExternalName processing here because
	// we need to check that host rewrites work in combination
	// with ExternalName.
	rh, c, done := setup(t, enableExternalNameService(t))
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "goodbye.planet",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/svc1/80/da39a3ee5e", "goodbye.planet"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Non-Host header
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "x-header",
						Value: "goodbye.planet",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
						RequestHeadersToAdd: []*envoy_core_v3.HeaderValueOption{{
							Header: &envoy_core_v3.HeaderValue{
								Key:   "X-Header",
								Value: "goodbye.planet",
							},
							Append: &wrappers.BoolValue{
								Value: false,
							},
						}},
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Empty value for replaceHeader in HeadersPolicy
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "hello.world"},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name: "Host",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
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
		WithSpec(v1.ServiceSpec{
			ExternalName: "goodbye.planet",
			Type:         v1.ServiceTypeExternalName,
			Ports: []v1.ServicePort{{
				Port: 443,
				Name: "https",
			}},
		}),
	)

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	// Proxy with SNI
	rh.OnAdd(fixture.NewProxy("simple").WithSpec(
		contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "hello.world",
				TLS:  &contour_api_v1.TLS{SecretName: "foo"},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "externalname",
					Port: 443,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "goodbye.planet",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match: routePrefix("/"),
						Action: &envoy_route_v3.Route_Redirect{
							Redirect: &envoy_route_v3.RedirectAction{
								SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
									HttpsRedirect: true,
								},
							},
						},
					}),
			),
			envoy_v3.RouteConfiguration("https/hello.world",
				envoy_v3.VirtualHost("hello.world",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/externalname/443/da39a3ee5e", "goodbye.planet"),
					},
				)),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(externalNameCluster("default/externalname/443/da39a3ee5e", "default/externalname/https", "default_externalname_443", "goodbye.planet", 443), nil, "goodbye.planet", "goodbye.planet", nil),
		),
		TypeUrl: clusterType,
	})
}
