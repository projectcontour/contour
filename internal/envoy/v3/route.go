package v3

import (
	"fmt"
	"regexp"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
)

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
		return &envoy_route_v3.RouteMatch{
			PathSpecifier: &envoy_route_v3.RouteMatch_Prefix{
				Prefix: c.Prefix,
			},
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	default:
		return &envoy_route_v3.RouteMatch{
			Headers: headerMatcher(route.HeaderMatchConditions),
		}
	}
}

// VirtualHost creates a new route.VirtualHost.
func VirtualHost(hostname string, routes ...*envoy_route_v3.Route) *envoy_route_v3.VirtualHost {
	domains := []string{hostname}
	if hostname != "*" {
		// NOTE(jpeach) see also envoy.FilterMisdirectedRequests().
		domains = append(domains, hostname+":*")
	}

	return &envoy_route_v3.VirtualHost{
		Name:    envoy.Hashname(60, hostname),
		Domains: domains,
		Routes:  routes,
	}
}

// RouteConfiguration returns a *v3.RouteConfiguration.
func RouteConfiguration(name string, virtualhosts ...*envoy_route_v3.VirtualHost) *envoy_route_v3.RouteConfiguration {
	return &envoy_route_v3.RouteConfiguration{
		Name:         name,
		VirtualHosts: virtualhosts,
		RequestHeadersToAdd: Headers(
			AppendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
		),
	}
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
		case "exact":
			header.HeaderMatchSpecifier = &envoy_route_v3.HeaderMatcher_ExactMatch{ExactMatch: h.Value}
		case "contains":
			header.HeaderMatchSpecifier = containsMatch(h.Value)
		case "present":
			header.HeaderMatchSpecifier = &envoy_route_v3.HeaderMatcher_PresentMatch{PresentMatch: true}
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
