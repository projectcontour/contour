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
	"net/http"
	"sort"
	"strings"
	"text/template"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_cors_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_filter_http_jwt_authn_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoy_filter_http_lua_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	envoy_filter_http_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	envoy_internal_redirect_previous_routes_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/internal_redirect/previous_routes/v3"
	envoy_internal_redirect_safe_cross_scheme_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/internal_redirect/safe_cross_scheme/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// VirtualHostAndRoutes converts a DAG virtual host and routes to an Envoy virtual host.
func VirtualHostAndRoutes(vh *dag.VirtualHost, dagRoutes []*dag.Route, secure bool) *envoy_config_route_v3.VirtualHost {
	var envoyRoutes []*envoy_config_route_v3.Route
	for _, route := range dagRoutes {
		envoyRoutes = append(envoyRoutes, buildRoute(route, vh.Name, secure))
	}

	evh := VirtualHost(vh.Name, envoyRoutes...)

	if vh.CORSPolicy != nil {
		if evh.TypedPerFilterConfig == nil {
			evh.TypedPerFilterConfig = map[string]*anypb.Any{}
		}
		evh.TypedPerFilterConfig[CORSFilterName] = protobuf.MustMarshalAny(corsPolicy(vh.CORSPolicy))
	}
	if vh.RateLimitPolicy != nil && vh.RateLimitPolicy.Local != nil {
		if evh.TypedPerFilterConfig == nil {
			evh.TypedPerFilterConfig = map[string]*anypb.Any{}
		}
		evh.TypedPerFilterConfig[LocalRateLimitFilterName] = LocalRateLimitConfig(vh.RateLimitPolicy.Local, "vhost."+vh.Name)
	}

	if vh.RateLimitPolicy != nil && vh.RateLimitPolicy.Global != nil {
		evh.RateLimits = GlobalRateLimits(vh.RateLimitPolicy.Global.Descriptors)
	}

	if len(vh.IPFilterRules) > 0 {
		if evh.TypedPerFilterConfig == nil {
			evh.TypedPerFilterConfig = map[string]*anypb.Any{}
		}
		evh.TypedPerFilterConfig[RBACFilterName] = protobuf.MustMarshalAny(
			ipFilterConfig(vh.IPFilterAllow, vh.IPFilterRules),
		)
	}

	return evh
}

func getRouteMetadata(dagRoute *dag.Route) *envoy_config_core_v3.Metadata {
	metadataFields := map[string]*structpb.Value{}
	if len(dagRoute.Kind) > 0 {
		metadataFields["io.projectcontour.kind"] = structpb.NewStringValue(dagRoute.Kind)
	}
	if len(dagRoute.Namespace) > 0 {
		metadataFields["io.projectcontour.namespace"] = structpb.NewStringValue(dagRoute.Namespace)
	}
	if len(dagRoute.Name) > 0 {
		metadataFields["io.projectcontour.name"] = structpb.NewStringValue(dagRoute.Name)
	}

	if len(metadataFields) == 0 {
		return nil
	}

	return &envoy_config_core_v3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{
			"envoy.access_loggers.file": {
				Fields: metadataFields,
			},
		},
	}
}

