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
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Test that contour correctly recognizes the upstream-protocol.tls
// service annotation.
func TestUpstreamProtocolTLS(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.tls": "securebackend",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "securebackend",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8888),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(443),
			},
		},
	}
	rh.OnAdd(i1)

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443"), nil, "", ""),
		),
		TypeUrl: clusterType,
	})

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "securebackend",
			},
		},
		Spec: s1.Spec,
	}
	rh.OnUpdate(s1, s2)

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443"), nil, "", ""),
		),
		TypeUrl: clusterType,
	})
}

// Test that contour correctly recognizes the upstream-protocol.h2c
// service annotation.
func TestUpstreamProtocolH2C(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.h2c": "securebackend",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "securebackend",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8888),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(443),
			},
		},
	}
	rh.OnAdd(i1)

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443")),
		),
		TypeUrl: clusterType,
	})

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2c": "securebackend",
			},
		},
		Spec: s1.Spec,
	}
	rh.OnUpdate(s1, s2)

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443")),
		),
		TypeUrl: clusterType,
	})
}

// Test that contour correctly recognizes the upstream-protocol.h2
// service annotation.
func TestUpstreamProtocolH2(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/upstream-protocol.h2": "securebackend",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "securebackend",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8888),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(443),
			},
		},
	}
	rh.OnAdd(i1)

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(tlsCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", "h2")),
		),
		TypeUrl: clusterType,
	})

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2": "securebackend",
			},
		},
		Spec: s1.Spec,
	}
	rh.OnUpdate(s1, s2)

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			h2cCluster(tlsCluster(cluster("default/kuard/443/da39a3ee5e", "default/kuard/securebackend", "default_kuard_443"), nil, "", "", "h2")),
		),
		TypeUrl: clusterType,
	})
}
