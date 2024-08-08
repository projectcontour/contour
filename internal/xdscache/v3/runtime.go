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

package v3

import (
	"fmt"
	"sync"

	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
)

type ConfigurableRuntimeSettings struct {
	MaxRequestsPerIOCycle     *uint32
	MaxConnectionsPerListener *uint32
}

// RuntimeCache manages the contents of the gRPC RTDS cache.
type RuntimeCache struct {
	runtimeKV map[string]*structpb.Value

	dynamicRuntimeKV map[string]*structpb.Value
	mu               sync.Mutex

	maxConnectionsPerListener *uint32
}

// NewRuntimeCache builds a RuntimeCache with the provided runtime
// settings that will be set in the runtime layer configured by Contour.
func NewRuntimeCache(runtimeSettings ConfigurableRuntimeSettings) *RuntimeCache {
	runtimeKV := make(map[string]*structpb.Value)
	dynamicRuntimeKV := make(map[string]*structpb.Value)
	if runtimeSettings.MaxRequestsPerIOCycle != nil && *runtimeSettings.MaxRequestsPerIOCycle > 0 {
		runtimeKV["http.max_requests_per_io_cycle"] = structpb.NewNumberValue(float64(*runtimeSettings.MaxRequestsPerIOCycle))
	}
	return &RuntimeCache{
		runtimeKV:                 runtimeKV,
		dynamicRuntimeKV:          dynamicRuntimeKV,
		maxConnectionsPerListener: runtimeSettings.MaxConnectionsPerListener,
	}
}

func (c *RuntimeCache) buildDynamicLayer() []proto.Message {
	values := make(map[string]*structpb.Value)
	for k, v := range c.runtimeKV {
		values[k] = v
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.dynamicRuntimeKV {
		values[k] = v
	}
	return protobuf.AsMessages(envoy_v3.RuntimeLayers(values))
}

// Contents returns all Runtime layers (currently only the dynamic layer).
func (c *RuntimeCache) Contents() []proto.Message {
	return c.buildDynamicLayer()
}

func (*RuntimeCache) TypeURL() string { return resource.RuntimeType }

// Update replaces the contents of the cache with the supplied map.
func (c *RuntimeCache) Update(v map[string]*structpb.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dynamicRuntimeKV = v
}

func (c *RuntimeCache) OnChange(root *dag.DAG) {
	dynamicRuntimeKV := make(map[string]*structpb.Value)
	if c.maxConnectionsPerListener != nil && *c.maxConnectionsPerListener > 0 {
		for _, listener := range root.Listeners {
			fieldName := fmt.Sprintf("envoy.resource_limits.listener.%s.connection_limit", listener.Name)

			dynamicRuntimeKV[fieldName] = structpb.NewNumberValue(float64(*c.maxConnectionsPerListener))
		}
	}

	c.Update(dynamicRuntimeKV)
}