// buildRoute converts a DAG route to an Envoy route.
func buildRoute(dagRoute *dag.Route, vhostName string, secure bool) *envoy_config_route_v3.Route {
	route := &envoy_config_route_v3.Route{
		Match:    RouteMatch(dagRoute),
		Metadata: getRouteMetadata(dagRoute),
	}

	switch {
	case dagRoute.HTTPSUpgrade && !secure:
		// TODO(dfc) if we ensure the builder never returns a dag.Route connected
		// to a SecureVirtualHost that requires upgrade, this logic can move to
		// envoy.RouteRoute. Currently the DAG processor adds any HTTP->HTTPS
		// redirect routes to *both* the insecure and secure vhosts.
		route.Action = UpgradeHTTPS()
	case dagRoute.DirectResponse != nil:
		route.Action = routeDirectResponse(dagRoute.DirectResponse)
	case dagRoute.Redirect != nil:
		// TODO request/response headers?
		route.Action = routeRedirect(dagRoute.Redirect)
	default:
		route.Action = routeRoute(dagRoute)

		if dagRoute.RequestHeadersPolicy != nil {
			route.RequestHeadersToAdd = append(headerValueList(dagRoute.RequestHeadersPolicy.Set, false), headerValueList(dagRoute.RequestHeadersPolicy.Add, true)...)
			route.RequestHeadersToRemove = dagRoute.RequestHeadersPolicy.Remove
		}
		if dagRoute.ResponseHeadersPolicy != nil {
			route.ResponseHeadersToAdd = append(headerValueList(dagRoute.ResponseHeadersPolicy.Set, false), headerValueList(dagRoute.ResponseHeadersPolicy.Add, true)...)
			route.ResponseHeadersToRemove = dagRoute.ResponseHeadersPolicy.Remove
		}

		route.TypedPerFilterConfig = map[string]*anypb.Any{}

		// Apply per-route local rate limit policy.
		if dagRoute.RateLimitPolicy != nil && dagRoute.RateLimitPolicy.Local != nil {
			route.TypedPerFilterConfig[LocalRateLimitFilterName] = LocalRateLimitConfig(dagRoute.RateLimitPolicy.Local, "vhost."+vhostName)
		}

		if dagRoute.RateLimitPerRoute != nil {
			route.TypedPerFilterConfig[GlobalRateLimitFilterName] = rateLimitPerRoute(dagRoute.RateLimitPerRoute)
		}

		// Apply per-route authorization policy modifications.
		if dagRoute.AuthDisabled {
			route.TypedPerFilterConfig[ExtAuthzFilterName] = routeAuthzDisabled()
		} else if len(dagRoute.AuthContext) > 0 {
			route.TypedPerFilterConfig[ExtAuthzFilterName] = routeAuthzContext(dagRoute.AuthContext)
		}

		// If JWT verification is enabled, add per-route filter
		// config referencing a requirement in the main filter
		// config.
		if len(dagRoute.JWTProvider) > 0 {
			route.TypedPerFilterConfig[JWTAuthnFilterName] = protobuf.MustMarshalAny(&envoy_filter_http_jwt_authn_v3.PerRouteConfig{
				RequirementSpecifier: &envoy_filter_http_jwt_authn_v3.PerRouteConfig_RequirementName{RequirementName: dagRoute.JWTProvider},
			})
		}

		// If IP filtering is enabled, add per-route filtering
		if len(dagRoute.IPFilterRules) > 0 {
			route.TypedPerFilterConfig[RBACFilterName] = protobuf.MustMarshalAny(
				ipFilterConfig(dagRoute.IPFilterAllow, dagRoute.IPFilterRules),
			)
		}

		// Remove empty map if no per-filter config was added.
		if len(route.TypedPerFilterConfig) == 0 {
			route.TypedPerFilterConfig = nil
		}
	}

	return route
}

// routeAuthzDisabled returns a per-route config to disable authorization.
func routeAuthzDisabled() *anypb.Any {
	return protobuf.MustMarshalAny(
		&envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute_Disabled{
				Disabled: true,
			},
		},
	)
}

// routeAuthzContext returns a per-route config to pass the given
// context entries in the check request.
func routeAuthzContext(settings map[string]string) *anypb.Any {
	return protobuf.MustMarshalAny(
		&envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_filter_http_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: &envoy_filter_http_ext_authz_v3.CheckSettings{
					ContextExtensions: settings,
				},
			},
		},
	)
}

