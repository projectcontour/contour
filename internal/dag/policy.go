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
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	networking_v1 "k8s.io/api/networking/v1"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	// LoadBalancerPolicyWeightedLeastRequest specifies the backend with least
	// active requests will be chosen by the load balancer.
	LoadBalancerPolicyWeightedLeastRequest = "WeightedLeastRequest"

	// LoadBalancerPolicyRandom denotes the load balancer will choose a random
	// backend when routing a request.
	LoadBalancerPolicyRandom = "Random"

	// LoadBalancerPolicyRoundRobin denotes the load balancer will route
	// requests in a round-robin fashion among backend instances.
	LoadBalancerPolicyRoundRobin = "RoundRobin"

	// LoadBalancerPolicyCookie denotes load balancing will be performed via a
	// Contour specified cookie.
	LoadBalancerPolicyCookie = "Cookie"

	// LoadBalancerPolicyRequestHash denotes request attribute hashing is used
	// to make load balancing decisions.
	LoadBalancerPolicyRequestHash = "RequestHash"
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

func headersPolicyService(defaultPolicy *HeadersPolicy, policy *contour_api_v1.HeadersPolicy, dynamicHeaders map[string]string) (*HeadersPolicy, error) {
	if defaultPolicy == nil {
		return headersPolicyRoute(policy, false, dynamicHeaders)
	}
	userPolicy, err := headersPolicyRoute(policy, false, dynamicHeaders)
	if err != nil {
		return nil, err
	}
	if userPolicy == nil {
		userPolicy = &HeadersPolicy{}
	}

	if userPolicy.Set == nil {
		userPolicy.Set = make(map[string]string, len(defaultPolicy.Set))
	}
	for k, v := range defaultPolicy.Set {
		key := http.CanonicalHeaderKey(k)
		if key == "Host" {
			return nil, fmt.Errorf("rewriting %q header is not supported", key)
		}
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			return nil, fmt.Errorf("invalid set header %q: %v", key, msgs)
		}
		// if the user policy set on the object does not contain this header then use the default
		if _, exists := userPolicy.Set[key]; !exists {
			userPolicy.Set[key] = escapeHeaderValue(v, dynamicHeaders)
		}
	}
	// add any default remove header policy if not already set
	remove := sets.NewString()
	for _, entry := range userPolicy.Remove {
		remove.Insert(entry)
	}
	for _, entry := range defaultPolicy.Remove {
		key := http.CanonicalHeaderKey(entry)
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			return nil, fmt.Errorf("invalid set header %q: %v", key, msgs)
		}
		if !remove.Has(key) {
			userPolicy.Remove = append(userPolicy.Remove, key)
		}
	}

	return userPolicy, nil
}

func headersPolicyRoute(policy *contour_api_v1.HeadersPolicy, allowHostRewrite bool, dynamicHeaders map[string]string) (*HeadersPolicy, error) {
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
		set[key] = escapeHeaderValue(entry.Value, dynamicHeaders)
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

// headersPolicyGatewayAPI builds a *HeaderPolicy for the supplied HTTPRequestHeaderFilter.
// TODO: Take care about the order of operators once https://github.com/kubernetes-sigs/gateway-api/issues/480 was solved.
func headersPolicyGatewayAPI(hf *gatewayapi_v1alpha1.HTTPRequestHeaderFilter) (*HeadersPolicy, error) {
	set, add := make(map[string]string, len(hf.Set)), make(map[string]string, len(hf.Add))
	hostRewrite := ""
	errlist := []error{}
	for k, v := range hf.Set {
		key := http.CanonicalHeaderKey(k)
		if _, ok := set[key]; ok {
			errlist = append(errlist, fmt.Errorf("duplicate header addition: %q", key))
			continue
		}
		if key == "Host" {
			hostRewrite = v
			continue
		}
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			errlist = append(errlist, fmt.Errorf("invalid set header %q: %v", key, msgs))
			continue
		}
		set[key] = escapeHeaderValue(v, nil)
	}
	for k, v := range hf.Add {
		key := http.CanonicalHeaderKey(k)
		if _, ok := add[key]; ok {
			errlist = append(errlist, fmt.Errorf("duplicate header addition: %q", key))
			continue
		}
		if key == "Host" {
			hostRewrite = v
			continue
		}
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			errlist = append(errlist, fmt.Errorf("invalid add header %q: %v", key, msgs))
			continue
		}
		add[key] = escapeHeaderValue(v, nil)
	}

	remove := sets.NewString()
	for _, k := range hf.Remove {
		key := http.CanonicalHeaderKey(k)
		if remove.Has(key) {
			errlist = append(errlist, fmt.Errorf("duplicate header removal: %q", key))
			continue
		}
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			errlist = append(errlist, fmt.Errorf("invalid remove header %q: %v", key, msgs))
			continue
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
		Add:         add,
		Set:         set,
		HostRewrite: hostRewrite,
		Remove:      rl,
	}, utilerrors.NewAggregate(errlist)
}

