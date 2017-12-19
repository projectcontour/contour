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
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/api"
)

func TestClusterCacheValuesReturnsACopyOfItsInternalSlice(t *testing.T) {
	var cc clusterCache
	c := &v2.Cluster{
		Name: "alpha",
	}
	cc.Add(c)

	v1 := cc.Values()
	v2 := cc.Values()

	if &v1[0] == &v2[0] {
		// the address of the 0th element of the values slice should not be the same
		// if it is, then we don't have a copy.
		t.Fatalf("ClusterCache, consecutive calls to Values return the same backing slice: got: %p, want: %p", &v1[0], &v2[0])
	}
}

func TestClusterCacheValuesReturnsTheSameContents(t *testing.T) {
	var cc clusterCache
	c := &v2.Cluster{
		Name: "alpha",
	}
	cc.Add(c)

	v1 := cc.Values()
	v2 := cc.Values()

	if v1[0] != v2[0] {
		// the value of the 0th element, a pointer to a v2.Cluster should be the same
		t.Fatalf("ClusterCache, consecutive calls to Values returned different slice contents: got: %p, want: %p", v1[0], v2[0])
	}
}

func TestClusterCacheAddInsertsTwoElementsInSortOrder(t *testing.T) {
	var cc clusterCache
	c1 := &v2.Cluster{
		Name: "beta",
	}
	cc.Add(c1)
	c2 := &v2.Cluster{
		Name: "alpha",
	}
	cc.Add(c2)
	got := cc.Values()
	want := []*v2.Cluster{{
		Name: "alpha",
	}, {
		Name: "beta",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterCache.Add/Values returned elements missing or out of order, got: %v, want: %v", got, want)
	}
}

func TestClusterCacheAddOverwritesElementsWithTheSameName(t *testing.T) {
	var cc clusterCache
	c1 := &v2.Cluster{
		Name: "alpha",
		Type: 1,
	}
	cc.Add(c1)
	c2 := &v2.Cluster{
		Name: "alpha",
		Type: 2,
	}
	cc.Add(c2)
	got := cc.Values()
	want := []*v2.Cluster{
		c2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterCache.Add/Values returned a stale element, got: %v, want: %v", got, want)
	}
}

func TestClusterCacheAddIsCopyOnWrite(t *testing.T) {
	var cc clusterCache
	c1 := &v2.Cluster{
		Name: "alpha",
	}
	cc.Add(c1)
	v1 := cc.Values()

	c2 := &v2.Cluster{
		Name: "beta",
	}
	cc.Add(c2)
	v2 := cc.Values()

	if reflect.DeepEqual(v1, v2) {
		t.Fatalf("ClusterCache.Add affected the contents of a previous call to Values")
	}
}

func TestClusterCacheRemove(t *testing.T) {
	var cc clusterCache
	c1 := &v2.Cluster{
		Name: "alpha",
	}
	cc.Add(c1)
	cc.Remove("alpha")
	got := cc.Values()
	want := []*v2.Cluster{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterCache.Remove: got: %v, want: %v", got, want)
	}
}

func TestClusterLoadAssignmentCacheValuesReturnsACopyOfItsInternalSlice(t *testing.T) {
	var cc clusterLoadAssignmentCache
	c := &v2.ClusterLoadAssignment{
		ClusterName: "alpha",
	}
	cc.Add(c)

	v1 := cc.Values()
	v2 := cc.Values()

	if &v1[0] == &v2[0] {
		// the address of the 0th element of the values slice should not be the same
		// if it is, then we don't have a copy.
		t.Fatalf("ClusterLoadAssignmentCache, consecutive calls to Values return the same backing slice: got: %p, want: %p", &v1[0], &v2[0])
	}
}

func TestClusterLoadAssignmentCacheValuesReturnsTheSameContents(t *testing.T) {
	var cc clusterLoadAssignmentCache
	c := &v2.ClusterLoadAssignment{
		ClusterName: "alpha",
	}
	cc.Add(c)

	v1 := cc.Values()
	v2 := cc.Values()

	if v1[0] != v2[0] {
		// the value of the 0th element, a pointer to a v2.ClusterLoadAssignment should be the same
		t.Fatalf("ClusterLoadAssignmentCache, consecutive calls to Values returned different slice contents: got: %p, want: %p", v1[0], v2[0])
	}
}

func TestClusterLoadAssignmentCacheAddInsertsTwoElementsInSortOrder(t *testing.T) {
	var cc clusterLoadAssignmentCache
	c1 := &v2.ClusterLoadAssignment{
		ClusterName: "beta",
	}
	cc.Add(c1)
	c2 := &v2.ClusterLoadAssignment{
		ClusterName: "alpha",
	}
	cc.Add(c2)
	got := cc.Values()
	want := []*v2.ClusterLoadAssignment{{
		ClusterName: "alpha",
	}, {
		ClusterName: "beta",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterLoadAssignmentCache.Add/Values returned elements missing or out of order, got: %v, want: %v", got, want)
	}
}

func TestClusterLoadAssignmentCacheAddOverwritesElementsWithTheSameName(t *testing.T) {
	var cc clusterLoadAssignmentCache
	c1 := &v2.ClusterLoadAssignment{
		ClusterName: "alpha",
		Policy: &v2.ClusterLoadAssignment_Policy{
			DropOverload: 0.0,
		},
	}
	cc.Add(c1)
	c2 := &v2.ClusterLoadAssignment{
		ClusterName: "alpha",
		Policy: &v2.ClusterLoadAssignment_Policy{
			DropOverload: 1.0,
		},
	}
	cc.Add(c2)
	got := cc.Values()
	want := []*v2.ClusterLoadAssignment{
		c2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterLoadAssignmentCache.Add/Values returned a stale element, got: %v, want: %v", got, want)
	}
}

func TestClusterLoadAssignmentCacheAddIsCopyOnWrite(t *testing.T) {
	var cc clusterLoadAssignmentCache
	c1 := &v2.ClusterLoadAssignment{
		ClusterName: "alpha",
	}
	cc.Add(c1)
	v1 := cc.Values()

	c2 := &v2.ClusterLoadAssignment{
		ClusterName: "beta",
	}
	cc.Add(c2)
	v2 := cc.Values()

	if reflect.DeepEqual(v1, v2) {
		t.Fatalf("ClusterLoadAssignmentCache.Add affected the contents of a previous call to Values")
	}
}

func TestClusterLoadAssignmentCacheRemove(t *testing.T) {
	var cc clusterLoadAssignmentCache
	c1 := &v2.ClusterLoadAssignment{
		ClusterName: "alpha",
	}
	cc.Add(c1)
	cc.Remove("alpha")
	got := cc.Values()
	want := []*v2.ClusterLoadAssignment{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterLoadAssignmentCache.Remove: got: %v, want: %v", got, want)
	}
}

func TestListenerCacheValuesReturnsACopyOfItsInternalSlice(t *testing.T) {
	var cc listenerCache
	l := &v2.Listener{
		Name: "alpha",
	}
	cc.Add(l)

	v1 := cc.Values()
	v2 := cc.Values()

	if &v1[0] == &v2[0] {
		// the address of the 0th element of the values slice should not be the same
		// if it is, then we don't have a copy.
		t.Fatalf("ListenerCache, consecutive calls to Values return the same backing slice: got: %p, want: %p", &v1[0], &v2[0])
	}
}

func TestListenerCacheValuesReturnsTheSameContents(t *testing.T) {
	var cc listenerCache
	l := &v2.Listener{
		Name: "alpha",
	}
	cc.Add(l)

	v1 := cc.Values()
	v2 := cc.Values()

	if v1[0] != v2[0] {
		// the value of the 0th element, a pointer to a v2.Listener should be the same
		t.Fatalf("ListenerCache, consecutive calls to Values returned different slice contents: got: %p, want: %p", v1[0], v2[0])
	}
}

func TestListenerCacheAddInsertsTwoElementsInSortOrder(t *testing.T) {
	var cc listenerCache
	l1 := &v2.Listener{
		Name: "beta",
	}
	cc.Add(l1)
	l2 := &v2.Listener{
		Name: "alpha",
	}
	cc.Add(l2)
	got := cc.Values()
	want := []*v2.Listener{{
		Name: "alpha",
	}, {
		Name: "beta",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListenerCache.Add/Values returned elements missing or out of order, got: %v, want: %v", got, want)
	}
}

func TestListenerCacheAddOverwritesElementsWithTheSameName(t *testing.T) {
	var cc listenerCache
	l1 := &v2.Listener{
		Name:      "alpha",
		DrainType: 7,
	}
	cc.Add(l1)
	l2 := &v2.Listener{
		Name:      "alpha",
		DrainType: 99,
	}
	cc.Add(l2)
	got := cc.Values()
	want := []*v2.Listener{
		l2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListenerCache.Add/Values returned a stale element, got: %v, want: %v", got, want)
	}
}

func TestListenerCacheAddIsCopyOnWrite(t *testing.T) {
	var cc listenerCache
	l1 := &v2.Listener{
		Name: "alpha",
	}
	cc.Add(l1)
	v1 := cc.Values()

	l2 := &v2.Listener{
		Name: "beta",
	}
	cc.Add(l2)
	v2 := cc.Values()

	if reflect.DeepEqual(v1, v2) {
		t.Fatalf("ListenerCache.Add affected the contents of a previous call to Values")
	}
}

func TestListenerCacheRemove(t *testing.T) {
	var cc listenerCache
	l1 := &v2.Listener{
		Name: "alpha",
	}
	cc.Add(l1)
	cc.Remove("alpha")
	got := cc.Values()
	want := []*v2.Listener{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListenerCache.Remove: got: %v, want: %v", got, want)
	}
}

func TestVirtualHostCacheValuesReturnsACopyOfItsInternalSlice(t *testing.T) {
	var cc virtualHostCache
	c := &v2.VirtualHost{
		Name: "alpha",
	}
	cc.Add(c)

	v1 := cc.Values()
	v2 := cc.Values()

	if &v1[0] == &v2[0] {
		// the address of the 0th element of the values slice should not be the same
		// if it is, then we don't have a copy.
		t.Fatalf("VirtualHostCache, consecutive calls to Values return the same backing slice: got: %p, want: %p", &v1[0], &v2[0])
	}
}

func TestVirtualHostCacheValuesReturnsTheSameContents(t *testing.T) {
	var cc virtualHostCache
	c := &v2.VirtualHost{
		Name: "alpha",
	}
	cc.Add(c)

	v1 := cc.Values()
	v2 := cc.Values()

	if v1[0] != v2[0] {
		// the value of the 0th element, a pointer to a v2.VirtualHost should be the same
		t.Fatalf("VirtualHostCache, consecutive calls to Values returned different slice contents: got: %p, want: %p", v1[0], v2[0])
	}
}

func TestVirtualHostCacheAddInsertsTwoElementsInSortOrder(t *testing.T) {
	var cc virtualHostCache
	c1 := &v2.VirtualHost{
		Name: "beta",
	}
	cc.Add(c1)
	c2 := &v2.VirtualHost{
		Name: "alpha",
	}
	cc.Add(c2)
	got := cc.Values()
	want := []*v2.VirtualHost{{
		Name: "alpha",
	}, {
		Name: "beta",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("VirtualHostCache.Add/Values returned elements missing or out of order, got: %v, want: %v", got, want)
	}
}

func TestVirtualHostCacheAddOverwritesElementsWithTheSameName(t *testing.T) {
	var cc virtualHostCache
	c1 := &v2.VirtualHost{
		Name: "alpha",
		Domains: []string{
			"example.com",
		},
	}
	cc.Add(c1)
	c2 := &v2.VirtualHost{
		Name: "alpha",
		Domains: []string{
			"heptio.com",
		},
	}
	cc.Add(c2)
	got := cc.Values()
	want := []*v2.VirtualHost{
		c2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("VirtualHostCache.Add/Values returned a stale element, got: %v, want: %v", got, want)
	}
}

func TestVirtualHostCacheAddIsCopyOnWrite(t *testing.T) {
	var cc virtualHostCache
	c1 := &v2.VirtualHost{
		Name: "alpha",
	}
	cc.Add(c1)
	v1 := cc.Values()

	c2 := &v2.VirtualHost{
		Name: "beta",
	}
	cc.Add(c2)
	v2 := cc.Values()

	if reflect.DeepEqual(v1, v2) {
		t.Fatalf("VirtualHostCache.Add affected the contents of a previous call to Values")
	}
}

func TestVirtualHostCacheRemove(t *testing.T) {
	var cc virtualHostCache
	c1 := &v2.VirtualHost{
		Name: "alpha",
	}
	cc.Add(c1)
	cc.Remove("alpha")
	got := cc.Values()
	want := []*v2.VirtualHost{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("VirtualHostCache.Remove: got: %v, want: %v", got, want)
	}
}
