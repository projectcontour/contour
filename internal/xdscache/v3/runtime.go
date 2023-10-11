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
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type ConfigurableRuntimeSettings struct {
	MaxRequestsPerIOCycle *uint32
}

// RuntimeCache manages the contents of the gRPC RTDS cache.
type runtimeCache struct {
	contour.Cond
	runtimeKV map[string]*structpb.Value
}

// NewRuntimeCache builds a RuntimeCache with the provided runtime
// settings that will be set in the runtime layer configured by Contour.
func NewRuntimeCache(runtimeSettings ConfigurableRuntimeSettings) *runtimeCache {
	runtimeKV := make(map[string]*structpb.Value)
	if runtimeSettings.MaxRequestsPerIOCycle != nil && *runtimeSettings.MaxRequestsPerIOCycle > 0 {
		runtimeKV["http.max_requests_per_io_cycle"] = structpb.NewNumberValue(float64(*runtimeSettings.MaxRequestsPerIOCycle))
	}
	return &runtimeCache{runtimeKV: runtimeKV}
}

// Contents returns all Runtime layers.
func (c *runtimeCache) Contents() []proto.Message {
	return protobuf.AsMessages(envoy_v3.RuntimeLayers(c.runtimeKV))
}

// Query returns only the "dynamic" layer if requested, otherwise empty.
func (c *runtimeCache) Query(names []string) []proto.Message {
	for _, name := range names {
		if name == envoy_v3.DynamicRuntimeLayerName {
			return protobuf.AsMessages(envoy_v3.RuntimeLayers(c.runtimeKV))
		}
	}
	return []proto.Message{}
}

func (*runtimeCache) TypeURL() string { return resource.RuntimeType }

func (c *runtimeCache) OnChange(root *dag.DAG) {
	// DAG changes do not affect runtime layers at the moment.
}
