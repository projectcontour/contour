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
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_filter_http_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

func TestLocalRateLimitConfig(t *testing.T) {
	tests := map[string]struct {
		policy     *dag.LocalRateLimitPolicy
		statPrefix string
		want       *anypb.Any
	}{
		"nil config": {
			policy: nil,
			want:   nil,
		},
		"normal config": {
			policy: &dag.LocalRateLimitPolicy{
				MaxTokens:          100,
				TokensPerFill:      50,
				FillInterval:       time.Second,
				ResponseStatusCode: 503,
				ResponseHeadersToAdd: map[string]string{
					"X-Header-1": "foo",
					"X-Header-2": "bar",
				},
			},
			statPrefix: "stat-prefix",
			want: protobuf.MustMarshalAny(
				&envoy_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "stat-prefix",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     100,
						TokensPerFill: wrapperspb.UInt32(50),
						FillInterval:  durationpb.New(time.Second),
					},
					Status: &envoy_type_v3.HttpStatus{Code: envoy_type_v3.StatusCode_ServiceUnavailable},
					ResponseHeadersToAdd: []*envoy_config_core_v3.HeaderValueOption{
						{Header: &envoy_config_core_v3.HeaderValue{Key: "X-Header-1", Value: "foo"}, AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD},
						{Header: &envoy_config_core_v3.HeaderValue{Key: "X-Header-2", Value: "bar"}, AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD},
					},
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
				}),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := localRateLimitConfig(tc.policy, tc.statPrefix)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGlobalRateLimits(t *testing.T) {
	tests := map[string]struct {
		descriptors []*dag.RateLimitDescriptor
		want        []*envoy_config_route_v3.RateLimit
	}{
		"nil descriptors": {
			descriptors: nil,
			want:        nil,
		},
		"normal descriptors": {
			descriptors: []*dag.RateLimitDescriptor{
				{
					Entries: []dag.RateLimitDescriptorEntry{
						{
							RemoteAddress: &dag.RemoteAddressDescriptorEntry{},
						},
						{
							GenericKey: &dag.GenericKeyDescriptorEntry{
								Value: "generic-key-val",
							},
						},
						{
							GenericKey: &dag.GenericKeyDescriptorEntry{
								Key:   "generic-key-custom-key",
								Value: "generic-key-val",
							},
						},
					},
				},
				{
					Entries: []dag.RateLimitDescriptorEntry{
						{
							HeaderMatch: &dag.HeaderMatchDescriptorEntry{
								HeaderName: "X-Header-1",
								Key:        "foo",
							},
						},
						{
							RemoteAddress: &dag.RemoteAddressDescriptorEntry{},
						},
						{
							GenericKey: &dag.GenericKeyDescriptorEntry{
								Value: "generic-key-val-2",
							},
						},
					},
				},
				{
					Entries: []dag.RateLimitDescriptorEntry{
						{
							HeaderValueMatch: &dag.HeaderValueMatchDescriptorEntry{
								Headers: []dag.HeaderMatchCondition{
									{
										Name:      "A-Header",
										Value:     "foo",
										MatchType: dag.HeaderMatchTypeExact,
									},
								},
								ExpectMatch: true,
								Value:       "A-Header-Equals-Foo",
							},
						},
					},
				},
			},
			want: []*envoy_config_route_v3.RateLimit{
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
									DescriptorValue: "generic-key-val",
								},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
									DescriptorKey:   "generic-key-custom-key",
									DescriptorValue: "generic-key-val",
								},
							},
						},
					},
				},
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
								RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
									HeaderName:    "X-Header-1",
									DescriptorKey: "foo",
								},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
									DescriptorValue: "generic-key-val-2",
								},
							},
						},
					},
				},
				{
					Actions: []*envoy_config_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch_{
								HeaderValueMatch: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch{
									Headers: []*envoy_config_route_v3.HeaderMatcher{
										{
											Name: "A-Header",
											HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_StringMatch{
												StringMatch: &envoy_matcher_v3.StringMatcher{
													MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
														Exact: "foo",
													},
												},
											},
										},
									},
									ExpectMatch:     wrapperspb.Bool(true),
									DescriptorValue: "A-Header-Equals-Foo",
								},
							},
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := GlobalRateLimits(tc.descriptors)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGlobalRateLimitFilter(t *testing.T) {
	tests := map[string]struct {
		cfg  *GlobalRateLimitConfig
		want *envoy_filter_network_http_connection_manager_v3.HttpFilter
	}{
		"nil config produces nil filter": {
			cfg:  nil,
			want: nil,
		},
		"all fields configured correctly with FailOpen=false": {
			cfg: &GlobalRateLimitConfig{
				ExtensionService: k8s.NamespacedNameFrom("projectcontour/ratelimit"),
				Timeout:          timeout.DurationSetting(7 * time.Second),
				Domain:           "domain",
				FailOpen:         false,
			},
			want: &envoy_filter_network_http_connection_manager_v3.HttpFilter{
				Name: wellknown.HTTPRateLimit,
				ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
						Domain:          "domain",
						Timeout:         durationpb.New(7 * time.Second),
						FailureModeDeny: true,
						RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
							GrpcService: &envoy_config_core_v3.GrpcService{
								TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
										ClusterName: "extension/projectcontour/ratelimit",
										Authority:   "extension.projectcontour.ratelimit",
									},
								},
							},
							TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
						},
					}),
				},
			},
		},
		"all fields configured correctly with FailOpen=true": {
			cfg: &GlobalRateLimitConfig{
				ExtensionService: k8s.NamespacedNameFrom("projectcontour/ratelimit"),
				Timeout:          timeout.DurationSetting(7 * time.Second),
				Domain:           "domain",
				FailOpen:         true,
			},
			want: &envoy_filter_network_http_connection_manager_v3.HttpFilter{
				Name: wellknown.HTTPRateLimit,
				ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
						Domain:          "domain",
						Timeout:         durationpb.New(7 * time.Second),
						FailureModeDeny: false,
						RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
							GrpcService: &envoy_config_core_v3.GrpcService{
								TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
										ClusterName: "extension/projectcontour/ratelimit",
										Authority:   "extension.projectcontour.ratelimit",
									},
								},
							},
							TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
						},
					}),
				},
			},
		},
		"when rate limit server has SNI set": {
			cfg: &GlobalRateLimitConfig{
				ExtensionService: k8s.NamespacedNameFrom("projectcontour/ratelimit"),
				SNI:              "some-server.com",
				Timeout:          timeout.DurationSetting(7 * time.Second),
				Domain:           "domain",
				FailOpen:         false,
			},
			want: &envoy_filter_network_http_connection_manager_v3.HttpFilter{
				Name: wellknown.HTTPRateLimit,
				ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
						Domain:          "domain",
						Timeout:         durationpb.New(7 * time.Second),
						FailureModeDeny: true,
						RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
							GrpcService: &envoy_config_core_v3.GrpcService{
								TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
										ClusterName: "extension/projectcontour/ratelimit",
										Authority:   "some-server.com",
									},
								},
							},
							TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
						},
					}),
				},
			},
		},
		"EnableXRateLimitHeaders=true is configured correctly": {
			cfg: &GlobalRateLimitConfig{
				ExtensionService:        k8s.NamespacedNameFrom("projectcontour/ratelimit"),
				Timeout:                 timeout.DurationSetting(7 * time.Second),
				Domain:                  "domain",
				FailOpen:                true,
				EnableXRateLimitHeaders: true,
			},
			want: &envoy_filter_network_http_connection_manager_v3.HttpFilter{
				Name: wellknown.HTTPRateLimit,
				ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
						Domain:          "domain",
						Timeout:         durationpb.New(7 * time.Second),
						FailureModeDeny: false,
						RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
							GrpcService: &envoy_config_core_v3.GrpcService{
								TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
										ClusterName: "extension/projectcontour/ratelimit",
										Authority:   "extension.projectcontour.ratelimit",
									},
								},
							},
							TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
						},
						EnableXRatelimitHeaders: envoy_filter_http_ratelimit_v3.RateLimit_DRAFT_VERSION_03,
					}),
				},
			},
		},
		"EnableResourceExhaustedCode=true is configured correctly": {
			cfg: &GlobalRateLimitConfig{
				ExtensionService:            k8s.NamespacedNameFrom("projectcontour/ratelimit"),
				Timeout:                     timeout.DurationSetting(7 * time.Second),
				Domain:                      "domain",
				FailOpen:                    true,
				EnableResourceExhaustedCode: true,
			},
			want: &envoy_filter_network_http_connection_manager_v3.HttpFilter{
				Name: wellknown.HTTPRateLimit,
				ConfigType: &envoy_filter_network_http_connection_manager_v3.HttpFilter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimit{
						Domain:          "domain",
						Timeout:         durationpb.New(7 * time.Second),
						FailureModeDeny: false,
						RateLimitService: &envoy_config_ratelimit_v3.RateLimitServiceConfig{
							GrpcService: &envoy_config_core_v3.GrpcService{
								TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
										ClusterName: "extension/projectcontour/ratelimit",
										Authority:   "extension.projectcontour.ratelimit",
									},
								},
							},
							TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
						},
						RateLimitedAsResourceExhausted: true,
					}),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, GlobalRateLimitFilter(tc.cfg))
		})
	}
}

func TestRateLimitPerRoute(t *testing.T) {
	tests := map[string]struct {
		name string
		cfg  *dag.RateLimitPerRoute
		want *anypb.Any
	}{
		"VhRateLimits in Override mode": {
			cfg: &dag.RateLimitPerRoute{
				VhRateLimits: dag.VhRateLimitsOverride,
			},
			want: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimitPerRoute{
				VhRateLimits: 0,
			}),
		}, "VhRateLimits in Include mode": {
			cfg: &dag.RateLimitPerRoute{
				VhRateLimits: dag.VhRateLimitsInclude,
			},
			want: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimitPerRoute{
				VhRateLimits: 1,
			}),
		}, "VhRateLimits in Ignore mode": {
			cfg: &dag.RateLimitPerRoute{
				VhRateLimits: dag.VhRateLimitsIgnore,
			},
			want: protobuf.MustMarshalAny(&envoy_filter_http_ratelimit_v3.RateLimitPerRoute{
				VhRateLimits: 2,
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, rateLimitPerRoute(tc.cfg))
		})
	}
}
