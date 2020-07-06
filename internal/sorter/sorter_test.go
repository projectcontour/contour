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
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	tcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestInvalidSorter(t *testing.T) {
	assert.Equal(t, nil, For([]string{"invalid"}))
}

func TestSortRouteConfiguration(t *testing.T) {
	want := []*v2.RouteConfiguration{
		&v2.RouteConfiguration{Name: "bar"},
		&v2.RouteConfiguration{Name: "baz"},
		&v2.RouteConfiguration{Name: "foo"},
		&v2.RouteConfiguration{Name: "same", InternalOnlyHeaders: []string{"z", "y"}},
		&v2.RouteConfiguration{Name: "same", InternalOnlyHeaders: []string{"a", "b"}},
	}

	have := []*v2.RouteConfiguration{
		want[3], // Ensure the "same" element stays stable.
		want[4],
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortVirtualHosts(t *testing.T) {
	want := []*envoy_api_v2_route.VirtualHost{
		&envoy_api_v2_route.VirtualHost{Name: "bar"},
		&envoy_api_v2_route.VirtualHost{Name: "baz"},
		&envoy_api_v2_route.VirtualHost{Name: "foo"},
		&envoy_api_v2_route.VirtualHost{Name: "same", Domains: []string{"z", "y"}},
		&envoy_api_v2_route.VirtualHost{Name: "same", Domains: []string{"a", "b"}},
	}

	have := []*envoy_api_v2_route.VirtualHost{
		want[3], // Ensure the "same" element stays stable.
		want[4],
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func matchPrefix(str string) *envoy_api_v2_route.RouteMatch_Prefix {
	return &envoy_api_v2_route.RouteMatch_Prefix{
		Prefix: str,
	}
}

func matchRegex(str string) *envoy_api_v2_route.RouteMatch_SafeRegex {
	return &envoy_api_v2_route.RouteMatch_SafeRegex{
		SafeRegex: &matcher.RegexMatcher{
			Regex: str,
		},
	}
}

func exactHeader(name string, value string) *envoy_api_v2_route.HeaderMatcher {
	return &envoy_api_v2_route.HeaderMatcher{
		Name: name,
		HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_ExactMatch{
			ExactMatch: value,
		},
	}
}

func presentHeader(name string) *envoy_api_v2_route.HeaderMatcher {
	return &envoy_api_v2_route.HeaderMatcher{
		Name: name,
		HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_PresentMatch{
			PresentMatch: true,
		},
	}
}

func TestSortRoutesLongestPath(t *testing.T) {
	want := []*envoy_api_v2_route.Route{
		&envoy_api_v2_route.Route{
			Match: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: matchRegex("/this/is/the/longest"),
			}},

		// Note that regex matches sort before prefix matches.
		&envoy_api_v2_route.Route{
			Match: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: matchRegex("."),
			}},

		&envoy_api_v2_route.Route{
			Match: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: matchPrefix("/path/prefix2"),
			}},

		&envoy_api_v2_route.Route{
			Match: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: matchPrefix("/path/prefix"),
			}},
	}

	have := []*envoy_api_v2_route.Route{
		want[1],
		want[3],
		want[0],
		want[2],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortRoutesLongestHeaders(t *testing.T) {
	want := []*envoy_api_v2_route.Route{
		// Although the header names are the same, this value
		// should sort before the next one because it is
		// textually longer.
		&envoy_api_v2_route.Route{
			Match: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: matchPrefix("/path"),
				Headers: []*envoy_api_v2_route.HeaderMatcher{
					exactHeader("header-name", "header-value"),
				},
			}},
		&envoy_api_v2_route.Route{
			Match: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: matchPrefix("/path"),
				Headers: []*envoy_api_v2_route.HeaderMatcher{
					presentHeader("header-name"),
				},
			}},
		&envoy_api_v2_route.Route{
			Match: &envoy_api_v2_route.RouteMatch{
				PathSpecifier: matchPrefix("/path"),
				Headers: []*envoy_api_v2_route.HeaderMatcher{
					exactHeader("long-header-name", "long-header-value"),
				},
			}},
	}

	have := []*envoy_api_v2_route.Route{
		want[1],
		want[0],
		want[2],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortSecrets(t *testing.T) {
	want := []*envoy_api_v2_auth.Secret{
		&envoy_api_v2_auth.Secret{Name: "first"},
		&envoy_api_v2_auth.Secret{Name: "second"},
	}

	have := []*envoy_api_v2_auth.Secret{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortHeaderMatchers(t *testing.T) {
	want := []*envoy_api_v2_route.HeaderMatcher{
		// Note that if the header names are the same, we
		// order by the protobuf string, in which case "exact"
		// is less than "present".
		exactHeader("header-name", "anything"),
		presentHeader("header-name"),
		exactHeader("long-header-name", "long-header-value"),
	}

	have := []*envoy_api_v2_route.HeaderMatcher{
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortClusters(t *testing.T) {
	want := []*v2.Cluster{
		&v2.Cluster{Name: "first"},
		&v2.Cluster{Name: "second"},
	}

	have := []*v2.Cluster{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortClusterLoadAssignments(t *testing.T) {
	want := []*v2.ClusterLoadAssignment{
		&v2.ClusterLoadAssignment{ClusterName: "first"},
		&v2.ClusterLoadAssignment{ClusterName: "second"},
	}

	have := []*v2.ClusterLoadAssignment{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortHTTPWeightedClusters(t *testing.T) {
	want := []*envoy_api_v2_route.WeightedCluster_ClusterWeight{
		&envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   "first",
			Weight: protobuf.UInt32(10),
		},
		&envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   "second",
			Weight: protobuf.UInt32(10),
		},
		&envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   "second",
			Weight: protobuf.UInt32(20),
		},
	}

	have := []*envoy_api_v2_route.WeightedCluster_ClusterWeight{
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortTCPWeightedClusters(t *testing.T) {
	want := []*tcp.TcpProxy_WeightedCluster_ClusterWeight{
		&tcp.TcpProxy_WeightedCluster_ClusterWeight{
			Name:   "first",
			Weight: 10,
		},
		&tcp.TcpProxy_WeightedCluster_ClusterWeight{
			Name:   "second",
			Weight: 10,
		},
		&tcp.TcpProxy_WeightedCluster_ClusterWeight{
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
	assert.Equal(t, have, want)
}

func TestSortListeners(t *testing.T) {
	want := []*v2.Listener{
		&v2.Listener{Name: "first"},
		&v2.Listener{Name: "second"},
	}

	have := []*v2.Listener{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortFilterChains(t *testing.T) {
	names := func(n ...string) *envoy_api_v2_listener.FilterChainMatch {
		return &envoy_api_v2_listener.FilterChainMatch{
			ServerNames: n,
		}
	}

	want := []*envoy_api_v2_listener.FilterChain{
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
			FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{},
		},
	}

	have := []*envoy_api_v2_listener.FilterChain{
		want[1], // zzzzz
		want[3], // blank
		want[2], // aaaaa
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}
