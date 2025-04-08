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

	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestDirectResponsePolicy_HTTProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	proxy403 := fixture.NewProxy("simple-403").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "directresponse.projectcontour.io"},
			Routes: []contour_v1.Route{{
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
					StatusCode: 403,
					Body:       "forbidden",
				},
			}},
		})

	rh.OnAdd(proxy403)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("directresponse.projectcontour.io",

					&envoy_config_route_v3.Route{
						Match: routePrefix("/"),
						Action: &envoy_config_route_v3.Route_DirectResponse{
							DirectResponse: &envoy_config_route_v3.DirectResponseAction{
								Status: 403,
								Body: &envoy_config_core_v3.DataSource{
									Specifier: &envoy_config_core_v3.DataSource_InlineString{
										InlineString: "forbidden",
									},
								},
							},
						},
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxyNobody := fixture.NewProxy("simple-nobody").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "directresponse.projectcontour.io"},
			Routes: []contour_v1.Route{{
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
					StatusCode: 200,
				},
			}},
		})

	rh.OnUpdate(proxy403, proxyNobody)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("directresponse.projectcontour.io",

					&envoy_config_route_v3.Route{
						Match: routePrefix("/"),
						Action: &envoy_config_route_v3.Route_DirectResponse{
							DirectResponse: &envoy_config_route_v3.DirectResponseAction{
								Status: 200,
							},
						},
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	proxyInvalid := fixture.NewProxy("simple-multiple-match").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "directresponse.projectcontour.io"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
					StatusCode: 200,
				},
			}},
		})

	rh.OnUpdate(proxyNobody, proxyInvalid)

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	})
}

func TestCustomErrorPagePolicy_HTTPProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	// Create an HTTPProxy with a ResponseOverride for 503 errors
	errorPageProxy := fixture.NewProxy("custom-error-page").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "errorpage.projectcontour.io"},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc1",
					Port: 80,
				}},
				ResponseOverridePolicy: []contour_v1.HTTPResponseOverridePolicy{{
					Match: contour_v1.ResponseOverrideMatch{
						StatusCodes: []contour_v1.StatusCodeMatch{{
							Type:  "Value",
							Value: 503,
						}},
					},
					Response: contour_v1.ResponseOverrideResponse{
						ContentType: "text/html",
						Body: contour_v1.ResponseBodyConfig{
							Type:   "Inline",
							Inline: "<html><body><h1>Custom 503 error page</h1></body></html>",
						},
					},
				}},
			}},
		})

	rh.OnAdd(errorPageProxy)

	// The filter should be set with a local_reply_config on the HTTP connection manager
	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("errorpage.projectcontour.io",
					&envoy_config_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Check that the HTTP connection manager has the local_reply_config
	httpListener := defaultHTTPListener()
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})

	hcm := envoyGen.HTTPConnectionManagerBuilder().
		RouteConfigName("ingress_http").
		MetricsPrefix("ingress_http").
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil, contour_v1alpha1.LogLevelInfo)).
		DefaultFilters().
		LocalReplyConfig(&envoy_filter_network_http_connection_manager_v3.LocalReplyConfig{
			Mappers: []*envoy_filter_network_http_connection_manager_v3.ResponseMapper{{
				Filter: &envoy_config_accesslog_v3.AccessLogFilter{
					FilterSpecifier: &envoy_config_accesslog_v3.AccessLogFilter_StatusCodeFilter{
						StatusCodeFilter: &envoy_config_accesslog_v3.StatusCodeFilter{
							Comparison: &envoy_config_accesslog_v3.ComparisonFilter{
								Op: envoy_config_accesslog_v3.ComparisonFilter_EQ,
								Value: &envoy_config_core_v3.RuntimeUInt32{
									DefaultValue: 503,
									RuntimeKey:   "unused",
								},
							},
						},
					},
				},
				Body: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineString{
						InlineString: "<html><body><h1>Custom 503 error page</h1></body></html>",
					},
				},
				BodyFormatOverride: &envoy_config_core_v3.SubstitutionFormatString{
					Format: &envoy_config_core_v3.SubstitutionFormatString_TextFormat{
						TextFormat: "%LOCAL_REPLY_BODY%",
					},
					ContentType: "text/html",
				},
			}},
		}).
		Get()

	httpListener.FilterChains = envoy_v3.FilterChains(hcm)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			httpListener,
			statsListener(),
		),
		TypeUrl: listenerType,
	})
}
