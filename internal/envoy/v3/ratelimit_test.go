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

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimit_config_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	ratelimit_filter_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
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
				&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
					StatPrefix: "stat-prefix",
					TokenBucket: &envoy_type_v3.TokenBucket{
						MaxTokens:     100,
						TokensPerFill: protobuf.UInt32(50),
						FillInterval:  protobuf.Duration(time.Second),
					},
					Status: &envoy_type_v3.HttpStatus{Code: envoy_type_v3.StatusCode_ServiceUnavailable},
					ResponseHeadersToAdd: []*envoy_core_v3.HeaderValueOption{
						{Header: &envoy_core_v3.HeaderValue{Key: "X-Header-1", Value: "foo"}, Append: wrapperspb.Bool(false)},
						{Header: &envoy_core_v3.HeaderValue{Key: "X-Header-2", Value: "bar"}, Append: wrapperspb.Bool(false)},
					},
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
				}),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := LocalRateLimitConfig(tc.policy, tc.statPrefix)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGlobalRateLimits(t *testing.T) {
	tests := map[string]struct {
		descriptors []*dag.RateLimitDescriptor
		want        []*envoy_route_v3.RateLimit
	}{
		"nil descriptors": {
			descriptors: nil,
			want:        nil,
		},
		"normal descriptors": {
			descriptors: []*dag.RateLimitDescriptor{
				{
					Entries: []dag.RateLimitDescriptorEntry{
						{RemoteAddress: true},
						{GenericKeyValue: "generic-key-val"},
					},
				},
				{
					Entries: []dag.RateLimitDescriptorEntry{
						{HeaderMatchHeaderName: "X-Header-1", HeaderMatchDescriptorKey: "foo"},
						{RemoteAddress: true},
						{GenericKeyValue: "generic-key-val-2"},
					},
				},
			},
			want: []*envoy_route_v3.RateLimit{
				{
					Actions: []*envoy_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_route_v3.RateLimit_Action_GenericKey{
									DescriptorValue: "generic-key-val",
								},
							},
						},
					},
				},
				{
					Actions: []*envoy_route_v3.RateLimit_Action{
						{
							ActionSpecifier: &envoy_route_v3.RateLimit_Action_RequestHeaders_{
								RequestHeaders: &envoy_route_v3.RateLimit_Action_RequestHeaders{
									HeaderName:    "X-Header-1",
									DescriptorKey: "foo",
								},
							},
						},
						{
							ActionSpecifier: &envoy_route_v3.RateLimit_Action_RemoteAddress_{
								RemoteAddress: &envoy_route_v3.RateLimit_Action_RemoteAddress{},
							},
						},
						{
							ActionSpecifier: &envoy_route_v3.RateLimit_Action_GenericKey_{
								GenericKey: &envoy_route_v3.RateLimit_Action_GenericKey{
									DescriptorValue: "generic-key-val-2",
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
	assert.Nil(t, GlobalRateLimitFilter(nil))

	want := &http.HttpFilter{
		Name: wellknown.HTTPRateLimit,
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
				Domain:          "domain",
				Timeout:         protobuf.Duration(time.Second),
				FailureModeDeny: false,
				RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
					GrpcService: &envoy_core_v3.GrpcService{
						TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
								ClusterName: "extension/projectcontour/ratelimit",
							},
						},
					},
					TransportApiVersion: envoy_core_v3.ApiVersion_V3,
				},
			}),
		},
	}

	cfg := &GlobalRateLimitConfig{
		ExtensionService: k8s.NamespacedNameFrom("projectcontour/ratelimit"),
		FailOpen:         true,
		Timeout:          timeout.DurationSetting(time.Second),
		Domain:           "domain",
	}

	assert.Equal(t, want, GlobalRateLimitFilter(cfg))
}
