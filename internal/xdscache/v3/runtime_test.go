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
	"github.com/projectcontour/contour/internal/ref"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRuntimeCacheContents(t *testing.T) {
	testCases := map[string]struct {
		runtimeSettings  ConfigurableRuntimeSettings
		additionalFields map[string]*structpb.Value
	}{
		"no values set": {
			runtimeSettings: ConfigurableRuntimeSettings{},
		},
		"http max requests per io cycle set": {
			runtimeSettings: ConfigurableRuntimeSettings{
				MaxRequestsPerIOCycle: ref.To(uint32(1)),
			},
			additionalFields: map[string]*structpb.Value{
				"http.max_requests_per_io_cycle": structpb.NewNumberValue(1),
			},
		},
		"http max requests per io cycle set invalid": {
			runtimeSettings: ConfigurableRuntimeSettings{
				MaxRequestsPerIOCycle: ref.To(uint32(0)),
			},
		},
		"http max requests per io cycle set nil": {
			runtimeSettings: ConfigurableRuntimeSettings{
				MaxRequestsPerIOCycle: nil,
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rc := NewRuntimeCache(tc.runtimeSettings)
			fields := map[string]*structpb.Value{
				"re2.max_program_size.error_level": structpb.NewNumberValue(1 << 20),
				"re2.max_program_size.warn_level":  structpb.NewNumberValue(1000),
			}
			for k, v := range tc.additionalFields {
				fields[k] = v
			}
			protobuf.ExpectEqual(t, []proto.Message{
				&envoy_service_runtime_v3.Runtime{
					Name: "dynamic",
					Layer: &structpb.Struct{
						Fields: fields,
					},
				},
			}, rc.Contents())
		})
	}
}

func TestRuntimeCacheQuery(t *testing.T) {
	baseRuntimeLayers := []proto.Message{
		&envoy_service_runtime_v3.Runtime{
			Name: "dynamic",
			Layer: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"re2.max_program_size.error_level": structpb.NewNumberValue(1 << 20),
					"re2.max_program_size.warn_level":  structpb.NewNumberValue(1000),
				},
			},
		},
	}
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
			expected: baseRuntimeLayers,
		},
		"names excludes dynamic": {
			names:    []string{"foo", "bar", "baz"},
			expected: []proto.Message{},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rc := NewRuntimeCache(ConfigurableRuntimeSettings{})
			protobuf.ExpectEqual(t, tc.expected, rc.Query(tc.names))
		})
	}
}
