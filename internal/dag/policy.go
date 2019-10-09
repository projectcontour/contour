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
	"time"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"k8s.io/api/extensions/v1beta1"
)

func retryPolicy(rp *projcontour.RetryPolicy) *RetryPolicy {
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

func ingressRetryPolicy(ingress *v1beta1.Ingress) *RetryPolicy {
	retryOn, ok := ingress.Annotations[annotationRetryOn]
	if !ok || len(retryOn) < 1 {
		return nil
	}
	// if there is a non empty retry-on annotation, build a RetryPolicy manually.
	return &RetryPolicy{
		RetryOn: retryOn,
		// TODO(dfc) NumRetries may parse as 0, which is inconsistent with
		// retryPolicyIngressRoute()'s default value of 1.
		NumRetries: numRetries(ingress),
		// TODO(dfc) PerTryTimeout will parse to -1, infinite, in the case of
		// invalid data, this is inconsistent with retryPolicyIngressRoute()'s default value
		// of 0 duration.
		PerTryTimeout: parseTimeout(ingress.Annotations[annotationPerTryTimeout]),
	}
}

func ingressTimeoutPolicy(ingress *v1beta1.Ingress) *TimeoutPolicy {
	response, ok := ingress.Annotations["projectcontour.io/response-timeout"]
	if !ok {
		// Note: due to a misunderstanding the name of the annotation is
		// request timeout, but it is actually applied as a timeout on
		// the response body.
		response, ok = ingress.Annotations["contour.heptio.com/request-timeout"]
		if !ok {
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
func ingressrouteHealthCheckPolicy(hc *ingressroutev1.HealthCheck) *HealthCheckPolicy {
	if hc == nil {
		return nil
	}
	return &HealthCheckPolicy{
		Path:               hc.Path,
		Host:               hc.Host,
		Interval:           time.Duration(hc.IntervalSeconds) * time.Second,
		Timeout:            time.Duration(hc.TimeoutSeconds) * time.Second,
		UnhealthyThreshold: hc.UnhealthyThresholdCount,
		HealthyThreshold:   hc.HealthyThresholdCount,
	}
}

func healthCheckPolicy(hc *projcontour.HTTPHealthCheckPolicy) *HealthCheckPolicy {
	if hc == nil {
		return nil
	}
	return &HealthCheckPolicy{
		Path:               hc.Path,
		Host:               hc.Host,
		Interval:           time.Duration(hc.IntervalSeconds) * time.Second,
		Timeout:            time.Duration(hc.TimeoutSeconds) * time.Second,
		UnhealthyThreshold: hc.UnhealthyThresholdCount,
		HealthyThreshold:   hc.HealthyThresholdCount,
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
