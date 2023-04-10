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

package dag

import (
	"testing"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDetermineSNI(t *testing.T) {
	tests := map[string]struct {
		routeRequestHeaders   *HeadersPolicy
		clusterRequestHeaders *HeadersPolicy
		service               *Service
		want                  string
	}{
		"default SNI": {
			routeRequestHeaders:   nil,
			clusterRequestHeaders: nil,
			service:               &Service{},
			want:                  "",
		},
		"route request headers set": {
			routeRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			clusterRequestHeaders: nil,
			service:               &Service{},
			want:                  "containersteve.com",
		},
		"service request headers set": {
			routeRequestHeaders: nil,
			clusterRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			service: &Service{},
			want:    "containersteve.com",
		},
		"service request headers set overrides route": {
			routeRequestHeaders: &HeadersPolicy{
				HostRewrite: "incorrect.com",
			},
			clusterRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			service: &Service{},
			want:    "containersteve.com",
		},
		"route request headers override externalName": {
			routeRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			clusterRequestHeaders: nil,
			service: &Service{
				ExternalName: "externalname.com",
			},
			want: "containersteve.com",
		},
		"service request headers override externalName": {
			routeRequestHeaders: nil,
			clusterRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			service: &Service{
				ExternalName: "externalname.com",
			},
			want: "containersteve.com",
		},
		"only externalName set": {
			routeRequestHeaders:   nil,
			clusterRequestHeaders: nil,
			service: &Service{
				ExternalName: "externalname.com",
			},
			want: "externalname.com",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := determineSNI(tc.routeRequestHeaders, tc.clusterRequestHeaders, tc.service)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEnforceRoute(t *testing.T) {
	tests := map[string]struct {
		tlsEnabled     bool
		permitInsecure bool
		want           bool
	}{
		"tls not enabled": {
			tlsEnabled:     false,
			permitInsecure: false,
			want:           false,
		},
		"tls enabled": {
			tlsEnabled:     true,
			permitInsecure: false,
			want:           true,
		},
		"tls enabled but insecure requested": {
			tlsEnabled:     true,
			permitInsecure: true,
			want:           false,
		},
		"tls not enabled but insecure requested": {
			tlsEnabled:     false,
			permitInsecure: true,
			want:           false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := routeEnforceTLS(tc.tlsEnabled, tc.permitInsecure)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestToCORSPolicy(t *testing.T) {
	tests := map[string]struct {
		cp      *contour_api_v1.CORSPolicy
		want    *CORSPolicy
		wantErr bool
	}{
		"no CORS policy": {
			cp:   nil,
			want: nil,
		},
		"all fields present and valid": {
			cp: &contour_api_v1.CORSPolicy{
				AllowCredentials: true,
				AllowHeaders:     []contour_api_v1.CORSHeaderValue{"X-Some-Header-A", "X-Some-Header-B"},
				AllowMethods:     []contour_api_v1.CORSHeaderValue{"GET", "PUT"},
				AllowOrigin:      []string{"*"},
				ExposeHeaders:    []contour_api_v1.CORSHeaderValue{"X-Expose-A", "X-Expose-B"},
				MaxAge:           "5h",
			},
			want: &CORSPolicy{
				AllowCredentials: true,
				AllowHeaders:     []string{"X-Some-Header-A", "X-Some-Header-B"},
				AllowMethods:     []string{"GET", "PUT"},
				AllowOrigin:      []CORSAllowOriginMatch{{Type: CORSAllowOriginMatchExact, Value: "*"}},
				ExposeHeaders:    []string{"X-Expose-A", "X-Expose-B"},
				MaxAge:           timeout.DurationSetting(5 * time.Hour),
			},
		},
		"allow origin wildcard": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  []string{"*"},
			},
			want: &CORSPolicy{
				AllowHeaders:  []string{},
				AllowMethods:  []string{"GET"},
				AllowOrigin:   []CORSAllowOriginMatch{{Type: CORSAllowOriginMatchExact, Value: "*"}},
				ExposeHeaders: []string{},
			},
		},
		"allow origin specific valid": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  []string{"http://foo-1.bar.com", "https://foo-2.com:443"},
			},
			want: &CORSPolicy{
				AllowHeaders: []string{},
				AllowMethods: []string{"GET"},
				AllowOrigin: []CORSAllowOriginMatch{
					{Type: CORSAllowOriginMatchExact, Value: "http://foo-1.bar.com"},
					{Type: CORSAllowOriginMatchExact, Value: "https://foo-2.com:443"},
				},
				ExposeHeaders: []string{},
			},
		},
		"allow origin invalid specific but valid regex": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin: []string{
					"no-scheme.bar.com",
					"http://bar.com/foo",
					"http://baz.com?query1=2",
					"http://example.org#fragment",
				},
			},
			want: &CORSPolicy{
				AllowHeaders: []string{},
				AllowMethods: []string{"GET"},
				AllowOrigin: []CORSAllowOriginMatch{
					{Type: CORSAllowOriginMatchRegex, Value: "no-scheme.bar.com"},
					{Type: CORSAllowOriginMatchRegex, Value: "http://bar.com/foo"},
					{Type: CORSAllowOriginMatchRegex, Value: "http://baz.com?query1=2"},
					{Type: CORSAllowOriginMatchRegex, Value: "http://example.org#fragment"},
				},
				ExposeHeaders: []string{},
			},
		},
		"allow origin regex valid": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  []string{`.*\.foo\.com`, `https://example\.bar-[0-9]+\.org`},
			},
			want: &CORSPolicy{
				AllowHeaders: []string{},
				AllowMethods: []string{"GET"},
				AllowOrigin: []CORSAllowOriginMatch{
					{Type: CORSAllowOriginMatchRegex, Value: `.*\.foo\.com`},
					{Type: CORSAllowOriginMatchRegex, Value: `https://example\.bar-[0-9]+\.org`},
				},
				ExposeHeaders: []string{},
			},
		},
		"allow origin regex invalid": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  []string{"**"},
			},
			wantErr: true,
		},
		"nil allow origin": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  nil,
			},
			wantErr: true,
		},
		"nil allow methods": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: nil,
				AllowOrigin:  []string{"*"},
			},
			wantErr: true,
		},
		"empty allow origin": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  []string{},
			},
			wantErr: true,
		},
		"empty allow methods": {
			cp: &contour_api_v1.CORSPolicy{
				AllowMethods: []contour_api_v1.CORSHeaderValue{},
				AllowOrigin:  []string{"*"},
			},
			wantErr: true,
		},
		"invalid max age": {
			cp: &contour_api_v1.CORSPolicy{
				MaxAge:       "xxm",
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  []string{"*"},
			},
			wantErr: true,
		},
		"negative max age": {
			cp: &contour_api_v1.CORSPolicy{
				MaxAge:       "-5s",
				AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				AllowOrigin:  []string{"*"},
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := toCORSPolicy(tc.cp)
			if tc.wantErr {
				require.Error(t, gotErr)
			}
			require.Equal(t, tc.want, got)
		})
	}

}

