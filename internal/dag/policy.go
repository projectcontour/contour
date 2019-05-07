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
	"time"

	"github.com/heptio/contour/apis/contour/v1beta1"
)

func retryPolicy(rp *v1beta1.RetryPolicy) *RetryPolicy {
	if rp == nil {
		return nil
	}
	perTryTimeout, _ := time.ParseDuration(rp.PerTryTimeout)
	return &RetryPolicy{
		RetryOn:       "5xx",
		NumRetries:    max(1, rp.NumRetries),
		PerTryTimeout: perTryTimeout,
	}
}

func timeoutPolicy(tp *v1beta1.TimeoutPolicy) *TimeoutPolicy {
	if tp == nil {
		return nil
	}
	return &TimeoutPolicy{
		Timeout: parseTimeout(tp.Request),
	}
}

func parseTimeout(timeout string) time.Duration {
	if timeout == "" {
		// Blank is interpreted as no timeout specified, use envoy defaults
		// By default envoy applies a 15 second timeout to all backend requests.
		// The explicit value 0 turns off the timeout, implying "never time out"
		// https://www.envoyproxy.io/docs/envoy/v1.5.0/api-v2/rds.proto#routeaction
		return 0
	}

	// Interpret "infinity" explicitly as an infinite timeout, which envoy config
	// expects as a timeout of 0. This could be specified with the duration string
	// "0s" but want to give an explicit out for operators.
	if timeout == "infinity" {
		return -1
	}

	d, err := time.ParseDuration(timeout)
	if err != nil {
		// TODO(cmalonty) plumb a logger in here so we can log this error.
		// Assuming infinite duration is going to surprise people less for
		// a not-parseable duration than a implicit 15 second one.
		return -1
	}
	return d
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
