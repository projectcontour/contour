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
	"testing"

	envoy_service_runtime_v3 "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRuntimeCacheContents(t *testing.T) {
	rc := &RuntimeCache{}
	protobuf.ExpectEqual(t, runtimeLayers(), rc.Contents())
}

func TestRuntimeCacheQuery(t *testing.T) {
	testCases := map[string]struct {
		names    []string
		expected []proto.Message
	}{
		"empty names": {
			names:    []string{},
			expected: []proto.Message{},
		},
		"names include dynamic": {
			names:    []string{"foo", "dynamic", "bar"},
			expected: runtimeLayers(),
		},
		"names excludes dynamic": {
			names:    []string{"foo", "bar", "baz"},
			expected: []proto.Message{},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rc := &RuntimeCache{}
			protobuf.ExpectEqual(t, tc.expected, rc.Query(tc.names))
		})
	}
}

func runtimeLayers() []proto.Message {
	return []proto.Message{
		&envoy_service_runtime_v3.Runtime{
			Name: "dynamic",
			Layer: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"re2.max_program_size.error_level": {Kind: &structpb.Value_NumberValue{NumberValue: 1 << 20}},
					"re2.max_program_size.warn_level":  {Kind: &structpb.Value_NumberValue{NumberValue: 1000}},
				},
			},
		},
	}
}
