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

	"github.com/stretchr/testify/assert"
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
