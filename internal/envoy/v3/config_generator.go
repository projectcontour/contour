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
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/projectcontour/contour/internal/timeout"
)

const defaultXDSServerClusterName = "contour"

// ConfigGenerator holds common configuration that can be applied across
// various Envoy xDS resource types to reduce duplication. It should
// typically be instantiated once and shared between various consumers
// that need to generate Envoy config.
type ConfigGenerator struct {
	// Name of xDS server Cluster. Referenced in ConfigSources across
	// various resources.
	xdsServerClusterName string
}

func NewConfigGenerator() *ConfigGenerator {
	return &ConfigGenerator{
		xdsServerClusterName: defaultXDSServerClusterName,
	}
}

// ConfigSource returns a *envoy_core_v3.ConfigSource for cluster.
func (g *ConfigGenerator) ConfigSource() *envoy_core_v3.ConfigSource {
	return &envoy_core_v3.ConfigSource{
		ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
		ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
			ApiConfigSource: &envoy_core_v3.ApiConfigSource{
				ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
				TransportApiVersion: envoy_core_v3.ApiVersion_V3,
				GrpcServices: []*envoy_core_v3.GrpcService{
					grpcService(g.xdsServerClusterName, "", timeout.DefaultSetting()),
				},
			},
		},
	}
}
