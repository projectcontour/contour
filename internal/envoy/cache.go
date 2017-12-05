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

package envoy

import (
	"sort"

	v2 "github.com/envoyproxy/go-control-plane/api"
)

// clusterCache is a thread safe, atomic, copy on write cache of *v2.Cluster objects.
type clusterCache chan []*v2.Cluster

// init must be called before clusterCache is used for the first time.
func (cc *clusterCache) init() {
	*cc = make(clusterCache, 1)
	*cc <- nil // prime cache
}

// Values returns a copy of the contents of the cache.
func (cc clusterCache) Values() []*v2.Cluster {
	v := <-cc
	r := make([]*v2.Cluster, len(v))
	copy(r, v)
	cc <- v
	return r
}

// with executes f with the value of the stored in the cache.
// the value returned from f replaces the contents in the cache.
func (cc clusterCache) with(f func([]*v2.Cluster) []*v2.Cluster) {
	v := <-cc
	v = f(v)
	// TODO(dfc) Add and Remove do not (currently) affect the sort order
	// so it might be possible to avoid always sorting.
	sort.Sort(clusterByName(v))
	cc <- v
}

// Add adds an entry to the cache. If a Cluster with the same
// name exists, it is replaced.
// TODO(dfc) make Add variadic to support atomic addition of several clusters
// also niladic Add can be used as a no-op notify for watchers.
func (cc clusterCache) Add(c *v2.Cluster) {
	cc.with(func(in []*v2.Cluster) []*v2.Cluster {
		sort.Sort(clusterByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].Name >= c.Name })
		if i < len(in) && in[i].Name == c.Name {
			// c is already present, replace
			in[i] = c
			return in
		}
		// c is not present, append and sort
		in = append(in, c)
		sort.Sort(clusterByName(in))
		return in
	})
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (cc clusterCache) Remove(name string) {
	cc.with(func(in []*v2.Cluster) []*v2.Cluster {
		sort.Sort(clusterByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].Name >= name })
		if i < len(in) && in[i].Name == name {
			// c is present, remove
			in = append(in[:i], in[i+1:]...)
		}
		return in
	})
}

type clusterByName []*v2.Cluster

func (c clusterByName) Len() int           { return len(c) }
func (c clusterByName) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c clusterByName) Less(i, j int) bool { return c[i].Name < c[j].Name }

// clusterLoadAssignmentCache is a thread safe, atomic, copy on write cache of v2.ClusterLoadAssignment objects.
type clusterLoadAssignmentCache chan []*v2.ClusterLoadAssignment

// init must be called before clusterLoadAssignmentCache is used for the first time.
func (c *clusterLoadAssignmentCache) init() {
	*c = make(clusterLoadAssignmentCache, 1)
	*c <- nil // prime cache
}

// Values returns a copy of the contents of the cache.
func (c clusterLoadAssignmentCache) Values() []*v2.ClusterLoadAssignment {
	v := <-c
	r := make([]*v2.ClusterLoadAssignment, len(v))
	copy(r, v)
	c <- v
	return r
}

// with executes f with the value of the stored in the cache.
// the value returned from f replaces the contents in the cache.
func (c clusterLoadAssignmentCache) with(f func([]*v2.ClusterLoadAssignment) []*v2.ClusterLoadAssignment) {
	v := <-c
	v = f(v)
	// TODO(dfc) Add and Remove do not (currently) affect the sort order
	// so it might be possible to avoid always sorting.
	sort.Sort(clusterLoadAssignmentsByName(v))
	c <- v
}

// Add adds an entry to the cache. If a ClusterLoadAssignment with the same
// name exists, it is replaced.
// TODO(dfc) make Add variadic to support atomic addition of several clusterLoadAssignments
// also niladic Add can be used as a no-op notify for watchers.
func (c clusterLoadAssignmentCache) Add(e *v2.ClusterLoadAssignment) {
	c.with(func(in []*v2.ClusterLoadAssignment) []*v2.ClusterLoadAssignment {
		sort.Sort(clusterLoadAssignmentsByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].ClusterName >= e.ClusterName })
		if i < len(in) && in[i].ClusterName == e.ClusterName {
			in[i] = e
			return in
		}
		in = append(in, e)
		sort.Sort(clusterLoadAssignmentsByName(in))
		return in
	})
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (c clusterLoadAssignmentCache) Remove(name string) {
	c.with(func(in []*v2.ClusterLoadAssignment) []*v2.ClusterLoadAssignment {
		sort.Sort(clusterLoadAssignmentsByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].ClusterName >= name })
		if i < len(in) && in[i].ClusterName == name {
			// c is present, remove
			in = append(in[:i], in[i+1:]...)
		}
		return in
	})
}

