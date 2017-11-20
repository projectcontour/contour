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

// ClusterCache represents a cache of computed *v2.Cluster objects.
type ClusterCache interface {
	// Values returns a copy of the contents of the cache.
	Values() []*v2.Cluster

	// Add adds an entry to the cache. If a Cluster with the same
	// name exists, it is replaced.
	Add(*v2.Cluster)

	// Remove removes the named entry from the cache. If the entry
	// is not present in the cache, the operation is a no-op.
	Remove(string)
}

// NewClusterCache returns a new ClusterCache.
func NewClusterCache() ClusterCache {
	cc := make(clusterCache, 1)
	cc <- nil // prime cache
	return cc
}

// clusterCache is a thread safe, atomic, copy on write cache of *v2.Cluster objects.
type clusterCache chan []*v2.Cluster

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

func (c clusterByName) Len() int {
	return len(c)
}

func (c clusterByName) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c clusterByName) Less(i, j int) bool {
	return c[i].Name < c[j].Name
}

// ClusterLoadAssignemntCache represents a cache of computed *v2.ClusterLoadAssignment objects.
type ClusterLoadAssignmentCache interface {
	// Values returns a copy of the contents of the cache.
	Values() []*v2.ClusterLoadAssignment

	// Add adds an entry to the cache. If a ClusterLoadAssignment with the same
	// name exists, it is replaced.
	Add(*v2.ClusterLoadAssignment)

	// Remove removes the named entry from the cache. If the entry
	// is not present in the cache, the operation is a no-op.
	Remove(string)
}

// NewClusterLoadAssignmentCache returns a new ClusterLoadAssignmentCache.
func NewClusterLoadAssignmentCache() ClusterLoadAssignmentCache {
	c := make(clusterLoadAssignmentCache, 1)
	c <- nil // prime cache
	return c
}

// clusterLoadAssignmentCache is a thread safe, atomic, copy on write cache of v2.ClusterLoadAssignment objects.
type clusterLoadAssignmentCache chan []*v2.ClusterLoadAssignment

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

// TODO(dfc) make Add variadic to support atomic addition of several clusters
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
type ListenerCache chan []*v2.Listener

func (lc ListenerCache) Values() []*v2.Listener {
	v := <-lc
	r := make([]*v2.Listener, len(v))
	copy(r, v)
	lc <- v
	return r
}

// VirtualHostCache represents a cache of computed *v2.VirtualHost objects.
type VirtualHostCache interface {
	// Values returns a copy of the contents of the cache.
	Values() []*v2.VirtualHost

	// Add adds an entry to the cache. If a VirtualHost with the same
	// name exists, it is replaced.
	Add(*v2.VirtualHost)

	// Remove removes the named entry from the cache. If the entry
	// is not present in the cache, the operation is a no-op.
	Remove(string)
}

// NewVirtualHostCache returns a new VirtualHostCache.
func NewVirtualHostCache() VirtualHostCache {
	v := make(virtualHostCache, 1)
	v <- nil // prime cache
	return v
}

// VirtualHostCache is a thread safe, atomic, copy on write cache of v2.VirtualHost objects.
type virtualHostCache chan []*v2.VirtualHost

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
