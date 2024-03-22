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

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

// Test that Ingress without TLS secrets generate the
// appropriate route config.
func TestIngressWildcardHostHTTP(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("svc").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)
	defaultBackend := fixture.NewService("default").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(defaultBackend)

	wildcardIngressV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "wildcard-v1",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			// Add a default backend to keep us honest.
			DefaultBackend: featuretests.IngressBackend(defaultBackend),
			Rules: []networking_v1.IngressRule{{
				Host: "*.foo.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(wildcardIngressV1)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*", &envoy_config_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routecluster("default/default/80/da39a3ee5e"),
				}),
				envoy_v3.VirtualHost("*.foo.com",
					&envoy_config_route_v3.Route{
						Match: &envoy_config_route_v3.RouteMatch{
							PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
								Prefix: "/",
							},
							Headers: []*envoy_config_route_v3.HeaderMatcher{{
								Name: ":authority",
								HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_StringMatch{
									StringMatch: &envoy_matcher_v3.StringMatcher{
										MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
											SafeRegex: &envoy_matcher_v3.RegexMatcher{
												Regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.foo\\.com(:[0-9]+)?",
											},
										},
									},
								},
							}},
						},
						Action: routeCluster("default/svc/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

// Test that HTTPProxy without TLS secrets generate the
// appropriate route config.
func TestHTTPProxyWildcardFQDN(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc := fixture.NewService("svc").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)

	rh.OnAdd(fixture.NewProxy("wildcard").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "*.projectcontour.io",
			}, Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "svc",
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*.projectcontour.io", &envoy_config_route_v3.Route{
					Match: &envoy_config_route_v3.RouteMatch{
						PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
							Prefix: "/",
						},
						Headers: []*envoy_config_route_v3.HeaderMatcher{{
							Name: ":authority",
							HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_StringMatch{
								StringMatch: &envoy_matcher_v3.StringMatcher{
									MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
										SafeRegex: &envoy_matcher_v3.RegexMatcher{
											Regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.projectcontour\\.io(:[0-9]+)?",
										},
									},
								},
							},
						}},
					},
					Action: routecluster("default/svc/80/da39a3ee5e"),
				}),
			),
		),
		TypeUrl: routeType,
	})
}

// Test Ingress with wildcard host and TLS secret for the same wildcard generates
// the correct filter chain and secret.
func TestIngressWildcardHostHTTPSWildcardSecret(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	sec := featuretests.TLSSecret(t, "wildcard-tls-secret", &featuretests.ServerCertificate)
	rh.OnAdd(sec)

	svc := fixture.NewService("svc").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc)
	defaultBackend := fixture.NewService("default").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(defaultBackend)

	wildcardIngressTLS := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "wildcard-tls",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"*.foo-tls.com"},
				SecretName: sec.Name,
			}},
			// Add a default backend to keep us honest.
			DefaultBackend: featuretests.IngressBackend(defaultBackend),
			Rules: []networking_v1.IngressRule{{
				Host: "*.foo-tls.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(wildcardIngressTLS)

	c.Request(secretType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, secret(sec)),
		TypeUrl:   secretType,
	})

	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("*.foo-tls.com", sec,
						httpsFilterFor("*.foo-tls.com"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	c.Request(routeType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("https/*.foo-tls.com",
				envoy_v3.VirtualHost("*.foo-tls.com",
					&envoy_config_route_v3.Route{
						Match: &envoy_config_route_v3.RouteMatch{
							PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
								Prefix: "/",
							},
							Headers: []*envoy_config_route_v3.HeaderMatcher{{
								Name: ":authority",
								HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_StringMatch{
									StringMatch: &envoy_matcher_v3.StringMatcher{
										MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
											SafeRegex: &envoy_matcher_v3.RegexMatcher{
												Regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.foo-tls\\.com(:[0-9]+)?",
											},
										},
									},
								},
							}},
						},
						Action: routeCluster("default/svc/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*", &envoy_config_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routecluster("default/default/80/da39a3ee5e"),
				}),
				envoy_v3.VirtualHost("*.foo-tls.com",
					&envoy_config_route_v3.Route{
						Match: &envoy_config_route_v3.RouteMatch{
							PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
								Prefix: "/",
							},
							Headers: []*envoy_config_route_v3.HeaderMatcher{{
								Name: ":authority",
								HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_StringMatch{
									StringMatch: &envoy_matcher_v3.StringMatcher{
										MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
											SafeRegex: &envoy_matcher_v3.RegexMatcher{
												Regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.foo-tls\\.com(:[0-9]+)?",
											},
										},
									},
								},
							}},
						},
						Action: routeCluster("default/svc/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}
