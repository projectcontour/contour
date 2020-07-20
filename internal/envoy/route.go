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

package envoy

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes/duration"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// RouteMatch creates a *envoy_api_v2_route.RouteMatch for the supplied *dag.Route.
func RouteMatch(route *dag.Route) *envoy_api_v2_route.RouteMatch {
	switch c := route.PathMatchCondition.(type) {
	case *dag.RegexMatchCondition:
		return &envoy_api_v2_route.RouteMatch{
			PathSpecifier: &envoy_api_v2_route.RouteMatch_SafeRegex{
				SafeRegex: SafeRegexMatch(c.Regex),
			},
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	case *dag.PrefixMatchCondition:
		return &envoy_api_v2_route.RouteMatch{
			PathSpecifier: &envoy_api_v2_route.RouteMatch_Prefix{
				Prefix: c.Prefix,
			},
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	default:
		return &envoy_api_v2_route.RouteMatch{
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	}
}

// RouteRoute creates a *envoy_api_v2_route.Route_Route for the services supplied.
// If len(services) is greater than one, the route's action will be a
// weighted cluster.
func RouteRoute(r *dag.Route) *envoy_api_v2_route.Route_Route {
	ra := envoy_api_v2_route.RouteAction{
		RetryPolicy:           retryPolicy(r),
		Timeout:               responseTimeout(r),
		IdleTimeout:           idleTimeout(r),
		PrefixRewrite:         r.PrefixRewrite,
		HashPolicy:            hashPolicy(r),
		RequestMirrorPolicies: mirrorPolicy(r),
	}

	// Check for host header policy and set if found
	if val := hostReplaceHeader(r.RequestHeadersPolicy); val != "" {
		ra.HostRewriteSpecifier = &envoy_api_v2_route.RouteAction_HostRewrite{
			HostRewrite: val,
		}
	}

	if r.Websocket {
		ra.UpgradeConfigs = append(ra.UpgradeConfigs,
			&envoy_api_v2_route.RouteAction_UpgradeConfig{
				UpgradeType: "websocket",
			},
		)
	}

	if singleSimpleCluster(r.Clusters) {
		ra.ClusterSpecifier = &envoy_api_v2_route.RouteAction_Cluster{
			Cluster: Clustername(r.Clusters[0]),
		}
	} else {
		ra.ClusterSpecifier = &envoy_api_v2_route.RouteAction_WeightedClusters{
			WeightedClusters: weightedClusters(r.Clusters),
		}
	}
	return &envoy_api_v2_route.Route_Route{
		Route: &ra,
	}
}

// hashPolicy returns a slice of hash policies iff at least one of the route's
// clusters supplied uses the `Cookie` load balancing strategy.
func hashPolicy(r *dag.Route) []*envoy_api_v2_route.RouteAction_HashPolicy {
	for _, c := range r.Clusters {
		if c.LoadBalancerPolicy == "Cookie" {
			return []*envoy_api_v2_route.RouteAction_HashPolicy{{
				PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
					Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
						Name: "X-Contour-Session-Affinity",
						Ttl:  protobuf.Duration(0),
						Path: "/",
					},
				},
			}}
		}
	}
	return nil
}

func mirrorPolicy(r *dag.Route) []*envoy_api_v2_route.RouteAction_RequestMirrorPolicy {
	if r.MirrorPolicy == nil {
		return nil
	}

	return []*envoy_api_v2_route.RouteAction_RequestMirrorPolicy{{
		Cluster: Clustername(r.MirrorPolicy.Cluster),
	}}
}

func hostReplaceHeader(hp *dag.HeadersPolicy) string {
	if hp == nil {
		return ""
	}
	return hp.HostRewrite
}

func responseTimeout(r *dag.Route) *duration.Duration {
	if r.TimeoutPolicy == nil {
		return nil
	}
	return timeout(r.TimeoutPolicy.ResponseTimeout)
}

func idleTimeout(r *dag.Route) *duration.Duration {
	if r.TimeoutPolicy == nil {
		return nil
	}
	return timeout(r.TimeoutPolicy.IdleTimeout)
}

// timeout interprets a time.Duration with respect to
// Envoy's timeout logic. Zero durations are interpreted
// as nil, therefore remaining unset. Negative durations
// are interpreted as infinity, which is represented as
// an explicit value of 0. Positive durations behave as
// expected.
func timeout(d time.Duration) *duration.Duration {
	switch {
	case d == 0:
		// no timeout specified
		return nil
	case d < 0:
		// infinite timeout, set timeout value to a pointer to zero which tells
		// envoy "infinite timeout"
		return protobuf.Duration(0)
	default:
		return protobuf.Duration(d)
	}
}

func retryPolicy(r *dag.Route) *envoy_api_v2_route.RetryPolicy {
	if r.RetryPolicy == nil {
		return nil
	}
	if r.RetryPolicy.RetryOn == "" {
		return nil
	}

	rp := &envoy_api_v2_route.RetryPolicy{
		RetryOn:              r.RetryPolicy.RetryOn,
		RetriableStatusCodes: r.RetryPolicy.RetriableStatusCodes,
	}
	if r.RetryPolicy.NumRetries > 0 {
		rp.NumRetries = protobuf.UInt32(r.RetryPolicy.NumRetries)
	}
	if r.RetryPolicy.PerTryTimeout > 0 {
		rp.PerTryTimeout = protobuf.Duration(r.RetryPolicy.PerTryTimeout)
	}
	return rp
}

// UpgradeHTTPS returns a route Action that redirects the request to HTTPS.
func UpgradeHTTPS() *envoy_api_v2_route.Route_Redirect {
	return &envoy_api_v2_route.Route_Redirect{
		Redirect: &envoy_api_v2_route.RedirectAction{
			SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}
}

// HeaderValueList creates a list of Envoy HeaderValueOptions from the provided map.
func HeaderValueList(hvm map[string]string, app bool) []*envoy_api_v2_core.HeaderValueOption {
	var hvs []*envoy_api_v2_core.HeaderValueOption

	for key, value := range hvm {
		hvs = append(hvs, &envoy_api_v2_core.HeaderValueOption{
			Header: &envoy_api_v2_core.HeaderValue{
				Key:   key,
				Value: value,
			},
			Append: &wrappers.BoolValue{
				Value: app,
			},
		})
	}

	sort.Slice(hvs, func(i, j int) bool {
		return hvs[i].Header.Key < hvs[j].Header.Key
	})

	return hvs
}

// singleSimpleCluster determines whether we can use a RouteAction_Cluster
// or must use a RouteAction_WeighedCluster to encode additional routing data.
func singleSimpleCluster(clusters []*dag.Cluster) bool {
	// If there are multiple clusters, than we cannot simply dispatch
	// to it by name.
	if len(clusters) != 1 {
		return false
	}
	cluster := clusters[0]

	// If the target cluster performs any kind of header manipulation,
	// then we should use a WeightedCluster to encode the additional
	// configuration.
	if cluster.RequestHeadersPolicy == nil {
		// no request headers policy
	} else if len(cluster.RequestHeadersPolicy.Set) != 0 ||
		len(cluster.RequestHeadersPolicy.Remove) != 0 {
		return false
	}
	if cluster.ResponseHeadersPolicy == nil {
		// no response headers policy
	} else if len(cluster.ResponseHeadersPolicy.Set) != 0 ||
		len(cluster.ResponseHeadersPolicy.Remove) != 0 {
		return false
	}

	return true
}

// weightedClusters returns a route.WeightedCluster for multiple services.
func weightedClusters(clusters []*dag.Cluster) *envoy_api_v2_route.WeightedCluster {
	var wc envoy_api_v2_route.WeightedCluster
	var total uint32
	for _, cluster := range clusters {
		total += cluster.Weight

		c := &envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   Clustername(cluster),
			Weight: protobuf.UInt32(cluster.Weight),
		}
		if cluster.RequestHeadersPolicy != nil {
			c.RequestHeadersToAdd = HeaderValueList(cluster.RequestHeadersPolicy.Set, false)
			c.RequestHeadersToRemove = cluster.RequestHeadersPolicy.Remove
		}
		if cluster.ResponseHeadersPolicy != nil {
			c.ResponseHeadersToAdd = HeaderValueList(cluster.ResponseHeadersPolicy.Set, false)
			c.ResponseHeadersToRemove = cluster.ResponseHeadersPolicy.Remove
		}
		wc.Clusters = append(wc.Clusters, c)
	}
	// Check if no weights were defined, if not default to even distribution
	if total == 0 {
		for _, c := range wc.Clusters {
			c.Weight.Value = 1
		}
		total = uint32(len(clusters))
	}
	wc.TotalWeight = protobuf.UInt32(total)

	sort.Stable(sorter.For(wc.Clusters))
	return &wc
}

// VirtualHost creates a new route.VirtualHost.
func VirtualHost(hostname string, routes ...*envoy_api_v2_route.Route) *envoy_api_v2_route.VirtualHost {
	domains := []string{hostname}
	if hostname != "*" {
		// NOTE(jpeach) see also envoy.FilterMisdirectedRequests().
		domains = append(domains, hostname+":*")
	}

	return &envoy_api_v2_route.VirtualHost{
		Name:    hashname(60, hostname),
		Domains: domains,
		Routes:  routes,
	}
}

// RouteConfiguration returns a *v2.RouteConfiguration.
func RouteConfiguration(name string, virtualhosts ...*envoy_api_v2_route.VirtualHost) *v2.RouteConfiguration {
	return &v2.RouteConfiguration{
		Name:         name,
		VirtualHosts: virtualhosts,
		RequestHeadersToAdd: Headers(
			AppendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
		),
	}
}

func Headers(first *envoy_api_v2_core.HeaderValueOption, rest ...*envoy_api_v2_core.HeaderValueOption) []*envoy_api_v2_core.HeaderValueOption {
	return append([]*envoy_api_v2_core.HeaderValueOption{first}, rest...)
}

func AppendHeader(key, value string) *envoy_api_v2_core.HeaderValueOption {
	return &envoy_api_v2_core.HeaderValueOption{
		Header: &envoy_api_v2_core.HeaderValue{
			Key:   key,
			Value: value,
		},
		Append: protobuf.Bool(true),
	}
}

func headerMatcher(headers []dag.HeaderMatchCondition) []*envoy_api_v2_route.HeaderMatcher {
	var envoyHeaders []*envoy_api_v2_route.HeaderMatcher

	for _, h := range headers {
		header := &envoy_api_v2_route.HeaderMatcher{
			Name:        h.Name,
			InvertMatch: h.Invert,
		}

		switch h.MatchType {
		case "exact":
			header.HeaderMatchSpecifier = &envoy_api_v2_route.HeaderMatcher_ExactMatch{ExactMatch: h.Value}
		case "contains":
			header.HeaderMatchSpecifier = containsMatch(h.Value)
		case "present":
			header.HeaderMatchSpecifier = &envoy_api_v2_route.HeaderMatcher_PresentMatch{PresentMatch: true}
		}
		envoyHeaders = append(envoyHeaders, header)
	}
	return envoyHeaders
}

// containsMatch returns a HeaderMatchSpecifier which will match the
// supplied substring
func containsMatch(s string) *envoy_api_v2_route.HeaderMatcher_SafeRegexMatch {
	// convert the substring s into a regular expression that matches s.
	// note that Envoy expects the expression to match the entire string, not just the substring
	// formed from s. see [projectcontour/contour/#1751 & envoyproxy/envoy#8283]
	regex := fmt.Sprintf(".*%s.*", regexp.QuoteMeta(s))

	return &envoy_api_v2_route.HeaderMatcher_SafeRegexMatch{
		SafeRegexMatch: SafeRegexMatch(regex),
	}
}
