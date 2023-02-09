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
	"math"
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
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func shuffleSlice[T any](original []T) []T {
	shuffled := make([]T, len(original))
	copy(shuffled, original)
	rand.Shuffle(len(original), func(i, j int) {
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

func ignoreCaseQueryParam(h dag.QueryParamMatchCondition) dag.QueryParamMatchCondition {
	h.IgnoreCase = true
	return h
}

func exactQueryParam(name string, value string) dag.QueryParamMatchCondition {
	return dag.QueryParamMatchCondition{
		Name:      name,
		MatchType: dag.QueryParamMatchTypeExact,
		Value:     value,
	}
}

func prefixQueryParam(name string, value string) dag.QueryParamMatchCondition {
	return dag.QueryParamMatchCondition{
		Name:      name,
		MatchType: dag.QueryParamMatchTypePrefix,
		Value:     value,
	}
}

func suffixQueryParam(name string, value string) dag.QueryParamMatchCondition {
	return dag.QueryParamMatchCondition{
		Name:      name,
		MatchType: dag.QueryParamMatchTypeSuffix,
		Value:     value,
	}
}

func regexQueryParam(name string, value string) dag.QueryParamMatchCondition {
	return dag.QueryParamMatchCondition{
		Name:      name,
		MatchType: dag.QueryParamMatchTypeRegex,
		Value:     value,
	}
}

func containsQueryParam(name string, value string) dag.QueryParamMatchCondition {
	return dag.QueryParamMatchCondition{
		Name:      name,
		MatchType: dag.QueryParamMatchTypeContains,
		Value:     value,
	}
}

func presentQueryParam(name string) dag.QueryParamMatchCondition {
	return dag.QueryParamMatchCondition{
		Name:      name,
		MatchType: dag.QueryParamMatchTypePresent,
	}
}

// Routes that have a higher priority should be sorted first when compared to
// others that have identical path matches, number of header matches, and
// number of query matches.
// This is mainly to support Gateway API route match preference.
// See: https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.HTTPRouteRule
func TestSortRoutesPriority(t *testing.T) {
	want := []*dag.Route{
		{
			// Highest priority so this sorts first.
			Priority:           0,
			PathMatchCondition: matchPrefixSegment("/"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				presentHeader("a-header-name"),
			},
		},
		{
			// This route sorts ahead of the next one since the priority
			// is higher, even though the header match would normally
			// sort it after.
			Priority:           1,
			PathMatchCondition: matchPrefixSegment("/"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				presentHeader("z-header-name"),
			},
		},
		{
			Priority:           2,
			PathMatchCondition: matchPrefixSegment("/"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				presentHeader("a-header-name"),
			},
		},
		{
			// This route sorts ahead of the next one since the priority
			// is higher, even though the query match would normally
			// sort it after.
			Priority:           2,
			PathMatchCondition: matchPrefixSegment("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				containsQueryParam("z-query-param", "query-value"),
			},
		},
		{
			// Same priority as the next one, so sorted on query param match
			// name.
			Priority:           3,
			PathMatchCondition: matchPrefixSegment("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				exactQueryParam("a-query-param", "query-value"),
			},
		},
		{
			Priority:           3,
			PathMatchCondition: matchPrefixSegment("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				exactQueryParam("b-query-param", "query-value"),
			},
		},
		{
			Priority:           math.MaxUint8,
			PathMatchCondition: matchPrefixSegment("/"),
		},
	}

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
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

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortRoutesLongestHeaders(t *testing.T) {
	want := []*dag.Route{
		{
			// More conditions sort higher.
			PathMatchCondition: matchExact("/pathexact"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("header-name", "header-value"),
				exactHeader("header-name-2", "header-value"),
				exactHeader("header-name-3", "header-value"),
			},
		},
		{
			// Per-element name comparison means this sorts later.
			PathMatchCondition: matchExact("/pathexact"),
			HeaderMatchConditions: []dag.HeaderMatchCondition{
				exactHeader("header-name", "header-value"),
				exactHeader("header-name-2", "header-value"),
				exactHeader("header-name-999", "header-value"),
			},
		},
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

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortRoutesQueryParams(t *testing.T) {
	want := []*dag.Route{
		{
			PathMatchCondition: matchExact("/"),
			// More match conditions sort higher.
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				containsQueryParam("query-param-1", "query-value-1"),
				containsQueryParam("query-param-2", "query-value-2"),
				containsQueryParam("query-param-3", "query-value-3"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			// If same number of matches, sort on element-by-element
			// comparison of query param name.
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				exactQueryParam("aaa-param", "val"),
				exactQueryParam("query-param-1", "query-value-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				exactQueryParam("bbb-param", "val"),
				exactQueryParam("query-param-1", "query-value-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			// Exact is most specific.
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				exactQueryParam("query-param-1", "query-value-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			// If matching type, sort on value.
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				exactQueryParam("query-param-1", "query-value-1-other"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			// If matching type, value, sort on case sensitivity.
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				ignoreCaseQueryParam(exactQueryParam("query-param-1", "query-value-1-other")),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				regexQueryParam("query-param-1", "query-value-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				prefixQueryParam("query-param-1", "query-value-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				suffixQueryParam("query-param-1", "query-value-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				containsQueryParam("query-param-1", "query-value-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				presentQueryParam("query-param-1"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				prefixQueryParam("query-param-2", "query-value-2"),
			},
		},
		{
			PathMatchCondition: matchExact("/"),
			QueryParamMatchConditions: []dag.QueryParamMatchCondition{
				prefixQueryParam("query-param-2", "query-value-2-after"),
			},
		},
	}

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortSecrets(t *testing.T) {
	want := []*envoy_tls_v3.Secret{
		{Name: "first"},
		{Name: "second"},
	}

	have := shuffleSlice(want)

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

	have := shuffleSlice(want)

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

	have := shuffleSlice(want)

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

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortQueryParamMatchConditionsValue(t *testing.T) {
	want := []dag.QueryParamMatchCondition{
		exactQueryParam("query-name", "a"),
		ignoreCaseQueryParam(exactQueryParam("query-name", "a")),
		exactQueryParam("query-name", "b"),
		exactQueryParam("query-name", "c"),
		regexQueryParam("query-name", "a"),
		ignoreCaseQueryParam(regexQueryParam("query-name", "a")),
		regexQueryParam("query-name", "b"),
		prefixQueryParam("query-name", "a"),
		ignoreCaseQueryParam(prefixQueryParam("query-name", "a")),
		prefixQueryParam("query-name", "b"),
		suffixQueryParam("query-name", "a"),
		ignoreCaseQueryParam(suffixQueryParam("query-name", "a")),
		suffixQueryParam("query-name", "b"),
		containsQueryParam("query-name", "a"),
		ignoreCaseQueryParam(containsQueryParam("query-name", "a")),
		containsQueryParam("query-name", "b"),
		containsQueryParam("query-name", "c"),
	}

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortClusters(t *testing.T) {
	want := []*envoy_cluster_v3.Cluster{
		{Name: "first"},
		{Name: "second"},
	}

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortClusterLoadAssignments(t *testing.T) {
	want := []*envoy_endpoint_v3.ClusterLoadAssignment{
		{ClusterName: "first"},
		{ClusterName: "second"},
	}

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortHTTPWeightedClusters(t *testing.T) {
	want := []*envoy_route_v3.WeightedCluster_ClusterWeight{
		{
			Name:   "first",
			Weight: wrapperspb.UInt32(10),
		},
		{
			Name:   "second",
			Weight: wrapperspb.UInt32(10),
		},
		{
			Name:   "second",
			Weight: wrapperspb.UInt32(20),
		},
	}

	have := shuffleSlice(want)

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

	have := shuffleSlice(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortListeners(t *testing.T) {
	want := []*envoy_listener_v3.Listener{
		{Name: "first"},
		{Name: "second"},
	}

	have := shuffleSlice(want)

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
