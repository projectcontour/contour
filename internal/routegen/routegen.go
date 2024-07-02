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

package routegen

import (
	"encoding/json"
	"log" // Assuming a simple logging approach

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/projectcontour/contour/internal/dag"
	xdscache "github.com/projectcontour/contour/internal/xdscache/v3"
)

// RouteGenerator generates routing configurations.
type RouteGenerator struct {
	dagBuilder *dag.Builder
	routeCache *xdscache.RouteCache
}

// NewRouteGenerator creates a new RouteGenerator instance with the provided DAG builder and route cache.
func NewRouteGenerator(dagBuilder *dag.Builder, routeCache *xdscache.RouteCache) *RouteGenerator {
	return &RouteGenerator{
		dagBuilder: dagBuilder,
		routeCache: routeCache,
	}
}

// Run generates routing configurations for the provided resources.
// It populates the DAG and route cache, then returns the serialized route configurations.
func (rg *RouteGenerator) Run(resources []runtime.Object) []json.RawMessage {
	rg.populateCache(resources)
	dag := rg.dagBuilder.Build()

	rg.routeCache.OnChange(dag)

	return rg.routeCache.Raw()
}

// populateCache processes the resources to populate the DAG's cache.
func (rg *RouteGenerator) populateCache(resources []runtime.Object) {
	for _, r := range resources {
		if ok := rg.dagBuilder.Source.Insert(r); !ok {
			// Log the error; adjust logging strategy as needed.
			log.Printf("Failed to insert resource into DAG")
		}
	}
}
