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

package v2

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_config_filter_http_ext_authz_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/golang/protobuf/ptypes/any"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// RouteAuthzDisabled returns a per-route config to disable authorization.
func RouteAuthzDisabled() *any.Any {
	return protobuf.MustMarshalAny(
		&envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute{
			Override: &envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute_Disabled{
				Disabled: true,
			},
		},
	)
}

// RouteAuthzContext returns a per-route config to pass the given
// context entries in the check request.
func RouteAuthzContext(settings map[string]string) *any.Any {
	return protobuf.MustMarshalAny(
		&envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute{
			Override: &envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: &envoy_config_filter_http_ext_authz_v2.CheckSettings{
					ContextExtensions: settings,
				},
			},
		},
	)
}

// RouteMatch creates a *envoy_api_v2_route.RouteMatch for the supplied *dag.Route.
func RouteMatch(route *dag.Route) *envoy_api_v2_route.RouteMatch {
	switch c := route.PathMatchCondition.(type) {
	case *dag.RegexMatchCondition:
		return &envoy_api_v2_route.RouteMatch{
			PathSpecifier: &envoy_api_v2_route.RouteMatch_SafeRegex{
				SafeRegex: envoy.SafeRegexMatch(c.Regex),
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
		Timeout:               envoy.Timeout(r.TimeoutPolicy.ResponseTimeout),
		IdleTimeout:           envoy.Timeout(r.TimeoutPolicy.IdleTimeout),
		PrefixRewrite:         r.PrefixRewrite,
		HashPolicy:            hashPolicy(r),
		RequestMirrorPolicies: mirrorPolicy(r),
	}

	// Check for host header policy and set if found
	if val := envoy.HostReplaceHeader(r.RequestHeadersPolicy); val != "" {
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

	if envoy.SingleSimpleCluster(r.Clusters) {
		ra.ClusterSpecifier = &envoy_api_v2_route.RouteAction_Cluster{
			Cluster: envoy.Clustername(r.Clusters[0]),
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
		Cluster: envoy.Clustername(r.MirrorPolicy.Cluster),
	}}
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
	rp.PerTryTimeout = envoy.Timeout(r.RetryPolicy.PerTryTimeout)

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

// weightedClusters returns a route.WeightedCluster for multiple services.
func weightedClusters(clusters []*dag.Cluster) *envoy_api_v2_route.WeightedCluster {
	var wc envoy_api_v2_route.WeightedCluster
	var total uint32
	for _, cluster := range clusters {
		total += cluster.Weight

		c := &envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   envoy.Clustername(cluster),
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
		Name:    envoy.Hashname(60, hostname),
		Domains: domains,
		Routes:  routes,
	}
}

// CORSVirtualHost creates a new route.VirtualHost with a CORS policy.
func CORSVirtualHost(hostname string, corspolicy *envoy_api_v2_route.CorsPolicy, routes ...*envoy_api_v2_route.Route) *envoy_api_v2_route.VirtualHost {
	vh := VirtualHost(hostname, routes...)
	vh.Cors = corspolicy
	return vh
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

// CORSPolicy returns a *v2.CORSPolicy
func CORSPolicy(cp *dag.CORSPolicy) *envoy_api_v2_route.CorsPolicy {
	if cp == nil {
		return nil
	}
	rcp := &envoy_api_v2_route.CorsPolicy{
		AllowCredentials: protobuf.Bool(cp.AllowCredentials),
		AllowHeaders:     strings.Join(cp.AllowHeaders, ","),
		AllowMethods:     strings.Join(cp.AllowMethods, ","),
		ExposeHeaders:    strings.Join(cp.ExposeHeaders, ","),
	}

	if cp.MaxAge.IsDisabled() {
		rcp.MaxAge = "0"
	} else if !cp.MaxAge.UseDefault() {
		rcp.MaxAge = fmt.Sprintf("%.0f", cp.MaxAge.Duration().Seconds())
	}

	rcp.AllowOriginStringMatch = []*matcher.StringMatcher{}
	for _, ao := range cp.AllowOrigin {
		rcp.AllowOriginStringMatch = append(rcp.AllowOriginStringMatch, &matcher.StringMatcher{
			// Even though we use the exact matcher, Envoy always makes an exception for the `*` value
			// https://github.com/envoyproxy/envoy/blob/d6e2fd0185ca620745479da2c43c0564eeaf35c5/source/extensions/filters/http/cors/cors_filter.cc#L142
			MatchPattern: &matcher.StringMatcher_Exact{
				Exact: ao,
			},
			IgnoreCase: true,
		})
	}
	return rcp
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
		SafeRegexMatch: envoy.SafeRegexMatch(regex),
	}
}
