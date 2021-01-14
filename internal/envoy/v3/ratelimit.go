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
	envoy_config_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

// LocalRateLimitConfig returns a config for the HTTP local rate
// limit filter.
func LocalRateLimitConfig(config *dag.LocalRateLimitPolicy, statPrefix string) *any.Any {
	if config == nil {
		return nil
	}

	c := &envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix: statPrefix,
		TokenBucket: &envoy_type_v3.TokenBucket{
			MaxTokens:     config.MaxTokens,
			TokensPerFill: protobuf.UInt32(config.TokensPerFill),
			FillInterval:  protobuf.Duration(config.FillInterval),
		},
		ResponseHeadersToAdd: HeaderValueList(config.ResponseHeadersToAdd, false),
		FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
		FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
	}

	// Envoy defaults to 429 (Too Many Requests) if this is not specified.
	if config.ResponseStatusCode > 0 {
		c.Status = &envoy_type_v3.HttpStatus{Code: envoy_type_v3.StatusCode(config.ResponseStatusCode)}
	}

	return protobuf.MustMarshalAny(c)
}
