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

package dag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func TestVirtualHostValid(t *testing.T) {
	vh := VirtualHost{}
	assert.False(t, vh.Valid())

	vh = VirtualHost{
		Routes: map[string]*Route{
			"/": {},
		},
	}
	assert.True(t, vh.Valid())
}

func TestSecureVirtualHostValid(t *testing.T) {
	vh := SecureVirtualHost{}
	assert.False(t, vh.Valid())

	vh = SecureVirtualHost{
		Secret: new(Secret),
	}
	assert.False(t, vh.Valid())

	vh = SecureVirtualHost{
		VirtualHost: VirtualHost{
			Routes: map[string]*Route{
				"/": {},
			},
		},
	}
	assert.False(t, vh.Valid())

	vh = SecureVirtualHost{
		Secret: new(Secret),
		VirtualHost: VirtualHost{
			Routes: map[string]*Route{
				"/": {},
			},
		},
	}
	assert.True(t, vh.Valid())

	vh = SecureVirtualHost{
		TCPProxy: new(TCPProxy),
	}
	assert.True(t, vh.Valid())

	vh = SecureVirtualHost{
		Secret:   new(Secret),
		TCPProxy: new(TCPProxy),
	}
	assert.True(t, vh.Valid())
}

func TestPeerValidationContext(t *testing.T) {
	pvc1 := PeerValidationContext{
		CACertificates: []*Secret{
			{
				Object: &core_v1.Secret{
					Data: map[string][]byte{
						CACertificateKey: []byte("cacert"),
					},
				},
			},
		},
		SubjectNames: []string{"subject"},
	}
	pvc2 := PeerValidationContext{}
	var pvc3 *PeerValidationContext
	pvc4 := PeerValidationContext{
		CACertificates: []*Secret{
			{
				Object: &core_v1.Secret{
					Data: map[string][]byte{
						CACertificateKey: []byte("-cacert-"),
					},
				},
			},
			{
				Object: &core_v1.Secret{
					Data: map[string][]byte{
						CACertificateKey: []byte("-cacert2-"),
					},
				},
			},
			{
				Object: &core_v1.Secret{
					Data: map[string][]byte{},
				},
			},
			nil,
		},
		SubjectNames: []string{"subject"},
	}
	pvc5 := PeerValidationContext{
		CACertificates: []*Secret{},
	}

	assert.ElementsMatch(t, []string{"subject"}, pvc1.GetSubjectNames())
	assert.Equal(t, []byte("cacert"), pvc1.GetCACertificate())
	assert.Equal(t, []string(nil), pvc2.GetSubjectNames())
	assert.Equal(t, []byte(nil), pvc2.GetCACertificate())
	assert.Equal(t, []string(nil), pvc3.GetSubjectNames())
	assert.Equal(t, []byte(nil), pvc3.GetCACertificate())
	assert.ElementsMatch(t, []string{"subject"}, pvc4.GetSubjectNames())
	assert.Equal(t, []byte("-cacert--cacert2-"), pvc4.GetCACertificate())
	assert.Equal(t, []string(nil), pvc5.GetSubjectNames())
	assert.Equal(t, []byte(nil), pvc5.GetCACertificate())
}

func TestObserverFunc(t *testing.T) {
	// Ensure nil doesn't panic.
	ObserverFunc(nil).OnChange(nil)

	// Ensure the given function gets called.
	result := false
	ObserverFunc(func(*DAG) { result = true }).OnChange(nil)
	require.True(t, result)
}

func TestServiceClusterValid(t *testing.T) {
	invalid := []ServiceCluster{
		{},
		{ClusterName: "foo"},
		{ClusterName: "foo", Services: []WeightedService{{}}},
		{ClusterName: "foo", Services: []WeightedService{{ServiceName: "foo"}}},
		{ClusterName: "foo", Services: []WeightedService{{ServiceNamespace: "foo"}}},
	}

	for _, c := range invalid {
		require.Errorf(t, c.Validate(), "invalid cluster %#v", c)
	}
}

