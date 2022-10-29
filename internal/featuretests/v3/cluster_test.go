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

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// projectcontour/contour#186
// Cluster.ServiceName and ClusterLoadAssignment.ClusterName should not be truncated.
func TestClusterLongServiceName(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("default/kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r").
		WithPorts(v1.ServicePort{Port: 8080})

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(s1)

	// check that it's been translated correctly.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kbujbkuh-c83ceb/8080/da39a3ee5e", "default/kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r", "default_kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r_8080"),
		),
		TypeUrl: clusterType,
	})
}

// Test adding, updating, and removing a service
// doesn't leave objects in the CDS cache.
func TestClusterAddUpdateDelete(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// s1 is a simple tcp 80 -> 8080 service.
	s1 := fixture.NewService("default/kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	i2 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "www.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path: "/kuarder",
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "kuard",
									Port: networking_v1.ServiceBackendPort{Name: "https"},
								},
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	rh.OnAdd(s1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// s2 is the same as s1, but the service port has a name
	s2 := fixture.NewService("default/kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})

	// replace s1 with s2
	rh.OnUpdate(s1, s2)

	// check that we get two CDS records because the port is now named.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/80/da39a3ee5e", "default/kuard/http", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// s3 is like s2, but has a second named port. The k8s spec
	// requires all ports to be named if there is more than one of them.
	s3 := fixture.NewService("default/kuard").
		WithPorts(
			v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			v1.ServicePort{Name: "https", Port: 443, TargetPort: intstr.FromInt(8443)},
		)

	// replace s2 with s3
	rh.OnUpdate(s2, s3)

	// check that we get four CDS records. Order is important
	// because the CDS cache is sorted.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/443/da39a3ee5e", "default/kuard/https", "default_kuard_443"),
			cluster("default/kuard/80/da39a3ee5e", "default/kuard/http", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// s4 is s3 with the http port removed.
	s4 := fixture.NewService("default/kuard").
		WithPorts(
			v1.ServicePort{Name: "https", Port: 443, TargetPort: intstr.FromInt(8443)},
		)

	// replace s3 with s4
	rh.OnUpdate(s3, s4)

	// check that we get two CDS records only, and that the 80 and http
	// records have been removed even though the service object remains.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/443/da39a3ee5e", "default/kuard/https", "default_kuard_443"),
		),
		TypeUrl: clusterType,
	})
}

// pathological hard case, one service is removed, the other is moved to a different port, and its name removed.
func TestClusterRenameUpdateDelete(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "www.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "kuard",
									Port: networking_v1.ServiceBackendPort{Name: "http"},
								},
							},
						}, {
							Path: "/kuarder",
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "kuard",
									Port: networking_v1.ServiceBackendPort{Number: 443},
								},
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	s1 := fixture.NewService("default/kuard").
		WithPorts(
			v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			v1.ServicePort{Name: "https", Port: 443, TargetPort: intstr.FromInt(8443)},
		)

	rh.OnAdd(s1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/443/da39a3ee5e", "default/kuard/https", "default_kuard_443"),
			cluster("default/kuard/80/da39a3ee5e", "default/kuard/http", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// s2 removes the name on port 80, moves it to port 443 and deletes the https port
	s2 := fixture.NewService("default/kuard").
		WithPorts(v1.ServicePort{Port: 443, TargetPort: intstr.FromInt(8080)})

	rh.OnUpdate(s1, s2)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/443/da39a3ee5e", "default/kuard", "default_kuard_443"),
		),
		TypeUrl: clusterType,
	})

	// now replace s2 with s1 to check it works in the other direction.
	rh.OnUpdate(s2, s1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/443/da39a3ee5e", "default/kuard/https", "default_kuard_443"),
			cluster("default/kuard/80/da39a3ee5e", "default/kuard/http", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// cleanup and check
	rh.OnDelete(s1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   clusterType,
	})
}

// issue#243. A single unnamed service with a different numeric target port
func TestIssue243(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	t.Run("single unnamed service with a different numeric target port", func(t *testing.T) {
		s1 := fixture.NewService("default/kuard").
			WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})

		i1 := &networking_v1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kuard",
				Namespace: "default",
			},
			Spec: networking_v1.IngressSpec{
				DefaultBackend: featuretests.IngressBackend(s1),
			},
		}

		rh.OnAdd(i1)

		rh.OnAdd(s1)

		c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
			Resources: resources(t,
				cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
			),
			TypeUrl: clusterType,
		})
	})
}

// issue 247, a single unnamed service with a named target port
func TestIssue247(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// spec:
	//   ports:
	//   - port: 80
	//     protocol: TCP
	//     targetPort: kuard
	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromString("kuard")})

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(s1)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})
}

func TestCDSResourceFiltering(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromString("kuard")})
	s2 := fixture.NewService("httpbin").
		WithPorts(v1.ServicePort{Port: 8080, TargetPort: intstr.FromString("httpbin")})

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "www.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(s1),
						}, {
							Path:    "/httpbin",
							Backend: *featuretests.IngressBackend(s2),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// add two services, check that they are there
	rh.OnAdd(s1)
	rh.OnAdd(s2)

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// note, resources are sorted by Cluster.Name
			cluster("default/httpbin/8080/da39a3ee5e", "default/httpbin", "default_httpbin_8080"),
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// assert we can filter on one resource
	c.Request(clusterType, "default/kuard/80/da39a3ee5e").Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80")),
		TypeUrl: clusterType,
	})

	// assert a non matching filter returns a response with no entries.
	c.Request(clusterType, "default/httpbin/9000").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
	})
}

