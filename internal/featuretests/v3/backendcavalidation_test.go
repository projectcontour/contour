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

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestClusterServiceTLSBackendCAValidation(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	caSecret := featuretests.CASecret(t, "foo", &featuretests.CACertificate)

	svc := fixture.NewService("default/kuard").
		Annotate("projectcontour.io/upstream-protocol.tls", "securebackend,443").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8080)})

	p1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 443,
				}},
			}},
		},
	}
	rh.OnAdd(caSecret)
	rh.OnAdd(svc)
	rh.OnAdd(p1)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that there is a regular, non validation enabled cluster in CDS.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/4929fca9d4", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", nil, nil),
		),
		TypeUrl: clusterType,
	})

	p2 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 443,
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: caSecret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		},
	}
	rh.OnUpdate(p1, p2)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	expectedResponse := &envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/c6ccd34de5", "default/kuard/securebackend", "default_kuard_443"), caSecret, "subjname", "", nil, nil),
		),
		TypeUrl: clusterType,
	}

	// assert that the cluster now has a certificate and subject name.
	c.Request(clusterType).Equals(expectedResponse)

	// Contour does not use SDS to transmit the CA for upstream validation, issue 1405,
	// assert that SDS is empty.
	c.Request(secretType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		// we are asking for all SDS responses, the list is empty so
		// resources is nil, not []any.Any{} -- an empty slice.
		Resources: nil,
		TypeUrl:   secretType,
	})

	rh.OnDelete(p2)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: fixture.ObjectMeta("simple"),
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/a")),
				Services: []contour_v1.Service{{
					Name: svc.Name,
					Port: 443,
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: caSecret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the cluster now has a certificate and subject name.
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/c6ccd34de5", "default/kuard/securebackend", "default_kuard_443"), caSecret, "subjname", "", nil, nil),
		),
		TypeUrl: clusterType,
	})

	// Contour does not use SDS to transmit the CA for upstream validation, issue 1405,
	// assert that SDS is empty.
	c.Request(secretType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		// we are asking for all SDS responses, the list is empty so
		// resources is nil, not []any.Any{} -- an empty slice.
		Resources: nil,
		TypeUrl:   secretType,
	})

	rh.OnDelete(hp1)

	serverSecret := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(serverSecret)

	tcpproxy := fixture.NewProxy("tcpproxy").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverSecret.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name:     svc.Name,
					Port:     443,
					Protocol: ptr.To("tls"),
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: caSecret.Name,
						SubjectName:   "subjname",
					},
				}},
			},
		})
	rh.OnAdd(tcpproxy)

	c.Request(clusterType).Equals(expectedResponse)
}
