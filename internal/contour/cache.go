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
)

// clusterCache is a thread safe, atomic, copy on write cache of *v2.Cluster objects.
type clusterCache struct {
	sync.Mutex
	values []*v2.Cluster
}

// Values returns a copy of the contents of the cache.
func (cc *clusterCache) Values() []*v2.Cluster {
	cc.Lock()
	r := append([]*v2.Cluster{}, cc.values...)
	cc.Unlock()
	return r
}

// Add adds an entry to the cache. If a Cluster with the same
// name exists, it is replaced.
func (cc *clusterCache) Add(clusters ...*v2.Cluster) {
	if len(clusters) == 0 {
		return
	}
	cc.Lock()
	sort.Sort(clusterByName(cc.values))
	for _, c := range clusters {
		cc.add(c)
	}
	cc.Unlock()
}

// add adds c to the cache. If c is already present, the cached value of c is overwritten.
// invariant: cc.values should be sorted on entry.
func (cc *clusterCache) add(c *v2.Cluster) {
	i := sort.Search(len(cc.values), func(i int) bool { return cc.values[i].Name >= c.Name })
	if i < len(cc.values) && cc.values[i].Name == c.Name {
		// c is already present, replace
		cc.values[i] = c
	} else {
		// c is not present, append
		cc.values = append(cc.values, c)
		// restort to convert append into insert
		sort.Sort(clusterByName(cc.values))
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (cc *clusterCache) Remove(names ...string) {
	if len(names) == 0 {
		return
	}
	cc.Lock()
	sort.Sort(clusterByName(cc.values))
	for _, n := range names {
		cc.remove(n)
	}
	cc.Unlock()
}

// remove removes the named entry from the cache.
// invariant: cc.values should be sorted on entry.
func (cc *clusterCache) remove(name string) {
	i := sort.Search(len(cc.values), func(i int) bool { return cc.values[i].Name >= name })
	if i < len(cc.values) && cc.values[i].Name == name {
		// c is present, remove
		cc.values = append(cc.values[:i], cc.values[i+1:]...)
	}
}

type clusterByName []*v2.Cluster

func (c clusterByName) Len() int           { return len(c) }
func (c clusterByName) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c clusterByName) Less(i, j int) bool { return c[i].Name < c[j].Name }

// clusterLoadAssignmentCache is a thread safe, atomic, copy on write cache of v2.ClusterLoadAssignment objects.
type clusterLoadAssignmentCache struct {
	sync.Mutex
	values []*v2.ClusterLoadAssignment
}

// Values returns a copy of the contents of the cache.
func (c *clusterLoadAssignmentCache) Values() []*v2.ClusterLoadAssignment {
	c.Lock()
	r := append([]*v2.ClusterLoadAssignment{}, c.values...)
	c.Unlock()
	return r
}

// Add adds an entry to the cache. If a ClusterLoadAssignment with the same
// name exists, it is replaced.
func (c *clusterLoadAssignmentCache) Add(assignments ...*v2.ClusterLoadAssignment) {
	if len(assignments) == 0 {
		return
	}
	c.Lock()
	sort.Sort(clusterLoadAssignmentsByName(c.values))
	for _, a := range assignments {
		c.add(a)
	}
	c.Unlock()
}

// add adds a to the cache. If a is already present, the cached value of a is overwritten.
// invariant: c.values should be sorted on entry.
func (c *clusterLoadAssignmentCache) add(a *v2.ClusterLoadAssignment) {
	i := sort.Search(len(c.values), func(i int) bool { return c.values[i].ClusterName >= a.ClusterName })
	if i < len(c.values) && c.values[i].ClusterName == a.ClusterName {
		c.values[i] = a
	} else {
		c.values = append(c.values, a)
		sort.Sort(clusterLoadAssignmentsByName(c.values))
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (c *clusterLoadAssignmentCache) Remove(names ...string) {
	if len(names) == 0 {
		return
	}
	c.Lock()
	sort.Sort(clusterLoadAssignmentsByName(c.values))
	for _, n := range names {
		c.remove(n)
	}
	c.Unlock()
}

// remove removes the named entry from the cache.
// invariant: c.values should be sorted on entry.
func (c *clusterLoadAssignmentCache) remove(name string) {
	i := sort.Search(len(c.values), func(i int) bool { return c.values[i].ClusterName >= name })
	if i < len(c.values) && c.values[i].ClusterName == name {
		// c is present, remove
		c.values = append(c.values[:i], c.values[i+1:]...)
	}
}

type clusterLoadAssignmentsByName []*v2.ClusterLoadAssignment

func (c clusterLoadAssignmentsByName) Len() int           { return len(c) }
func (c clusterLoadAssignmentsByName) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c clusterLoadAssignmentsByName) Less(i, j int) bool { return c[i].ClusterName < c[j].ClusterName }

// ListenerCache is a thread safe, atomic, copy on write cache of v2.Listener objects.
type listenerCache struct {
	sync.Mutex
	values []*v2.Listener
}

// Values returns a copy of the contents of the cache.
func (lc *listenerCache) Values() []*v2.Listener {
	lc.Lock()
	r := append([]*v2.Listener{}, lc.values...)
	lc.Unlock()
	return r
}

// Add adds an entry to the cache. If a Listener with the same
// name exists, it is replaced.
func (lc *listenerCache) Add(listeners ...*v2.Listener) {
	if len(listeners) == 0 {
		return
	}
	lc.Lock()
	sort.Sort(listenersByName(lc.values))
	for _, l := range listeners {
		lc.add(l)
	}
	lc.Unlock()
}

// add adds l to the cache. If l is already present, the cached value of l is overwritten.
// invariant: lc.values should be sorted on entry.
func (lc *listenerCache) add(l *v2.Listener) {
	i := sort.Search(len(lc.values), func(i int) bool { return lc.values[i].Name >= l.Name })
	if i < len(lc.values) && lc.values[i].Name == l.Name {
		// c is already present, replace
		lc.values[i] = l
	} else {
		// c is not present, append and sort
		lc.values = append(lc.values, l)
		sort.Sort(listenersByName(lc.values))
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (lc *listenerCache) Remove(names ...string) {
	if len(names) == 0 {
		return
	}
	lc.Lock()
	sort.Sort(listenersByName(lc.values))
	for _, n := range names {
		lc.remove(n)
	}
	lc.Unlock()
}

// remove removes the named entry from the cache.
// invariant: lc.values should be sorted on entry.
func (lc *listenerCache) remove(name string) {
	i := sort.Search(len(lc.values), func(i int) bool { return lc.values[i].Name >= name })
	if i < len(lc.values) && lc.values[i].Name == name {
		// c is present, remove
		lc.values = append(lc.values[:i], lc.values[i+1:]...)
	}
}

type listenersByName []*v2.Listener

func (l listenersByName) Len() int           { return len(l) }
func (l listenersByName) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l listenersByName) Less(i, j int) bool { return l[i].Name < l[j].Name }

// VirtualHostCache is a thread safe, atomic, copy on write cache of route.VirtualHost objects.
type virtualHostCache struct {
	sync.Mutex
	values []route.VirtualHost
}

// Values returns a copy of the contents of the cache.
func (vc *virtualHostCache) Values() []route.VirtualHost {
	vc.Lock()
	r := append([]route.VirtualHost{}, vc.values...)
	vc.Unlock()
	return r
}

// Add adds an entry to the cache. If a VirtualHost with the same
// name exists, it is replaced.
func (vc *virtualHostCache) Add(virtualhosts ...route.VirtualHost) {
	if len(virtualhosts) == 0 {
		return
	}
	vc.Lock()
	sort.Sort(virtualHostsByName(vc.values))
	for _, v := range virtualhosts {
		vc.add(v)
	}
	vc.Unlock()
}

// add adds v to the cache. If v is already present, the cached value of v is overwritten.
// invariant: vc.values should be sorted on entry.
func (vc *virtualHostCache) add(v route.VirtualHost) {
	i := sort.Search(len(vc.values), func(i int) bool { return vc.values[i].Name >= v.Name })
	if i < len(vc.values) && vc.values[i].Name == v.Name {
		// c is already present, replace
		vc.values[i] = v
	} else {
		// c is not present, append and sort
		vc.values = append(vc.values, v)
		sort.Sort(virtualHostsByName(vc.values))
	}
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (vc *virtualHostCache) Remove(names ...string) {
	if len(names) == 0 {
		return
	}
	vc.Lock()
	sort.Sort(virtualHostsByName(vc.values))
	for _, n := range names {
		vc.remove(n)
	}
	vc.Unlock()
}

// remove removes the named entry from the cache.
// invariant: vc.values should be sorted on entry.
func (vc *virtualHostCache) remove(name string) {
	i := sort.Search(len(vc.values), func(i int) bool { return vc.values[i].Name >= name })
	if i < len(vc.values) && vc.values[i].Name == name {
		// c is present, remove
		vc.values = append(vc.values[:i], vc.values[i+1:]...)
	}
}

type virtualHostsByName []route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }
