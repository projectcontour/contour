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
	"math/rand"
	"sort"
	"testing"

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/stretchr/testify/assert"
)

func shuffleRoutes(routes []*dag.Route) []*dag.Route {
	shuffled := make([]*dag.Route, len(routes))

	copy(shuffled, routes)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled
}

func TestInvalidSorter(t *testing.T) {
	assert.Equal(t, nil, For([]string{"invalid"}))
}

func TestSortRouteConfiguration(t *testing.T) {
	want := []*envoy_route_v3.RouteConfiguration{
		{Name: "bar"},
		{Name: "baz"},
		{Name: "foo"},
		{Name: "same", InternalOnlyHeaders: []string{"z", "y"}},
		{Name: "same", InternalOnlyHeaders: []string{"a", "b"}},
	}

	have := []*envoy_route_v3.RouteConfiguration{
		want[3], // Ensure the "same" element stays stable.
		want[4],
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortVirtualHosts(t *testing.T) {
	want := []*envoy_route_v3.VirtualHost{
		{Name: "bar"},
		{Name: "baz"},
		{Name: "foo"},
		{Name: "same", Domains: []string{"z", "y"}},
		{Name: "same", Domains: []string{"a", "b"}},
	}

	have := []*envoy_route_v3.VirtualHost{
		want[3], // Ensure the "same" element stays stable.
		want[4],
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func matchPrefixString(str string) *dag.PrefixMatchCondition {
	return &dag.PrefixMatchCondition{
		Prefix:          str,
		PrefixMatchType: dag.PrefixMatchString,
	}
}

func matchPrefixSegment(str string) *dag.PrefixMatchCondition {
	return &dag.PrefixMatchCondition{
		Prefix:          str,
		PrefixMatchType: dag.PrefixMatchSegment,
	}
}

func matchRegex(str string) *dag.RegexMatchCondition {
	return &dag.RegexMatchCondition{
		Regex: str,
	}
}

func matchExact(str string) *dag.ExactMatchCondition {
	return &dag.ExactMatchCondition{
		Path: str,
	}
}

func invertHeaderMatch(h dag.HeaderMatchCondition) dag.HeaderMatchCondition {
	h.Invert = true
	return h
}

func regexHeader(name string, value string) dag.HeaderMatchCondition {
	return dag.HeaderMatchCondition{
		Name:      name,
		MatchType: dag.HeaderMatchTypeRegex,
		Value:     value,
	}
}

func exactHeader(name string, value string) dag.HeaderMatchCondition {
	return dag.HeaderMatchCondition{
		Name:      name,
		MatchType: dag.HeaderMatchTypeExact,
		Value:     value,
	}
}

func containsHeader(name string, value string) dag.HeaderMatchCondition {
	return dag.HeaderMatchCondition{
		Name:      name,
		MatchType: dag.HeaderMatchTypeContains,
		Value:     value,
	}
}

func presentHeader(name string) dag.HeaderMatchCondition {
	return dag.HeaderMatchCondition{
		Name:      name,
		MatchType: dag.HeaderMatchTypePresent,
	}
}

func TestSortRoutesPathMatch(t *testing.T) {
	want := []*dag.Route{
		// Note that exact matches sort before regex matches.
		{
			PathMatchCondition: matchExact("/aab/a"),
		},
		{
			PathMatchCondition: matchExact("/aab"),
		},
		{
			PathMatchCondition: matchExact("/aaa"),
		},
		// Note that regex matches sort before prefix matches.
		{
			PathMatchCondition: matchRegex("/this/is/the/longest"),
		},
		{
			PathMatchCondition: matchRegex(`/foo((\/).*)*`),
		},
		{
			PathMatchCondition: matchRegex("/"),
		},
		{
			PathMatchCondition: matchRegex("."),
		},
		// Prefix segment matches sort before string matches.
		{
			PathMatchCondition: matchPrefixSegment("/path/prefix2"),
		},
		{
			PathMatchCondition: matchPrefixString("/path/prefix2"),
		},
		{
			PathMatchCondition: matchPrefixSegment("/path/prefix/a"),
		},
		{
			PathMatchCondition: matchPrefixString("/path/prefix/a"),
		},
		{
			PathMatchCondition: matchPrefixString("/path/prefix"),
		},
		{
			PathMatchCondition: matchPrefixSegment("/path/p"),
		},
	}

	have := shuffleRoutes(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortRoutesLongestHeaders(t *testing.T) {
	want := []*dag.Route{
		{
			// Although the header names are the same, this value
			// should sort before the next one because it is
			// textually longer.
			PathMatchCondition: matchExact("/pathexact"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("header-name", "header-value"),
			},
		},
		{
			PathMatchCondition: matchExact("/pathexact"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				presentHeader("header-name"),
			},
		},
		{
			PathMatchCondition: matchExact("/pathexact"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("long-header-name", "long-header-value"),
			},
		},
		{
			PathMatchCondition: matchExact("/pathexact"),
		},
		{
			PathMatchCondition: matchRegex("/pathregex"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("header-name", "header-value"),
			},
		},
		{
			PathMatchCondition: matchRegex("/pathregex"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				presentHeader("header-name"),
			},
		},
		{
			PathMatchCondition: matchRegex("/pathregex"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("long-header-name", "long-header-value"),
			},
		},
		{
			PathMatchCondition: matchRegex("/pathregex"),
		},
		{
			PathMatchCondition: matchPrefixSegment("/path"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				presentHeader("header-name"),
			},
		},
		{
			PathMatchCondition: matchPrefixString("/path"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("header-name", "header-value"),
			},
		},
		{
			PathMatchCondition: matchPrefixString("/path"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				presentHeader("header-name"),
			},
		},
		{
			PathMatchCondition: matchPrefixString("/path"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("long-header-name", "long-header-value"),
			},
		},
		{
			PathMatchCondition: matchPrefixString("/path"),
		},
	}

	have := shuffleRoutes(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortRoutesQueryParams(t *testing.T) {
	want := []*dag.Route{
		{

			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				{Name: "query-param-1", Value: "query-value-1"},
				{Name: "query-param-2", Value: "query-value-2"},
				{Name: "query-param-3", Value: "query-value-3"},
			},
		},
		{

			PathMatchCondition: matchExact("/"),
			// If same number of matches, sort on element-by-element
			// comparison of each query param name provided.
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				{Name: "aaa-query-param-1", Value: "query-value-1"},
				{Name: "query-param-2", Value: "query-value-2"},
			},
		},
		{

			PathMatchCondition: matchExact("/"),
			// If same number of matches, sort on element-by-element
			// comparison of each query param name provided.
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				{Name: "query-param-1", Value: "query-value-1"},
				{Name: "bbb-query-param-2", Value: "query-value-2"},
			},
		},
		{

			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				{Name: "query-param-1", Value: "query-value-1"},
				{Name: "query-param-2", Value: "query-value-2"},
			},
		},
		{

			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				{Name: "query-param-1", Value: "query-value-1"},
			},
		},
	}

	have := shuffleRoutes(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortSecrets(t *testing.T) {
	want := []*envoy_tls_v3.Secret{
		{Name: "first"},
		{Name: "second"},
	}

	have := []*envoy_tls_v3.Secret{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortHeaderMatchConditions(t *testing.T) {
	want := []dag.HeaderMatchCondition{
		// Note that if the header names are the same, we
		// order by the type (in order: "exact", "regex", "contains",
		// "present").
		presentHeader("ashort"),
		exactHeader("header-name", "anything"),
		regexHeader("header-name", "a.*regex"),
		containsHeader("header-name", "something"),
		presentHeader("header-name"),
		exactHeader("long-header-name", "long-header-value"),
	}

	have := []dag.HeaderMatchCondition{
		want[5],
		want[4],
		want[3],
		want[0],
		want[2],
		want[1],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortHeaderMatchConditionsInverted(t *testing.T) {
	// Inverted matches sort after standard matches.
	want := []dag.HeaderMatchCondition{
		exactHeader("header-name", "anything"),
		invertHeaderMatch(exactHeader("header-name", "anything")),
		containsHeader("header-name", "something"),
		invertHeaderMatch(containsHeader("header-name", "something")),
		presentHeader("header-name"),
		invertHeaderMatch(presentHeader("header-name")),
	}

	have := []dag.HeaderMatchCondition{
		want[5],
		want[4],
		want[3],
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortHeaderMatchConditionsValue(t *testing.T) {
	// Use string comparison to sort values.
	want := []dag.HeaderMatchCondition{
		exactHeader("header-name", "a"),
		invertHeaderMatch(exactHeader("header-name", "a")),
		exactHeader("header-name", "b"),
		exactHeader("header-name", "c"),
		containsHeader("header-name", "a"),
		invertHeaderMatch(containsHeader("header-name", "a")),
		containsHeader("header-name", "b"),
		containsHeader("header-name", "c"),
	}

	have := []dag.HeaderMatchCondition{
		want[6],
		want[5],
		want[4],
		want[7],
		want[2],
		want[1],
		want[0],
		want[3],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortClusters(t *testing.T) {
	want := []*envoy_cluster_v3.Cluster{
		{Name: "first"},
		{Name: "second"},
	}

	have := []*envoy_cluster_v3.Cluster{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortClusterLoadAssignments(t *testing.T) {
	want := []*envoy_endpoint_v3.ClusterLoadAssignment{
		{ClusterName: "first"},
		{ClusterName: "second"},
	}

	have := []*envoy_endpoint_v3.ClusterLoadAssignment{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortHTTPWeightedClusters(t *testing.T) {
	want := []*envoy_route_v3.WeightedCluster_ClusterWeight{
		{
			Name:   "first",
			Weight: protobuf.UInt32(10),
		},
		{
			Name:   "second",
			Weight: protobuf.UInt32(10),
		},
		{
			Name:   "second",
			Weight: protobuf.UInt32(20),
		},
	}

	have := []*envoy_route_v3.WeightedCluster_ClusterWeight{
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortTCPWeightedClusters(t *testing.T) {
	want := []*tcp.TcpProxy_WeightedCluster_ClusterWeight{
		{
			Name:   "first",
			Weight: 10,
		},
		{
			Name:   "second",
			Weight: 10,
		},
		{
			Name:   "second",
			Weight: 20,
		},
	}

	have := []*tcp.TcpProxy_WeightedCluster_ClusterWeight{
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortListeners(t *testing.T) {
	want := []*envoy_listener_v3.Listener{
		{Name: "first"},
		{Name: "second"},
	}

	have := []*envoy_listener_v3.Listener{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortFilterChains(t *testing.T) {
	names := func(n ...string) *envoy_listener_v3.FilterChainMatch {
		return &envoy_listener_v3.FilterChainMatch{
			ServerNames: n,
		}
	}

	want := []*envoy_listener_v3.FilterChain{
		{
			FilterChainMatch: names("first"),
		},

		// The following two entries should match the order
		// in "have" because we are doing a stable sort, and
		// they are equal since we only compare the first
		// server name.
		{
			FilterChainMatch: names("second", "zzzzz"),
		},
		{
			FilterChainMatch: names("second", "aaaaa"),
		},
		{
			FilterChainMatch: &envoy_listener_v3.FilterChainMatch{},
		},
	}

	have := []*envoy_listener_v3.FilterChain{
		want[1], // zzzzz
		want[3], // blank
		want[2], // aaaaa
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}
