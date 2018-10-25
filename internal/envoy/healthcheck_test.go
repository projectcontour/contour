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
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/google/go-cmp/cmp"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
)

func TestHealthCheck(t *testing.T) {
	tests := map[string]struct {
		hc   *ingressroutev1.HealthCheck
		want *core.HealthCheck
	}{
		// this is an odd case because contour.edshealthcheck will not call envoy.HealthCheck
		// when hc is nil, so if hc is not nil, at least one of the parameters on it must be set.
		"blank healthcheck": {
			hc: new(ingressroutev1.HealthCheck),
			want: &core.HealthCheck{
				Timeout:            duration(hcTimeout),
				Interval:           duration(hcInterval),
				UnhealthyThreshold: u32(3),
				HealthyThreshold:   u32(2),
				HealthChecker: &core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
						// TODO(dfc) this doesn't seem right
						Host: "contour-envoy-healthcheck",
					},
				},
			},
		},
		"healthcheck path only": {
			hc: &ingressroutev1.HealthCheck{
				Path: "/healthy",
			},
			want: &core.HealthCheck{
				Timeout:            duration(hcTimeout),
				Interval:           duration(hcInterval),
				UnhealthyThreshold: u32(3),
				HealthyThreshold:   u32(2),
				HealthChecker: &core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
						Path: "/healthy",
						Host: "contour-envoy-healthcheck",
					},
				},
			},
		},
		"explicit healthcheck": {
			hc: &ingressroutev1.HealthCheck{
				Host:                    "foo-bar-host",
				Path:                    "/healthy",
				TimeoutSeconds:          99,
				IntervalSeconds:         98,
				UnhealthyThresholdCount: 97,
				HealthyThresholdCount:   96,
			},
			want: &core.HealthCheck{
				Timeout:            duration(99 * time.Second),
				Interval:           duration(98 * time.Second),
				UnhealthyThreshold: u32(97),
				HealthyThreshold:   u32(96),
				HealthChecker: &core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
						Path: "/healthy",
						Host: "foo-bar-host",
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := healthCheck(tc.hc)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}

		})
	}
}