func TestSlowStart(t *testing.T) {
	tests := map[string]struct {
		input   *contour_api_v1.SlowStartPolicy
		want    *SlowStartConfig
		wantErr bool
	}{
		"window only": {
			input: &contour_api_v1.SlowStartPolicy{
				Window: "10s",
			},
			want: &SlowStartConfig{
				Window:           10 * time.Second,
				Aggression:       1.0,
				MinWeightPercent: 0, // Default value 10% is set only via CRD defaulting, so we get 0 here.
			},
		},
		"with all fields": {
			input: &contour_api_v1.SlowStartPolicy{
				Window:               "10s",
				Aggression:           "1.1",
				MinimumWeightPercent: 5,
			},
			want: &SlowStartConfig{
				Window:           10 * time.Second,
				Aggression:       1.1,
				MinWeightPercent: 5,
			},
		},
		"invalid window, missing unit": {
			input: &contour_api_v1.SlowStartPolicy{
				Window: "10",
			},
			wantErr: true,
		},
		"invalid aggression, not float": {
			input: &contour_api_v1.SlowStartPolicy{
				Window:     "10s",
				Aggression: "not-a-float",
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := slowStartConfig(tc.input)
			if tc.wantErr {
				require.Error(t, gotErr)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIncludeMatchConditionsIdentical(t *testing.T) {
	tests := map[string]struct {
		includeConds []contour_api_v1.MatchCondition
		seenConds    map[string][]matchConditionAggregate
		duplicate    bool
	}{
		"empty conditions, no seen": {
			includeConds: []contour_api_v1.MatchCondition{},
			seenConds:    make(map[string][]matchConditionAggregate),
			duplicate:    false,
		},
		"empty conditions, seen some": {
			includeConds: []contour_api_v1.MatchCondition{},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{{Name: "param-1", MatchType: QueryParamMatchTypeContains, Value: "foo"}},
					},
				},
				"/api": {
					{
						headerConds:     []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"prefix /, no seen": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/"},
			},
			seenConds: make(map[string][]matchConditionAggregate),
			duplicate: false,
		},
		"prefix /, seen prefix / only should not be duplicate": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"prefix /, seen headers only": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds:     []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"prefix /, seen query only": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{{Name: "param-1", MatchType: QueryParamMatchTypeContains, Value: "foo"}},
					},
				},
			},
			duplicate: false,
		},
		"prefix /, seen some": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{{Name: "param-1", MatchType: QueryParamMatchTypeContains, Value: "foo"}},
					},
					{
						headerConds:     []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"prefix nonroot, no seen": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/api"},
			},
			seenConds: make(map[string][]matchConditionAggregate),
			duplicate: false,
		},
		"prefix nonroot, seen": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/api"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/api": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: true,
		},
		"prefix nonroot, seen duplicate and others": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/api"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/api/v2": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
				"/api": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: true,
		},
		"prefix nonroot, seen others": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/api"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/api/v2": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
				"/api/v3": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"prefix nonroot, seen headers only": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/api"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/api": {
					{
						headerConds:     []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"prefix nonroot, seen query only": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/api"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/api": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{{Name: "param-1", MatchType: QueryParamMatchTypeContains, Value: "foo"}},
					},
				},
			},
			duplicate: false,
		},
		"prefix nonroot, seen some": {
			includeConds: []contour_api_v1.MatchCondition{
				{Prefix: "/api"},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/api": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{{Name: "param-1", MatchType: QueryParamMatchTypeContains, Value: "foo"}},
					},
					{
						headerConds:     []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
				"/api/v2": {
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"headers only, seen headers only non-duplicate": {
			includeConds: []contour_api_v1.MatchCondition{
				{Header: &contour_api_v1.HeaderMatchCondition{Name: "x-foo", NotPresent: true}},
				{Header: &contour_api_v1.HeaderMatchCondition{Name: "x-bar", Exact: "bar"}},
			},
			seenConds: map[string][]matchConditionAggregate{
				// Same header conditions but different prefix.
				"/other": {
					{
						headerConds: []HeaderMatchCondition{
							{Name: "x-foo", MatchType: HeaderMatchTypePresent, Invert: true},
							{Name: "x-bar", MatchType: HeaderMatchTypeExact, Value: "bar"},
						},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
				"/": {
					{
						headerConds: []HeaderMatchCondition{
							{Name: "x-foo", MatchType: HeaderMatchTypePresent},
							{Name: "x-bar", MatchType: HeaderMatchTypeExact, Value: "notbar"},
						},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: false,
		},
		"headers only, seen headers only duplicate": {
			includeConds: []contour_api_v1.MatchCondition{
				{Header: &contour_api_v1.HeaderMatchCondition{Name: "x-foo", Present: true}},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds:     []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{},
					},
				},
			},
			duplicate: true,
		},
		"query only, seen query only non-duplicate": {
			includeConds: []contour_api_v1.MatchCondition{
				{QueryParameter: &contour_api_v1.QueryParameterMatchCondition{Name: "param-1", Present: true}},
				{QueryParameter: &contour_api_v1.QueryParameterMatchCondition{Name: "param-2", Exact: "bar"}},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds: []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{
							{Name: "param-1", MatchType: QueryParamMatchTypePresent},
							{Name: "param-2", MatchType: QueryParamMatchTypeExact, Value: "notbar"},
						},
					},
				},
				// Same query params but different prefix.
				"/foo": {
					{
						headerConds: []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{
							{Name: "param-1", MatchType: QueryParamMatchTypePresent},
							{Name: "param-2", MatchType: QueryParamMatchTypeExact, Value: "bar"},
						},
					},
				},
			},
			duplicate: false,
		},
		"query only, seen query only duplicate": {
			includeConds: []contour_api_v1.MatchCondition{
				{QueryParameter: &contour_api_v1.QueryParameterMatchCondition{Name: "param-1", Prefix: "foo"}},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds: []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{
							{Name: "param-1", MatchType: QueryParamMatchTypePrefix, Value: "foo"},
						},
					},
				},
			},
			duplicate: true,
		},
		"combination of header and query, duplicate": {
			includeConds: []contour_api_v1.MatchCondition{
				{QueryParameter: &contour_api_v1.QueryParameterMatchCondition{Name: "param-2", Prefix: "foo"}},
				{Header: &contour_api_v1.HeaderMatchCondition{Name: "x-foo", Present: true}},
				{QueryParameter: &contour_api_v1.QueryParameterMatchCondition{Name: "param-1", Prefix: "foo"}},
			},
			seenConds: map[string][]matchConditionAggregate{
				"/": {
					{
						headerConds: []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{
							{Name: "param-1", MatchType: QueryParamMatchTypePrefix, Value: "foo"},
							{Name: "param-2", MatchType: QueryParamMatchTypePrefix, Value: "foo"},
						},
					},
				},
			},
			duplicate: true,
		},
		"combination of header and query, non-duplicate": {
			includeConds: []contour_api_v1.MatchCondition{
				{Header: &contour_api_v1.HeaderMatchCondition{Name: "x-foo", Present: true}},
				{QueryParameter: &contour_api_v1.QueryParameterMatchCondition{Name: "param-1", Prefix: "foo"}},
			},
			seenConds: map[string][]matchConditionAggregate{
				// Header and query params are the same, but different prefix.
				"/api": {
					{
						headerConds: []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{
							{Name: "param-1", MatchType: QueryParamMatchTypePrefix, Value: "foo"},
						},
					},
				},
				"/": {
					{
						headerConds:     []HeaderMatchCondition{{Name: "x-foo", MatchType: HeaderMatchTypePresent}},
						queryParamConds: []QueryParamMatchCondition{},
					},
					{
						headerConds:     []HeaderMatchCondition{},
						queryParamConds: []QueryParamMatchCondition{{Name: "param-1", MatchType: QueryParamMatchTypePrefix, Value: "foo"}},
					},
				},
			},
			duplicate: false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.duplicate, includeMatchConditionsIdentical(tc.includeConds, tc.seenConds))
		})
	}
}

