// Copyright © 2019 VMware
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
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

func TestRetryPolicyIngressRoute(t *testing.T) {
	tests := map[string]struct {
		rp   *projcontour.RetryPolicy
		want *RetryPolicy
	}{
		"nil retry policy": {
			rp:   nil,
			want: nil,
		},
		"empty policy": {
			rp: &projcontour.RetryPolicy{},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"explicitly zero retries": {
			rp: &projcontour.RetryPolicy{
				NumRetries: 0, // zero value for NumRetries
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"no retry count, per try timeout": {
			rp: &projcontour.RetryPolicy{
				PerTryTimeout: "10s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: 10 * time.Second,
			},
		},
		"explicit 0s timeout": {
			rp: &projcontour.RetryPolicy{
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
			got := retryPolicy(tc.rp)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestTimeoutPolicyIngressRoute(t *testing.T) {
	tests := map[string]struct {
		tp   *ingressroutev1.TimeoutPolicy
		want *TimeoutPolicy
	}{
		"nil timeout policy": {
			tp:   nil,
			want: nil,
		},
		"empty timeout policy": {
			tp: &ingressroutev1.TimeoutPolicy{},
			want: &TimeoutPolicy{
				ResponseTimeout: 0 * time.Second,
			},
		},
		"valid request timeout": {
			tp: &ingressroutev1.TimeoutPolicy{
				Request: "1m30s",
			},
			want: &TimeoutPolicy{
				ResponseTimeout: 90 * time.Second,
			},
		},
		"invalid request timeout": {
			tp: &ingressroutev1.TimeoutPolicy{
				Request: "90", // 90 what?
			},
			want: &TimeoutPolicy{
				// the documentation for an invalid timeout says the duration will
				// be undefined. In practice we take the spec from the
				// contour.heptio.com/request-timeout annotation, which is defined
				// to choose infinite when its valid cannot be parsed.
				ResponseTimeout: -1,
			},
		},
		"infinite request timeout": {
			tp: &ingressroutev1.TimeoutPolicy{
				Request: "infinite",
			},
			want: &TimeoutPolicy{
				ResponseTimeout: -1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ingressrouteTimeoutPolicy(tc.tp)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestTimeoutPolicy(t *testing.T) {
	tests := map[string]struct {
		tp   *projcontour.TimeoutPolicy
		want *TimeoutPolicy
	}{
		"nil timeout policy": {
			tp:   nil,
			want: nil,
		},
		"empty timeout policy": {
			tp: &projcontour.TimeoutPolicy{},
			want: &TimeoutPolicy{
				ResponseTimeout: 0 * time.Second,
			},
		},
		"valid response timeout": {
			tp: &projcontour.TimeoutPolicy{
				Response: "1m30s",
			},
			want: &TimeoutPolicy{
				ResponseTimeout: 90 * time.Second,
			},
		},
		"invalid response timeout": {
			tp: &projcontour.TimeoutPolicy{
				Response: "90", // 90 what?
			},
			want: &TimeoutPolicy{
				// the documentation for an invalid timeout says the duration will
				// be undefined. In practice we take the spec from the
				// contour.heptio.com/request-timeout annotation, which is defined
				// to choose infinite when its valid cannot be parsed.
				ResponseTimeout: -1,
			},
		},
		"infinite response timeout": {
			tp: &projcontour.TimeoutPolicy{
				Response: "infinite",
			},
			want: &TimeoutPolicy{
				ResponseTimeout: -1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := timeoutPolicy(tc.tp)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestParseTimeout(t *testing.T) {
	tests := map[string]struct {
		duration string
		want     time.Duration
	}{
		"empty": {
			duration: "",
			want:     0,
		},
		"infinity": {
			duration: "infinity",
			want:     -1,
		},
		"10 seconds": {
			duration: "10s",
			want:     10 * time.Second,
		},
		"invalid": {
			duration: "10", // 10 what?
			want:     -1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseTimeout(tc.duration)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