func ipFilterConfig(allow bool, rules []dag.IPFilterRule) *envoy_filter_http_rbac_v3.RBACPerRoute {
	action := envoy_config_rbac_v3.RBAC_ALLOW
	if !allow {
		action = envoy_config_rbac_v3.RBAC_DENY
	}

	principals := make([]*envoy_config_rbac_v3.Principal, 0, len(rules))

	for _, f := range rules {
		var principal *envoy_config_rbac_v3.Principal

		prefixLen, _ := f.CIDR.Mask.Size()
		cidr := &envoy_config_core_v3.CidrRange{
			AddressPrefix: f.CIDR.IP.String(),
			PrefixLen:     wrapperspb.UInt32(uint32(prefixLen)),
		}

		if f.Remote {
			principal = &envoy_config_rbac_v3.Principal{
				Identifier: &envoy_config_rbac_v3.Principal_RemoteIp{
					RemoteIp: cidr,
				},
			}
		} else {
			principal = &envoy_config_rbac_v3.Principal{
				Identifier: &envoy_config_rbac_v3.Principal_DirectRemoteIp{
					DirectRemoteIp: cidr,
				},
			}
		}
		// Note that `source_ip` is not supported: `source_ip` respects
		// PROXY, but not X-Forwarded-For.
		principals = append(principals, principal)
	}

	return &envoy_filter_http_rbac_v3.RBACPerRoute{
		Rbac: &envoy_filter_http_rbac_v3.RBAC{
			Rules: &envoy_config_rbac_v3.RBAC{
				Action: action,
				Policies: map[string]*envoy_config_rbac_v3.Policy{
					"ip-rules": {
						Permissions: []*envoy_config_rbac_v3.Permission{
							{
								Rule: &envoy_config_rbac_v3.Permission_Any{Any: true},
							},
						},
						Principals: principals,
					},
				},
			},
		},
	}
}

// RouteMatch creates a *envoy_config_route_v3.RouteMatch for the supplied *dag.Route.
func RouteMatch(route *dag.Route) *envoy_config_route_v3.RouteMatch {
	routeMatch := PathRouteMatch(route.PathMatchCondition)

	routeMatch.Headers = headerMatcher(route.HeaderMatchConditions)
	routeMatch.QueryParameters = queryParamMatcher(route.QueryParamMatchConditions)

	return routeMatch
}

// PathRouteMatch creates a *envoy_config_route_v3.RouteMatch with *only* a PathSpecifier
// populated.
func PathRouteMatch(pathMatchCondition dag.MatchCondition) *envoy_config_route_v3.RouteMatch {
	switch c := pathMatchCondition.(type) {
	case *dag.RegexMatchCondition:
		return &envoy_config_route_v3.RouteMatch{
			PathSpecifier: &envoy_config_route_v3.RouteMatch_SafeRegex{
				// Add an anchor since we at the very least have a / as a string literal prefix.
				// Reduces regex program size so Envoy doesn't reject long prefix matches.
				SafeRegex: SafeRegexMatch("^" + c.Regex),
			},
		}
	case *dag.PrefixMatchCondition:
		switch c.PrefixMatchType {
		case dag.PrefixMatchSegment:
			return &envoy_config_route_v3.RouteMatch{
				PathSpecifier: &envoy_config_route_v3.RouteMatch_PathSeparatedPrefix{
					// Trim trailing slash as PathSeparatedPrefix expects
					// no trailing slashes.
					PathSeparatedPrefix: strings.TrimRight(c.Prefix, "/"),
				},
			}
		case dag.PrefixMatchString:
			fallthrough
		default:
			return &envoy_config_route_v3.RouteMatch{
				PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
					Prefix: c.Prefix,
				},
			}
		}
	case *dag.ExactMatchCondition:
		return &envoy_config_route_v3.RouteMatch{
			PathSpecifier: &envoy_config_route_v3.RouteMatch_Path{
				Path: c.Path,
			},
		}
	default:
		return &envoy_config_route_v3.RouteMatch{}
	}
}

// routeDirectResponse creates a *envoy_config_route_v3.Route_DirectResponse for the
// http status code and body supplied. This allows a direct response to a route request
// with an HTTP status code without needing to route to a specific cluster.
func routeDirectResponse(response *dag.DirectResponse) *envoy_config_route_v3.Route_DirectResponse {
	r := &envoy_config_route_v3.Route_DirectResponse{
		DirectResponse: &envoy_config_route_v3.DirectResponseAction{
			Status: response.StatusCode,
		},
	}
	if response.Body != "" {
		r.DirectResponse.Body = &envoy_config_core_v3.DataSource{
			Specifier: &envoy_config_core_v3.DataSource_InlineString{
				InlineString: response.Body,
			},
		}
	}
	return r
}

