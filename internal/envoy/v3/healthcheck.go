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
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
)

// httpHealthCheck returns a *envoy_config_core_v3.HealthCheck value for HTTP Routes
func httpHealthCheck(cluster *dag.Cluster) *envoy_config_core_v3.HealthCheck {
	hc := cluster.HTTPHealthCheckPolicy
	host := envoy.HCHost
	if hc.Host != "" {
		host = hc.Host
	}

	return &envoy_config_core_v3.HealthCheck{
		Timeout:            durationOrDefault(hc.Timeout, envoy.HCTimeout),
		Interval:           durationOrDefault(hc.Interval, envoy.HCInterval),
		UnhealthyThreshold: protobuf.UInt32OrDefault(hc.UnhealthyThreshold, envoy.HCUnhealthyThreshold),
		HealthyThreshold:   protobuf.UInt32OrDefault(hc.HealthyThreshold, envoy.HCHealthyThreshold),
		HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
				Path:             hc.Path,
				Host:             host,
				CodecClientType:  codecClientType(cluster),
				ExpectedStatuses: expectedStatuses(hc.ExpectedStatuses),
			},
		},
	}
}

func expectedStatuses(statusRanges []dag.HTTPStatusRange) []*envoy_type_v3.Int64Range {
	if len(statusRanges) == 0 {
		return nil
	}

	var res []*envoy_type_v3.Int64Range

	for _, statusRange := range statusRanges {
		res = append(res, &envoy_type_v3.Int64Range{
			Start: statusRange.Start,
			End:   statusRange.End,
		})
	}

	return res
}

// tcpHealthCheck returns a *envoy_config_core_v3.HealthCheck value for TCPProxies
func tcpHealthCheck(cluster *dag.Cluster) *envoy_config_core_v3.HealthCheck {
	hc := cluster.TCPHealthCheckPolicy

	return &envoy_config_core_v3.HealthCheck{
		Timeout:            durationOrDefault(hc.Timeout, envoy.HCTimeout),
		Interval:           durationOrDefault(hc.Interval, envoy.HCInterval),
		UnhealthyThreshold: protobuf.UInt32OrDefault(hc.UnhealthyThreshold, envoy.HCUnhealthyThreshold),
		HealthyThreshold:   protobuf.UInt32OrDefault(hc.HealthyThreshold, envoy.HCHealthyThreshold),
		HealthChecker: &envoy_config_core_v3.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoy_config_core_v3.HealthCheck_TcpHealthCheck{},
		},
	}
}

func durationOrDefault(d, def time.Duration) *durationpb.Duration {
	if d != 0 {
		return durationpb.New(d)
	}
	return durationpb.New(def)
}

func codecClientType(cluster *dag.Cluster) envoy_type_v3.CodecClientType {
	if cluster.Protocol == "h2" || cluster.Protocol == "h2c" {
		return envoy_type_v3.CodecClientType_HTTP2
	}
	return envoy_type_v3.CodecClientType_HTTP1
}
