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
	"io/ioutil"
	"testing"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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
				PerTryTimeout: timeout.DurationSetting(10 * time.Second),
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
				PerTryTimeout: timeout.DefaultSetting(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ingressRetryPolicy(tc.i, &logrus.Logger{Out: ioutil.Discard})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRetryPolicy(t *testing.T) {
	tests := map[string]struct {
		rp   *contour_api_v1.RetryPolicy
		want *RetryPolicy
	}{
		"nil retry policy": {
			rp:   nil,
			want: nil,
		},
		"empty policy": {
			rp: &contour_api_v1.RetryPolicy{},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"explicitly zero retries": {
			rp: &contour_api_v1.RetryPolicy{
				NumRetries: 0, // zero value for NumRetries
			},
			want: &RetryPolicy{
				RetryOn:    "5xx",
				NumRetries: 1,
			},
		},
		"no retry count, per try timeout": {
			rp: &contour_api_v1.RetryPolicy{
				PerTryTimeout: "10s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DurationSetting(10 * time.Second),
			},
		},
		"explicit 0s timeout": {
			rp: &contour_api_v1.RetryPolicy{
				PerTryTimeout: "0s",
			},
			want: &RetryPolicy{
				RetryOn:       "5xx",
				NumRetries:    1,
				PerTryTimeout: timeout.DefaultSetting(),
			},
		},
		"retry on": {
			rp: &contour_api_v1.RetryPolicy{
				RetryOn: []contour_api_v1.RetryOn{"gateway-error", "connect-failure"},
			},
			want: &RetryPolicy{
				RetryOn:    "gateway-error,connect-failure",
				NumRetries: 1,
			},
		},
		"retriable status codes": {
			rp: &contour_api_v1.RetryPolicy{
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
		tp      *contour_api_v1.TimeoutPolicy
		want    TimeoutPolicy
		wantErr bool
	}{
		"nil timeout policy": {
			tp:   nil,
			want: TimeoutPolicy{},
		},
		"empty timeout policy": {
			tp:   &contour_api_v1.TimeoutPolicy{},
			want: TimeoutPolicy{},
		},
		"valid response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "1m30s",
			},
			want: TimeoutPolicy{
				ResponseTimeout: timeout.DurationSetting(90 * time.Second),
			},
		},
		"invalid response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "90", // 90 what?
			},
			wantErr: true,
		},
		"infinite response timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Response: "infinite",
			},
			want: TimeoutPolicy{
				ResponseTimeout: timeout.DisabledSetting(),
			},
		},
		"idle timeout": {
			tp: &contour_api_v1.TimeoutPolicy{
				Idle: "900s",
			},
			want: TimeoutPolicy{
				IdleTimeout: timeout.DurationSetting(900 * time.Second),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := timeoutPolicy(tc.tp)
			if tc.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.Equal(t, tc.want, got)
				assert.NoError(t, gotErr)
			}

		})
	}
}

func TestLoadBalancerPolicy(t *testing.T) {
	tests := map[string]struct {
		lbp  *contour_api_v1.LoadBalancerPolicy
		want string
	}{
		"nil": {
			lbp:  nil,
			want: "",
		},
		"empty": {
			lbp:  &contour_api_v1.LoadBalancerPolicy{},
			want: "",
		},
		"WeightedLeastRequest": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "WeightedLeastRequest",
			},
			want: "WeightedLeastRequest",
		},
		"Random": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "Random",
			},
			want: "Random",
		},
		"Cookie": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
				Strategy: "Cookie",
			},
			want: "Cookie",
		},
		"unknown": {
			lbp: &contour_api_v1.LoadBalancerPolicy{
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

func TestHeadersPolicy(t *testing.T) {
	tests := map[string]struct {
		hp      *contour_api_v1.HeadersPolicy
		want    HeadersPolicy
		wantErr bool
	}{
		"no percentage unchanged": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-App-Weight",
					Value: "100",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-App-Weight": "100",
				},
			},
		},
		"simple percentage escape": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-App-Weight",
					Value: "100%",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-App-Weight": "100%%",
				},
			},
		},
		"known good Envoy dynamic header unescaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Envoy-Hostname",
					Value: "%HOSTNAME%",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Envoy-Hostname": "%HOSTNAME%",
				},
			},
		},
		"unknown Envoy dynamic header is escaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Envoy-Unknown",
					Value: "%UNKNOWN%",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Envoy-Unknown": "%%UNKNOWN%%",
				},
			},
		},
		"valid Envoy REQ header unescaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Request-Host",
					Value: "%REQ(Host)%",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Request-Host": "%REQ(Host)%",
				},
			},
		},
		"invalid Envoy REQ header is escaped": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Request-Host",
					Value: "%REQ(inv@lid-header)%",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Request-Host": "%%REQ(inv@lid-header)%%",
				},
			},
		},
		"header value with dynamic and non-dynamic content and multiple dynamic fields": {
			hp: &contour_api_v1.HeadersPolicy{
				Set: []contour_api_v1.HeaderValue{{
					Name:  "X-Host-Protocol",
					Value: "%HOSTNAME% - %PROTOCOL%",
				}},
			},
			want: HeadersPolicy{
				Set: map[string]string{
					"X-Host-Protocol": "%HOSTNAME% - %PROTOCOL%",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := headersPolicyService(tc.hp)
			if tc.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.Equal(t, tc.want, *got)
				assert.NoError(t, gotErr)
			}
		})
	}
}

func TestRateLimitPolicy(t *testing.T) {
	tests := map[string]struct {
		in      *contour_api_v1.RateLimitPolicy
		want    *RateLimitPolicy
		wantErr string
	}{
		"nil input": {
			in:   nil,
			want: nil,
		},
		"nil local rate limit policy": {
			in:   &contour_api_v1.RateLimitPolicy{},
			want: nil,
		},
		"no burst": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 3,
					Unit:     "second",
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:     3,
					TokensPerFill: 3,
					FillInterval:  time.Second,
				},
			},
		},
		"burst": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 3,
					Unit:     "second",
					Burst:    4,
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:     7,
					TokensPerFill: 3,
					FillInterval:  time.Second,
				},
			},
		},
		"custom response status code": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests:           10,
					Unit:               "minute",
					ResponseStatusCode: 431,
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:          10,
					TokensPerFill:      10,
					FillInterval:       time.Minute,
					ResponseStatusCode: 431,
				},
			},
		},
		"custom response headers to add": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 10,
					Unit:     "hour",
					ResponseHeadersToAdd: []contour_api_v1.HeaderValue{
						{
							Name:  "header-1",
							Value: "header-value-1",
						},
						{
							Name:  "header-2",
							Value: "header-value-2",
						},
					},
				},
			},
			want: &RateLimitPolicy{
				Local: &LocalRateLimitPolicy{
					MaxTokens:     10,
					TokensPerFill: 10,
					FillInterval:  time.Hour,
					ResponseHeadersToAdd: map[string]string{
						"header-1": "header-value-1",
						"header-2": "header-value-2",
					},
				},
			},
		},
		"invalid unit": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 10,
					Unit:     "invalid-unit",
				},
			},
			wantErr: "invalid unit \"invalid-unit\" in local rate limit policy",
		},
		"invalid requests": {
			in: &contour_api_v1.RateLimitPolicy{
				Local: &contour_api_v1.LocalRateLimitPolicy{
					Requests: 0,
					Unit:     "second",
				},
			},
			wantErr: "invalid requests value 0 in local rate limit policy",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rlp, err := rateLimitPolicy(tc.in)

			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.Equal(t, tc.want, rlp)
			}
		})
	}
}
