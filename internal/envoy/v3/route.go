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

package v3

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
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
		&envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute_Disabled{
				Disabled: true,
			},
		},
	)
}

// RouteAuthzContext returns a per-route config to pass the given
// context entries in the check request.
func RouteAuthzContext(settings map[string]string) *any.Any {
	return protobuf.MustMarshalAny(
		&envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_config_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: &envoy_config_filter_http_ext_authz_v3.CheckSettings{
					ContextExtensions: settings,
				},
			},
		},
	)
}

const prefixPathMatchSegmentRegex = `((\/).*)?`

var _ = regexp.MustCompile(prefixPathMatchSegmentRegex)

// RouteMatch creates a *envoy_route_v3.RouteMatch for the supplied *dag.Route.
func RouteMatch(route *dag.Route) *envoy_route_v3.RouteMatch {
	switch c := route.PathMatchCondition.(type) {
	case *dag.RegexMatchCondition:
		return &envoy_route_v3.RouteMatch{
			PathSpecifier: &envoy_route_v3.RouteMatch_SafeRegex{
				SafeRegex: SafeRegexMatch(c.Regex),
			},
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	case *dag.PrefixMatchCondition:
		switch c.PrefixMatchType {
		case dag.PrefixMatchSegment:
			return &envoy_route_v3.RouteMatch{
				PathSpecifier: &envoy_route_v3.RouteMatch_SafeRegex{
					SafeRegex: SafeRegexMatch(regexp.QuoteMeta(c.Prefix) + prefixPathMatchSegmentRegex),
				},
				Headers: headerMatcher(route.HeaderMatchConditions),
			}
		case dag.PrefixMatchString:
			fallthrough
		default:
			return &envoy_route_v3.RouteMatch{
				PathSpecifier: &envoy_route_v3.RouteMatch_Prefix{
					Prefix: c.Prefix,
				},
				Headers: headerMatcher(route.HeaderMatchConditions),
			}
		}
	case *dag.ExactMatchCondition:
		return &envoy_route_v3.RouteMatch{
			PathSpecifier: &envoy_route_v3.RouteMatch_Path{
				Path: c.Path,
			},
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	default:
		return &envoy_route_v3.RouteMatch{
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	}
}

// Route_DirectResponse creates a *envoy_route_v3.Route_DirectResponse for the
// http status code supplied. This allows a direct response to a route request
// with an HTTP status code without needing to route to a specific cluster.
func RouteDirectResponse(response *dag.DirectResponse) *envoy_route_v3.Route_DirectResponse {
	return &envoy_route_v3.Route_DirectResponse{
		DirectResponse: &envoy_route_v3.DirectResponseAction{
			Status: response.StatusCode,
		},
	}
}

// RouteRoute creates a *envoy_route_v3.Route_Route for the services supplied.
// If len(services) is greater than one, the route's action will be a
// weighted cluster.
func RouteRoute(r *dag.Route) *envoy_route_v3.Route_Route {
	ra := envoy_route_v3.RouteAction{
		RetryPolicy:           retryPolicy(r),
		Timeout:               envoy.Timeout(r.TimeoutPolicy.ResponseTimeout),
		IdleTimeout:           envoy.Timeout(r.TimeoutPolicy.IdleTimeout),
		PrefixRewrite:         r.PrefixRewrite,
		HashPolicy:            hashPolicy(r.RequestHashPolicies),
		RequestMirrorPolicies: mirrorPolicy(r),
	}

	if r.RateLimitPolicy != nil && r.RateLimitPolicy.Global != nil {
		ra.RateLimits = GlobalRateLimits(r.RateLimitPolicy.Global.Descriptors)
	}

	// Check for host header policy and set if found
	if val := envoy.HostReplaceHeader(r.RequestHeadersPolicy); val != "" {
		ra.HostRewriteSpecifier = &envoy_route_v3.RouteAction_HostRewriteLiteral{
			HostRewriteLiteral: val,
		}
	}

	if r.Websocket {
		ra.UpgradeConfigs = append(ra.UpgradeConfigs,
			&envoy_route_v3.RouteAction_UpgradeConfig{
				UpgradeType: "websocket",
			},
		)
	}

	if envoy.SingleSimpleCluster(r.Clusters) {
		ra.ClusterSpecifier = &envoy_route_v3.RouteAction_Cluster{
			Cluster: envoy.Clustername(r.Clusters[0]),
		}
	} else {
		ra.ClusterSpecifier = &envoy_route_v3.RouteAction_WeightedClusters{
			WeightedClusters: weightedClusters(r.Clusters),
		}
	}
	return &envoy_route_v3.Route_Route{
		Route: &ra,
	}
}

// hashPolicy returns a slice of Envoy hash policies from the passed in Contour
// request hash policy configuration. Only one of header or cookie hash policies
// should be set on any RequestHashPolicy element.
func hashPolicy(requestHashPolicies []dag.RequestHashPolicy) []*envoy_route_v3.RouteAction_HashPolicy {
	if len(requestHashPolicies) == 0 {
		return nil
	}
	hashPolicies := []*envoy_route_v3.RouteAction_HashPolicy{}
	for _, rhp := range requestHashPolicies {
		newHP := &envoy_route_v3.RouteAction_HashPolicy{
			Terminal: rhp.Terminal,
		}
		if rhp.HeaderHashOptions != nil {
			newHP.PolicySpecifier = &envoy_route_v3.RouteAction_HashPolicy_Header_{
				Header: &envoy_route_v3.RouteAction_HashPolicy_Header{
					HeaderName: rhp.HeaderHashOptions.HeaderName,
				},
			}
		}
		if rhp.CookieHashOptions != nil {
			newHP.PolicySpecifier = &envoy_route_v3.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoy_route_v3.RouteAction_HashPolicy_Cookie{
					Name: rhp.CookieHashOptions.CookieName,
					Ttl:  protobuf.Duration(rhp.CookieHashOptions.TTL),
					Path: rhp.CookieHashOptions.Path,
				},
			}
		}
		hashPolicies = append(hashPolicies, newHP)
	}
	return hashPolicies
}

func mirrorPolicy(r *dag.Route) []*envoy_route_v3.RouteAction_RequestMirrorPolicy {
	if r.MirrorPolicy == nil {
		return nil
	}

	return []*envoy_route_v3.RouteAction_RequestMirrorPolicy{{
		Cluster: envoy.Clustername(r.MirrorPolicy.Cluster),
	}}
}

func retryPolicy(r *dag.Route) *envoy_route_v3.RetryPolicy {
	if r.RetryPolicy == nil {
		return nil
	}
	if r.RetryPolicy.RetryOn == "" {
		return nil
	}

	rp := &envoy_route_v3.RetryPolicy{
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
func UpgradeHTTPS() *envoy_route_v3.Route_Redirect {
	return &envoy_route_v3.Route_Redirect{
		Redirect: &envoy_route_v3.RedirectAction{
			SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}
}

// HeaderValueList creates a list of Envoy HeaderValueOptions from the provided map.
func HeaderValueList(hvm map[string]string, app bool) []*envoy_core_v3.HeaderValueOption {
	var hvs []*envoy_core_v3.HeaderValueOption

	for key, value := range hvm {
		hvs = append(hvs, &envoy_core_v3.HeaderValueOption{
			Header: &envoy_core_v3.HeaderValue{
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
func weightedClusters(clusters []*dag.Cluster) *envoy_route_v3.WeightedCluster {
	var wc envoy_route_v3.WeightedCluster
	var total uint32
	for _, cluster := range clusters {
		total += cluster.Weight

		c := &envoy_route_v3.WeightedCluster_ClusterWeight{
			Name:   envoy.Clustername(cluster),
			Weight: protobuf.UInt32(cluster.Weight),
		}
		if cluster.RequestHeadersPolicy != nil {
			c.RequestHeadersToAdd = append(HeaderValueList(cluster.RequestHeadersPolicy.Set, false), HeaderValueList(cluster.RequestHeadersPolicy.Add, true)...)
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
func VirtualHost(hostname string, routes ...*envoy_route_v3.Route) *envoy_route_v3.VirtualHost {
	return &envoy_route_v3.VirtualHost{
		Name:    envoy.Hashname(60, hostname),
		Domains: []string{hostname},
		Routes:  routes,
	}
}

// CORSVirtualHost creates a new route.VirtualHost with a CORS policy.
func CORSVirtualHost(hostname string, corspolicy *envoy_route_v3.CorsPolicy, routes ...*envoy_route_v3.Route) *envoy_route_v3.VirtualHost {
	vh := VirtualHost(hostname, routes...)
	vh.Cors = corspolicy
	return vh
}

// RouteConfiguration returns a *envoy_route_v3.RouteConfiguration.
func RouteConfiguration(name string, virtualhosts ...*envoy_route_v3.VirtualHost) *envoy_route_v3.RouteConfiguration {
	return &envoy_route_v3.RouteConfiguration{
		Name:         name,
		VirtualHosts: virtualhosts,
		RequestHeadersToAdd: Headers(
			AppendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
		),
	}
}

// CORSPolicy returns a *envoy_route_v3.CORSPolicy
func CORSPolicy(cp *dag.CORSPolicy) *envoy_route_v3.CorsPolicy {
	if cp == nil {
		return nil
	}
	rcp := &envoy_route_v3.CorsPolicy{
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

func Headers(first *envoy_core_v3.HeaderValueOption, rest ...*envoy_core_v3.HeaderValueOption) []*envoy_core_v3.HeaderValueOption {
	return append([]*envoy_core_v3.HeaderValueOption{first}, rest...)
}

func AppendHeader(key, value string) *envoy_core_v3.HeaderValueOption {
	return &envoy_core_v3.HeaderValueOption{
		Header: &envoy_core_v3.HeaderValue{
			Key:   key,
			Value: value,
		},
		Append: protobuf.Bool(true),
	}
}

func headerMatcher(headers []dag.HeaderMatchCondition) []*envoy_route_v3.HeaderMatcher {
	var envoyHeaders []*envoy_route_v3.HeaderMatcher

	for _, h := range headers {
		header := &envoy_route_v3.HeaderMatcher{
			Name:        h.Name,
			InvertMatch: h.Invert,
		}

		switch h.MatchType {
		case dag.HeaderMatchTypeExact:
			header.HeaderMatchSpecifier = &envoy_route_v3.HeaderMatcher_ExactMatch{ExactMatch: h.Value}
		case dag.HeaderMatchTypeContains:
			header.HeaderMatchSpecifier = containsMatch(h.Value)
		case dag.HeaderMatchTypePresent:
			header.HeaderMatchSpecifier = &envoy_route_v3.HeaderMatcher_PresentMatch{PresentMatch: true}
		case dag.HeaderMatchTypeRegex:
			header.HeaderMatchSpecifier = &envoy_route_v3.HeaderMatcher_SafeRegexMatch{
				SafeRegexMatch: SafeRegexMatch(h.Value),
			}
		}
		envoyHeaders = append(envoyHeaders, header)
	}
	return envoyHeaders
}

// containsMatch returns a HeaderMatchSpecifier which will match the
// supplied substring
func containsMatch(s string) *envoy_route_v3.HeaderMatcher_SafeRegexMatch {
	// convert the substring s into a regular expression that matches s.
	// note that Envoy expects the expression to match the entire string, not just the substring
	// formed from s. see [projectcontour/contour/#1751 & envoyproxy/envoy#8283]
	regex := fmt.Sprintf(".*%s.*", regexp.QuoteMeta(s))

	return &envoy_route_v3.HeaderMatcher_SafeRegexMatch{
		SafeRegexMatch: SafeRegexMatch(regex),
	}
}
