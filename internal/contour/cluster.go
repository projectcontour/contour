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

package contour

import (
	"sort"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// translateClusters returns an array of CDS resources.
func translateClusters(clusters map[string]*v2.Cluster) []xds.Resource {
	var values []*v2.Cluster
	for _, v := range clusters {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

type clusterVisitor struct {
	clusters map[string]*v2.Cluster
}

// visitCluster produces an array of xds.Resources
func visitClusters(root dag.Vertex) []xds.Resource {
	cv := clusterVisitor{
		clusters: make(map[string]*v2.Cluster),
	}
	cv.visit(root)

	return translateClusters(cv.clusters)
}

func (v *clusterVisitor) visit(vertex dag.Vertex) {
	if cluster, ok := vertex.(*dag.Cluster); ok {
		name := envoy.Clustername(cluster)
		if _, ok := v.clusters[name]; !ok {
			c := envoy.Cluster(cluster)
			v.clusters[c.Name] = c
		}
	}

	// recurse into children of v
	vertex.Visit(v.visit)
}
