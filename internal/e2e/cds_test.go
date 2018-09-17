// Copyright © 2018 Heptio
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

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// heptio/contour#186
// Cluster.ServiceName and ClusterLoadAssignment.ClusterName should not be truncated.
func TestClusterLongServiceName(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "kuard",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(service(
		"default",
		"kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r",
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		},
	))

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kbujbkuh-c83ceb/8080/da39a3ee5e", "default/kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))
}

// Test adding, updating, and removing a service
// doesn't leave turds in the CDS cache.
func TestClusterAddUpdateDelete(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(i1)

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "www.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/kuarder",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("https"),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	// s1 is a simple tcp 80 -> 8080 service.
	s1 := service("default", "kuard", v1.ServicePort{
		Protocol:   "TCP",
		Port:       80,
		TargetPort: intstr.FromInt(8080),
	})
	rh.OnAdd(s1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// s2 is the same as s2, but the service port has a name
	s2 := service("default", "kuard", v1.ServicePort{
		Name:       "http",
		Protocol:   "TCP",
		Port:       80,
		TargetPort: intstr.FromInt(8080),
	})

	// replace s1 with s2
	rh.OnUpdate(s1, s2)

	// check that we get two CDS records because the port is now named.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard/http")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// s3 is like s2, but has a second named port. The k8s spec
	// requires all ports to be named if there is more than one of them.
	s3 := service("default", "kuard",
		v1.ServicePort{
			Name:       "http",
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(8080),
		},
		v1.ServicePort{
			Name:       "https",
			Protocol:   "TCP",
			Port:       443,
			TargetPort: intstr.FromInt(8443),
		},
	)

	// replace s2 with s3
	rh.OnUpdate(s2, s3)

	// check that we get four CDS records. Order is important
	// because the CDS cache is sorted.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/443/da39a3ee5e", "default/kuard/https")),
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard/http")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// s4 is s3 with the http port removed.
	s4 := service("default", "kuard",
		v1.ServicePort{
			Name:       "https",
			Protocol:   "TCP",
			Port:       443,
			TargetPort: intstr.FromInt(8443),
		},
	)

	// replace s3 with s4
	rh.OnUpdate(s3, s4)

	// check that we get two CDS records only, and that the 80 and http
	// records have been removed even though the service object remains.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/443/da39a3ee5e", "default/kuard/https")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))
}

// pathological hard case, one service is removed, the other is moved to a different port, and its name removed.
func TestClusterRenameUpdateDelete(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "www.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromString("http"),
							},
						}, {
							Path: "/kuarder",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(443),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	s1 := service("default", "kuard",
		v1.ServicePort{
			Name:       "http",
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(8080),
		},
		v1.ServicePort{
			Name:       "https",
			Protocol:   "TCP",
			Port:       443,
			TargetPort: intstr.FromInt(8443),
		},
	)

	rh.OnAdd(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/443/da39a3ee5e", "default/kuard/https")),
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard/http")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// s2 removes the name on port 80, moves it to port 443 and deletes the https port
	s2 := service("default", "kuard",
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       443,
			TargetPort: intstr.FromInt(8000),
		},
	)

	rh.OnUpdate(s1, s2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/443/da39a3ee5e", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// now replace s2 with s1 to check it works in the other direction.
	rh.OnUpdate(s2, s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/443/da39a3ee5e", "default/kuard/https")),
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard/http")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// cleanup and check
	rh.OnDelete(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     clusterType,
		Nonce:       "0",
	}, streamCDS(t, cc))
}

// issue#243. A single unnamed service with a different numeric target port
func TestIssue243(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	t.Run("single unnamed service with a different numeric target port", func(t *testing.T) {

		i1 := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kuard",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "kuard",
					ServicePort: intstr.FromInt(80),
				},
			},
		}
		rh.OnAdd(i1)
		s1 := service("default", "kuard",
			v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			},
		)
		rh.OnAdd(s1)
		assertEqual(t, &v2.DiscoveryResponse{
			VersionInfo: "0",
			Resources: []types.Any{
				any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard")),
			},
			TypeUrl: clusterType,
			Nonce:   "0",
		}, streamCDS(t, cc))
	})
}

