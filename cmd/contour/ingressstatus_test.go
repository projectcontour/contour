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

package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func Test_parseStatusFlag(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   core_v1.LoadBalancerStatus
	}{
		{
			name:   "IPv4",
			status: "10.0.0.1",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						IP: "10.0.0.1",
					},
				},
			},
		},
		{
			name:   "IPv6",
			status: "2001:4860:4860::8888",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						IP: "2001:4860:4860::8888",
					},
				},
			},
		},
		{
			name:   "arbitrary string",
			status: "anarbitrarystring",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						Hostname: "anarbitrarystring",
					},
				},
			},
		},
		{
			name:   "WhitespacePadded",
			status: "  anarbitrarystring      ",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						Hostname: "anarbitrarystring",
					},
				},
			},
		},
		{
			name:   "Empty",
			status: "",
			want:   core_v1.LoadBalancerStatus{},
		},
		{
			name:   "EmptyComma",
			status: ",",
			want:   core_v1.LoadBalancerStatus{},
		},
		{
			name:   "EmptySpace",
			status: "    ",
			want:   core_v1.LoadBalancerStatus{},
		},
		{
			name:   "SingleComma",
			status: "10.0.0.1,",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						IP: "10.0.0.1",
					},
				},
			},
		},
		{
			name:   "SingleCommaBefore",
			status: ",10.0.0.1",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						IP: "10.0.0.1",
					},
				},
			},
		},
		{
			name:   "Multi",
			status: "10.0.0.1,2001:4860:4860::8888,anarbitrarystring",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						IP: "10.0.0.1",
					},
					{
						IP: "2001:4860:4860::8888",
					},
					{
						Hostname: "anarbitrarystring",
					},
				},
			},
		},
		{
			name:   "MultiSpace",
			status: "10.0.0.1, 2001:4860:4860::8888, anarbitrarystring",
			want: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						IP: "10.0.0.1",
					},
					{
						IP: "2001:4860:4860::8888",
					},
					{
						Hostname: "anarbitrarystring",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseStatusFlag(tt.status))
		})
	}
}

func Test_lbAddress(t *testing.T) {
	tests := []struct {
		name string
		lb   core_v1.LoadBalancerStatus
		want string
	}{
		{
			name: "empty Loadbalancer",
			lb: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{},
			},
			want: "",
		},
		{
			name: "IP address loadbalancer",
			lb: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						IP: "10.0.0.1",
					},
				},
			},
			want: "10.0.0.1",
		},
		{
			name: "Hostname loadbalancer",
			lb: core_v1.LoadBalancerStatus{
				Ingress: []core_v1.LoadBalancerIngress{
					{
						Hostname: "somedomain.com",
					},
				},
			},
			want: "somedomain.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(lbAddress(tt.lb), tt.want); diff != "" {
				t.Errorf("lbAddress failed: %s", diff)
			}
		})
	}
}

func TestRequireLeaderElection(t *testing.T) {
	var l manager.LeaderElectionRunnable = &loadBalancerStatusWriter{}
	require.True(t, l.NeedLeaderElection())
}
