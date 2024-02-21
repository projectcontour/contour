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

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestHealthCheck(t *testing.T) {
	tests := map[string]struct {
		cluster *dag.Cluster
		want    *envoy_config_core_v3.HealthCheck
	}{
		// this is an odd case because contour.edshealthcheck will not call envoy.HealthCheck
		// when hc is nil, so if hc is not nil, at least one of the parameters on it must be set.
		"blank healthcheck": {
			cluster: &dag.Cluster{
				HTTPHealthCheckPolicy: new(dag.HTTPHealthCheckPolicy),
			},
			want: &envoy_config_core_v3.HealthCheck{
				Timeout:            durationpb.New(envoy.HCTimeout),
				Interval:           durationpb.New(envoy.HCInterval),
				UnhealthyThreshold: wrapperspb.UInt32(3),
				HealthyThreshold:   wrapperspb.UInt32(2),
				HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
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
			want: &envoy_config_core_v3.HealthCheck{
				Timeout:            durationpb.New(envoy.HCTimeout),
				Interval:           durationpb.New(envoy.HCInterval),
				UnhealthyThreshold: wrapperspb.UInt32(3),
				HealthyThreshold:   wrapperspb.UInt32(2),
				HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
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
			want: &envoy_config_core_v3.HealthCheck{
				Timeout:            durationpb.New(99 * time.Second),
				Interval:           durationpb.New(98 * time.Second),
				UnhealthyThreshold: wrapperspb.UInt32(97),
				HealthyThreshold:   wrapperspb.UInt32(96),
				HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
						Path: "/healthy",
						Host: "foo-bar-host",
					},
				},
			},
		},
		"h2 healthcheck": {
			cluster: &dag.Cluster{
				Protocol:              "h2",
				HTTPHealthCheckPolicy: new(dag.HTTPHealthCheckPolicy),
			},
			want: &envoy_config_core_v3.HealthCheck{
				Timeout:            durationpb.New(envoy.HCTimeout),
				Interval:           durationpb.New(envoy.HCInterval),
				UnhealthyThreshold: wrapperspb.UInt32(3),
				HealthyThreshold:   wrapperspb.UInt32(2),
				HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
						Host:            "contour-envoy-healthcheck",
						CodecClientType: envoy_type_v3.CodecClientType_HTTP2,
					},
				},
			},
		},
		"h2c healthcheck": {
			cluster: &dag.Cluster{
				Protocol:              "h2c",
				HTTPHealthCheckPolicy: new(dag.HTTPHealthCheckPolicy),
			},
			want: &envoy_config_core_v3.HealthCheck{
				Timeout:            durationpb.New(envoy.HCTimeout),
				Interval:           durationpb.New(envoy.HCInterval),
				UnhealthyThreshold: wrapperspb.UInt32(3),
				HealthyThreshold:   wrapperspb.UInt32(2),
				HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
						Host:            "contour-envoy-healthcheck",
						CodecClientType: envoy_type_v3.CodecClientType_HTTP2,
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := httpHealthCheck(tc.cluster)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}
