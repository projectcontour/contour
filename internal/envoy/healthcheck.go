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
	"time"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

const (
	// Default healthcheck / lb algorithm values
	hcTimeout            = 2 * time.Second
	hcInterval           = 10 * time.Second
	hcUnhealthyThreshold = 3
	hcHealthyThreshold   = 2
	hcHost               = "contour-envoy-healthcheck"
)

// httpHealthCheck returns a *envoy_api_v2_core.HealthCheck value for HTTP Routes
func httpHealthCheck(cluster *dag.Cluster) *envoy_api_v2_core.HealthCheck {
	hc := cluster.HTTPHealthCheckPolicy
	host := hcHost
	if hc.Host != "" {
		host = hc.Host
	}

	// TODO(dfc) why do we need to specify our own default, what is the default
	// that envoy applies if these fields are left nil?
	return &envoy_api_v2_core.HealthCheck{
		Timeout:            durationOrDefault(hc.Timeout, hcTimeout),
		Interval:           durationOrDefault(hc.Interval, hcInterval),
		UnhealthyThreshold: countOrDefault(hc.UnhealthyThreshold, hcUnhealthyThreshold),
		HealthyThreshold:   countOrDefault(hc.HealthyThreshold, hcHealthyThreshold),
		HealthChecker: &envoy_api_v2_core.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoy_api_v2_core.HealthCheck_HttpHealthCheck{
				Path: hc.Path,
				Host: host,
			},
		},
	}
}

// tcpHealthCheck returns a *envoy_api_v2_core.HealthCheck value for TCPProxies
func tcpHealthCheck(cluster *dag.Cluster) *envoy_api_v2_core.HealthCheck {
	hc := cluster.TCPHealthCheckPolicy

	return &envoy_api_v2_core.HealthCheck{
		Timeout:            durationOrDefault(hc.Timeout, hcTimeout),
		Interval:           durationOrDefault(hc.Interval, hcInterval),
		UnhealthyThreshold: countOrDefault(hc.UnhealthyThreshold, hcUnhealthyThreshold),
		HealthyThreshold:   countOrDefault(hc.HealthyThreshold, hcHealthyThreshold),
		HealthChecker: &envoy_api_v2_core.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoy_api_v2_core.HealthCheck_TcpHealthCheck{},
		},
	}
}

func durationOrDefault(d, def time.Duration) *duration.Duration {
	if d != 0 {
		return protobuf.Duration(d)
	}
	return protobuf.Duration(def)
}

func countOrDefault(count uint32, def uint32) *wrappers.UInt32Value {
	switch count {
	case 0:
		return protobuf.UInt32(def)
	default:
		return protobuf.UInt32(count)
	}
}
