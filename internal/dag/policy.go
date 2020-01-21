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
	"fmt"
	"net/http"
	"time"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
)

func retryPolicy(rp *projcontour.RetryPolicy) *RetryPolicy {
	if rp == nil {
		return nil
	}
	perTryTimeout, _ := time.ParseDuration(rp.PerTryTimeout)
	return &RetryPolicy{
		RetryOn:       "5xx",
		NumRetries:    max(1, uint32(rp.NumRetries)),
		PerTryTimeout: perTryTimeout,
	}
}

func headersPolicy(policy *projcontour.HeadersPolicy, allowHostRewrite bool) (*HeadersPolicy, error) {
	if policy == nil {
		return nil, nil
	}

	set := make(map[string]string, len(policy.Set))
	hostRewrite := ""
	for _, entry := range policy.Set {
		key := http.CanonicalHeaderKey(entry.Name)
		if _, ok := set[key]; ok {
			return nil, fmt.Errorf("duplicate header addition: %q", key)
		}
		if key == "Host" {
			if !allowHostRewrite {
				return nil, fmt.Errorf("rewriting %q header is not supported", key)
			}
			hostRewrite = entry.Value
			continue
		}
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			return nil, fmt.Errorf("invalid set header %q: %v", key, msgs)
		}
		set[key] = escapeHeaderValue(entry.Value)
	}

	remove := sets.NewString()
	for _, entry := range policy.Remove {
		key := http.CanonicalHeaderKey(entry)
		if remove.Has(key) {
			return nil, fmt.Errorf("duplicate header removal: %q", key)
		}
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			return nil, fmt.Errorf("invalid remove header %q: %v", key, msgs)
		}
		remove.Insert(key)
	}
	rl := remove.List()

	if len(set) == 0 {
		set = nil
	}
	if len(rl) == 0 {
		rl = nil
	}

	return &HeadersPolicy{
		Set:         set,
		HostRewrite: hostRewrite,
		Remove:      rl,
	}, nil
}

// ingressRetryPolicy builds a RetryPolicy from ingress annotations.
func ingressRetryPolicy(ingress *v1beta1.Ingress) *RetryPolicy {
	retryOn := compatAnnotation(ingress, "retry-on")
	if len(retryOn) < 1 {
		return nil
	}
	// if there is a non empty retry-on annotation, build a RetryPolicy manually.
	return &RetryPolicy{
		RetryOn: retryOn,
		// TODO(dfc) numRetries may parse as 0, which is inconsistent with
		// retryPolicyIngressRoute()'s default value of 1.
		NumRetries: numRetries(ingress),
		// TODO(dfc) perTryTimeout will parse to -1, infinite, in the case of
		// invalid data, this is inconsistent with retryPolicyIngressRoute()'s default value
		// of 0 duration.
		PerTryTimeout: perTryTimeout(ingress),
	}
}

func ingressTimeoutPolicy(ingress *v1beta1.Ingress) *TimeoutPolicy {
	response := compatAnnotation(ingress, "response-timeout")
	if len(response) == 0 {
		// Note: due to a misunderstanding the name of the annotation is
		// request timeout, but it is actually applied as a timeout on
		// the response body.
		response = compatAnnotation(ingress, "request-timeout")
		if len(response) == 0 {
			return nil
		}
	}
	// if the request timeout annotation is present on this ingress
	// construct and use the ingressroute timeout policy logic.
	return timeoutPolicy(&projcontour.TimeoutPolicy{
		Response: response,
	})
}

func ingressrouteTimeoutPolicy(tp *ingressroutev1.TimeoutPolicy) *TimeoutPolicy {
	if tp == nil {
		return nil
	}
	return &TimeoutPolicy{
		// due to a misunderstanding the name of the field ingressroute is
		// Request, however the timeout applies to the response resulting from
		// a request.
		ResponseTimeout: parseTimeout(tp.Request),
	}
}

func timeoutPolicy(tp *projcontour.TimeoutPolicy) *TimeoutPolicy {
	if tp == nil {
		return nil
	}
	return &TimeoutPolicy{
		ResponseTimeout: parseTimeout(tp.Response),
		IdleTimeout:     parseTimeout(tp.Idle),
	}
}
func ingressrouteHealthCheckPolicy(hc *ingressroutev1.HealthCheck) *HTTPHealthCheckPolicy {
	if hc == nil {
		return nil
	}
	return &HTTPHealthCheckPolicy{
		Path:               hc.Path,
		Host:               hc.Host,
		Interval:           time.Duration(hc.IntervalSeconds) * time.Second,
		Timeout:            time.Duration(hc.TimeoutSeconds) * time.Second,
		UnhealthyThreshold: uint32(hc.UnhealthyThresholdCount),
		HealthyThreshold:   uint32(hc.HealthyThresholdCount),
	}
}

func httpHealthCheckPolicy(hc *projcontour.HTTPHealthCheckPolicy) *HTTPHealthCheckPolicy {
	if hc == nil {
		return nil
	}
	return &HTTPHealthCheckPolicy{
		Path:               hc.Path,
		Host:               hc.Host,
		Interval:           time.Duration(hc.IntervalSeconds) * time.Second,
		Timeout:            time.Duration(hc.TimeoutSeconds) * time.Second,
		UnhealthyThreshold: uint32(hc.UnhealthyThresholdCount),
		HealthyThreshold:   uint32(hc.HealthyThresholdCount),
	}
}

func tcpHealthCheckPolicy(hc *projcontour.TCPHealthCheckPolicy) *TCPHealthCheckPolicy {
	if hc == nil {
		return nil
	}
	return &TCPHealthCheckPolicy{
		Interval:           time.Duration(hc.IntervalSeconds) * time.Second,
		Timeout:            time.Duration(hc.TimeoutSeconds) * time.Second,
		UnhealthyThreshold: hc.UnhealthyThresholdCount,
		HealthyThreshold:   hc.HealthyThresholdCount,
	}
}

// loadBalancerPolicy returns the load balancer strategy or
// blank if no valid strategy is supplied.
func loadBalancerPolicy(lbp *projcontour.LoadBalancerPolicy) string {
	if lbp == nil {
		return ""
	}
	switch lbp.Strategy {
	case "WeightedLeastRequest":
		return "WeightedLeastRequest"
	case "Random":
		return "Random"
	case "Cookie":
		return "Cookie"
	default:
		return ""
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

func max(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func prefixReplacementsAreValid(replacements []projcontour.ReplacePrefix) error {
	prefixes := map[string]bool{}

	for _, r := range replacements {
		if prefixes[r.Prefix] {
			if len(r.Prefix) > 0 {
				// The replacements are not valid if there are duplicates.
				return fmt.Errorf("duplicate replacement prefix '%s'", r.Prefix)
			}
			// Can't replace the empty prefix multiple times.
			return fmt.Errorf("ambiguous prefix replacement")
		}

		prefixes[r.Prefix] = true
	}

	return nil
}
