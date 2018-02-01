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
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/gogo/protobuf/types"
	cgrpc "github.com/heptio/contour/internal/grpc"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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
		Resources: []*types.Any{
			any(t, &v2.Cluster{
				Name: "default/kuard/80",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
		},
		TypeUrl: cgrpc.ClusterType,
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
		Resources: []*types.Any{
			any(t, &v2.Cluster{
				Name: "default/kuard/80",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
			any(t, &v2.Cluster{
				Name: "default/kuard/http",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
		},
		TypeUrl: cgrpc.ClusterType,
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
		Resources: []*types.Any{
			any(t, &v2.Cluster{
				Name: "default/kuard/443",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
			any(t, &v2.Cluster{
				Name: "default/kuard/80",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
			any(t, &v2.Cluster{
				Name: "default/kuard/http",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
			any(t, &v2.Cluster{
				Name: "default/kuard/https",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
		},
		TypeUrl: cgrpc.ClusterType,
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
		Resources: []*types.Any{
			any(t, &v2.Cluster{
				Name: "default/kuard/443",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
			any(t, &v2.Cluster{
				Name: "default/kuard/https",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
		},
		TypeUrl: cgrpc.ClusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))
}
