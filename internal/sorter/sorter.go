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

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/projectcontour/contour/internal/dag"
)

// Sorts the given route configuration values by name.
type routeConfigurationSorter []*envoy_route_v3.RouteConfiguration

func (s routeConfigurationSorter) Len() int           { return len(s) }
func (s routeConfigurationSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s routeConfigurationSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts the given host values by name.
type virtualHostSorter []*envoy_route_v3.VirtualHost

func (s virtualHostSorter) Len() int           { return len(s) }
func (s virtualHostSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s virtualHostSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts HeaderMatchCondition objects, first by the header name,
// then by their matcher conditions type.
type headerMatchConditionSorter []dag.HeaderMatchCondition

func (s headerMatchConditionSorter) Len() int      { return len(s) }
func (s headerMatchConditionSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s headerMatchConditionSorter) Less(i, j int) bool {
	compareValue := func(a dag.HeaderMatchCondition, b dag.HeaderMatchCondition) bool {
		switch strings.Compare(a.Value, b.Value) {
		case -1:
			return true
		case 1:
			return false
		default:
			// The match that is not inverted sorts first.
			return !a.Invert
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
			switch s[j].MatchType {
			case dag.HeaderMatchTypePresent:
				// The match that is not inverted sorts first.
				return !s[i].Invert
			}
		}
		return false
	}
}

// longestRouteByHeaderConditions compares the HeaderMatchCondition slices for
// lhs and rhs and returns true if lhs is longer.
func longestRouteByHeaderConditions(lhs, rhs *dag.Route) bool {
	if len(lhs.HeaderMatchConditions) == len(rhs.HeaderMatchConditions) {
		pair := make([]dag.HeaderMatchCondition, 2)

		for i := 0; i < len(lhs.HeaderMatchConditions); i++ {
			pair[0] = lhs.HeaderMatchConditions[i]
			pair[1] = rhs.HeaderMatchConditions[i]

			if headerMatchConditionSorter(pair).Less(0, 1) {
				return true
			}
		}
	}

	return len(lhs.HeaderMatchConditions) > len(rhs.HeaderMatchConditions)
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
		switch b := s[j].PathMatchCondition.(type) {
		case *dag.PrefixMatchCondition:
			cmp := strings.Compare(a.Prefix, b.Prefix)
			switch cmp {
			case 1:
				// Sort longest prefix first.
				return true
			case -1:
				return false
			default:
				if a.PrefixMatchType == b.PrefixMatchType {
					return longestRouteByHeaderConditions(s[i], s[j])
				}
				// Segment prefixes sort first as they are more specific.
				return a.PrefixMatchType == dag.PrefixMatchSegment
			}
		}
	case *dag.RegexMatchCondition:
		switch b := s[j].PathMatchCondition.(type) {
		case *dag.RegexMatchCondition:
			cmp := strings.Compare(a.Regex, b.Regex)
			switch cmp {
			case 1:
				// Sort longest regex first.
				return true
			case -1:
				return false
			default:
				return longestRouteByHeaderConditions(s[i], s[j])
			}
		case *dag.PrefixMatchCondition:
			return true
		}
	case *dag.ExactMatchCondition:
		switch b := s[j].PathMatchCondition.(type) {
		case *dag.ExactMatchCondition:
			cmp := strings.Compare(a.Path, b.Path)
			switch cmp {
			case 1:
				// Sort longest path first.
				return true
			case -1:
				return false
			default:
				return longestRouteByHeaderConditions(s[i], s[j])
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
type clusterSorter []*envoy_cluster_v3.Cluster

func (s clusterSorter) Len() int           { return len(s) }
func (s clusterSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s clusterSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts cluster load assignments by name.
type clusterLoadAssignmentSorter []*envoy_endpoint_v3.ClusterLoadAssignment

func (s clusterLoadAssignmentSorter) Len() int           { return len(s) }
func (s clusterLoadAssignmentSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s clusterLoadAssignmentSorter) Less(i, j int) bool { return s[i].ClusterName < s[j].ClusterName }

// Sorts the weighted clusters by name, then by weight.
type httpWeightedClusterSorter []*envoy_route_v3.WeightedCluster_ClusterWeight

func (s httpWeightedClusterSorter) Len() int      { return len(s) }
func (s httpWeightedClusterSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s httpWeightedClusterSorter) Less(i, j int) bool {
	if s[i].Name == s[j].Name {
		return s[i].Weight.Value < s[j].Weight.Value
	}

	return s[i].Name < s[j].Name
}

// Sorts the weighted clusters by name, then by weight.
type tcpWeightedClusterSorter []*tcp.TcpProxy_WeightedCluster_ClusterWeight

func (s tcpWeightedClusterSorter) Len() int      { return len(s) }
func (s tcpWeightedClusterSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s tcpWeightedClusterSorter) Less(i, j int) bool {
	if s[i].Name == s[j].Name {
		return s[i].Weight < s[j].Weight
	}

	return s[i].Name < s[j].Name
}

// Listeners sorts the listeners by name.
type listenerSorter []*envoy_listener_v3.Listener

func (s listenerSorter) Len() int           { return len(s) }
func (s listenerSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s listenerSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// FilterChains sorts the filter chains by the first server name in the chain match.
type filterChainSorter []*envoy_listener_v3.FilterChain

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
type secretSorter []*envoy_tls_v3.Secret

func (s secretSorter) Len() int           { return len(s) }
func (s secretSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s secretSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// For returns a sort.Interface object that can be used to sort the
// given value. It returns nil if there is no sorter for the type of
// value.
func For(v interface{}) sort.Interface {
	switch v := v.(type) {
	case []*envoy_tls_v3.Secret:
		return secretSorter(v)
	case []*envoy_route_v3.RouteConfiguration:
		return routeConfigurationSorter(v)
	case []*envoy_route_v3.VirtualHost:
		return virtualHostSorter(v)
	case []*dag.Route:
		return routeSorter(v)
	case []dag.HeaderMatchCondition:
		return headerMatchConditionSorter(v)
	case []*envoy_cluster_v3.Cluster:
		return clusterSorter(v)
	case []*envoy_endpoint_v3.ClusterLoadAssignment:
		return clusterLoadAssignmentSorter(v)
	case []*envoy_route_v3.WeightedCluster_ClusterWeight:
		return httpWeightedClusterSorter(v)
	case []*tcp.TcpProxy_WeightedCluster_ClusterWeight:
		return tcpWeightedClusterSorter(v)
	case []*envoy_listener_v3.Listener:
		return listenerSorter(v)
	case []*envoy_listener_v3.FilterChain:
		return filterChainSorter(v)
	default:
		return nil
	}
}