// issue 247, a single unnamed service with a named target port
func TestIssue247(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(i1)

	// spec:
	//   ports:
	//   - port: 80
	//     protocol: TCP
	//     targetPort: kuard
	s1 := service("default", "kuard",
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromString("kuard"),
		},
	)
	rh.OnAdd(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))
}
func TestCDSResourceFiltering(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "www.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}, {
							Path: "/httpbin",
							Backend: v1beta1.IngressBackend{
								ServiceName: "httpbin",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// add two services, check that they are there
	s1 := service("default", "kuard",
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromString("kuard"),
		},
	)
	rh.OnAdd(s1)
	s2 := service("default", "httpbin",
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromString("httpbin"),
		},
	)
	rh.OnAdd(s2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			// note, resources are sorted by Cluster.Name
			any(t, cluster("default/httpbin/8080/da39a3ee5e", "default/httpbin")),
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// assert we can filter on one resource
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc, "default/kuard/80/da39a3ee5e"))

	// assert a non matching filter returns no results
	// note: streamCDS would stall at this point.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     clusterType,
		Nonce:       "0",
	}, streamCDS(t, cc, "default/httpbin/9000"))
}

func TestClusterCircuitbreakerAnnotations(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "kuard",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnAdd(i1)

	s1 := serviceWithAnnotations(
		"default",
		"kuard",
		map[string]string{
			"contour.heptio.com/max-connections":      "9000",
			"contour.heptio.com/max-pending-requests": "4096",
			"contour.heptio.com/max-requests":         "404",
			"contour.heptio.com/max-retries":          "7",
		},
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		},
	)
	rh.OnAdd(s1)

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Cluster{
				Name: "default/kuard/8080/da39a3ee5e",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
					ServiceName: "default/kuard",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxConnections:     uint32t(9000),
						MaxPendingRequests: uint32t(4096),
						MaxRequests:        uint32t(404),
						MaxRetries:         uint32t(7),
					}},
				},
				CommonLbConfig: &v2.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
						Value: 0,
					},
				},
			}),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))

	// update s1 with slightly weird values
	s2 := serviceWithAnnotations(
		"default",
		"kuard",
		map[string]string{
			"contour.heptio.com/max-pending-requests": "9999",
			"contour.heptio.com/max-requests":         "1e6",
			"contour.heptio.com/max-retries":          "0",
		},
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		},
	)
	rh.OnUpdate(s1, s2)

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Cluster{
				Name: "default/kuard/8080/da39a3ee5e",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
					ServiceName: "default/kuard",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
				CircuitBreakers: &envoy_cluster.CircuitBreakers{
					Thresholds: []*envoy_cluster.CircuitBreakers_Thresholds{{
						MaxPendingRequests: uint32t(9999),
					}},
				},
				CommonLbConfig: &v2.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
						Value: 0,
					},
				},
			}),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))
}

// issue 581, different service parameters should generate
// a single CDS entry if they differ only in weight.
func TestClusterPerServiceParameters(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/a",
				Services: []ingressroutev1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}},
			}, {
				Match: "/b",
				Services: []ingressroutev1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	})

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/80/da39a3ee5e", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))
}

// issue 581, different load balancer parameters should
// generate multiple cds entries.
func TestClusterLoadBalancerStrategyPerRoute(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/a",
				Services: []ingressroutev1.Service{{
					Name:     "kuard",
					Port:     80,
					Strategy: "Random",
				}},
			}, {
				Match: "/b",
				Services: []ingressroutev1.Service{{
					Name:     "kuard",
					Port:     80,
					Strategy: "Maglev",
				}},
			}},
		},
	})

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Cluster{
				Name: "default/kuard/80/58d888c08a",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
					ServiceName: "default/kuard",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_RANDOM,
				CommonLbConfig: &v2.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
						Value: 0,
					},
				},
			}),
			any(t, &v2.Cluster{
				Name: "default/kuard/80/843e4ded8f",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
					ServiceName: "default/kuard",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_MAGLEV,
				CommonLbConfig: &v2.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
						Value: 0,
					},
				},
			}),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, streamCDS(t, cc))
}

func uint32t(v int) *types.UInt32Value {
	return &types.UInt32Value{Value: uint32(v)}
}

func serviceWithAnnotations(ns, name string, annotations map[string]string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

func streamCDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewClusterDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	st, err := rds.StreamClusters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       clusterType,
		ResourceNames: rn,
	})
}

func cluster(name, servicename string) *v2.Cluster {
	return &v2.Cluster{
		Name: name,
		Type: v2.Cluster_EDS,
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
			ServiceName: servicename,
		},
		ConnectTimeout: 250 * time.Millisecond,
		LbPolicy:       v2.Cluster_ROUND_ROBIN,
		CommonLbConfig: &v2.Cluster_CommonLbConfig{
			HealthyPanicThreshold: &envoy_type.Percent{ // Disable HealthyPanicThreshold
				Value: 0,
			},
		},
	}
}

func apiconfigsource(clusters ...string) *core.ConfigSource {
	return &core.ConfigSource{
		ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &core.ApiConfigSource{
				ApiType:      core.ApiConfigSource_GRPC,
				ClusterNames: clusters,
			},
		},
	}
}
