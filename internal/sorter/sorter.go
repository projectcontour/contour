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

package sorter

import (
	"sort"
	"strings"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_network_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

	"github.com/projectcontour/contour/internal/dag"
)

// Sorts the given route configuration values by name.
type routeConfigurationSorter []*envoy_config_route_v3.RouteConfiguration

func (s routeConfigurationSorter) Len() int           { return len(s) }
func (s routeConfigurationSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s routeConfigurationSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts the given host values by name.
type virtualHostSorter []*envoy_config_route_v3.VirtualHost

func (s virtualHostSorter) Len() int           { return len(s) }
func (s virtualHostSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s virtualHostSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts HeaderMatchCondition objects, first by the header name,
// then by their matcher conditions type.
type headerMatchConditionSorter []dag.HeaderMatchCondition

func (s headerMatchConditionSorter) Len() int      { return len(s) }
func (s headerMatchConditionSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s headerMatchConditionSorter) Less(i, j int) bool {
	compareValue := func(a, b dag.HeaderMatchCondition) bool {
		switch strings.Compare(a.Value, b.Value) {
		case -1:
			return true
		case 1:
			return false
		default:
			// The match that is not inverted sorts first.
			if a.Invert != b.Invert {
				return !a.Invert
			}
			return false
		}
	}

	val := strings.Compare(s[i].Name, s[j].Name)
	switch val {
	case -1:
		return true
	case 1:
		return false
	default:
		switch s[i].MatchType {
		case dag.HeaderMatchTypeExact:
			// Exact matches are most specific so they sort first.
			switch s[j].MatchType {
			case dag.HeaderMatchTypeExact:
				return compareValue(s[i], s[j])
			case dag.HeaderMatchTypeRegex:
				return true
			case dag.HeaderMatchTypeContains:
				return true
			case dag.HeaderMatchTypePresent:
				return true
			}
		case dag.HeaderMatchTypeRegex:
			// Regex matches sort ahead of Contains matches.
			switch s[j].MatchType {
			case dag.HeaderMatchTypeRegex:
				return compareValue(s[i], s[j])
			case dag.HeaderMatchTypeContains:
				return true
			case dag.HeaderMatchTypePresent:
				return true
			}
		case dag.HeaderMatchTypeContains:
			// Contains matches sort ahead of Present matches.
			switch s[j].MatchType {
			case dag.HeaderMatchTypeContains:
				return compareValue(s[i], s[j])
			case dag.HeaderMatchTypePresent:
				return true
			}
		case dag.HeaderMatchTypePresent:
			if s[j].MatchType == dag.HeaderMatchTypePresent {
				// The match that is not inverted sorts first.
				if s[i].Invert != s[j].Invert {
					return !s[i].Invert
				}
			}
		}
		return false
	}
}

// Sorts QueryParameterMatchCondition objects, first by the query param name,
// then by their matcher condition type and value.
type queryParamMatchConditionSorter []dag.QueryParamMatchCondition

func (s queryParamMatchConditionSorter) Len() int      { return len(s) }
func (s queryParamMatchConditionSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s queryParamMatchConditionSorter) Less(i, j int) bool {
	compareValue := func(a, b dag.QueryParamMatchCondition) bool {
		switch strings.Compare(a.Value, b.Value) {
		case -1:
			return true
		case 1:
			return false
		default:
			// The match that is case sensitive sorts first.
			if a.IgnoreCase != b.IgnoreCase {
				return !a.IgnoreCase
			}
			return false
		}
	}

	val := strings.Compare(s[i].Name, s[j].Name)
	switch val {
	case -1:
		return true
	case 1:
		return false
	default:
		switch s[i].MatchType {
		case dag.QueryParamMatchTypeExact:
			// Exact matches are most specific so they sort first.
			switch s[j].MatchType {
			case dag.QueryParamMatchTypeExact:
				return compareValue(s[i], s[j])
			case dag.QueryParamMatchTypeRegex:
				return true
			case dag.QueryParamMatchTypePrefix:
				return true
			case dag.QueryParamMatchTypeSuffix:
				return true
			case dag.QueryParamMatchTypeContains:
				return true
			case dag.QueryParamMatchTypePresent:
				return true
			}
		case dag.QueryParamMatchTypeRegex:
			// Regex matches sort ahead of Prefix matches.
			switch s[j].MatchType {
			case dag.QueryParamMatchTypeRegex:
				return compareValue(s[i], s[j])
			case dag.QueryParamMatchTypePrefix:
				return true
			case dag.QueryParamMatchTypeSuffix:
				return true
			case dag.QueryParamMatchTypeContains:
				return true
			case dag.QueryParamMatchTypePresent:
				return true
			}
		case dag.QueryParamMatchTypePrefix:
			// Prefix matches sort ahead of Suffix matches.
			switch s[j].MatchType {
			case dag.QueryParamMatchTypePrefix:
				return compareValue(s[i], s[j])
			case dag.QueryParamMatchTypeSuffix:
				return true
			case dag.QueryParamMatchTypeContains:
				return true
			case dag.QueryParamMatchTypePresent:
				return true
			}
		case dag.QueryParamMatchTypeSuffix:
			// Suffix matches sort ahead of Contains matches.
			switch s[j].MatchType {
			case dag.QueryParamMatchTypeSuffix:
				return compareValue(s[i], s[j])
			case dag.QueryParamMatchTypeContains:
				return true
			case dag.QueryParamMatchTypePresent:
				return true
			}
		case dag.QueryParamMatchTypeContains:
			// Contains matches sort ahead of Present matches.
			switch s[j].MatchType {
			case dag.QueryParamMatchTypeContains:
				return compareValue(s[i], s[j])
			case dag.QueryParamMatchTypePresent:
				return true
			}
		case dag.QueryParamMatchTypePresent:
		}
		return false
	}
}

// compareRoutesByMethodHeaderQueryParams compares any HTTP method match
// (:method header which is then excluded from the rest of the header match
// comparisons), HeaderMatchConditions, and QueryParamMatchConditions slices
// for lhs and rhs and returns true if the conditions for the lhs Route mean
// it should sort first.
func compareRoutesByMethodHeaderQueryParams(lhs, rhs *dag.Route) bool {
	// Find if method matches exist. Should only ever be one.
	// If found, exclude from HeaderMatchConditions slices we will
	// compare.
	lhsMethodMatchFound := false
	lhsHeaderMatchConditions := make([]dag.HeaderMatchCondition, 0, len(lhs.HeaderMatchConditions))
	for _, h := range lhs.HeaderMatchConditions {
		if h.Name == ":method" {
			lhsMethodMatchFound = true
		} else {
			lhsHeaderMatchConditions = append(lhsHeaderMatchConditions, h)
		}
	}
	rhsMethodMatchFound := false
	rhsHeaderMatchConditions := make([]dag.HeaderMatchCondition, 0, len(rhs.HeaderMatchConditions))
	for _, h := range rhs.HeaderMatchConditions {
		if h.Name == ":method" {
			rhsMethodMatchFound = true
		} else {
			rhsHeaderMatchConditions = append(rhsHeaderMatchConditions, h)
		}
	}

	// Now check if only one of the routes had a method match.
	if lhsMethodMatchFound != rhsMethodMatchFound {
		return lhsMethodMatchFound
	}

	// One route has a longer HeaderMatchConditions slice.
	if len(lhsHeaderMatchConditions) != len(rhsHeaderMatchConditions) {
		return len(lhsHeaderMatchConditions) > len(rhsHeaderMatchConditions)
	}

	// One route has a longer QueryParamMatchConditions slice.
	if len(lhs.QueryParamMatchConditions) != len(rhs.QueryParamMatchConditions) {
		return len(lhs.QueryParamMatchConditions) > len(rhs.QueryParamMatchConditions)
	}

	// If there are the same number of header and query parameter matches, sort
	// based on the priority of the route.
	// Note: lower values mean a higher priority.
	if lhs.Priority != rhs.Priority {
		return lhs.Priority < rhs.Priority
	}

	// HeaderMatchConditions are equal length: compare item by item.
	pair := make([]dag.HeaderMatchCondition, 2)
	for i := range len(lhsHeaderMatchConditions) {
		pair[0] = lhsHeaderMatchConditions[i]
		pair[1] = rhsHeaderMatchConditions[i]

		if headerMatchConditionSorter(pair).Less(0, 1) {
			return true
		}
	}

	// QueryParamMatchConditions are equal length: compare item by item.
	for i := range len(lhs.QueryParamMatchConditions) {
		qPair := make([]dag.QueryParamMatchCondition, 2)
		qPair[0] = lhs.QueryParamMatchConditions[i]
		qPair[1] = rhs.QueryParamMatchConditions[i]

		if queryParamMatchConditionSorter(qPair).Less(0, 1) {
			return true
		}
	}

	return false
}

// Sorts the given Route slice in place. Routes are ordered first by
// type (exact sorts before regex, sorts before prefix) and then
// longest path match value, then by the length of the HeaderMatch
// slice (if any). The HeaderMatch slice is also ordered by the matching
// header name.
type routeSorter []*dag.Route

func (s routeSorter) Len() int      { return len(s) }
func (s routeSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s routeSorter) Less(i, j int) bool {
	switch a := s[i].PathMatchCondition.(type) {
	case *dag.PrefixMatchCondition:
		if b, ok := s[j].PathMatchCondition.(*dag.PrefixMatchCondition); ok {
			switch {
			case len(a.Prefix) > len(b.Prefix):
				// Sort longest prefix first.
				return true
			case len(a.Prefix) < len(b.Prefix):
				return false
			default:
				cmp := strings.Compare(a.Prefix, b.Prefix)
				switch cmp {
				case 1:
					return true
				case -1:
					return false
				default:
					if a.PrefixMatchType == b.PrefixMatchType {
						return compareRoutesByMethodHeaderQueryParams(s[i], s[j])
					}
					// Segment prefixes sort first as they are more specific.
					return a.PrefixMatchType == dag.PrefixMatchSegment
				}
			}
		}
	case *dag.RegexMatchCondition:
		switch b := s[j].PathMatchCondition.(type) {
		case *dag.RegexMatchCondition:
			switch {
			case len(a.Regex) > len(b.Regex):
				// Sort longest regex first.
				return true
			case len(a.Regex) < len(b.Regex):
				return false
			default:
				cmp := strings.Compare(a.Regex, b.Regex)
				switch cmp {
				case 1:
					return true
				case -1:
					return false
				default:
					return compareRoutesByMethodHeaderQueryParams(s[i], s[j])
				}
			}
		case *dag.PrefixMatchCondition:
			return true
		}
	case *dag.ExactMatchCondition:
		switch b := s[j].PathMatchCondition.(type) {
		case *dag.ExactMatchCondition:
			cmp := strings.Compare(a.Path, b.Path)
			// Sorting function doesn't really matter here
			// since we want exact matching. Lexicographic sorting
			// is ok
			switch cmp {
			case 1:
				return true
			case -1:
				return false
			default:
				return compareRoutesByMethodHeaderQueryParams(s[i], s[j])
			}
		case *dag.PrefixMatchCondition:
			return true
		case *dag.RegexMatchCondition:
			return true
		}
	}

	return false
}

// Sorts clusters by name.
type clusterSorter []*envoy_config_cluster_v3.Cluster

func (s clusterSorter) Len() int           { return len(s) }
func (s clusterSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s clusterSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts cluster load assignments by name.
type clusterLoadAssignmentSorter []*envoy_config_endpoint_v3.ClusterLoadAssignment

func (s clusterLoadAssignmentSorter) Len() int           { return len(s) }
func (s clusterLoadAssignmentSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s clusterLoadAssignmentSorter) Less(i, j int) bool { return s[i].ClusterName < s[j].ClusterName }

// Sorts the weighted clusters by name, then by weight.
type httpWeightedClusterSorter []*envoy_config_route_v3.WeightedCluster_ClusterWeight

func (s httpWeightedClusterSorter) Len() int      { return len(s) }
func (s httpWeightedClusterSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s httpWeightedClusterSorter) Less(i, j int) bool {
	if s[i].Name == s[j].Name {
		return s[i].Weight.Value < s[j].Weight.Value
	}

	return s[i].Name < s[j].Name
}

// Sorts the weighted clusters by name, then by weight.
type tcpWeightedClusterSorter []*envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight

func (s tcpWeightedClusterSorter) Len() int      { return len(s) }
func (s tcpWeightedClusterSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s tcpWeightedClusterSorter) Less(i, j int) bool {
	if s[i].Name == s[j].Name {
		return s[i].Weight < s[j].Weight
	}

	return s[i].Name < s[j].Name
}

// Listeners sorts the listeners by name.
type listenerSorter []*envoy_config_listener_v3.Listener

func (s listenerSorter) Len() int           { return len(s) }
func (s listenerSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s listenerSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// FilterChains sorts the filter chains by the first server name in the chain match.
type filterChainSorter []*envoy_config_listener_v3.FilterChain

func (s filterChainSorter) Len() int      { return len(s) }
func (s filterChainSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s filterChainSorter) Less(i, j int) bool {
	// If i's ServerNames aren't defined, then it should not swap
	if len(s[i].FilterChainMatch.ServerNames) == 0 {
		return false
	}

	// If j's ServerNames aren't defined, then it should not swap
	if len(s[j].FilterChainMatch.ServerNames) == 0 {
		return true
	}

	// The ServerNames field will only ever have a single entry
	// in our FilterChain config, so it's okay to only sort
	// on the first slice entry.
	return s[i].FilterChainMatch.ServerNames[0] < s[j].FilterChainMatch.ServerNames[0]
}

// Sorts the secret values by name.
type secretSorter []*envoy_transport_socket_tls_v3.Secret

func (s secretSorter) Len() int           { return len(s) }
func (s secretSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s secretSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// For returns a sort.Interface object that can be used to sort the
// given value. It returns nil if there is no sorter for the type of
// value.
func For(v any) sort.Interface {
	switch v := v.(type) {
	case []*envoy_transport_socket_tls_v3.Secret:
		return secretSorter(v)
	case []*envoy_config_route_v3.RouteConfiguration:
		return routeConfigurationSorter(v)
	case []*envoy_config_route_v3.VirtualHost:
		return virtualHostSorter(v)
	case []*dag.Route:
		return routeSorter(v)
	case []dag.HeaderMatchCondition:
		return headerMatchConditionSorter(v)
	case []dag.QueryParamMatchCondition:
		return queryParamMatchConditionSorter(v)
	case []*envoy_config_cluster_v3.Cluster:
		return clusterSorter(v)
	case []*envoy_config_endpoint_v3.ClusterLoadAssignment:
		return clusterLoadAssignmentSorter(v)
	case []*envoy_config_route_v3.WeightedCluster_ClusterWeight:
		return httpWeightedClusterSorter(v)
	case []*envoy_filter_network_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight:
		return tcpWeightedClusterSorter(v)
	case []*envoy_config_listener_v3.Listener:
		return listenerSorter(v)
	case []*envoy_config_listener_v3.FilterChain:
		return filterChainSorter(v)
	default:
		return nil
	}
}