// routeRedirect creates a *envoy_config_route_v3.Route_Redirect for the
// redirect specified. This allows a redirect to be returned to the
// client.
func routeRedirect(redirect *dag.Redirect) *envoy_config_route_v3.Route_Redirect {
	r := &envoy_config_route_v3.Route_Redirect{
		Redirect: &envoy_config_route_v3.RedirectAction{},
	}

	if len(redirect.Hostname) > 0 {
		r.Redirect.HostRedirect = redirect.Hostname
	}

	if len(redirect.Scheme) > 0 {
		r.Redirect.SchemeRewriteSpecifier = &envoy_config_route_v3.RedirectAction_SchemeRedirect{
			SchemeRedirect: redirect.Scheme,
		}
	}

	if redirect.PortNumber > 0 {
		r.Redirect.PortRedirect = redirect.PortNumber
	}

	if redirect.PathRewritePolicy != nil {
		switch {
		case len(redirect.PathRewritePolicy.FullPathRewrite) > 0:
			r.Redirect.PathRewriteSpecifier = &envoy_config_route_v3.RedirectAction_PathRedirect{
				PathRedirect: redirect.PathRewritePolicy.FullPathRewrite,
			}
		case len(redirect.PathRewritePolicy.PrefixRewrite) > 0:
			r.Redirect.PathRewriteSpecifier = &envoy_config_route_v3.RedirectAction_PrefixRewrite{
				PrefixRewrite: redirect.PathRewritePolicy.PrefixRewrite,
			}
		case len(redirect.PathRewritePolicy.PrefixRegexRemove) > 0:
			r.Redirect.PathRewriteSpecifier = &envoy_config_route_v3.RedirectAction_RegexRewrite{
				RegexRewrite: &envoy_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      SafeRegexMatch(redirect.PathRewritePolicy.PrefixRegexRemove),
					Substitution: "/",
				},
			}
		}
	}

	// Envoy's default is a 301 if not otherwise specified.
	switch redirect.StatusCode {
	case http.StatusMovedPermanently:
		r.Redirect.ResponseCode = envoy_config_route_v3.RedirectAction_MOVED_PERMANENTLY
	case http.StatusFound:
		r.Redirect.ResponseCode = envoy_config_route_v3.RedirectAction_FOUND
	}

	return r
}

// routeRoute creates a *envoy_config_route_v3.Route_Route for the services supplied.
// If len(services) is greater than one, the route's action will be a
// weighted cluster.
func routeRoute(r *dag.Route) *envoy_config_route_v3.Route_Route {
	ra := envoy_config_route_v3.RouteAction{
		RetryPolicy:            retryPolicy(r),
		Timeout:                envoy.Timeout(r.TimeoutPolicy.ResponseTimeout),
		IdleTimeout:            envoy.Timeout(r.TimeoutPolicy.IdleStreamTimeout),
		HashPolicy:             hashPolicy(r.RequestHashPolicies),
		RequestMirrorPolicies:  mirrorPolicy(r),
		InternalRedirectPolicy: internalRedirectPolicy(r.InternalRedirectPolicy),
	}

	if r.PathRewritePolicy != nil {
		switch {
		case len(r.PathRewritePolicy.PrefixRewrite) > 0:
			ra.PrefixRewrite = r.PathRewritePolicy.PrefixRewrite
		case len(r.PathRewritePolicy.FullPathRewrite) > 0:
			ra.RegexRewrite = &envoy_matcher_v3.RegexMatchAndSubstitute{
				Pattern:      SafeRegexMatch("^/.*$"), // match the entire path
				Substitution: r.PathRewritePolicy.FullPathRewrite,
			}
		case len(r.PathRewritePolicy.PrefixRegexRemove) > 0:
			ra.RegexRewrite = &envoy_matcher_v3.RegexMatchAndSubstitute{
				Pattern:      SafeRegexMatch(r.PathRewritePolicy.PrefixRegexRemove),
				Substitution: "/",
			}
		}
	}

	if r.RateLimitPolicy != nil && r.RateLimitPolicy.Global != nil {
		ra.RateLimits = GlobalRateLimits(r.RateLimitPolicy.Global.Descriptors)
	}

	// Check for host header policy and set if found
	if val := envoy.HostRewriteLiteral(r.RequestHeadersPolicy); val != "" {
		ra.HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_HostRewriteLiteral{
			HostRewriteLiteral: val,
		}
	} else if val := envoy.HostRewriteHeader(r.RequestHeadersPolicy); val != "" {
		ra.HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_HostRewriteHeader{
			HostRewriteHeader: val,
		}
	}

	if r.Websocket {
		ra.UpgradeConfigs = append(ra.UpgradeConfigs,
			&envoy_config_route_v3.RouteAction_UpgradeConfig{
				UpgradeType: "websocket",
			},
		)
	}

	if envoy.SingleSimpleCluster(r) {
		ra.ClusterSpecifier = &envoy_config_route_v3.RouteAction_Cluster{
			Cluster: envoy.Clustername(r.Clusters[0]),
		}
	} else {
		ra.ClusterSpecifier = &envoy_config_route_v3.RouteAction_WeightedClusters{
			WeightedClusters: weightedClusters(r),
		}
	}
	return &envoy_config_route_v3.Route_Route{
		Route: &ra,
	}
}

