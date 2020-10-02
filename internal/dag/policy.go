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
	"fmt"
	"net/http"
	"strings"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/sirupsen/logrus"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
)

// retryOn transforms a slice of retry on values to a comma-separated string.
// CRD validation ensures that all retry on values are valid.
func retryOn(ro []contour_api_v1.RetryOn) string {
	if len(ro) == 0 {
		return "5xx"
	}

	ss := make([]string, len(ro))
	for i, value := range ro {
		ss[i] = string(value)
	}
	return strings.Join(ss, ",")
}

func retryPolicy(rp *contour_api_v1.RetryPolicy) *RetryPolicy {
	if rp == nil {
		return nil
	}

	// If PerTryTimeout is not a valid duration string, use the Envoy default
	// value, otherwise use the provided value.
	// TODO(sk) it might make sense to change the behavior here to be consistent
	// with other timeout parsing, meaning use timeout.Parse which would result
	// in a disabled per-try timeout if the input was not a valid duration.
	perTryTimeout := timeout.DefaultSetting()
	if perTryDuration, err := time.ParseDuration(rp.PerTryTimeout); err == nil {
		perTryTimeout = timeout.DurationSetting(perTryDuration)
	}

	return &RetryPolicy{
		RetryOn:              retryOn(rp.RetryOn),
		RetriableStatusCodes: rp.RetriableStatusCodes,
		NumRetries:           max(1, uint32(rp.NumRetries)),
		PerTryTimeout:        perTryTimeout,
	}
}

func headersPolicyService(policy *contour_api_v1.HeadersPolicy) (*HeadersPolicy, error) {
	return headersPolicyRoute(policy, false)

}

func headersPolicyRoute(policy *contour_api_v1.HeadersPolicy, allowHostRewrite bool) (*HeadersPolicy, error) {
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

func escapeHeaderValue(value string) string {
	// Envoy supports %-encoded variables, so literal %'s in the header's value must be escaped.  See:
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#custom-request-response-headers
	return strings.Replace(value, "%", "%%", -1)
}

// ingressRetryPolicy builds a RetryPolicy from ingress annotations.
func ingressRetryPolicy(ingress *v1beta1.Ingress, log logrus.FieldLogger) *RetryPolicy {
	retryOn := annotation.CompatAnnotation(ingress, "retry-on")
	if len(retryOn) < 1 {
		return nil
	}

	// if there is a non empty retry-on annotation, build a RetryPolicy manually.
	rp := &RetryPolicy{
		RetryOn: retryOn,
		// TODO(dfc) k8s.NumRetries may parse as 0, which is inconsistent with
		// retryPolicy()'s default value of 1.
		NumRetries: annotation.NumRetries(ingress),
	}

	perTryTimeout, err := annotation.PerTryTimeout(ingress)
	if err != nil {
		log.WithError(err).Error("Error parsing per-try-timeout annotation")

		return rp
	}

	rp.PerTryTimeout = perTryTimeout
	return rp
}

func ingressTimeoutPolicy(ingress *v1beta1.Ingress, log logrus.FieldLogger) TimeoutPolicy {
	response := annotation.CompatAnnotation(ingress, "response-timeout")
	if len(response) == 0 {
		// Note: due to a misunderstanding the name of the annotation is
		// request timeout, but it is actually applied as a timeout on
		// the response body.
		response = annotation.CompatAnnotation(ingress, "request-timeout")
		if len(response) == 0 {
			return TimeoutPolicy{
				ResponseTimeout: timeout.DefaultSetting(),
				IdleTimeout:     timeout.DefaultSetting(),
			}
		}
	}
	// if the request timeout annotation is present on this ingress
	// construct and use the HTTPProxy timeout policy logic.
	tp, err := timeoutPolicy(&contour_api_v1.TimeoutPolicy{
		Response: response,
	})
	if err != nil {
		log.WithError(err).Error("Error parsing response-timeout annotation, using the default value")
		return TimeoutPolicy{}
	}

	return tp
}

func timeoutPolicy(tp *contour_api_v1.TimeoutPolicy) (TimeoutPolicy, error) {
	if tp == nil {
		return TimeoutPolicy{
			ResponseTimeout: timeout.DefaultSetting(),
			IdleTimeout:     timeout.DefaultSetting(),
		}, nil
	}

	responseTimeout, err := timeout.Parse(tp.Response)
	if err != nil {
		return TimeoutPolicy{}, fmt.Errorf("error parsing response timeout: %w", err)
	}

	idleTimeout, err := timeout.Parse(tp.Idle)
	if err != nil {
		return TimeoutPolicy{}, fmt.Errorf("error parsing idle timeout: %w", err)
	}

	return TimeoutPolicy{
		ResponseTimeout: responseTimeout,
		IdleTimeout:     idleTimeout,
	}, nil
}

func httpHealthCheckPolicy(hc *contour_api_v1.HTTPHealthCheckPolicy) *HTTPHealthCheckPolicy {
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

func tcpHealthCheckPolicy(hc *contour_api_v1.TCPHealthCheckPolicy) *TCPHealthCheckPolicy {
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
func loadBalancerPolicy(lbp *contour_api_v1.LoadBalancerPolicy) string {
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

func max(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func prefixReplacementsAreValid(replacements []contour_api_v1.ReplacePrefix) (string, error) {
	prefixes := map[string]bool{}

	for _, r := range replacements {
		if prefixes[r.Prefix] {
			if len(r.Prefix) > 0 {
				// The replacements are not valid if there are duplicates.
				return "DuplicateReplacement", fmt.Errorf("duplicate replacement prefix '%s'", r.Prefix)
			}
			// Can't replace the empty prefix multiple times.
			return "AmbiguousReplacement", fmt.Errorf("ambiguous prefix replacement")
		}

		prefixes[r.Prefix] = true
	}

	return "", nil
}
