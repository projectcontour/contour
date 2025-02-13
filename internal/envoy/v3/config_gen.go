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
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/projectcontour/contour/internal/timeout"
)

// DefaultXDSClusterName is the default name of the Contour cluster as defined in the
// Envoy bootstrap configuration.
const DefaultXDSClusterName = "contour"

type EnvoyGen struct {
	xdsClusterName string
}

type EnvoyGenOpt struct {
	XDSClusterName string
}

func NewEnvoyGen(opt EnvoyGenOpt) *EnvoyGen {
	return &EnvoyGen{
		xdsClusterName: opt.XDSClusterName,
	}
}

func (e *EnvoyGen) GetConfigSource() *envoy_config_core_v3.ConfigSource {
	return &envoy_config_core_v3.ConfigSource{
		ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
		ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
			ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
				ApiType:             envoy_config_core_v3.ApiConfigSource_GRPC,
				TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
				GrpcServices: []*envoy_config_core_v3.GrpcService{
					grpcService(e.xdsClusterName, "", timeout.DefaultSetting()),
				},
			},
		},
	}
}
