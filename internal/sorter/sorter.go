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

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	tcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/golang/protobuf/proto"
)

// Sorts the given route configuration values by name.
type routeConfigurationSorter []*v2.RouteConfiguration

func (s routeConfigurationSorter) Len() int           { return len(s) }
func (s routeConfigurationSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s routeConfigurationSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts the given host values by name.
type virtualHostSorter []*envoy_api_v2_route.VirtualHost

func (s virtualHostSorter) Len() int           { return len(s) }
func (s virtualHostSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s virtualHostSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts HeaderMatcher objects, first by the header name,
// then by their matcher conditions (textually).
type headerMatcherSorter []*envoy_api_v2_route.HeaderMatcher

func (s headerMatcherSorter) Len() int      { return len(s) }
func (s headerMatcherSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s headerMatcherSorter) Less(i, j int) bool {
	val := strings.Compare(s[i].Name, s[j].Name)
	switch val {
	case -1:
		return true
	case 1:
		return false
	case 0:
		return proto.CompactTextString(s[i]) < proto.CompactTextString(s[j])
	}

	panic("bad comparison")
}

// longestRouteByHeaders compares the HeaderMatcher slices for lhs and rhs and
// returns true if lhs is longer.
func longestRouteByHeaders(lhs, rhs *envoy_api_v2_route.Route) bool {
	if len(lhs.Match.Headers) == len(rhs.Match.Headers) {
		pair := make([]*envoy_api_v2_route.HeaderMatcher, 2)

		for i := 0; i < len(lhs.Match.Headers); i++ {
			pair[0] = lhs.Match.Headers[i]
			pair[1] = rhs.Match.Headers[i]

			if headerMatcherSorter(pair).Less(0, 1) {
				return true
			}
		}
	}

	return len(lhs.Match.Headers) > len(rhs.Match.Headers)
}

// Sorts the given Route slice in place. Routes are ordered first by
// longest prefix (or regex), then by the length of the HeaderMatch
// slice (if any). The HeaderMatch slice is also ordered by the matching
// header name.
type routeSorter []*envoy_api_v2_route.Route

func (s routeSorter) Len() int      { return len(s) }
func (s routeSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s routeSorter) Less(i, j int) bool {
	switch a := s[i].Match.PathSpecifier.(type) {
	case *envoy_api_v2_route.RouteMatch_Prefix:
		switch b := s[j].Match.PathSpecifier.(type) {
		case *envoy_api_v2_route.RouteMatch_Prefix:
			cmp := strings.Compare(a.Prefix, b.Prefix)
			switch cmp {
			case 1:
				// Sort longest prefix first.
				return true
			case -1:
				return false
			default:
				return longestRouteByHeaders(s[i], s[j])
			}
		}
	case *envoy_api_v2_route.RouteMatch_SafeRegex:
		switch b := s[j].Match.PathSpecifier.(type) {
		case *envoy_api_v2_route.RouteMatch_SafeRegex:
			cmp := strings.Compare(a.SafeRegex.Regex, b.SafeRegex.Regex)
			switch cmp {
			case 1:
				// Sort longest regex first.
				return true
			case -1:
				return false
			default:
				return longestRouteByHeaders(s[i], s[j])
			}
		case *envoy_api_v2_route.RouteMatch_Prefix:
			return true
		}
	}

	return false
}

// Sorts clusters by name.
type clusterSorter []*v2.Cluster

func (s clusterSorter) Len() int           { return len(s) }
func (s clusterSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s clusterSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Sorts cluster load assignments by name.
type clusterLoadAssignmentSorter []*v2.ClusterLoadAssignment

func (s clusterLoadAssignmentSorter) Len() int           { return len(s) }
func (s clusterLoadAssignmentSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s clusterLoadAssignmentSorter) Less(i, j int) bool { return s[i].ClusterName < s[j].ClusterName }

// Sorts the weighted clusters by name, then by weight.
type httpWeightedClusterSorter []*envoy_api_v2_route.WeightedCluster_ClusterWeight

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
type listenerSorter []*v2.Listener

func (s listenerSorter) Len() int           { return len(s) }
func (s listenerSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s listenerSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// FilterChains sorts the filter chains by the first server name in the chain match.
type filterChainSorter []*envoy_api_v2_listener.FilterChain

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
type secretSorter []*envoy_api_v2_auth.Secret

func (s secretSorter) Len() int           { return len(s) }
func (s secretSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s secretSorter) Less(i, j int) bool { return s[i].Name < s[j].Name }

// For returns a sort.Interface object that can be used to sort the
// given value. It returns nil if there is no sorter for the type of
// value.
func For(v interface{}) sort.Interface {
	switch v := v.(type) {
	case []*envoy_api_v2_auth.Secret:
		return secretSorter(v)
	case []*v2.RouteConfiguration:
		return routeConfigurationSorter(v)
	case []*envoy_api_v2_route.VirtualHost:
		return virtualHostSorter(v)
	case []*envoy_api_v2_route.Route:
		return routeSorter(v)
	case []*envoy_api_v2_route.HeaderMatcher:
		return headerMatcherSorter(v)
	case []*v2.Cluster:
		return clusterSorter(v)
	case []*v2.ClusterLoadAssignment:
		return clusterLoadAssignmentSorter(v)
	case []*envoy_api_v2_route.WeightedCluster_ClusterWeight:
		return httpWeightedClusterSorter(v)
	case []*tcp.TcpProxy_WeightedCluster_ClusterWeight:
		return tcpWeightedClusterSorter(v)
	case []*v2.Listener:
		return listenerSorter(v)
	case []*envoy_api_v2_listener.FilterChain:
		return filterChainSorter(v)
	default:
		return nil
	}
}
