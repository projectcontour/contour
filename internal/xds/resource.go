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
	"google.golang.org/protobuf/proto"
)

// Resource represents a source of proto.Messages that can be registered
// for interest.
type Resource interface {
	// Contents returns the contents of this resource.
	Contents() []proto.Message

	// TypeURL returns the typeURL of messages returned from Values.
	TypeURL() string
}