// hashPolicy returns a slice of Envoy hash policies from the passed in Contour
// request hash policy configuration. Only one of header or cookie hash policies
// should be set on any RequestHashPolicy element.
func hashPolicy(requestHashPolicies []dag.RequestHashPolicy) []*envoy_config_route_v3.RouteAction_HashPolicy {
	if len(requestHashPolicies) == 0 {
		return nil
	}
	hashPolicies := []*envoy_config_route_v3.RouteAction_HashPolicy{}
	for _, rhp := range requestHashPolicies {
		newHP := &envoy_config_route_v3.RouteAction_HashPolicy{
			Terminal: rhp.Terminal,
		}
		if rhp.HeaderHashOptions != nil {
			newHP.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_Header_{
				Header: &envoy_config_route_v3.RouteAction_HashPolicy_Header{
					HeaderName: rhp.HeaderHashOptions.HeaderName,
				},
			}
		}
		if rhp.QueryParameterHashOptions != nil {
			newHP.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_QueryParameter_{
				QueryParameter: &envoy_config_route_v3.RouteAction_HashPolicy_QueryParameter{
					Name: rhp.QueryParameterHashOptions.ParameterName,
				},
			}
		}
		if rhp.CookieHashOptions != nil {
			newHP.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoy_config_route_v3.RouteAction_HashPolicy_Cookie{
					Name: rhp.CookieHashOptions.CookieName,
					Ttl:  durationpb.New(rhp.CookieHashOptions.TTL),
					Path: rhp.CookieHashOptions.Path,
				},
			}
		}
		if rhp.HashSourceIP {
			newHP.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties_{
				ConnectionProperties: &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties{
					SourceIp: true,
				},
			}
		}
		hashPolicies = append(hashPolicies, newHP)
	}
	return hashPolicies
}

func mirrorPolicy(r *dag.Route) []*envoy_config_route_v3.RouteAction_RequestMirrorPolicy {
	if len(r.MirrorPolicies) == 0 {
		return nil
	}

	mirrorPolicies := []*envoy_config_route_v3.RouteAction_RequestMirrorPolicy{}
	for _, mp := range r.MirrorPolicies {
		mirrorPolicies = append(mirrorPolicies, &envoy_config_route_v3.RouteAction_RequestMirrorPolicy{
			Cluster: envoy.Clustername(mp.Cluster),
			RuntimeFraction: &envoy_config_core_v3.RuntimeFractionalPercent{
				DefaultValue: &envoy_type_v3.FractionalPercent{
					Numerator:   uint32(mp.Weight),
					Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
				},
			},
		})
	}
	return mirrorPolicies
}

func retryPolicy(r *dag.Route) *envoy_config_route_v3.RetryPolicy {
	if r.RetryPolicy == nil {
		return nil
	}
	if r.RetryPolicy.RetryOn == "" {
		return nil
	}

	rp := &envoy_config_route_v3.RetryPolicy{
		RetryOn:              r.RetryPolicy.RetryOn,
		RetriableStatusCodes: r.RetryPolicy.RetriableStatusCodes,
	}
	if r.RetryPolicy.NumRetries > 0 {
		rp.NumRetries = wrapperspb.UInt32(r.RetryPolicy.NumRetries)
	}
	rp.PerTryTimeout = envoy.Timeout(r.RetryPolicy.PerTryTimeout)

	return rp
}

