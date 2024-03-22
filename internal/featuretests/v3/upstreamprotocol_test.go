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
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

// Test that contour correctly recognizes the upstream-protocol.tls
// service annotation.
func TestUpstreamProtocolTLS(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("kuard").
		Annotate("projectcontour.io/upstream-protocol.tls", "securebackend").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8888)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/4929fca9d4", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", nil, nil),
		),
		TypeUrl: clusterType,
	})

	s2 := fixture.NewService("kuard").
		Annotate("projectcontour.io/upstream-protocol.tls", "securebackend").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8888)})
	rh.OnUpdate(s1, s2)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/4929fca9d4", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", nil, nil),
		),
		TypeUrl: clusterType,
	})
}

// Test that contour correctly recognizes the upstream-protocol.h2c
// service annotation.
func TestUpstreamProtocolH2C(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("kuard").
		Annotate("projectcontour.io/upstream-protocol.h2c", "securebackend").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8888)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(cluster("default/kuard/443/f4f94965ec", "default/kuard/securebackend", "default_kuard_443")),
		),
		TypeUrl: clusterType,
	})

	s2 := fixture.NewService("kuard").
		Annotate("projectcontour.io/upstream-protocol.h2c", "securebackend").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8888)})
	rh.OnUpdate(s1, s2)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(cluster("default/kuard/443/f4f94965ec", "default/kuard/securebackend", "default_kuard_443")),
		),
		TypeUrl: clusterType,
	})
}

// Test that contour correctly recognizes the upstream-protocol.h2
// service annotation.
func TestUpstreamProtocolH2(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("kuard").
		Annotate("projectcontour.io/upstream-protocol.h2", "securebackend").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8888)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(tlsCluster(cluster("default/kuard/443/bf1c365741", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", nil, nil, "h2")),
		),
		TypeUrl: clusterType,
	})

	s2 := fixture.NewService("kuard").
		Annotate("projectcontour.io/upstream-protocol.h2", "securebackend").
		WithPorts(core_v1.ServicePort{Name: "securebackend", Port: 443, TargetPort: intstr.FromInt(8888)})
	rh.OnUpdate(s1, s2)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(tlsCluster(cluster("default/kuard/443/bf1c365741", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", nil, nil, "h2")),
		),
		TypeUrl: clusterType,
	})
}
