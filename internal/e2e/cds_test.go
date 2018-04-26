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
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// heptio/contour#186
// Cluster.ServiceName and ClusterLoadAssignment.ClusterName should not be truncated.
func TestClusterLongServiceName(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(service(
		"kuard",
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
			any(t, cluster("kuard/kbujbkuhdod66-edfcfc/8080", "kuard/kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))
}

// Test adding, updating, and removing a service
// doesn't leave turds in the CDS cache.
func TestClusterAddUpdateDelete(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// s1 is a simple tcp 80 -> 8080 sevice.
	s1 := service("default", "kuard", v1.ServicePort{
		Protocol:   "TCP",
		Port:       80,
		TargetPort: intstr.FromInt(8080),
	})
	rh.OnAdd(s1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/80", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))

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
			any(t, cluster("default/kuard/80", "default/kuard/http")),
			any(t, cluster("default/kuard/http", "default/kuard/http")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))

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
			any(t, cluster("default/kuard/443", "default/kuard/https")),
			any(t, cluster("default/kuard/80", "default/kuard/http")),
			any(t, cluster("default/kuard/http", "default/kuard/http")),
			any(t, cluster("default/kuard/https", "default/kuard/https")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))

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
			any(t, cluster("default/kuard/443", "default/kuard/https")),
			any(t, cluster("default/kuard/https", "default/kuard/https")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))
}

// pathological hard case, one service is removed, the other is moved to a different port, and its name removed.
func TestClusterRenameUpdateDelete(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

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
			any(t, cluster("default/kuard/443", "default/kuard/https")),
			any(t, cluster("default/kuard/80", "default/kuard/http")),
			any(t, cluster("default/kuard/http", "default/kuard/http")),
			any(t, cluster("default/kuard/https", "default/kuard/https")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))

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
			any(t, cluster("default/kuard/443", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))

	// now replace s2 with s1 to check it works in the other direction.
	rh.OnUpdate(s2, s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/443", "default/kuard/https")),
			any(t, cluster("default/kuard/80", "default/kuard/http")),
			any(t, cluster("default/kuard/http", "default/kuard/http")),
			any(t, cluster("default/kuard/https", "default/kuard/https")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))

	// cleanup and check
	rh.OnDelete(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     clusterType,
		Nonce:       "0",
	}, fetchCDS(t, cc))
}

// issue#243. A single unnamed service with a different numeric target port
func TestIssue243(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	t.Run("single unnamed service with a different numeric target port", func(t *testing.T) {
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
				any(t, cluster("default/kuard/80", "default/kuard")),
			},
			TypeUrl: clusterType,
			Nonce:   "0",
		}, fetchCDS(t, cc))
	})
}

// issue 247, a single unnamed service with a named target port
func TestIssue247(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

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
			any(t, cluster("default/kuard/80", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))
}
func TestCDSResourceFiltering(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

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
			any(t, cluster("default/httpbin/8080", "default/httpbin")),
			any(t, cluster("default/kuard/80", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))

	// assert we can filter on one resource
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, cluster("default/kuard/80", "default/kuard")),
		},
		TypeUrl: clusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc, "default/kuard/80"))

	// assert a non matching filter returns no results
	// note: streamCDS would stall at this point.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     clusterType,
		Nonce:       "0",
	}, fetchCDS(t, cc, "default/httpbin/9000"))
}

func fetchCDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewClusterDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	resp, err := rds.FetchClusters(ctx, &v2.DiscoveryRequest{
		TypeUrl:       clusterType,
		ResourceNames: rn,
	})
	if err != nil {
		t.Fatal(err)
	}
	return resp
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