type clusterLoadAssignmentsByName []*v2.ClusterLoadAssignment

func (c clusterLoadAssignmentsByName) Len() int           { return len(c) }
func (c clusterLoadAssignmentsByName) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c clusterLoadAssignmentsByName) Less(i, j int) bool { return c[i].ClusterName < c[j].ClusterName }

// ListenerCache is a thread safe, atomic, copy on write cache of v2.Listener objects.
type listenerCache chan []*v2.Listener

// init must be called before listenerCache is used for the first time.
func (lc *listenerCache) init() {
	*lc = make(listenerCache, 1)
	*lc <- nil // prime cache
}

// Values returns a copy of the contents of the cache.
func (lc listenerCache) Values() []*v2.Listener {
	v := <-lc
	r := make([]*v2.Listener, len(v))
	copy(r, v)
	lc <- v
	return r
}

// with executes f with the value of the stored in the cache.
// the value returned from f replaces the contents in the cache.
func (lc listenerCache) with(f func([]*v2.Listener) []*v2.Listener) {
	l := <-lc
	l = f(l)
	// TODO(dfc) Add and Remove do not (currently) affect the sort order
	// so it might be possible to avoid always sorting.
	sort.Sort(listenersByName(l))
	lc <- l
}

// Add adds an entry to the cache. If a Listener with the same
// name exists, it is replaced.
// TODO(dfc) make Add variadic to support atomic addition of several listeners
// also niladic Add can be used as a no-op notify for watchers.
func (lc listenerCache) Add(r *v2.Listener) {
	lc.with(func(in []*v2.Listener) []*v2.Listener {
		sort.Sort(listenersByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].Name >= r.Name })
		if i < len(in) && in[i].Name == r.Name {
			// c is already present, replace
			in[i] = r
			return in
		}
		// c is not present, append and sort
		in = append(in, r)
		sort.Sort(listenersByName(in))
		return in
	})
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (lc listenerCache) Remove(name string) {
	lc.with(func(in []*v2.Listener) []*v2.Listener {
		sort.Sort(listenersByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].Name >= name })
		if i < len(in) && in[i].Name == name {
			// c is present, remove
			in = append(in[:i], in[i+1:]...)
		}
		return in
	})
}

type listenersByName []*v2.Listener

func (l listenersByName) Len() int           { return len(l) }
func (l listenersByName) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l listenersByName) Less(i, j int) bool { return l[i].Name < l[j].Name }

// clusterLoadAssignmentCache is a thread safe, atomic, copy on write cache of v2.ClusterLoadAssignment objects.

// VirtualHostCache is a thread safe, atomic, copy on write cache of v2.VirtualHost objects.
type virtualHostCache chan []*v2.VirtualHost

// init must be called before virtualHostCache is used for the first time.
func (vc *virtualHostCache) init() {
	*vc = make(virtualHostCache, 1)
	*vc <- nil // prime cache
}

// Values returns a copy of the contents of the cache.
func (vc virtualHostCache) Values() []*v2.VirtualHost {
	v := <-vc
	r := make([]*v2.VirtualHost, len(v))
	copy(r, v)
	vc <- v
	return r
}

// with executes f with the value of the stored in the cache.
// the value returned from f replaces the contents in the cache.
func (vc virtualHostCache) with(f func([]*v2.VirtualHost) []*v2.VirtualHost) {
	v := <-vc
	v = f(v)
	// TODO(dfc) Add and Remove do not (currently) affect the sort order
	// so it might be possible to avoid always sorting.
	sort.Sort(virtualHostsByName(v))
	vc <- v
}

// Add adds an entry to the cache. If a VirtualHost with the same
// name exists, it is replaced.
// TODO(dfc) make Add variadic to support atomic addition of several clusters
// also niladic Add can be used as a no-op notify for watchers.
func (vc virtualHostCache) Add(r *v2.VirtualHost) {
	vc.with(func(in []*v2.VirtualHost) []*v2.VirtualHost {
		sort.Sort(virtualHostsByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].Name >= r.Name })
		if i < len(in) && in[i].Name == r.Name {
			// c is already present, replace
			in[i] = r
			return in
		}
		// c is not present, append and sort
		in = append(in, r)
		sort.Sort(virtualHostsByName(in))
		return in
	})
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (vc virtualHostCache) Remove(name string) {
	vc.with(func(in []*v2.VirtualHost) []*v2.VirtualHost {
		sort.Sort(virtualHostsByName(in))
		i := sort.Search(len(in), func(i int) bool { return in[i].Name >= name })
		if i < len(in) && in[i].Name == name {
			// c is present, remove
			in = append(in[:i], in[i+1:]...)
		}
		return in
	})
}

type virtualHostsByName []*v2.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }
