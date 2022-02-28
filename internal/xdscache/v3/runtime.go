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
)

// RuntimeCache manages the contents of the gRPC RTDS cache.
type RuntimeCache struct {
	contour.Cond
}

// Contents returns an empty set of layers for now.
func (c *RuntimeCache) Contents() []proto.Message {
	return []proto.Message{}
}

// Query returns an empty set of layers for now.
func (c *RuntimeCache) Query(names []string) []proto.Message {
	return []proto.Message{}
}

func (*RuntimeCache) TypeURL() string { return resource.RuntimeType }

func (c *RuntimeCache) OnChange(root *dag.DAG) {
	// DAG changes do not affect runtime layers at the moment.
}
