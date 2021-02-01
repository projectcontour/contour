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

// Package contour contains the translation business logic that listens
// to Kubernetes ResourceEventHandler events and translates those into
// additions/deletions in caches connected to the Envoy xDS gRPC API server.

package xdscache

import (
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/xds"
)

// ResourceCache is a store of an xDS resource type. It is able to
// visit the dag.DAG to update the its resource collection, then
// serve those resources over xDS.
type ResourceCache interface {
	dag.Observer
	xds.Resource
}

// ResourcesOf transliterates a slice of ResourceCache into a slice of xds.Resource.
func ResourcesOf(in []ResourceCache) []xds.Resource {
	out := make([]xds.Resource, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}

// ObserversOf transliterates a slice of ResourceCache into a slice of dag.Observer.
func ObserversOf(in []ResourceCache) []dag.Observer {
	out := make([]dag.Observer, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}
