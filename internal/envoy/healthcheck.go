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

package envoy

import (
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

const (
	// Default healthcheck / lb algorithm values
	hcTimeout            = 2 * time.Second
	hcInterval           = 10 * time.Second
	hcUnhealthyThreshold = 3
	hcHealthyThreshold   = 2
	hcHost               = "contour-envoy-healthcheck"
)

// healthCheck returns a *core.HealthCheck value.
func healthCheck(cluster *dag.Cluster) *core.HealthCheck {
	hc := cluster.HealthCheckPolicy
	host := hcHost
	if hc.Host != "" {
		host = hc.Host
	}

	// TODO(dfc) why do we need to specify our own default, what is the default
	// that envoy applies if these fields are left nil?
	return &core.HealthCheck{
		Timeout:            durationOrDefault(hc.Timeout, hcTimeout),
		Interval:           durationOrDefault(hc.Interval, hcInterval),
		UnhealthyThreshold: countOrDefault(hc.UnhealthyThreshold, hcUnhealthyThreshold),
		HealthyThreshold:   countOrDefault(hc.HealthyThreshold, hcHealthyThreshold),
		HealthChecker: &core.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
				Path: hc.Path,
				Host: host,
			},
		},
	}
}

func durationOrDefault(duration, def time.Duration) *time.Duration {
	if duration != 0 {
		return &duration
	}
	return &def
}

func countOrDefault(count int, def int) *types.UInt32Value {
	switch count {
	case 0:
		return u32(def)
	default:
		return u32(count)
	}
}
