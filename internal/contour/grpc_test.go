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
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/api"
)

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type testClusterCache map[string]*v2.Cluster

func (cc testClusterCache) Add(c *v2.Cluster) {
	cc[c.Name] = c
}

func (cc testClusterCache) Remove(name string) {
	delete(cc, name)
}

func (cc testClusterCache) Values() []*v2.Cluster {
	var r []*v2.Cluster
	for _, v := range cc {
		r = append(r, v)
	}
	return r
}

type testClusterLoadAssignmentCache map[string]*v2.ClusterLoadAssignment

func (cc testClusterLoadAssignmentCache) Add(c *v2.ClusterLoadAssignment) {
	cc[c.ClusterName] = c
}

func (cc testClusterLoadAssignmentCache) Remove(name string) {
	delete(cc, name)
}

func (cc testClusterLoadAssignmentCache) Values() []*v2.ClusterLoadAssignment {
	var r []*v2.ClusterLoadAssignment
	for _, v := range cc {
		r = append(r, v)
	}
	return r
}

type testVirtualHostCache map[string]*v2.VirtualHost

func (cc testVirtualHostCache) Add(c *v2.VirtualHost) {
	cc[c.Name] = c
}

func (cc testVirtualHostCache) Remove(name string) {
	delete(cc, name)
}

func (cc testVirtualHostCache) Values() []*v2.VirtualHost {
	var r []*v2.VirtualHost
	for _, v := range cc {
		r = append(r, v)
	}
	return r
}
