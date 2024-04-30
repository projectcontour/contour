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

//go:build e2e

package infra

import (
	"slices"
	"sort"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testSimpleEndpointSlice(namespace string) {
	Specify("test endpoint slices", func() {
		f.Fixtures.Echo.DeployN(namespace, "echo", 1)

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "endpoint-slice",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "eps.projectcontour.io",
				},
				Routes: []contour_v1.Route{
					{
						Conditions: []contour_v1.MatchCondition{
							{
								Prefix: "/",
							},
						},
						Services: []contour_v1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}

		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		require.Eventually(f.T(), func() bool {
			return IsEnvoyProgrammedWithAllPodIPs(namespace)
		}, f.RetryTimeout, f.RetryInterval)

		// scale up to 10 pods
		f.Fixtures.Echo.ScaleAndWaitDeployment("echo", namespace, 10)

		require.Eventually(f.T(), func() bool {
			return IsEnvoyProgrammedWithAllPodIPs(namespace)
		}, f.RetryTimeout, f.RetryInterval)

		// scale down to 2 pods
		f.Fixtures.Echo.ScaleAndWaitDeployment("echo", namespace, 2)

		require.Eventually(f.T(), func() bool {
			return IsEnvoyProgrammedWithAllPodIPs(namespace)
		}, f.RetryTimeout, f.RetryInterval)

		// scale to 0
		f.Fixtures.Echo.ScaleAndWaitDeployment("echo", namespace, 0)

		require.Eventually(f.T(), func() bool {
			return IsEnvoyProgrammedWithAllPodIPs(namespace)
		}, f.RetryTimeout, f.RetryInterval)
	})
}

func IsEnvoyProgrammedWithAllPodIPs(namespace string) bool {
	k8sPodIPs, err := f.Fixtures.Echo.ListPodIPs(namespace, "echo")
	if err != nil {
		return false
	}

	envoyEndpoints, err := GetIPsFromAdminRequest()
	if err != nil {
		return false
	}

	sort.Strings(k8sPodIPs)
	sort.Strings(envoyEndpoints)

	return slices.Equal(k8sPodIPs, envoyEndpoints)
}

// GetIPsFromAdminRequest makes a call to the envoy admin endpoint and parses
// all the IPs as a list from the echo cluster
func GetIPsFromAdminRequest() ([]string, error) {
	resp, _ := f.HTTP.AdminRequestUntil(&e2e.HTTPRequestOpts{
		Path:      "/clusters?format=json",
		Condition: e2e.HasStatusCode(200),
	})

	ips := make([]string, 0)

	clusters := &envoy_config_cluster_v3.Clusters{}
	err := protojson.Unmarshal(resp.Body, clusters)
	if err != nil {
		return nil, err
	}

	for _, cluster := range clusters.ClusterStatuses {
		if cluster.Name == "simple-endpoint-slice/echo/80/da39a3ee5e" {
			for _, host := range cluster.HostStatuses {
				ips = append(ips, host.Address.GetSocketAddress().Address)
			}
		}
	}

	return ips, nil
}
