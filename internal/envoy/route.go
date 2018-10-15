// Copyright © 2018 Heptio
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

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

// RouteRoute returns a route.Route_Route for the services supplied.
// If len(services) is greater than one, the route's action will be a
// weighted cluster.
func RouteRoute(services []*dag.Service) route.Route_Route {
	switch len(services) {
	case 1:
		return RouteCluster(services[0])
	default:
		return route.Route_Route{
			Route: &route.RouteAction{
				ClusterSpecifier: &route.RouteAction_WeightedClusters{
					WeightedClusters: WeightedClusters(services),
				},
			},
		}
	}
}

// RouteCluster returns a route.Route_Route for a single service.
func RouteCluster(service *dag.Service) route.Route_Route {
	return route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_Cluster{
				Cluster: Clustername(service),
			},
			RequestHeadersToAdd: headers(
				appendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
			),
		},
	}
}

// WeightedClusters returns a route.WeightedCluster for multiple services.
func WeightedClusters(services []*dag.Service) *route.WeightedCluster {
	var wc route.WeightedCluster
	var total int
	for _, svc := range services {
		total += svc.Weight
		wc.Clusters = append(wc.Clusters, &route.WeightedCluster_ClusterWeight{
			Name:   Clustername(svc),
			Weight: u32(svc.Weight),
			RequestHeadersToAdd: headers(
				appendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
			),
		})
	}
	// Check if no weights were defined, if not default to even distribution
	if total == 0 {
		for _, c := range wc.Clusters {
			c.Weight.Value = 1
		}
		total = len(services)
	}
	wc.TotalWeight = u32(total)

	sort.Stable(clusterWeightByName(wc.Clusters))
	return &wc
}

type clusterWeightByName []*route.WeightedCluster_ClusterWeight

func (c clusterWeightByName) Len() int      { return len(c) }
func (c clusterWeightByName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clusterWeightByName) Less(i, j int) bool {
	if c[i].Name == c[j].Name {
		return c[i].Weight.Value < c[j].Weight.Value
	}
	return c[i].Name < c[j].Name

}

func headers(first *core.HeaderValueOption, rest ...*core.HeaderValueOption) []*core.HeaderValueOption {
	return append([]*core.HeaderValueOption{first}, rest...)
}

func appendHeader(key, value string) *core.HeaderValueOption {
	return &core.HeaderValueOption{
		Header: &core.HeaderValue{
			Key:   key,
			Value: value,
		},
		Append: bv(true),
	}
}

func u32(val int) *types.UInt32Value { return &types.UInt32Value{Value: uint32(val)} }
func bv(val bool) *types.BoolValue   { return &types.BoolValue{Value: val} }
