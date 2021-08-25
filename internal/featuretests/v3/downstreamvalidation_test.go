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

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDownstreamTLSCertificateValidation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	serverTLSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "serverTLSSecret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}
	rh.OnAdd(serverTLSSecret)

	clientCASecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clientCASecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			dag.CACertificateKey: []byte(featuretests.CERTIFICATE),
		},
	}
	rh.OnAdd(clientCASecret)

	service := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(service)

	proxy := fixture.NewProxy("example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: clientCASecret.Name,
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})

	rh.OnAdd(proxy)

	ingressHTTPS := &envoy_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					CACertificate: &dag.Secret{
						Object: clientCASecret,
					},
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPS,
			statsListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy).IsValid()

	rh.OnUpdate(proxy, fixture.NewProxy("example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						SkipClientCertValidation: true,
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		}))

	ingressHTTPSSkipVerify := &envoy_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					SkipClientCertValidation: true,
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSSkipVerify,
			statsListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy).IsValid()

	rh.OnUpdate(proxy, fixture.NewProxy("example.com").
		WithSpec(contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						SkipClientCertValidation: true,
						CACertificate:            clientCASecret.Name,
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		}))

	ingressHTTPSSkipVerifyWithCA := &envoy_listener_v3.Listener{
		Name:    "ingress_https",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy_v3.ListenerFilters(
			envoy_v3.TLSInspector(),
		),
		FilterChains: appendFilterChains(
			filterchaintls("example.com", serverTLSSecret,
				httpsFilterFor("example.com"),
				&dag.PeerValidationContext{
					SkipClientCertValidation: true,
					CACertificate: &dag.Secret{
						Object: clientCASecret,
					},
				},
				"h2", "http/1.1",
			),
		),
		SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			ingressHTTPSSkipVerifyWithCA,
			statsListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy).IsValid()
}
