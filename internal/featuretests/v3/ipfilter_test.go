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

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestIPFilterPolicy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	hp1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vhfilter",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test1.test.com",
				IPAllowFilterPolicy: []contour_api_v1.IPFilterPolicy{{
					Source: contour_api_v1.IPFilterSourceRemote,
					CIDR:   "8.8.8.8/24",
				}},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", virtualHostWithFilters(envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
				&envoy_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routeCluster("default/backend/80/da39a3ee5e"),
				},
			), withFilterConfig("envoy.filters.http.rbac", &envoy_rbac_v3.RBACPerRoute{Rbac: &envoy_rbac_v3.RBAC{
				Rules: &envoy_config_rbac_v3.RBAC{
					Action: envoy_config_rbac_v3.RBAC_ALLOW,
					Policies: map[string]*envoy_config_rbac_v3.Policy{
						"ip-rules": {
							Permissions: []*envoy_config_rbac_v3.Permission{
								{
									Rule: &envoy_config_rbac_v3.Permission_Any{Any: true},
								},
							},
							Principals: []*envoy_config_rbac_v3.Principal{{
								Identifier: &envoy_config_rbac_v3.Principal_RemoteIp{
									RemoteIp: &corev3.CidrRange{
										AddressPrefix: "8.8.8.0",
										PrefixLen:     wrapperspb.UInt32(24),
									},
								},
							}},
						},
					},
				},
			}}),
			)),
		),
		TypeUrl: routeType,
	})

	hp2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "vhfilter",
			Namespace:       s1.Namespace,
			ResourceVersion: "2",
			Generation:      2,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test1.test.com",
				IPAllowFilterPolicy: []contour_api_v1.IPFilterPolicy{{
					Source: contour_api_v1.IPFilterSourceRemote,
					CIDR:   "8.8.8.8/24",
				}},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				IPDenyFilterPolicy: []contour_api_v1.IPFilterPolicy{{
					Source: contour_api_v1.IPFilterSourcePeer,
					CIDR:   "2001:db8::68",
				}},
			}},
		},
	}
	rh.OnUpdate(hp1, hp2)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", virtualHostWithFilters(envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
				&envoy_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routeCluster("default/backend/80/da39a3ee5e"),
					TypedPerFilterConfig: withFilterConfig("envoy.filters.http.rbac", &envoy_rbac_v3.RBACPerRoute{
						Rbac: &envoy_rbac_v3.RBAC{
							Rules: &envoy_config_rbac_v3.RBAC{
								Action: envoy_config_rbac_v3.RBAC_DENY,
								Policies: map[string]*envoy_config_rbac_v3.Policy{
									"ip-rules": {
										Permissions: []*envoy_config_rbac_v3.Permission{
											{
												Rule: &envoy_config_rbac_v3.Permission_Any{Any: true},
											},
										},
										Principals: []*envoy_config_rbac_v3.Principal{{
											Identifier: &envoy_config_rbac_v3.Principal_DirectRemoteIp{
												DirectRemoteIp: &corev3.CidrRange{
													AddressPrefix: "2001:db8::68",
													PrefixLen:     wrapperspb.UInt32(128),
												},
											},
										}},
									},
								},
							},
						},
					}),
				},
			), withFilterConfig("envoy.filters.http.rbac", &envoy_rbac_v3.RBACPerRoute{Rbac: &envoy_rbac_v3.RBAC{
				Rules: &envoy_config_rbac_v3.RBAC{
					Action: envoy_config_rbac_v3.RBAC_ALLOW,
					Policies: map[string]*envoy_config_rbac_v3.Policy{
						"ip-rules": {
							Permissions: []*envoy_config_rbac_v3.Permission{
								{
									Rule: &envoy_config_rbac_v3.Permission_Any{Any: true},
								},
							},
							Principals: []*envoy_config_rbac_v3.Principal{{
								Identifier: &envoy_config_rbac_v3.Principal_RemoteIp{
									RemoteIp: &corev3.CidrRange{
										AddressPrefix: "8.8.8.0",
										PrefixLen:     wrapperspb.UInt32(24),
									},
								},
							}},
						},
					},
				},
			}}),
			)),
		),
		TypeUrl: routeType,
	})

	hp3 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "vhfilter",
			Namespace:       s1.Namespace,
			ResourceVersion: "3",
			Generation:      3,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test1.test.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(hp2, hp3)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http", envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
				&envoy_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routeCluster("default/backend/80/da39a3ee5e"),
				},
			))),
		TypeUrl: routeType,
	})
	rh.OnDelete(hp3)
}

func virtualHostWithFilters(vh *envoy_route_v3.VirtualHost, typedPerFilterConfig map[string]*anypb.Any) *envoy_route_v3.VirtualHost {
	vh.TypedPerFilterConfig = typedPerFilterConfig
	return vh
}