func TestValidateExternalAuthExtensionService(t *testing.T) {
	tests := map[string]struct {
		ref                 contour_api_v1.ExtensionServiceReference
		wantValidCond       *contour_api_v1.DetailedCondition
		httpproxy           *contour_api_v1.HTTPProxy
		getExtensionCluster func(name string) *ExtensionCluster
		want                *ExtensionCluster
		wantBool            bool
	}{
		"Unsupported API version": {
			ref: contour_api_v1.ExtensionServiceReference{
				APIVersion: "wrong version",
				Namespace:  "ns",
				Name:       "test",
			},
			wantValidCond: &contour_api_v1.DetailedCondition{
				Condition: v1.Condition{
					Status:  contour_api_v1.ConditionTrue,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []contour_api_v1.SubCondition{
					{
						Type:    "AuthError",
						Reason:  "AuthBadResourceVersion",
						Message: "Spec.Virtualhost.Authorization.extensionRef specifies an unsupported resource version \"wrong version\"",
						Status:  contour_api_v1.ConditionTrue,
					},
				},
			},
			httpproxy: &contour_api_v1.HTTPProxy{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "ns",
				},
			},
			want: nil,
			getExtensionCluster: func(name string) *ExtensionCluster {
				return &ExtensionCluster{
					Name: "test",
				}
			},
			wantBool: false,
		},
		"ExtensionService does not exist": {
			ref: contour_api_v1.ExtensionServiceReference{
				APIVersion: "projectcontour.io/v1alpha1",
				Namespace:  "ns",
				Name:       "test",
			},
			wantValidCond: &contour_api_v1.DetailedCondition{
				Condition: v1.Condition{
					Status:  contour_api_v1.ConditionTrue,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []contour_api_v1.SubCondition{
					{
						Type:    "AuthError",
						Reason:  "ExtensionServiceNotFound",
						Message: "Spec.Virtualhost.Authorization.ServiceRef extension service \"ns/test\" not found",
						Status:  contour_api_v1.ConditionTrue,
					},
				},
			},
			httpproxy: &contour_api_v1.HTTPProxy{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "ns",
				},
			},
			getExtensionCluster: func(name string) *ExtensionCluster {
				return nil
			},
			want:     nil,
			wantBool: false,
		},
		"Validation successful": {
			ref: contour_api_v1.ExtensionServiceReference{
				APIVersion: "projectcontour.io/v1alpha1",
				Namespace:  "ns",
				Name:       "test",
			},
			wantValidCond: &contour_api_v1.DetailedCondition{},
			httpproxy: &contour_api_v1.HTTPProxy{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "ns",
				},
			},
			getExtensionCluster: func(name string) *ExtensionCluster {
				return &ExtensionCluster{
					Name: "test",
				}
			},
			want: &ExtensionCluster{
				Name: "test",
			},
			wantBool: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			validCond := &contour_api_v1.DetailedCondition{}
			gotBool, got := validateExternalAuthExtensionService(tc.ref, validCond, tc.httpproxy, tc.getExtensionCluster)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantBool, gotBool)
			require.Equal(t, tc.wantValidCond, validCond)
		})
	}
}

