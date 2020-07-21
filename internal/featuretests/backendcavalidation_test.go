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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestClusterServiceTLSBackendCAValidation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string][]byte{
			dag.CACertificateKey: []byte(CERTIFICATE),
		},
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: secret.Namespace,
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.tls": "securebackend,443",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "securebackend",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 443,
				}},
			}},
		},
	}
	rh.OnAdd(secret)
	rh.OnAdd(svc)
	rh.OnAdd(p1)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that there is a regular, non validation enabled cluster in CDS.
	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443"), nil, "", ""),
		),
		TypeUrl: clusterType,
	})

	p2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 443,
					UpstreamValidation: &projcontour.UpstreamValidation{
						CACertificate: secret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		},
	}
	rh.OnUpdate(p1, p2)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the cluster now has a certificate and subject name.
	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/98c0f31c72", "default/kuard/securebackend", "default_kuard_443"), []byte(CERTIFICATE), "subjname", ""),
		),
		TypeUrl: clusterType,
	})

	// Contour does not use SDS to transmit the CA for upstream validation, issue 1405,
	// assert that SDS is empty.
	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		// we are asking for all SDS responses, the list is empty so
		// resources is nil, not []any.Any{} -- an empty slice.
		Resources: nil,
		TypeUrl:   secretType,
	})

	rh.OnDelete(p2)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "www.example.com"},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/a")),
				Services: []projcontour.Service{{
					Name: svc.Name,
					Port: 443,
					UpstreamValidation: &projcontour.UpstreamValidation{
						CACertificate: secret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			&v2.Listener{
				Name:    "ingress_http",
				Address: envoy.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy.FilterChains(
					envoy.HTTPConnectionManager("ingress_http", envoy.FileAccessLogEnvoy("/dev/stdout"), 0),
				),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
			},
			staticListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the cluster now has a certificate and subject name.
	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/98c0f31c72", "default/kuard/securebackend", "default_kuard_443"), []byte(CERTIFICATE), "subjname", ""),
		),
		TypeUrl: clusterType,
	})

	// Contour does not use SDS to transmit the CA for upstream validation, issue 1405,
	// assert that SDS is empty.
	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		// we are asking for all SDS responses, the list is empty so
		// resources is nil, not []any.Any{} -- an empty slice.
		Resources: nil,
		TypeUrl:   secretType,
	})

}
