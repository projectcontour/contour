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

package dag

import (
	"testing"
	"time"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/assert"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRetryPolicyIngress(t *testing.T) {
	tests := map[string]struct {
		i    *v1beta1.Ingress
		want *RetryPolicy
	}{
		"no anotations": {
			i:    &v1beta1.Ingress{},
			want: nil,
		},
		"retry-on": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on": "5xx",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn: "5xx",
			},
		},
		"explicitly zero retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":    "5xx",
						"projectcontour.io/num-retries": "0",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 0,
			},
		},
		"legacy explicitly zero retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":     "5xx",
						"contour.heptio.com/num-retries": "0",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 0,
			},
		},
		"num-retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":    "5xx",
						"projectcontour.io/num-retries": "7",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 7,
			},
		},
		"legacy num-retries": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":     "5xx",
						"contour.heptio.com/num-retries": "7",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 7,
			},
		},
		"no retry count, per try timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":        "5xx",
						"projectcontour.io/per-try-timeout": "10s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: 10 * time.Second,
			},
		},
		"no retry count, legacy per try timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":         "5xx",
						"contour.heptio.com/per-try-timeout": "10s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: 10 * time.Second,
			},
		},
		"explicit 0s timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":        "5xx",
						"projectcontour.io/per-try-timeout": "0s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: 0 * time.Second,
			},
		},
		"legacy explicit 0s timeout": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/retry-on":         "5xx",
						"contour.heptio.com/per-try-timeout": "0s",
					},
				},
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    0,
				PerTryTimeout: 0 * time.Second,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ingressRetryPolicy(tc.i)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRetryPolicy(t *testing.T) {
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
		"retry on": {
			rp: &projcontour.RetryPolicy{
				RetryOn: []projcontour.RetryOn{"gateway-error", "connect-failure"},
			},
			want: &RetryPolicy{
				RetryOn:    "gateway-error,connect-failure",
				NumRetries: 1,
			},
		},
		"retriable status codes": {
			rp: &projcontour.RetryPolicy{
				RetriableStatusCodes: []uint32{502, 503, 504},
			},
			want: &RetryPolicy{
				RetryOn:              "5xx",
				RetriableStatusCodes: []uint32{502, 503, 504},
				NumRetries:           1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := retryPolicy(tc.rp)
			assert.Equal(t, tc.want, got)
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
		"idle timeout": {
			tp: &projcontour.TimeoutPolicy{
				Idle: "900s",
			},
			want: &TimeoutPolicy{
				IdleTimeout: 900 * time.Second,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := timeoutPolicy(tc.tp)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLoadBalancerPolicy(t *testing.T) {
	tests := map[string]struct {
		lbp  *projcontour.LoadBalancerPolicy
		want string
	}{
		"nil": {
			lbp:  nil,
			want: "",
		},
		"empty": {
			lbp:  &projcontour.LoadBalancerPolicy{},
			want: "",
		},
		"WeightedLeastRequest": {
			lbp: &projcontour.LoadBalancerPolicy{
				Strategy: "WeightedLeastRequest",
			},
			want: "WeightedLeastRequest",
		},
		"Random": {
			lbp: &projcontour.LoadBalancerPolicy{
				Strategy: "Random",
			},
			want: "Random",
		},
		"Cookie": {
			lbp: &projcontour.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
			want: "Cookie",
		},
		"unknown": {
			lbp: &projcontour.LoadBalancerPolicy{
				Strategy: "please",
			},
			want: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := loadBalancerPolicy(tc.lbp)
			assert.Equal(t, tc.want, got)
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
			got := annotation.ParseTimeout(tc.duration)
			assert.Equal(t, tc.want, got)
		})
	}
}