func TestDetermineExternalAuthTimeout(t *testing.T) {
	tests := map[string]struct {
		responseTimeout string
		wantValidCond   *contour_api_v1.DetailedCondition
		ext             *ExtensionCluster
		want            *timeout.Setting
		wantBool        bool
	}{
		"invalid timeout": {
			responseTimeout: "foo",
			wantValidCond: &contour_api_v1.DetailedCondition{
				Condition: v1.Condition{
					Status:  contour_api_v1.ConditionTrue,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []contour_api_v1.SubCondition{
					{
						Type:    "AuthError",
						Reason:  "AuthResponseTimeoutInvalid",
						Message: "Spec.Virtualhost.Authorization.ResponseTimeout is invalid: unable to parse timeout string \"foo\": time: invalid duration \"foo\"",
						Status:  contour_api_v1.ConditionTrue,
					},
				},
			},
		},
		"default timeout": {
			responseTimeout: "",
			wantValidCond:   &contour_api_v1.DetailedCondition{},
			ext: &ExtensionCluster{
				Name: "test",
				RouteTimeoutPolicy: RouteTimeoutPolicy{
					ResponseTimeout: timeout.DurationSetting(time.Second * 10),
				},
			},
			want:     ref.To(timeout.DurationSetting(time.Second * 10)),
			wantBool: true,
		},
		"success": {
			responseTimeout: "20s",
			wantValidCond:   &contour_api_v1.DetailedCondition{},
			ext: &ExtensionCluster{
				Name: "test",
				RouteTimeoutPolicy: RouteTimeoutPolicy{
					ResponseTimeout: timeout.DurationSetting(time.Second * 10),
				},
			},
			want:     ref.To(timeout.DurationSetting(time.Second * 20)),
			wantBool: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			validCond := &contour_api_v1.DetailedCondition{}
			gotBool, got := determineExternalAuthTimeout(tc.responseTimeout, validCond, tc.ext)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantBool, gotBool)
			require.Equal(t, tc.wantValidCond, validCond)
		})
	}
}
