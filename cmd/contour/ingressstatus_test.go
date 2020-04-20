// Copyright © 2020 VMware
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
	v1 "k8s.io/api/core/v1"
)

func Test_parseStatusFlag(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   v1.LoadBalancerStatus
	}{
		{
			name:   "IPv4",
			status: "10.0.0.1",
			want: v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					{
						IP: "10.0.0.1",
					},
				},
			},
		},
		{
			name:   "IPv6",
			status: "2001:4860:4860::8888",
			want: v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					{
						IP: "2001:4860:4860::8888",
					},
				},
			},
		},
		{
			name:   "arbitrary string",
			status: "anarbitrarystring",
			want: v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					{
						Hostname: "anarbitrarystring",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(parseStatusFlag(tt.status), tt.want); diff != "" {
				t.Errorf("parseStatusFlag failed: %s", diff)
			}
		})
	}
}
