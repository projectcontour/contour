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
	"sort"
	"sync"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
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
func (c *cache) Values() []proto.Message {
	c.mu.Lock()
	values := make([]proto.Message, 0, len(c.entries))
	for _, v := range c.entries {
		values = append(values, v)
	}
	c.mu.Unlock()
	return values
}

// clusterCache is a thread safe, atomic, copy on write cache of *v2.Cluster objects.
type clusterCache struct {
	cache
}

// Add adds an entry to the cache. If a Cluster with the same
// name exists, it is replaced.
func (cc *clusterCache) Add(clusters ...*v2.Cluster) {
	for _, c := range clusters {
		cc.insert(c.Name, c)
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (cc *clusterCache) Remove(names ...string) {
	for _, n := range names {
		cc.remove(n)
	}
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

// ListenerCache is a thread safe, atomic, copy on write cache of v2.Listener objects.
type listenerCache struct {
	cache
}

// Add adds an entry to the cache. If a Listener with the same
// name exists, it is replaced.
func (lc *listenerCache) Add(listeners ...*v2.Listener) {
	for _, l := range listeners {
		lc.insert(l.Name, l)
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (lc *listenerCache) Remove(names ...string) {
	for _, n := range names {
		lc.remove(n)
	}
}

// VirtualHostCache is a thread safe, atomic, copy on write cache of route.VirtualHost objects.
type virtualHostCache struct {
	cache
}

// Values returns a copy of the contents of the cache.
// Although internally we store pointers to route.VirtualHost
// items, because the output of Values is used in a v2.RouteConfiguration
// rather than copying the pointer values, we create a slice of dereferenced
// values, this creates a copy of each element in the cache.
func (vc *virtualHostCache) Values() []route.VirtualHost {
	values := []route.VirtualHost{}
	for _, v := range vc.cache.Values() {
		values = append(values, *v.(*route.VirtualHost))
	}
	sort.Sort(virtualHostsByName(values))
	return values
}

// Add adds an entry to the cache. If a VirtualHost with the same
// name exists, it is replaced.
func (vc *virtualHostCache) Add(virtualhosts ...*route.VirtualHost) {
next:
	for _, v := range virtualhosts {
		if v.Name == "" {
			logrus.WithField("virtualhost", v).Println("skipping VirtualHost with empty name")
			continue
		}
		if len(v.Domains) == 0 {
			logrus.WithField("virtualhost", v).Println("skipping VirtualHost with blank domain list")
			continue
		}
		for _, d := range v.Domains {
			if d == "" {
				logrus.WithField("virtualhost", v).Println("skipping VirtualHost with blank entry in domain list")
				continue next
			}
		}
		vc.insert(v.Name, v)
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (vc *virtualHostCache) Remove(names ...string) {
	for _, n := range names {
		vc.remove(n)
	}
}

type virtualHostsByName []route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }
