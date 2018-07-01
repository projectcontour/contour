// Copyright © 2017 Heptio
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

package contour

import (
	"reflect"
	"sort"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
)

func TestCacheInsert(t *testing.T) {
	var val, val2 v2.ClusterLoadAssignment

	tests := map[string]*struct {
		cache
		key   string
		value proto.Message
		want  map[string]proto.Message
	}{
		"empty, add new key": {
			key:   "alpha",
			value: &val,
			want: map[string]proto.Message{
				"alpha": &val,
			},
		},
		"one key, add second": {
			cache: cache{
				entries: map[string]proto.Message{
					"alpha": &val,
				},
			},
			key:   "beta",
			value: &val,
			want: map[string]proto.Message{
				"alpha": &val,
				"beta":  &val,
			},
		},
		"one key overwritten": {
			cache: cache{
				entries: map[string]proto.Message{
					"alpha": &val,
				},
			},
			key:   "alpha",
			value: &val2,
			want: map[string]proto.Message{
				"alpha": &val2,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.cache.insert(tc.key, tc.value)
			if !reflect.DeepEqual(tc.cache.entries, tc.want) {
				t.Fatalf("expected: %#v, got %#v", tc.want, tc.cache.entries)
			}
		})
	}
}

func TestCacheRemove(t *testing.T) {
	var val v2.ClusterLoadAssignment

	tests := map[string]*struct {
		cache
		key  string
		want map[string]proto.Message
	}{
		"one key, remove": {
			cache: cache{
				entries: map[string]proto.Message{
					"alpha": &val,
				},
			},
			key:  "alpha",
			want: map[string]proto.Message{},
		},
		"one key, remove unrelated": {
			cache: cache{
				entries: map[string]proto.Message{
					"alpha": &val,
				},
			},
			key: "beta",
			want: map[string]proto.Message{
				"alpha": &val,
			},
		},
		"empty, remove anything": {
			key:  "alpha",
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.cache.remove(tc.key)
			if !reflect.DeepEqual(tc.cache.entries, tc.want) {
				t.Fatalf("expected: %#v, got %#v", tc.want, tc.cache.entries)
			}
		})
	}
}

func TestCacheValues(t *testing.T) {
	var (
		c  cache
		c1 = v2.ClusterLoadAssignment{
			ClusterName: "c1",
		}
		c2 = v2.ClusterLoadAssignment{
			ClusterName: "c2",
		}
		c3 = v2.ClusterLoadAssignment{
			ClusterName: "c3",
		}
	)

	c.insert("c1", &c1)
	c.insert("c2", &c2)
	c.insert("c3", &c3)

	tests := map[string]*struct {
		filter func(string) bool
		want   []proto.Message
	}{
		"match none": {
			filter: func(string) bool { return false },
			want:   []proto.Message{}, // not nil TODO(dfc) should Values return nil if len(values) == 0
		},
		"match all": {
			filter: func(string) bool { return true },
			want: []proto.Message{
				&c1, &c2, &c3, // sorted to match sort of got
			},
		},
		"match c3": {
			filter: func(s string) bool { return s == "c3" },
			want: []proto.Message{
				&c3,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := c.Values(tc.filter)
			sort.Stable(clusterLoadAssignmentsByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected: %#v, got %#v", tc.want, got)
			}
		})
	}
}
