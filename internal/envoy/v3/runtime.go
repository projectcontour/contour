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
	envoy_service_runtime_v3 "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	DynamicRuntimeLayerName  = "dynamic"
	maxRegexProgramSizeError = 1 << 20
	maxRegexProgramSizeWarn  = 1000
)

func RuntimeLayers() []*envoy_service_runtime_v3.Runtime {
	return []*envoy_service_runtime_v3.Runtime{
		{
			Name:  DynamicRuntimeLayerName,
			Layer: baseRuntimeLayer(),
		},
	}
}

func baseRuntimeLayer() *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"re2.max_program_size.error_level": {Kind: &structpb.Value_NumberValue{NumberValue: maxRegexProgramSizeError}},
			"re2.max_program_size.warn_level":  {Kind: &structpb.Value_NumberValue{NumberValue: maxRegexProgramSizeWarn}},
		},
	}
}