func TestClusterCircuitbreakerAnnotations(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("kuard").
		Annotate("projectcontour.io/max-connections", "9000").
		Annotate("projectcontour.io/max-pending-requests", "4096").
		Annotate("projectcontour.io/max-requests", "404").
		Annotate("projectcontour.io/max-retries", "7").
		WithPorts(v1.ServicePort{Port: 8080, TargetPort: intstr.FromString("8080")})

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "kuard",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(s1)

	// check that it's been translated correctly.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/8080/da39a3ee5e",
				AltStatName:          "default_kuard_8080",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: "default/kuard",
				},
				CircuitBreakers: &envoy_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
						MaxConnections:     wrapperspb.UInt32(9000),
						MaxPendingRequests: wrapperspb.UInt32(4096),
						MaxRequests:        wrapperspb.UInt32(404),
						MaxRetries:         wrapperspb.UInt32(7),
					}},
				},
			}),
		),
		TypeUrl: clusterType,
	})

	// update s1 with slightly weird values
	s2 := fixture.NewService("kuard").
		Annotate("projectcontour.io/max-pending-requests", "9999").
		Annotate("projectcontour.io/max-requests", "1e6").
		Annotate("projectcontour.io/max-retries", "0").
		WithPorts(v1.ServicePort{Port: 8080, TargetPort: intstr.FromString("8080")})

	rh.OnUpdate(s1, s2)

	// check that it's been translated correctly.
	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/8080/da39a3ee5e",
				AltStatName:          "default_kuard_8080",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: "default/kuard",
				},
				CircuitBreakers: &envoy_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
						MaxPendingRequests: wrapperspb.UInt32(9999),
					}},
				},
			}),
		),
		TypeUrl: clusterType,
	})
}

// issue 581, different service parameters should generate
// a single CDS entry if they differ only in weight.
func TestClusterPerServiceParameters(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromString("8080")}),
	)

	rh.OnAdd(&contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			// note, resources are sorted by Cluster.Name
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})
}

// issue 581, different load balancer parameters should
// generate multiple cds entries.
func TestClusterLoadBalancerStrategyPerRoute(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromString("8080")}),
	)

	rh.OnAdd(&contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "Random",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/b",
				}},
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "WeightedLeastRequest",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/80/58d888c08a",
				AltStatName:          "default_kuard_80",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: "default/kuard",
				},
				LbPolicy: envoy_cluster_v3.Cluster_RANDOM,
			}),
			DefaultCluster(&envoy_cluster_v3.Cluster{
				Name:                 "default/kuard/80/8bf87fefba",
				AltStatName:          "default_kuard_80",
				ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
				EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   envoy_v3.ConfigSource("contour"),
					ServiceName: "default/kuard",
				},
				LbPolicy: envoy_cluster_v3.Cluster_LEAST_REQUEST,
			}),
		),
		TypeUrl: clusterType,
	})
}

func TestClusterWithHealthChecks(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromString("8080")}),
	)

	rh.OnAdd(&contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				HealthCheckPolicy: &contour_api_v1.HTTPHealthCheckPolicy{
					Path: "/healthz",
				},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}},
			}},
		},
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			clusterWithHealthCheck("default/kuard/80/bc862a33ca", "default/kuard", "default_kuard_80", "/healthz", true),
		),
		TypeUrl: clusterType,
	})
}

// Test processing a service that exists but is not referenced
func TestUnreferencedService(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// Equals(...) only checks resources, so explicitly
	// check version & nonce here and subsequently.
	assertEqualVersion := func(t *testing.T, expected string, r *Response) {
		t.Helper()
		assert.Equal(t, expected, r.VersionInfo, "got unexpected VersionInfo")
		assert.Equal(t, expected, r.Nonce, "got unexpected Nonce")
	}

	// This service which is added should cause a DAG rebuild
	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromString("8080")})
	rh.OnAdd(s1)

	rh.OnAdd(&contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/b",
				}},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	})

	res := c.Request(clusterType)
	res.Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// This service which is added should not cause a DAG rebuild
	s2 := fixture.NewService("kuard-notreferenced").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s2)

	res = c.Request(clusterType)
	res.Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})

	// verifying that deleting a Service that is not referenced by an HTTPProxy,
	// does not trigger a rebuild
	rh.OnDelete(s2)
	res = c.Request(clusterType)
	res.Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			cluster("default/kuard/80/da39a3ee5e", "default/kuard", "default_kuard_80"),
		),
		TypeUrl: clusterType,
	})
	assertEqualVersion(t, "1", res)

	// verifying that deleting a Service that is referenced by an HTTPProxy,
	// triggers a rebuild
	rh.OnDelete(s1)
	assertEqualVersion(t, "2", res)
}
