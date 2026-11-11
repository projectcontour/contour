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
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func withHealthyPanicThreshold(c *envoy_config_cluster_v3.Cluster, threshold float64) *envoy_config_cluster_v3.Cluster {
	c.CommonLbConfig = &envoy_config_cluster_v3.Cluster_CommonLbConfig{
		HealthyPanicThreshold: &envoy_type_v3.Percent{
			Value: threshold,
		},
	}
	return c
}

func TestHTTPHealthCheckPanicThreshold(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromString("8080")}),
	)

	proxy := func(panicThreshold uint32) *contour_v1.HTTPProxy {
		return fixture.NewProxy("default/simple").WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/a",
				}},
				HealthCheckPolicy: &contour_v1.HTTPHealthCheckPolicy{
					Path:           "/healthz",
					PanicThreshold: panicThreshold,
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		})
	}

	// proxy1 does not set a panic threshold, so panic mode stays disabled.
	proxy1 := proxy(0)
	rh.OnAdd(proxy1)

	c.Status(proxy1).IsValid()
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			clusterWithHealthCheck("default/kuard/80/bc862a33ca", "default/kuard", "default_kuard_80", "/healthz", nil),
		),
		TypeUrl: clusterType,
	})

	// proxy2 sets a panic threshold, which results in a separate cluster.
	proxy2 := proxy(50)
	rh.OnUpdate(proxy1, proxy2)

	c.Status(proxy2).IsValid()
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			withHealthyPanicThreshold(
				clusterWithHealthCheck("default/kuard/80/dc16ed6c9e", "default/kuard", "default_kuard_80", "/healthz", nil),
				50,
			),
		),
		TypeUrl: clusterType,
	})

	// proxy3 has a panic threshold above 100%.
	proxy3 := proxy(101)
	rh.OnUpdate(proxy2, proxy3)

	c.Status(proxy3).HasError(contour_v1.ConditionTypeRouteError, "HealthCheckPolicyInvalid", "invalid panic threshold: must be in the range [0, 100]")
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{})
}

func TestTCPHealthCheckPanicThreshold(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(s1)

	rh.OnAdd(fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	proxy := func(panicThreshold uint32) *contour_v1.HTTPProxy {
		return fixture.NewProxy("default/simple").WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard-tcp.example.com",
				TLS: &contour_v1.TLS{
					SecretName: s1.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				HealthCheckPolicy: &contour_v1.TCPHealthCheckPolicy{
					PanicThreshold: panicThreshold,
				},
				Services: []contour_v1.Service{{
					Name: "backend",
					Port: 80,
				}},
			},
		})
	}

	// proxy1 sets a panic threshold.
	proxy1 := proxy(25)
	rh.OnAdd(proxy1)

	c.Status(proxy1).IsValid()

	cluster := cluster("default/backend/80/886e43655f", "default/backend", "default_backend_80")
	cluster.HealthChecks = []*envoy_config_core_v3.HealthCheck{{
		Timeout:            durationpb.New(2 * time.Second),
		Interval:           durationpb.New(10 * time.Second),
		UnhealthyThreshold: wrapperspb.UInt32(3),
		HealthyThreshold:   wrapperspb.UInt32(2),
		HealthChecker: &envoy_config_core_v3.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoy_config_core_v3.HealthCheck_TcpHealthCheck{},
		},
	}}
	cluster.IgnoreHealthOnHostRemoval = true

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			withHealthyPanicThreshold(cluster, 25),
		),
		TypeUrl: clusterType,
	})

	// proxy2 has a panic threshold above 100%.
	proxy2 := proxy(101)
	rh.OnUpdate(proxy1, proxy2)

	c.Status(proxy2).HasError(contour_v1.ConditionTypeTCPProxyError, "HealthCheckPolicyInvalid", "invalid panic threshold: must be in the range [0, 100]")
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{})
}
