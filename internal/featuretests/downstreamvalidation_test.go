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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s"
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
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	rh.OnAdd(serverTLSSecret)

	clientCASecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clientCASecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			dag.CACertificateKey: []byte(CERTIFICATE),
		},
	}
	rh.OnAdd(clientCASecret)

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(service)

	proxy := fixture.NewProxy("example.com").
		WithSpec(projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName: serverTLSSecret.Name,
					ClientValidation: &projcontour.DownstreamValidation{
						CACertificate: clientCASecret.Name,
					},
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		})

	rh.OnAdd(proxy)

	ingress_http := &v2.Listener{
		Name:    "ingress_http",
		Address: envoy.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy.FilterChains(
			envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
		),
		SocketOptions: envoy.TCPKeepaliveSocketOptions(),
	}

	ingress_https := &v2.Listener{
		Name:    "ingress_https",
		Address: envoy.SocketAddress("0.0.0.0", 8443),
		ListenerFilters: envoy.ListenerFilters(
			envoy.TLSInspector(),
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
		SocketOptions: envoy.TCPKeepaliveSocketOptions(),
	}

	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			ingress_http,
			ingress_https,
			staticListener(),
		),
		TypeUrl: listenerType,
	}).Status(proxy).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)

}