func internalRedirectPolicy(p *dag.InternalRedirectPolicy) *envoy_config_route_v3.InternalRedirectPolicy {
	if p == nil {
		return nil
	}

	var predicates []*envoy_config_core_v3.TypedExtensionConfig
	allowCrossSchemeRedirect := false

	switch p.AllowCrossSchemeRedirect {
	case dag.InternalRedirectCrossSchemeAlways:
		allowCrossSchemeRedirect = true
	case dag.InternalRedirectCrossSchemeSafeOnly:
		allowCrossSchemeRedirect = true
		predicates = append(predicates, &envoy_config_core_v3.TypedExtensionConfig{
			Name:        "envoy.internal_redirect_predicates.safe_cross_scheme",
			TypedConfig: protobuf.MustMarshalAny(&envoy_internal_redirect_safe_cross_scheme_v3.SafeCrossSchemeConfig{}),
		})
	}

	if p.DenyRepeatedRouteRedirect {
		predicates = append(predicates, &envoy_config_core_v3.TypedExtensionConfig{
			Name:        "envoy.internal_redirect_predicates.previous_routes",
			TypedConfig: protobuf.MustMarshalAny(&envoy_internal_redirect_previous_routes_v3.PreviousRoutesConfig{}),
		})
	}

	return &envoy_config_route_v3.InternalRedirectPolicy{
		MaxInternalRedirects:     protobuf.UInt32OrNil(p.MaxInternalRedirects),
		RedirectResponseCodes:    p.RedirectResponseCodes,
		Predicates:               predicates,
		AllowCrossSchemeRedirect: allowCrossSchemeRedirect,
	}
}

// UpgradeHTTPS returns a route Action that redirects the request to HTTPS.
func UpgradeHTTPS() *envoy_config_route_v3.Route_Redirect {
	return &envoy_config_route_v3.Route_Redirect{
		Redirect: &envoy_config_route_v3.RedirectAction{
			SchemeRewriteSpecifier: &envoy_config_route_v3.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}
}

// headerValueList creates a list of Envoy HeaderValueOptions from the provided map.
func headerValueList(hvm map[string]string, app bool) []*envoy_config_core_v3.HeaderValueOption {
	var hvs []*envoy_config_core_v3.HeaderValueOption

	appendAction := envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD
	if app {
		appendAction = envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD
	}

	for key, value := range hvm {
		hvs = append(hvs, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   key,
				Value: value,
			},
			AppendAction: appendAction,
		})
	}

	sort.Slice(hvs, func(i, j int) bool {
		return hvs[i].Header.Key < hvs[j].Header.Key
	})

	return hvs
}

// weightedClusters returns a route.WeightedCluster for multiple services.
func weightedClusters(route *dag.Route) *envoy_config_route_v3.WeightedCluster {
	var wc envoy_config_route_v3.WeightedCluster
	var total uint32
	for _, cluster := range route.Clusters {
		total += cluster.Weight

		c := &envoy_config_route_v3.WeightedCluster_ClusterWeight{
			Name:   envoy.Clustername(cluster),
			Weight: wrapperspb.UInt32(cluster.Weight),
		}
		if cluster.RequestHeadersPolicy != nil {
			c.RequestHeadersToAdd = append(headerValueList(cluster.RequestHeadersPolicy.Set, false), headerValueList(cluster.RequestHeadersPolicy.Add, true)...)
			c.RequestHeadersToRemove = cluster.RequestHeadersPolicy.Remove
			// Check for host header policy and set if found
			if val := envoy.HostRewriteLiteral(cluster.RequestHeadersPolicy); val != "" {
				c.HostRewriteSpecifier = &envoy_config_route_v3.WeightedCluster_ClusterWeight_HostRewriteLiteral{
					HostRewriteLiteral: val,
				}
			}
		}
		if cluster.ResponseHeadersPolicy != nil {
			c.ResponseHeadersToAdd = append(headerValueList(cluster.ResponseHeadersPolicy.Set, false), headerValueList(cluster.ResponseHeadersPolicy.Add, true)...)
			c.ResponseHeadersToRemove = cluster.ResponseHeadersPolicy.Remove
		}
		if len(route.CookieRewritePolicies) > 0 || len(cluster.CookieRewritePolicies) > 0 {
			if c.TypedPerFilterConfig == nil {
				c.TypedPerFilterConfig = map[string]*anypb.Any{}
			}
			c.TypedPerFilterConfig[LuaFilterName] = cookieRewriteConfig(route.CookieRewritePolicies, cluster.CookieRewritePolicies)
		}
		wc.Clusters = append(wc.Clusters, c)
	}
	// Check if no weights were defined, if not default to even distribution
	if total == 0 {
		for _, c := range wc.Clusters {
			c.Weight.Value = 1
		}
	}

	sort.Stable(sorter.For(wc.Clusters))
	return &wc
}