func escapeHeaderValue(value string, dynamicHeaders map[string]string) string {
	// Envoy supports %-encoded variables, so literal %'s in the header's value must be escaped.  See:
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#custom-request-response-headers
	// Only allow a specific set of known good Envoy dynamic headers to pass through unescaped
	if !strings.Contains(value, "%") {
		return value
	}
	escapedValue := strings.Replace(value, "%", "%%", -1)
	for dynamicVar, dynamicVal := range dynamicHeaders {
		escapedValue = strings.ReplaceAll(escapedValue, "%%"+dynamicVar+"%%", dynamicVal)
	}
	for _, envoyVar := range []string{
		"DOWNSTREAM_REMOTE_ADDRESS",
		"DOWNSTREAM_REMOTE_ADDRESS_WITHOUT_PORT",
		"DOWNSTREAM_LOCAL_ADDRESS",
		"DOWNSTREAM_LOCAL_ADDRESS_WITHOUT_PORT",
		"DOWNSTREAM_LOCAL_PORT",
		"DOWNSTREAM_LOCAL_URI_SAN",
		"DOWNSTREAM_PEER_URI_SAN",
		"DOWNSTREAM_LOCAL_SUBJECT",
		"DOWNSTREAM_PEER_SUBJECT",
		"DOWNSTREAM_PEER_ISSUER",
		"DOWNSTREAM_TLS_SESSION_ID",
		"DOWNSTREAM_TLS_CIPHER",
		"DOWNSTREAM_TLS_VERSION",
		"DOWNSTREAM_PEER_FINGERPRINT_256",
		"DOWNSTREAM_PEER_FINGERPRINT_1",
		"DOWNSTREAM_PEER_SERIAL",
		"DOWNSTREAM_PEER_CERT",
		"DOWNSTREAM_PEER_CERT_V_START",
		"DOWNSTREAM_PEER_CERT_V_END",
		"HOSTNAME",
		"PROTOCOL",
		"UPSTREAM_REMOTE_ADDRESS",
		"RESPONSE_FLAGS",
		"RESPONSE_CODE_DETAILS",
	} {
		escapedValue = strings.ReplaceAll(escapedValue, "%%"+envoyVar+"%%", "%"+envoyVar+"%")
	}
	// REQ(header-name)
	var validReqEnvoyVar = regexp.MustCompile(`%(%REQ\([\w-]+\)%)%`)
	escapedValue = validReqEnvoyVar.ReplaceAllString(escapedValue, "$1")
	return escapedValue
}

