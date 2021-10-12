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
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	lua "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
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

	if envoy.SingleSimpleCluster(r) {
		ra.ClusterSpecifier = &envoy_route_v3.RouteAction_Cluster{
			Cluster: envoy.Clustername(r.Clusters[0]),
		}
	} else {
		ra.ClusterSpecifier = &envoy_route_v3.RouteAction_WeightedClusters{
			WeightedClusters: weightedClusters(r),
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
func weightedClusters(route *dag.Route) *envoy_route_v3.WeightedCluster {
	var wc envoy_route_v3.WeightedCluster
	var total uint32
	for _, cluster := range route.Clusters {
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
		if len(route.CookieRewritePolicies) > 0 || len(cluster.CookieRewritePolicies) > 0 {
			if c.TypedPerFilterConfig == nil {
				c.TypedPerFilterConfig = map[string]*any.Any{}
			}
			c.TypedPerFilterConfig["envoy.filters.http.lua"] = CookieRewriteConfig(route.CookieRewritePolicies, cluster.CookieRewritePolicies)
		}
		wc.Clusters = append(wc.Clusters, c)
	}
	// Check if no weights were defined, if not default to even distribution
	if total == 0 {
		for _, c := range wc.Clusters {
			c.Weight.Value = 1
		}
		total = uint32(len(route.Clusters))
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

func CookieRewriteConfig(routePolicies, clusterPolicies []dag.CookieRewritePolicy) *any.Any {
	// Merge route and cluster policies
	mergedPolicies := map[string]dag.CookieRewritePolicy{}
	for _, p := range append(routePolicies, clusterPolicies...) {
		if _, ok := mergedPolicies[p.Name]; !ok {
			mergedPolicies[p.Name] = p
		} else {
			merged := mergedPolicies[p.Name]
			// Merge this policy with an existing one.
			if p.Path != nil {
				merged.Path = p.Path
			}
			if p.Domain != nil {
				merged.Domain = p.Domain
			}
			if p.Secure != 0 {
				merged.Secure = p.Secure
			}
			if p.SameSite != nil {
				merged.SameSite = p.SameSite
			}
			mergedPolicies[p.Name] = merged
		}
	}
	policies := make([]dag.CookieRewritePolicy, len(mergedPolicies))
	i := 0
	for _, p := range mergedPolicies {
		policies[i] = p
		i++
	}

	codeTemplate := `
function envoy_on_response(response_handle)
	rewrite_table = {}

	{{range $i, $p := .}}
	function cookie_{{$i}}_attribute_rewrite(attributes)
		response_handle:logDebug("rewriting cookie \"{{$p.Name}}\"")

		{{if $p.Path}}attributes["Path"] = "Path={{$p.Path}}"{{end}}
		{{if $p.Domain}}attributes["Domain"] = "Domain={{$p.Domain}}"{{end}}
		{{if $p.SameSite}}attributes["SameSite"] = "SameSite={{$p.SameSite}}"{{end}}
		{{if eq $p.Secure 1}}attributes["Secure"] = nil{{end}}
		{{if eq $p.Secure 2}}attributes["Secure"] = "Secure"{{end}}
	end
	rewrite_table["{{$p.Name}}"] = cookie_{{$i}}_attribute_rewrite
	{{end}}

	function rewrite_cookie(original)
		local original_len = string.len(original)
		local name_end = string.find(original, "=")
		if name_end == nil then
			return original
		end
		local name = string.sub(original, 1, name_end - 1)

		local rewrite_func = rewrite_table[name]
		-- We don't have a rewrite rule for this cookie.
		if rewrite_func == nil then
			return original
		end

		-- Find cookie value via ; or end of string
		local value_end = string.find(original, ";", name_end)
		-- Save position to use as iterator
		local iter = value_end
		if value_end == nil then
			-- Set to 0 since we have to subtract below if we did find a ;
			value_end = 0
			iter = original_len
		end
		iter = iter + 1
		local value = string.sub(original, name_end + 1, value_end - 1)

		-- Parse original attributes into table
		-- Keyed by attribute name, values are <name>=<value> or just <name>
		-- so we can easily rebuild, esp for attributes like 'Secure' that
		-- do not have a value
		local attributes = {}
		while iter < original_len do
			local attr_end = string.find(original, ";", iter)
			local new_iter = attr_end
			if attr_end == nil then
				-- Set to 0 since we have to subtract below if we did find a ;
				attr_end = 0
				new_iter = original_len
			end
			local attr_value = string.sub(original, iter + 1, attr_end - 1)
			-- Strip whitespace from front
			attr_value = string.gsub(attr_value, "^%s*(.-)$", "%1")

			-- Get attribute name
			local attr_name_end = string.find(attr_value, "=")
			if attr_name_end == nil then
				-- Set to 0 since we have to subtract below if we did find a =
				attr_name_end = 0
			end
			local attr_name = string.sub(attr_value, 1, attr_name_end - 1)

			attributes[attr_name] = attr_value

			iter = new_iter + 1
		end

		rewrite_func(attributes)

		local rewritten = string.format("%s=%s", name, value)
		for k, v in next, attributes do
			if v then
				rewritten = string.format("%s; %s", rewritten, v)
			end
		end

		return rewritten
	end

	if response_handle:headers():get("set-cookie") then
		rewritten_cookies = {}
		for k, v in pairs(response_handle:headers()) do
			if k == "set-cookie" then
				table.insert(rewritten_cookies, rewrite_cookie(v))
			end
		end

		response_handle:headers():remove("set-cookie")
		for k, v in next, rewritten_cookies do
			response_handle:headers():add("set-cookie", v)
		end
	end
end
	`

	t := new(bytes.Buffer)
	if err := template.Must(template.New("code").Parse(codeTemplate)).Execute(t, policies); err != nil {
		// If template execution fails, return empty filter.
		return nil
	}

	c := &lua.LuaPerRoute{
		Override: &lua.LuaPerRoute_SourceCode{
			SourceCode: &envoy_core_v3.DataSource{
				Specifier: &envoy_core_v3.DataSource_InlineString{
					InlineString: t.String(),
				},
			},
		},
	}
	return protobuf.MustMarshalAny(c)
}