// VirtualHost creates a new route.VirtualHost.
func VirtualHost(hostname string, routes ...*envoy_config_route_v3.Route) *envoy_config_route_v3.VirtualHost {
	return &envoy_config_route_v3.VirtualHost{
		Name:    envoy.Hashname(60, hostname),
		Domains: []string{hostname},
		Routes:  routes,
	}
}

// CORSVirtualHost creates a new route.VirtualHost with a CORS policy.
func CORSVirtualHost(hostname string, corspolicy *envoy_filter_http_cors_v3.CorsPolicy, routes ...*envoy_config_route_v3.Route) *envoy_config_route_v3.VirtualHost {
	vh := VirtualHost(hostname, routes...)
	if corspolicy != nil {
		vh.TypedPerFilterConfig = map[string]*anypb.Any{
			CORSFilterName: protobuf.MustMarshalAny(corspolicy),
		}
	}
	return vh
}

// RouteConfiguration returns a *envoy_config_route_v3.RouteConfiguration.
func RouteConfiguration(name string, virtualhosts ...*envoy_config_route_v3.VirtualHost) *envoy_config_route_v3.RouteConfiguration {
	return &envoy_config_route_v3.RouteConfiguration{
		Name:         name,
		VirtualHosts: virtualhosts,
		RequestHeadersToAdd: headers(
			appendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
		),
		// We can ignore any port number supplied in the Host/:authority header
		// when selecting a virtualhost.
		IgnorePortInHostMatching: true,
	}
}

// corsPolicy returns a *envoy_filter_http_cors_v3.CorsPolicy
func corsPolicy(cp *dag.CORSPolicy) *envoy_filter_http_cors_v3.CorsPolicy {
	if cp == nil {
		return nil
	}
	ecp := &envoy_filter_http_cors_v3.CorsPolicy{
		AllowCredentials:          wrapperspb.Bool(cp.AllowCredentials),
		AllowHeaders:              strings.Join(cp.AllowHeaders, ","),
		AllowMethods:              strings.Join(cp.AllowMethods, ","),
		ExposeHeaders:             strings.Join(cp.ExposeHeaders, ","),
		AllowPrivateNetworkAccess: wrapperspb.Bool(cp.AllowPrivateNetwork),
	}

	if cp.MaxAge.IsDisabled() {
		ecp.MaxAge = "0"
	} else if !cp.MaxAge.UseDefault() {
		ecp.MaxAge = fmt.Sprintf("%.0f", cp.MaxAge.Duration().Seconds())
	}

	ecp.AllowOriginStringMatch = []*envoy_matcher_v3.StringMatcher{}
	for _, ao := range cp.AllowOrigin {
		m := &envoy_matcher_v3.StringMatcher{}
		switch ao.Type {
		case dag.CORSAllowOriginMatchExact:
			// Even though we use the exact matcher, Envoy always makes an exception for the `*` value
			// https://github.com/envoyproxy/envoy/blob/d6e2fd0185ca620745479da2c43c0564eeaf35c5/source/extensions/filters/http/cors/cors_filter.cc#L142
			m.MatchPattern = &envoy_matcher_v3.StringMatcher_Exact{
				Exact: ao.Value,
			}
			m.IgnoreCase = true
		case dag.CORSAllowOriginMatchRegex:
			m.MatchPattern = &envoy_matcher_v3.StringMatcher_SafeRegex{
				SafeRegex: SafeRegexMatch(ao.Value),
			}
		}
		ecp.AllowOriginStringMatch = append(ecp.AllowOriginStringMatch, m)
	}
	return ecp
}

func headers(first *envoy_config_core_v3.HeaderValueOption, rest ...*envoy_config_core_v3.HeaderValueOption) []*envoy_config_core_v3.HeaderValueOption {
	return append([]*envoy_config_core_v3.HeaderValueOption{first}, rest...)
}

