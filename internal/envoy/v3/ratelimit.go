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
	envoy_config_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_filter_http_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/types"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

// localRateLimitConfig returns a config for the HTTP local rate
// limit filter.
func localRateLimitConfig(config *dag.LocalRateLimitPolicy, statPrefix string) *anypb.Any {
	if config == nil {
		return nil
	}

	c := &envoy_filter_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix: statPrefix,
		TokenBucket: &envoy_type_v3.TokenBucket{
			MaxTokens:     config.MaxTokens,
			TokensPerFill: wrapperspb.UInt32(config.TokensPerFill),
			FillInterval:  durationpb.New(config.FillInterval),
		},
		ResponseHeadersToAdd: headerValueList(config.ResponseHeadersToAdd, false),
		FilterEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
		FilterEnforced: &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
	}

	// Envoy defaults to 429 (Too Many Requests) if this is not specified.
	if config.ResponseStatusCode > 0 {
		c.Status = &envoy_type_v3.HttpStatus{Code: envoy_type_v3.StatusCode(config.ResponseStatusCode)} //nolint:gosec // disable G115
	}

	return protobuf.MustMarshalAny(c)
}

// GlobalRateLimits converts DAG RateLimitDescriptors to Envoy RateLimits.
func GlobalRateLimits(descriptors []*dag.RateLimitDescriptor) []*envoy_config_route_v3.RateLimit {
	var rateLimits []*envoy_config_route_v3.RateLimit
	for _, descriptor := range descriptors {
		var rl envoy_config_route_v3.RateLimit

		for _, entry := range descriptor.Entries {
			switch {
			case entry.GenericKey != nil:
				rl.Actions = append(rl.Actions, &envoy_config_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
						GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
							DescriptorKey:   entry.GenericKey.Key,
							DescriptorValue: entry.GenericKey.Value,
						},
					},
				})
			case entry.HeaderMatch != nil:
				rl.Actions = append(rl.Actions, &envoy_config_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
						RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
							HeaderName:    entry.HeaderMatch.HeaderName,
							DescriptorKey: entry.HeaderMatch.Key,
						},
					},
				})
			case entry.HeaderValueMatch != nil:
				rl.Actions = append(rl.Actions, &envoy_config_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch_{
						HeaderValueMatch: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch{
							DescriptorValue: entry.HeaderValueMatch.Value,
							ExpectMatch:     wrapperspb.Bool(entry.HeaderValueMatch.ExpectMatch),
							Headers:         headerMatcher(entry.HeaderValueMatch.Headers),
						},
					},
				})
			case entry.RemoteAddress != nil:
				rl.Actions = append(rl.Actions, &envoy_config_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
						RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
					},
				})
			}
		}

		rateLimits = append(rateLimits, &rl)
	}

	return rateLimits
}

// GlobalRateLimitConfig stores configuration for
// an HTTP global rate limiting filter.
type GlobalRateLimitConfig struct {
	ExtensionService            types.NamespacedName
	SNI                         string
	FailOpen                    bool
	Timeout                     timeout.Setting
	Domain                      string
	EnableXRateLimitHeaders     bool
	EnableResourceExhaustedCode bool
}

// GlobalRateLimitFilter returns a configured HTTP global rate limit filter,
// or nil if config is nil.
func GlobalRateLimitFilter(config *GlobalRateLimitConfig) *envoy_filter_network_http_connection_manager_v3.HttpFilter {
	if config == nil {
		return nil
	}

	return &envoy_filter_network_http_connection_manager_v3.HttpFilter{
		Name: wellknown.HTTPRateLimit,
		ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
				Domain:          config.Domain,
				Timeout:         envoy.Timeout(config.Timeout),
				FailureModeDeny: !config.FailOpen,
				RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
					GrpcService:         grpcService(dag.ExtensionClusterName(config.ExtensionService), config.SNI, timeout.DefaultSetting()),
					TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
				},
				EnableXRatelimitHeaders:        enableXRateLimitHeaders(config.EnableXRateLimitHeaders),
				RateLimitedAsResourceExhausted: config.EnableResourceExhaustedCode,
			}),
		},
	}
}

func enableXRateLimitHeaders(enable bool) envoy_filter_http_ratelimit_v3.RateLimit_XRateLimitHeadersRFCVersion {
	if enable {
		return envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03
	}
	return envoy_filter_http_ratelimit_v3.RateLimit_OFF
}

// rateLimitPerRoute returns a per-route config to configure vhost rate limits.
func rateLimitPerRoute(r *dag.RateLimitPerRoute) *anypb.Any {
	return protobuf.MustMarshalAny(
		&envoy_filter_http_ratelimit_v3.RateLimitPerRoute{
			VhRateLimits: envoy_filter_http_ratelimit_v3.RateLimitPerRoute_VhRateLimitsOptions(r.VhRateLimits),
		},
	)
}