// ingressRetryPolicy builds a RetryPolicy from ingress annotations.
func ingressRetryPolicy(ingress *networking_v1.Ingress, log logrus.FieldLogger) *RetryPolicy {
	retryOn := annotation.ContourAnnotation(ingress, "retry-on")
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

func ingressTimeoutPolicy(ingress *networking_v1.Ingress, log logrus.FieldLogger) TimeoutPolicy {
	response := annotation.ContourAnnotation(ingress, "response-timeout")
	if len(response) == 0 {
		// Note: due to a misunderstanding the name of the annotation is
		// request timeout, but it is actually applied as a timeout on
		// the response body.
		response = annotation.ContourAnnotation(ingress, "request-timeout")
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
	case LoadBalancerPolicyWeightedLeastRequest, LoadBalancerPolicyRandom, LoadBalancerPolicyCookie, LoadBalancerPolicyRequestHash:
		return lbp.Strategy
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

func rateLimitPolicy(in *contour_api_v1.RateLimitPolicy) (*RateLimitPolicy, error) {
	if in == nil || (in.Local == nil && in.Global == nil) {
		return nil, nil
	}

	rp := &RateLimitPolicy{}

	local, err := localRateLimitPolicy(in.Local)
	if err != nil {
		return nil, err
	}
	rp.Local = local

	global, err := globalRateLimitPolicy(in.Global)
	if err != nil {
		return nil, err
	}
	rp.Global = global

	return rp, nil
}

func localRateLimitPolicy(in *contour_api_v1.LocalRateLimitPolicy) (*LocalRateLimitPolicy, error) {
	if in == nil {
		return nil, nil
	}

	if in.Requests <= 0 {
		return nil, fmt.Errorf("invalid requests value %d in local rate limit policy", in.Requests)
	}

	var fillInterval time.Duration
	switch in.Unit {
	case "second":
		fillInterval = time.Second
	case "minute":
		fillInterval = time.Minute
	case "hour":
		fillInterval = time.Hour
	default:
		return nil, fmt.Errorf("invalid unit %q in local rate limit policy", in.Unit)
	}

	res := &LocalRateLimitPolicy{
		MaxTokens:          in.Requests + in.Burst,
		TokensPerFill:      in.Requests,
		FillInterval:       fillInterval,
		ResponseStatusCode: in.ResponseStatusCode,
	}

	for _, header := range in.ResponseHeadersToAdd {
		// initialize map if we haven't yet
		if res.ResponseHeadersToAdd == nil {
			res.ResponseHeadersToAdd = map[string]string{}
		}

		key := http.CanonicalHeaderKey(header.Name)
		if _, ok := res.ResponseHeadersToAdd[key]; ok {
			return nil, fmt.Errorf("duplicate header addition: %q", key)
		}
		if msgs := validation.IsHTTPHeaderName(key); len(msgs) != 0 {
			return nil, fmt.Errorf("invalid header name %q: %v", key, msgs)
		}
		res.ResponseHeadersToAdd[key] = escapeHeaderValue(header.Value, map[string]string{})
	}

	return res, nil
}

func globalRateLimitPolicy(in *contour_api_v1.GlobalRateLimitPolicy) (*GlobalRateLimitPolicy, error) {
	if in == nil {
		return nil, nil
	}

	res := &GlobalRateLimitPolicy{}

	for _, d := range in.Descriptors {
		var rld RateLimitDescriptor

		for _, entry := range d.Entries {
			// ensure exactly one field is populated on the entry
			var set int

			if entry.GenericKey != nil {
				set++

				rld.Entries = append(rld.Entries, RateLimitDescriptorEntry{
					GenericKey: &GenericKeyDescriptorEntry{
						Key:   entry.GenericKey.Key,
						Value: entry.GenericKey.Value,
					},
				})
			}

			if entry.RequestHeader != nil {
				set++

				rld.Entries = append(rld.Entries, RateLimitDescriptorEntry{
					HeaderMatch: &HeaderMatchDescriptorEntry{
						HeaderName: entry.RequestHeader.HeaderName,
						Key:        entry.RequestHeader.DescriptorKey,
					},
				})
			}

			if entry.RequestHeaderValueMatch != nil {
				set++

				rld.Entries = append(rld.Entries, RateLimitDescriptorEntry{
					HeaderValueMatch: &HeaderValueMatchDescriptorEntry{
						Headers:     headerMatchConditions(entry.RequestHeaderValueMatch.Headers),
						ExpectMatch: entry.RequestHeaderValueMatch.ExpectMatch,
						Value:       entry.RequestHeaderValueMatch.Value,
					},
				})
			}

			if entry.RemoteAddress != nil {
				set++

				rld.Entries = append(rld.Entries, RateLimitDescriptorEntry{
					RemoteAddress: &RemoteAddressDescriptorEntry{},
				})
			}

			if set != 1 {
				return nil, errors.New("rate limit descriptor entry must have exactly one field set")
			}
		}

		res.Descriptors = append(res.Descriptors, &rld)
	}

	return res, nil
}

// Validates and returns list of hash policies along with lb actual strategy to
// be used. Will return default strategy and empty list of hash policies if
// validation fails.
func loadBalancerRequestHashPolicies(lbp *contour_api_v1.LoadBalancerPolicy, validCond *contour_api_v1.DetailedCondition) ([]RequestHashPolicy, string) {
	if lbp == nil {
		return nil, ""
	}
	strategy := loadBalancerPolicy(lbp)
	switch strategy {
	case LoadBalancerPolicyCookie:
		return []RequestHashPolicy{
			{CookieHashOptions: &CookieHashOptions{
				CookieName: "X-Contour-Session-Affinity",
				TTL:        time.Duration(0),
				Path:       "/",
			}},
		}, LoadBalancerPolicyCookie
	case LoadBalancerPolicyRequestHash:
		rhp := []RequestHashPolicy{}
		actualStrategy := strategy
		// Map of unique header names.
		headerHashPolicies := map[string]bool{}
		for _, hashPolicy := range lbp.RequestHashPolicies {
			if hashPolicy.HeaderHashOptions == nil {
				validCond.AddWarningf(contour_api_v1.ConditionTypeSpecError, "IgnoredField",
					"ignoring invalid nil hash policy options")
				continue
			}
			headerName := http.CanonicalHeaderKey(hashPolicy.HeaderHashOptions.HeaderName)
			if msgs := validation.IsHTTPHeaderName(headerName); len(msgs) != 0 {
				validCond.AddWarningf(contour_api_v1.ConditionTypeSpecError, "IgnoredField",
					"ignoring invalid header hash policy options with invalid header name %q: %v", headerName, msgs)
				continue
			}
			if _, ok := headerHashPolicies[headerName]; ok {
				validCond.AddWarningf("SpecError", "IgnoredField",
					"ignoring invalid header hash policy options with duplicated header name %s", headerName)
				continue
			}
			headerHashPolicies[headerName] = true

			rhp = append(rhp, RequestHashPolicy{
				Terminal: hashPolicy.Terminal,
				HeaderHashOptions: &HeaderHashOptions{
					HeaderName: headerName,
				},
			})
		}
		if len(rhp) == 0 {
			validCond.AddWarningf(contour_api_v1.ConditionTypeSpecError, "IgnoredField",
				"ignoring invalid header hash policy options, setting load balancer strategy to default %s", LoadBalancerPolicyRoundRobin)
			rhp = nil
			actualStrategy = LoadBalancerPolicyRoundRobin
		}
		return rhp, actualStrategy
	default:
		return nil, strategy
	}

}
