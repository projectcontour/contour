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

	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
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
			dag.CACertificateKey: []byte(featuretests.CERTIFICATE),
		},
	}

	svc := fixture.NewService("default/kuard").
		Annotate("projectcontour.io/upstream-protocol.tls", "securebackend,443").
		WithPorts(v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8080)})

	p1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
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
	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that there is a regular, non validation enabled cluster in CDS.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", nil),
		),
		TypeUrl: clusterType,
	})

	p2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
					Name: svc.Name,
					Port: 443,
					UpstreamValidation: &contour_api_v1.UpstreamValidation{
						CACertificate: secret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		},
	}
	rh.OnUpdate(p1, p2)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the cluster now has a certificate and subject name.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/98c0f31c72", "default/kuard/securebackend", "default_kuard_443"), []byte(featuretests.CERTIFICATE), "subjname", "", nil),
		),
		TypeUrl: clusterType,
	})

	// Contour does not use SDS to transmit the CA for upstream validation, issue 1405,
	// assert that SDS is empty.
	c.Request(secretType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		// we are asking for all SDS responses, the list is empty so
		// resources is nil, not []any.Any{} -- an empty slice.
		Resources: nil,
		TypeUrl:   secretType,
	})

	rh.OnDelete(p2)

	hp1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/a")),
				Services: []contour_api_v1.Service{{
					Name: svc.Name,
					Port: 443,
					UpstreamValidation: &contour_api_v1.UpstreamValidation{
						CACertificate: secret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	// assert that the insecure listener and the stats listener are present in LDS.
	c.Request(listenerType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			defaultHTTPListener(),
			statsListener(),
		),
		TypeUrl: listenerType,
	})

	// assert that the cluster now has a certificate and subject name.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/98c0f31c72", "default/kuard/securebackend", "default_kuard_443"), []byte(featuretests.CERTIFICATE), "subjname", "", nil),
		),
		TypeUrl: clusterType,
	})

	// Contour does not use SDS to transmit the CA for upstream validation, issue 1405,
	// assert that SDS is empty.
	c.Request(secretType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		// we are asking for all SDS responses, the list is empty so
		// resources is nil, not []any.Any{} -- an empty slice.
		Resources: nil,
		TypeUrl:   secretType,
	})

}
