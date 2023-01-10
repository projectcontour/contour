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
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/internal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestComputeHosts(t *testing.T) {
	tests := map[string]struct {
		listenerHost string
		hostnames    []gatewayapi_v1beta1.Hostname
		want         sets.Set[string]
		wantError    []error
	}{
		"single host": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"test.projectcontour.io",
			},
			want:      sets.New("test.projectcontour.io"),
			wantError: nil,
		},
		"single DNS label hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"projectcontour",
			},
			want:      sets.New("projectcontour"),
			wantError: nil,
		},
		"multiple hosts": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"test.projectcontour.io",
				"test1.projectcontour.io",
				"test2.projectcontour.io",
				"test3.projectcontour.io",
			},
			want: sets.New(
				"test.projectcontour.io",
				"test1.projectcontour.io",
				"test2.projectcontour.io",
				"test3.projectcontour.io",
			),
			wantError: nil,
		},
		"no host": {
			listenerHost: "",
			hostnames:    []gatewayapi_v1beta1.Hostname{},
			want:         sets.New("*"),
			wantError:    []error(nil),
		},
		"IP in host": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"1.2.3.4",
			},
			want: nil,
			wantError: []error{
				fmt.Errorf("invalid hostname \"1.2.3.4\": must be a DNS name, not an IP address"),
			},
		},
		"valid wildcard hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("*.projectcontour.io"),
			wantError: nil,
		},
		"invalid wildcard hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*.*.projectcontour.io",
			},
			want: nil,
			wantError: []error{
				fmt.Errorf("invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid wildcard hostname *": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*",
			},
			want:      nil,
			wantError: []error{fmt.Errorf("invalid hostname \"*\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]")},
		},
		"invalid hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"#projectcontour.io",
			},
			want: nil,
			wantError: []error{
				fmt.Errorf("invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"listener host & hostnames host do not exactly match": {
			listenerHost: "listener.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"http.projectcontour.io",
			},
			want:      nil,
			wantError: nil,
		},
		"listener host & hostnames host exactly match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"http.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & multi hostnames host exactly match one host": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"http.projectcontour.io",
				"http2.projectcontour.io",
				"http3.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & hostnames host match wildcard host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"http.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & hostnames host do not match wildcard host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"http.example.com",
			},
			want:      nil,
			wantError: nil,
		},
		"listener host & wildcard hostnames host do not match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & wildcard hostname and matching hostname match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*.projectcontour.io",
				"http.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & wildcard hostname and non-matching hostname don't match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*.projectcontour.io",
				"not.matching.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host wildcard & wildcard hostnames host match": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("*.projectcontour.io"),
			wantError: nil,
		},
		"listener host & hostname not defined match": {
			listenerHost: "http.projectcontour.io",
			hostnames:    []gatewayapi_v1beta1.Hostname{},
			want:         sets.New("http.projectcontour.io"),
			wantError:    nil,
		},
		"listener host with many labels matches hostnames wildcard host": {
			listenerHost: "very.many.labels.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("very.many.labels.projectcontour.io"),
			wantError: nil,
		},
		"listener wildcard host matches hostnames with many labels host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"very.many.labels.projectcontour.io",
			},
			want:      sets.New("very.many.labels.projectcontour.io"),
			wantError: nil,
		},
		"listener wildcard host doesn't match bare hostname": {
			listenerHost: "*.foo",
			hostnames: []gatewayapi_v1beta1.Hostname{
				"foo",
			},
			want:      nil,
			wantError: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			}

			got, gotError := processor.computeHosts(tc.hostnames, tc.listenerHost)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantError, gotError)
		})
	}
}

