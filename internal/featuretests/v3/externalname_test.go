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

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/sirupsen/logrus"

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	"google.golang.org/protobuf/types/known/anypb"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Assert that services of type v1.ServiceTypeExternalName can be
// referenced by an Ingress, or HTTPProxy document.
func TestExternalNameService(t *testing.T) {
	rh, c, done := setup(t, enableExternalNameService(t))
	defer done()

	s1 := fixture.NewService("kuard").
		WithSpec(v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
			ExternalName: "foo.io",
			Type:         v1.ServiceTypeExternalName,
		})

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: s1.Namespace,
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(s1)
	rh.OnAdd(i1)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			externalNameCluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80", "foo.io", 80),
		),
		TypeUrl: clusterType,
	})

	rh.OnDelete(i1)

	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.projectcontour.io").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/kuard/80/a28d1ec01b"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			externalNameCluster("default/kuard/80/a28d1ec01b", "default/kuard", "default_kuard_80", "foo.io", 80),
		),
		TypeUrl: clusterType,
	})

	// After we set the Host header, the cluster should remain
	// the same, but the Route should do update the Host header.
	rh.OnDelete(fixture.NewProxy("kuard").WithSpec(contour_api_v1.HTTPProxySpec{}))
	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.projectcontour.io").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "external.address",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/kuard/80/95e871afaf", "external.address"),
					},
				),
			),
		),
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			externalNameCluster("default/kuard/80/95e871afaf", "default/kuard", "default_kuard_80", "foo.io", 80),
		),
	})

	// Now try the same configuration, but enable HTTP/2. We
	// should still find that the same configuration applies, but
	// TLS is enabled and the SNI server name is overwritten from
	// the Host header.
	rh.OnDelete(fixture.NewProxy("kuard").WithSpec(contour_api_v1.HTTPProxySpec{}))
	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.projectcontour.io").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Protocol: ref.To("h2"),
					Name:     s1.Name,
					Port:     80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "external.address",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/kuard/80/cdbf075ad8", "external.address"),
					},
				),
			),
		),
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				externalNameCluster("default/kuard/80/cdbf075ad8", "default/kuard", "default_kuard_80", "foo.io", 80),
				&envoy_cluster_v3.Cluster{
					TypedExtensionProtocolOptions: map[string]*anypb.Any{
						"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
							&envoy_extensions_upstream_http_v3.HttpProtocolOptions{
								UpstreamProtocolOptions: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
									ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
										ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
									},
								},
							}),
					},
				},
				&envoy_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						envoy_v3.UpstreamTLSContext(nil, "external.address", nil, "h2"),
					),
				},
			),
		),
	})

	// Now try the same configuration, but enable TLS (which
	// means HTTP/1.1 over TLS) rather than HTTP/2. We should get
	// TLS enabled with the overridden SNI name. but no HTTP/2
	// protocol config.
	rh.OnDelete(fixture.NewProxy("kuard").WithSpec(contour_api_v1.HTTPProxySpec{}))
	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.projectcontour.io").
		WithSpec(contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Protocol: ref.To("tls"),
					Name:     s1.Name,
					Port:     80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "external.address",
					}},
				},
			}},
		}),
	)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("kuard.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/kuard/80/f9439c1de8", "external.address"),
					},
				),
			),
		),
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				externalNameCluster("default/kuard/80/f9439c1de8", "default/kuard", "default_kuard_80", "foo.io", 80),
				&envoy_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						envoy_v3.UpstreamTLSContext(nil, "external.address", nil),
					),
				},
			),
		),
	})

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}

	// Create TCPProxy with upstream protocol 'tls' to an externalName type service
	// and verify that the SNI on the upstream request matches the externalName value.
	rh.OnDelete(fixture.NewProxy("kuard").WithSpec(contour_api_v1.HTTPProxySpec{}))
	rh.OnAdd(sec1)
	rh.OnAdd(fixture.NewProxy("kuard").
		WithFQDN("kuard.projectcontour.io").
		WithCertificate(sec1.Name).
		WithSpec(contour_api_v1.HTTPProxySpec{
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Protocol: ref.To("tls"),
					Name:     s1.Name,
					Port:     80,
				}},
			},
		}),
	)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				externalNameCluster("default/kuard/80/7d449598f5", "default/kuard", "default_kuard_80", "foo.io", 80),
				&envoy_cluster_v3.Cluster{
					TransportSocket: envoy_v3.UpstreamTLSTransportSocket(
						envoy_v3.UpstreamTLSContext(nil, "foo.io", nil),
					),
				},
			),
		),
	})
}

func enableExternalNameService(t *testing.T) func(*dag.Builder) {
	return func(b *dag.Builder) {

		log := fixture.NewTestLogger(t)
		log.SetLevel(logrus.DebugLevel)

		b.Processors = []dag.Processor{
			&dag.ListenerProcessor{},
			&dag.IngressProcessor{
				EnableExternalNameService: true,
				FieldLogger:               log.WithField("context", "IngressProcessor"),
			},
			&dag.HTTPProxyProcessor{
				EnableExternalNameService: true,
			},
			&dag.ExtensionServiceProcessor{
				FieldLogger: log.WithField("context", "ExtensionServiceProcessor"),
			},
		}
	}
}
