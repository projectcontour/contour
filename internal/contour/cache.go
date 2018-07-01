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
	"sync"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
)

// cache holds a set of objects confirming to the proto.Message interface
type cache struct {
	mu      sync.Mutex
	entries map[string]proto.Message
}

// insert inserts the value into the cache with the key name.
func (c *cache) insert(name string, value proto.Message) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]proto.Message)
	}
	c.entries[name] = value
	c.mu.Unlock()
}

// remote removes a value from the cache.
func (c *cache) remove(name string) {
	c.mu.Lock()
	delete(c.entries, name)
	c.mu.Unlock()
}

// Values returns a slice of the value stored in the cache.
func (c *cache) Values(filter func(string) bool) []proto.Message {
	c.mu.Lock()
	values := make([]proto.Message, 0, len(c.entries))
	for n, v := range c.entries {
		if filter(n) {
			values = append(values, v)
		}
	}
	c.mu.Unlock()
	return values
}

// clusterLoadAssignmentCache is a thread safe, atomic, copy on write cache of v2.ClusterLoadAssignment objects.
type clusterLoadAssignmentCache struct {
	cache
}

// Add adds an entry to the cache. If a ClusterLoadAssignment with the same
// name exists, it is replaced.
func (c *clusterLoadAssignmentCache) Add(assignments ...*v2.ClusterLoadAssignment) {
	for _, a := range assignments {
		c.insert(a.ClusterName, a)
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (c *clusterLoadAssignmentCache) Remove(names ...string) {
	for _, n := range names {
		c.remove(n)
	}
}
