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

package xds

import (
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

// Resource represents a source of proto.Messages that can be registered
// for interest.
type Resource interface {
	// Contents returns the contents of this resource.
	Contents() []proto.Message

	// Query returns an entry for each resource name supplied.
	Query(names []string) []proto.Message

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int, ...string)

	// TypeURL returns the typeURL of messages returned from Values.
	TypeURL() string
}

// Counter holds an atomically incrementing counter.
type Counter uint64

func (c *Counter) Next() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}
