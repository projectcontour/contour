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
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestComputeHosts(t *testing.T) {
	tests := map[string]struct {
		route     *gatewayapi_v1alpha1.HTTPRoute
		want      []string
		wantError []error
	}{
		"single host": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
					},
				},
			},
			want:      []string{"test.projectcontour.io"},
			wantError: []error(nil),
		},
		"single DNS label hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"projectcontour",
					},
				},
			},
			want:      []string{"projectcontour"},
			wantError: []error(nil),
		},
		"multiple hosts": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
						"test1.projectcontour.io",
						"test2.projectcontour.io",
						"test3.projectcontour.io",
					},
				},
			},
			want:      []string{"test.projectcontour.io", "test1.projectcontour.io", "test2.projectcontour.io", "test3.projectcontour.io"},
			wantError: []error(nil),
		},
		"no host": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{},
			},
			want:      []string{"*"},
			wantError: []error(nil),
		},
		"IP in host": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"1.2.3.4",
					},
				},
			},
			want: []string(nil),
			wantError: []error{
				fmt.Errorf("hostname \"1.2.3.4\" must be a DNS name, not an IP address"),
			},
		},
		"valid wildcard hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"*.projectcontour.io",
					},
				},
			},
			want:      []string{"*.projectcontour.io"},
			wantError: []error(nil),
		},
		"invalid wildcard hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"*.*.projectcontour.io",
					},
				},
			},
			want: []string(nil),
			wantError: []error{
				fmt.Errorf("invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"#projectcontour.io",
					},
				},
			},
			want: []string(nil),
			wantError: []error{
				fmt.Errorf("invalid listener hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			}

			got, gotError := processor.computeHosts(tc.route)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantError, gotError)
		})
	}
}

func TestGatewayMatches(t *testing.T) {
	tests := map[string]struct {
		routeGateways *gatewayapi_v1alpha1.RouteGateways
		namespace     string
		want          bool
	}{
		"gateway allow all is always valid": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			namespace: "",
			want:      true,
		},
		"gateway allow from list matches configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowFromList),
				GatewayRefs: []gatewayapi_v1alpha1.GatewayReference{{
					Name:      "contour",
					Namespace: "projectcontour",
				}},
			},
			namespace: "projectcontour",
			want:      true,
		},
		"gateway allow from list doesn't match configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowFromList),
				GatewayRefs: []gatewayapi_v1alpha1.GatewayReference{{
					Name:      "different",
					Namespace: "gateway",
				}},
			},
			namespace: "projectcontour",
			want:      false,
		},
		"gateway allow same namespace matches configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
			},
			namespace: "projectcontour",
			want:      true,
		},
		"gateway allow same namespace doesn't match configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
			},
			namespace: "different",
			want:      false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
				source: &KubernetesCache{
					ConfiguredGateway: types.NamespacedName{
						Name:      "contour",
						Namespace: "projectcontour",
					},
				},
			}

			got := processor.gatewayMatches(tc.routeGateways, tc.namespace)
			assert.Equal(t, tc.want, got)
		})
	}
}
