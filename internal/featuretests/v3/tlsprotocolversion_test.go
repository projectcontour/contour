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
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestTLSProtocolVersion(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	sec1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(sec1)

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			&envoy_config_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("kuard.example.com", sec1,
						httpsFilterFor("kuard.example.com"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
			},
		),
		TypeUrl: listenerType,
	})

	makeIngress := func(minVer, maxVer string) *networking_v1.Ingress {
		return &networking_v1.Ingress{
			ObjectMeta: fixture.ObjectMetaWithAnnotations("simple", map[string]string{
				"projectcontour.io/tls-minimum-protocol-version": minVer,
				"projectcontour.io/tls-maximum-protocol-version": maxVer,
			}),
			Spec: networking_v1.IngressSpec{
				TLS: []networking_v1.IngressTLS{{
					Hosts:      []string{"kuard.example.com"},
					SecretName: sec1.Name,
				}},
				Rules: []networking_v1.IngressRule{{
					Host: "kuard.example.com",
					IngressRuleValue: networking_v1.IngressRuleValue{
						HTTP: &networking_v1.HTTPIngressRuleValue{
							Paths: []networking_v1.HTTPIngressPath{{
								Backend: *featuretests.IngressBackend(s1),
							}},
						},
					},
				}},
			},
		}
	}

	i2 := makeIngress("1.3", "1.2")
	rh.OnUpdate(i1, i2)

	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   listenerType,
	})

	i3 := makeIngress("1.3", "1.3")
	rh.OnUpdate(i1, i3)

	l1 := &envoy_config_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			envoy_v3.FilterChainTLS(
				"kuard.example.com",
				envoyGen.DownstreamTLSContext(
					&dag.Secret{Object: sec1},
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
					envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
					nil,
					nil,
					"h2", "http/1.1"),
				envoy_v3.Filters(httpsFilterFor("kuard.example.com")),
			),
		},
		SocketOptions: envoy_v3.NewSocketOptions().TCPKeepalive().Build(),
	}

	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			l1,
		),
		TypeUrl: listenerType,
	})

	rh.OnDelete(i2)
	rh.OnDelete(i3)

	makeHTTPProxy := func(minVer, maxVer string) *contour_v1.HTTPProxy {
		return &contour_v1.HTTPProxy{
			ObjectMeta: fixture.ObjectMeta("simple"),
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "kuard.example.com",
					TLS: &contour_v1.TLS{
						SecretName:             sec1.Namespace + "/" + sec1.Name,
						MinimumProtocolVersion: minVer,
						MaximumProtocolVersion: maxVer,
					},
				},
				Routes: []contour_v1.Route{{
					Conditions: matchconditions(prefixMatchCondition("/")),
					Services: []contour_v1.Service{{
						Name: s1.Name,
						Port: 80,
					}},
				}},
			},
		}
	}
	hp1 := makeHTTPProxy("1.3", "1.3")
	rh.OnAdd(hp1)
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, l1),
		TypeUrl:   listenerType,
	})

	hp2 := makeHTTPProxy("1.3", "1.2")
	rh.OnUpdate(hp1, hp2)
	c.Request(listenerType, "ingress_https").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   listenerType,
	})
}