func appendHeader(key, value string) *envoy_config_core_v3.HeaderValueOption {
	return &envoy_config_core_v3.HeaderValueOption{
		Header: &envoy_config_core_v3.HeaderValue{
			Key:   key,
			Value: value,
		},
		AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
	}
}

func headerMatcher(headers []dag.HeaderMatchCondition) []*envoy_config_route_v3.HeaderMatcher {
	var envoyHeaders []*envoy_config_route_v3.HeaderMatcher

	for _, h := range headers {
		header := &envoy_config_route_v3.HeaderMatcher{
			Name:        h.Name,
			InvertMatch: h.Invert,
			// We only want to turn on TreatMissingHeaderAsEmpty on invert matches
			TreatMissingHeaderAsEmpty: h.Invert && h.TreatMissingAsEmpty,
		}

		switch h.MatchType {
		case dag.HeaderMatchTypeExact:
			header.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_StringMatch{
				StringMatch: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{Exact: h.Value},
					IgnoreCase:   h.IgnoreCase,
				},
			}
		case dag.HeaderMatchTypeContains:
			header.HeaderMatchSpecifier = containsMatch(h.Value, h.IgnoreCase)
		case dag.HeaderMatchTypePresent:
			header.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_PresentMatch{PresentMatch: true}
		case dag.HeaderMatchTypeRegex:
			header.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_StringMatch{
				StringMatch: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
						SafeRegex: SafeRegexMatch(h.Value),
					},
				},
			}
		}
		envoyHeaders = append(envoyHeaders, header)
	}
	return envoyHeaders
}

func queryParamMatcher(queryParams []dag.QueryParamMatchCondition) []*envoy_config_route_v3.QueryParameterMatcher {
	var envoyQueryParamMatchers []*envoy_config_route_v3.QueryParameterMatcher

	for _, q := range queryParams {
		queryParam := &envoy_config_route_v3.QueryParameterMatcher{
			Name: q.Name,
		}

		switch q.MatchType {
		case dag.QueryParamMatchTypeExact:
			queryParam.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
				StringMatch: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{Exact: q.Value},
					IgnoreCase:   q.IgnoreCase,
				},
			}
		case dag.QueryParamMatchTypePrefix:
			queryParam.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
				StringMatch: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_Prefix{Prefix: q.Value},
					IgnoreCase:   q.IgnoreCase,
				},
			}
		case dag.QueryParamMatchTypeSuffix:
			queryParam.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
				StringMatch: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_Suffix{Suffix: q.Value},
					IgnoreCase:   q.IgnoreCase,
				},
			}
		case dag.QueryParamMatchTypeRegex:
			queryParam.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
				StringMatch: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
						SafeRegex: SafeRegexMatch(q.Value),
					},
				},
			}
		case dag.QueryParamMatchTypeContains:
			queryParam.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
				StringMatch: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_Contains{Contains: q.Value},
					IgnoreCase:   q.IgnoreCase,
				},
			}
		case dag.QueryParamMatchTypePresent:
			queryParam.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_PresentMatch{
				PresentMatch: true,
			}
		}

		envoyQueryParamMatchers = append(envoyQueryParamMatchers, queryParam)
	}

	return envoyQueryParamMatchers
}

// containsMatch returns a HeaderMatchSpecifier which will match the
// supplied substring
func containsMatch(s string, ignoreCase bool) *envoy_config_route_v3.HeaderMatcher_StringMatch {
	return &envoy_config_route_v3.HeaderMatcher_StringMatch{
		StringMatch: &envoy_matcher_v3.StringMatcher{
			IgnoreCase: ignoreCase,
			MatchPattern: &envoy_matcher_v3.StringMatcher_Contains{
				Contains: s,
			},
		},
	}
}

func cookieRewriteConfig(routePolicies, clusterPolicies []dag.CookieRewritePolicy) *anypb.Any {
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

	c := &envoy_filter_http_lua_v3.LuaPerRoute{
		Override: &envoy_filter_http_lua_v3.LuaPerRoute_SourceCode{
			SourceCode: &envoy_config_core_v3.DataSource{
				Specifier: &envoy_config_core_v3.DataSource_InlineString{
					InlineString: t.String(),
				},
			},
		},
	}
	return protobuf.MustMarshalAny(c)
}