func TestServiceClusterAdd(t *testing.T) {
	port := makeServicePort("foo", "TCP", 32)
	s := ServiceCluster{
		ClusterName: "test",
	}

	s.AddService(types.NamespacedName{Namespace: "ns", Name: "s1"}, port)
	assert.Equal(t,
		ServiceCluster{
			ClusterName: "test",
			Services: []WeightedService{{
				Weight:           1,
				ServiceName:      "s1",
				ServiceNamespace: "ns",
				ServicePort:      port,
			}},
		},
		s)

	s.AddWeightedService(9, types.NamespacedName{Namespace: "ns", Name: "s2"}, port)
	assert.Equal(t,
		ServiceCluster{
			ClusterName: "test",
			Services: []WeightedService{{
				Weight:           1,
				ServiceName:      "s1",
				ServiceNamespace: "ns",
				ServicePort:      port,
			}, {
				Weight:           9,
				ServiceName:      "s2",
				ServiceNamespace: "ns",
				ServicePort:      port,
			}},
		},
		s)
}

func TestServiceClusterRebalance(t *testing.T) {
	port := makeServicePort("foo", "TCP", 32)
	cases := map[string]struct {
		have ServiceCluster
		want ServiceCluster
	}{
		"default weights": {
			have: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
			want: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					Weight:           1,
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					Weight:           1,
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
		},
		"custom weights": {
			have: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					Weight:           6,
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
			want: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					Weight:           0,
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					Weight:           6,
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
		},
	}

	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			s := c.have
			s.Rebalance()
			assert.Equal(t, c.want, s)
		})
	}
}

func TestAddRoute(t *testing.T) {
	cases := map[string]struct {
		vHost       VirtualHost
		rs          []Route
		expectCount int
	}{
		"3 different routes all get added": {
			vHost: VirtualHost{
				Routes: map[string]*Route{},
			},
			rs: []Route{
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "a",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
				},
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "c",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/path1"),
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "f",
					Namespace:          "g",
					PathMatchCondition: prefixSegment("/path1"),
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-2", Value: "value-2", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			expectCount: 3,
		},
		"3 routes, 1 and 2 has header conflict, 2 and 3 has query param conflict, only 1st one gets added": {
			vHost: VirtualHost{
				Routes: map[string]*Route{},
			},
			rs: []Route{
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "a",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
						{Name: "param-2", Value: "value-2", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "c",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "f",
					Namespace:          "g",
					PathMatchCondition: prefixSegment("/path1"),
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			expectCount: 1,
		},
		"3 routes, 1 and 2 has header conflict, 2 and 3 has query param conflict, only 1st one gets added, but all has different paths, all get added": {
			vHost: VirtualHost{
				Routes: map[string]*Route{},
			},
			rs: []Route{
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "a",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/differentpath1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
						{Name: "param-2", Value: "value-2", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "c",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/differentpath2"),
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               contour_v1.KindHTTPProxy,
					Name:               "f",
					Namespace:          "g",
					PathMatchCondition: prefixSegment("/differentpath3"),
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			expectCount: 3,
		},
		"3 routes, 1 and 2 has header conflict, 2 and 3 has query param conflict, only 1st one gets added, but all are not http proxy, all get added": {
			vHost: VirtualHost{
				Routes: map[string]*Route{},
			},
			rs: []Route{
				{
					Kind:               "HTTPRoute",
					Name:               "a",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
						{Name: "param-2", Value: "value-2", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               "HTTPRoute",
					Name:               "c",
					Namespace:          "b",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               "HTTPRoute",
					Name:               "f",
					Namespace:          "g",
					PathMatchCondition: prefixSegment("/path1"),
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			expectCount: 3,
		},
	}

	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			for i := range c.rs {
				c.vHost.AddRoute(&c.rs[i])
			}

			assert.Len(t, c.vHost.Routes, c.expectCount)
		})
	}
}
