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
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
