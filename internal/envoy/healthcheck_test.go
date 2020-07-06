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

package envoy

import (
	"testing"
	"time"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestHealthCheck(t *testing.T) {
	tests := map[string]struct {
		cluster *dag.Cluster
		want    *envoy_api_v2_core.HealthCheck
	}{
		// this is an odd case because contour.edshealthcheck will not call envoy.HealthCheck
		// when hc is nil, so if hc is not nil, at least one of the parameters on it must be set.
		"blank healthcheck": {
			cluster: &dag.Cluster{
				HTTPHealthCheckPolicy: new(dag.HTTPHealthCheckPolicy),
			},
			want: &envoy_api_v2_core.HealthCheck{
				Timeout:            protobuf.Duration(hcTimeout),
				Interval:           protobuf.Duration(hcInterval),
				UnhealthyThreshold: protobuf.UInt32(3),
				HealthyThreshold:   protobuf.UInt32(2),
				HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
						// TODO(dfc) this doesn't seem right
						Host: "contour-envoy-healthcheck",
					},
				},
			},
		},
		"healthcheck path only": {
			cluster: &dag.Cluster{
				HTTPHealthCheckPolicy: &dag.HTTPHealthCheckPolicy{
					Path: "/healthy",
				},
			},
			want: &envoy_api_v2_core.HealthCheck{
				Timeout:            protobuf.Duration(hcTimeout),
				Interval:           protobuf.Duration(hcInterval),
				UnhealthyThreshold: protobuf.UInt32(3),
				HealthyThreshold:   protobuf.UInt32(2),
				HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
						Path: "/healthy",
						Host: "contour-envoy-healthcheck",
					},
				},
			},
		},
		"explicit healthcheck": {
			cluster: &dag.Cluster{
				HTTPHealthCheckPolicy: &dag.HTTPHealthCheckPolicy{
					Host:               "foo-bar-host",
					Path:               "/healthy",
					Timeout:            99 * time.Second,
					Interval:           98 * time.Second,
					UnhealthyThreshold: 97,
					HealthyThreshold:   96,
				},
			},
			want: &envoy_api_v2_core.HealthCheck{
				Timeout:            protobuf.Duration(99 * time.Second),
				Interval:           protobuf.Duration(98 * time.Second),
				UnhealthyThreshold: protobuf.UInt32(97),
				HealthyThreshold:   protobuf.UInt32(96),
				HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
						Path: "/healthy",
						Host: "foo-bar-host",
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := httpHealthCheck(tc.cluster)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}

		})
	}
}
