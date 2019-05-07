// Copyright © 2019 Heptio
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

package dag

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/heptio/contour/apis/contour/v1beta1"
)

func TestRetryPolicyIngressRoute(t *testing.T) {
	tests := map[string]struct {
		rp   *v1beta1.RetryPolicy
		want *RetryPolicy
	}{
		"nil retry policy": {
			rp:   nil,
			want: nil,
		},
		"empty policy": {
			rp: &v1beta1.RetryPolicy{},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"explicitly zero retries": {
			rp: &v1beta1.RetryPolicy{
				NumRetries: 0, // zero value for NumRetries
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"no retry count, per try timeout": {
			rp: &v1beta1.RetryPolicy{
				PerTryTimeout: "10s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: 10 * time.Second,
			},
		},
		"explicit 0s timeout": {
			rp: &v1beta1.RetryPolicy{
				PerTryTimeout: "0s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: 0 * time.Second,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := retryPolicyIngressRoute(tc.rp)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