func TestNamespaceMatches(t *testing.T) {
	tests := map[string]struct {
		namespaces *gatewayapi_v1beta1.RouteNamespaces
		namespace  string
		valid      bool
	}{
		"nil matches all": {
			namespaces: nil,
			namespace:  "projectcontour",
			valid:      true,
		},
		"nil From matches all": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: nil,
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromAll matches all": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromSame matches": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromSame doesn't match": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
			},
			namespace: "custom",
			valid:     false,
		},
		"From.NamespacesFromSelector matches labels, same ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "production",
					},
				},
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromSelector matches labels, different ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"something": "special",
					},
				},
			},
			namespace: "custom",
			valid:     true,
		},
		"From.NamespacesFromSelector doesn't matches labels, different ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"something": "special",
					},
				},
			},
			namespace: "projectcontour",
			valid:     false,
		},
		"From.NamespacesFromSelector matches expression 'In', different ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"special"},
					}},
				},
			},
			namespace: "custom",
			valid:     true,
		},
		"From.NamespacesFromSelector matches expression 'DoesNotExist', different ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "notthere",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					}},
				},
			},
			namespace: "custom",
			valid:     true,
		},
		"From.NamespacesFromSelector doesn't match expression 'DoesNotExist', different ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					}},
				},
			},
			namespace: "custom",
			valid:     false,
		},
		"From.NamespacesFromSelector matches expression 'Exists', different ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "notthere",
						Operator: metav1.LabelSelectorOpExists,
					}},
				},
			},
			namespace: "custom",
			valid:     false,
		},
		"From.NamespacesFromSelector doesn't match expression 'Exists', different ns as gateway": {
			namespaces: &gatewayapi_v1beta1.RouteNamespaces{
				From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpExists,
					}},
				},
			},
			namespace: "custom",
			valid:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
				source: &KubernetesCache{
					gateway: &gatewayapi_v1beta1.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "contour",
							Namespace: "projectcontour",
						},
					},
					namespaces: map[string]*v1.Namespace{
						"projectcontour": {
							ObjectMeta: metav1.ObjectMeta{
								Name: "projectcontour",
								Labels: map[string]string{
									"app": "production",
								},
							},
						},
						"custom": {
							ObjectMeta: metav1.ObjectMeta{
								Name: "custom",
								Labels: map[string]string{
									"something": "special",
									"another":   "val",
									"testkey":   "testval",
								},
							},
						},
						"customsimilar": {
							ObjectMeta: metav1.ObjectMeta{
								Name: "custom",
								Labels: map[string]string{
									"something": "special",
								},
							},
						},
					},
				},
			}

			var selector labels.Selector
			var err error
			if tc.namespaces != nil && tc.namespaces.Selector != nil {
				selector, err = metav1.LabelSelectorAsSelector(tc.namespaces.Selector)
				require.NoError(t, err)
			}

			got := processor.namespaceMatches(tc.namespaces, selector, tc.namespace)
			assert.Equal(t, tc.valid, got)
		})
	}
}

func TestGetListenersForRouteParentRef(t *testing.T) {
	tests := map[string]struct {
		routeParentRef gatewayapi_v1beta1.ParentReference
		routeNamespace string
		routeKind      string
		listeners      []*listenerInfo
		want           []int // specify the indexes of the listeners that should be selected
	}{
		"gateway namespace specified, no listener specified, gateway in same namespace as route": {
			routeParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
			routeNamespace: "projectcontour",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: []int{0, 1},
		},
		"gateway namespace specified, no listener specified, gateway in different namespace than route": {
			routeParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
			routeNamespace: "different-namespace-than-gateway",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: nil,
		},
		"no gateway namespace specified, no listener specified, gateway in same namespace as route": {
			routeParentRef: gatewayapi.GatewayParentRef("", "contour"),
			routeNamespace: "projectcontour",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: []int{0, 1},
		},
		"no gateway namespace specified, no listener specified, gateway in different namespace than route": {
			routeParentRef: gatewayapi.GatewayParentRef("", "contour"),
			routeNamespace: "different-namespace-than-gateway",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: nil,
		},

		"section name specified, matches first listener": {
			routeParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http-1", 0),
			routeNamespace: "projectcontour",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: []int{0},
		},
		"section name specified, matches second listener": {
			routeParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http-2", 0),
			routeNamespace: "projectcontour",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: []int{1},
		},
		"section name specified, does not match listener": {
			routeParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "different-listener-name", 0),
			routeNamespace: "projectcontour",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: nil,
		},
		"route kind only allowed by second listener": {
			routeParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
			routeNamespace: "projectcontour",
			routeKind:      "HTTPRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"TLSRoute"},
				},
				{
					listener: gatewayapi_v1beta1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1beta1.Kind{"HTTPRoute"},
				},
			},
			want: []int{1},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
				source: &KubernetesCache{
					gateway: &gatewayapi_v1beta1.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "contour",
							Namespace: "projectcontour",
						},
					},
				},
			}

			rsu := &status.RouteStatusUpdate{}
			rpsu := rsu.StatusUpdateFor(tc.routeParentRef)

			got := processor.getListenersForRouteParentRef(tc.routeParentRef, tc.routeNamespace, gatewayapi_v1beta1.Kind(tc.routeKind), tc.listeners, rpsu)

			var want []*listenerInfo
			for _, i := range tc.want {
				want = append(want, tc.listeners[i])
			}

			assert.Equal(t, want, got)
		})
	}
}
