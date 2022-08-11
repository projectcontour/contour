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
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
)

// RuntimeCache manages the contents of the gRPC RTDS cache.
type RuntimeCache struct {
	contour.Cond
}

// Contents returns all Runtime layers.
func (c *RuntimeCache) Contents() []proto.Message {
	return protobuf.AsMessages(envoy_v3.RuntimeLayers())
}

// Query returns only the "dynamic" layer if requested, otherwise empty.
func (c *RuntimeCache) Query(names []string) []proto.Message {
	for _, name := range names {
		if name == envoy_v3.DynamicRuntimeLayerName {
			return protobuf.AsMessages(envoy_v3.RuntimeLayers())
		}
	}
	return []proto.Message{}
}

func (*RuntimeCache) TypeURL() string { return resource.RuntimeType }

func (c *RuntimeCache) OnChange(root *dag.DAG) {
	// DAG changes do not affect runtime layers at the moment.
}
