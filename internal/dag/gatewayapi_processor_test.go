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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/status"
)

func TestComputeHosts(t *testing.T) {
	tests := map[string]struct {
		listenerHost       string
		otherListenerHosts []string
		hostnames          []gatewayapi_v1.Hostname
		want               sets.Set[string]
		wantError          []error
	}{
		"single host": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			want:      sets.New("test.projectcontour.io"),
			wantError: nil,
		},
		"single DNS label hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
				"projectcontour",
			},
			want:      sets.New("projectcontour"),
			wantError: nil,
		},
		"multiple hosts": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
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
			hostnames:    []gatewayapi_v1.Hostname{},
			want:         sets.New("*"),
			wantError:    []error(nil),
		},
		"IP in host": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
				"1.2.3.4",
			},
			want: nil,
			wantError: []error{
				fmt.Errorf("invalid hostname \"1.2.3.4\": must be a DNS name, not an IP address"),
			},
		},
		"valid wildcard hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("*.projectcontour.io"),
			wantError: nil,
		},
		"invalid wildcard hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
				"*.*.projectcontour.io",
			},
			want: nil,
			wantError: []error{
				fmt.Errorf("invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid wildcard hostname *": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
				"*",
			},
			want:      nil,
			wantError: []error{fmt.Errorf("invalid hostname \"*\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]")},
		},
		"invalid hostname": {
			listenerHost: "",
			hostnames: []gatewayapi_v1.Hostname{
				"#projectcontour.io",
			},
			want: nil,
			wantError: []error{
				fmt.Errorf("invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"listener host & hostnames host do not exactly match": {
			listenerHost: "listener.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"http.projectcontour.io",
			},
			want:      nil,
			wantError: nil,
		},
		"listener host & hostnames host exactly match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"http.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & multi hostnames host exactly match one host": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"http.projectcontour.io",
				"http2.projectcontour.io",
				"http3.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & hostnames host match wildcard host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"http.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & hostnames host do not match wildcard host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"http.example.com",
			},
			want:      nil,
			wantError: nil,
		},
		"listener host & wildcard hostnames host do not match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & wildcard hostname and matching hostname match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"*.projectcontour.io",
				"http.projectcontour.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host & wildcard hostname and non-matching hostname don't match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"*.projectcontour.io",
				"not.matching.io",
			},
			want:      sets.New("http.projectcontour.io"),
			wantError: nil,
		},
		"listener host wildcard & wildcard hostnames host match": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("*.projectcontour.io"),
			wantError: nil,
		},
		"listener host & hostname not defined match": {
			listenerHost: "http.projectcontour.io",
			hostnames:    []gatewayapi_v1.Hostname{},
			want:         sets.New("http.projectcontour.io"),
			wantError:    nil,
		},
		"listener host with many labels matches hostnames wildcard host": {
			listenerHost: "very.many.labels.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"*.projectcontour.io",
			},
			want:      sets.New("very.many.labels.projectcontour.io"),
			wantError: nil,
		},
		"listener wildcard host matches hostnames with many labels host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi_v1.Hostname{
				"very.many.labels.projectcontour.io",
			},
			want:      sets.New("very.many.labels.projectcontour.io"),
			wantError: nil,
		},
		"listener wildcard host doesn't match bare hostname": {
			listenerHost: "*.foo",
			hostnames: []gatewayapi_v1.Hostname{
				"foo",
			},
			want:      nil,
			wantError: nil,
		},
		"empty listener host with other listener specific hosts that match route hostnames": {
			listenerHost: "",
			otherListenerHosts: []string{
				"foo",
			},
			hostnames: []gatewayapi_v1.Hostname{
				"foo",
				"bar",
				"*.foo",
			},
			want:      sets.New("bar", "*.foo"),
			wantError: nil,
		},
		"empty listener host with other listener wildcard hosts that match route hostnames": {
			listenerHost: "",
			otherListenerHosts: []string{
				"*.foo",
			},
			hostnames: []gatewayapi_v1.Hostname{
				"a.bar",
				"foo",
				"a.foo",
				"*.foo",
			},
			want:      sets.New("a.bar", "foo"),
			wantError: nil,
		},
		"wildcard listener host with other listener specific hosts that match route hostnames": {
			listenerHost: "*.foo",
			otherListenerHosts: []string{
				"a.foo",
			},
			hostnames: []gatewayapi_v1.Hostname{
				"a.foo",
				"c.b.foo",
				"*.foo",
			},
			want:      sets.New("c.b.foo", "*.foo"),
			wantError: nil,
		},
		"wildcard listener host with other listener more specific wildcard hosts that match route hostnames": {
			listenerHost: "*.foo",
			otherListenerHosts: []string{
				"*.a.foo",
			},
			hostnames: []gatewayapi_v1.Hostname{
				"a.foo",
				"b.a.foo",
				"d.c.foo",
				"*.foo",
				"*.b.a.foo",
			},
			want:      sets.New("a.foo", "d.c.foo", "*.foo"),
			wantError: nil,
		},
		"wildcard listener host with other listener less specific wildcard hosts that match route hostnames": {
			listenerHost: "*.a.foo",
			otherListenerHosts: []string{
				"*.foo",
			},
			hostnames: []gatewayapi_v1.Hostname{
				"a.foo",
				"b.a.foo",
				"d.c.foo",
				"*.a.foo",
			},
			want:      sets.New("b.a.foo", "*.a.foo"),
			wantError: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			}

			got, gotError := processor.computeHosts(tc.hostnames, tc.listenerHost, tc.otherListenerHosts)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantError, gotError)
		})
	}
}

func TestNamespaceMatches(t *testing.T) {
	tests := map[string]struct {
		namespaces *gatewayapi_v1.RouteNamespaces
		namespace  string
		valid      bool
	}{
		"nil matches all": {
			namespaces: nil,
			namespace:  "projectcontour",
			valid:      true,
		},
		"nil From matches all": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: nil,
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromAll matches all": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromAll),
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromSame matches": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSame),
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromSame doesn't match": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSame),
			},
			namespace: "custom",
			valid:     false,
		},
		"From.NamespacesFromSelector matches labels, same ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "production",
					},
				},
			},
			namespace: "projectcontour",
			valid:     true,
		},
		"From.NamespacesFromSelector matches labels, different ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchLabels: map[string]string{
						"something": "special",
					},
				},
			},
			namespace: "custom",
			valid:     true,
		},
		"From.NamespacesFromSelector doesn't matches labels, different ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchLabels: map[string]string{
						"something": "special",
					},
				},
			},
			namespace: "projectcontour",
			valid:     false,
		},
		"From.NamespacesFromSelector matches expression 'In', different ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchExpressions: []meta_v1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: meta_v1.LabelSelectorOpIn,
						Values:   []string{"special"},
					}},
				},
			},
			namespace: "custom",
			valid:     true,
		},
		"From.NamespacesFromSelector matches expression 'DoesNotExist', different ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchExpressions: []meta_v1.LabelSelectorRequirement{{
						Key:      "notthere",
						Operator: meta_v1.LabelSelectorOpDoesNotExist,
					}},
				},
			},
			namespace: "custom",
			valid:     true,
		},
		"From.NamespacesFromSelector doesn't match expression 'DoesNotExist', different ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchExpressions: []meta_v1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: meta_v1.LabelSelectorOpDoesNotExist,
					}},
				},
			},
			namespace: "custom",
			valid:     false,
		},
		"From.NamespacesFromSelector matches expression 'Exists', different ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchExpressions: []meta_v1.LabelSelectorRequirement{{
						Key:      "notthere",
						Operator: meta_v1.LabelSelectorOpExists,
					}},
				},
			},
			namespace: "custom",
			valid:     false,
		},
		"From.NamespacesFromSelector doesn't match expression 'Exists', different ns as gateway": {
			namespaces: &gatewayapi_v1.RouteNamespaces{
				From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
				Selector: &meta_v1.LabelSelector{
					MatchExpressions: []meta_v1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: meta_v1.LabelSelectorOpExists,
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
					gateway: &gatewayapi_v1.Gateway{
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      "contour",
							Namespace: "projectcontour",
						},
					},
					namespaces: map[string]*core_v1.Namespace{
						"projectcontour": {
							ObjectMeta: meta_v1.ObjectMeta{
								Name: "projectcontour",
								Labels: map[string]string{
									"app": "production",
								},
							},
						},
						"custom": {
							ObjectMeta: meta_v1.ObjectMeta{
								Name: "custom",
								Labels: map[string]string{
									"something": "special",
									"another":   "val",
									"testkey":   "testval",
								},
							},
						},
						"customsimilar": {
							ObjectMeta: meta_v1.ObjectMeta{
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
				selector, err = meta_v1.LabelSelectorAsSelector(tc.namespaces.Selector)
				require.NoError(t, err)
			}

			got := processor.namespaceMatches(tc.namespaces, selector, tc.namespace)
			assert.Equal(t, tc.valid, got)
		})
	}
}

func TestGetListenersForRouteParentRef(t *testing.T) {
	tests := map[string]struct {
		routeParentRef gatewayapi_v1.ParentReference
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
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
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"TLSRoute"},
					ready:        true,
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
				},
			},
			want: []int{1},
		},
		"route kind only allowed by first listener for GRPCRoute": {
			routeParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
			routeNamespace: "projectcontour",
			routeKind:      "GRPCRoute",
			listeners: []*listenerInfo{
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-1",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"GRPCRoute"},
					ready:        true,
				},
				{
					listener: gatewayapi_v1.Listener{
						Name: "http-2",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSame),
							},
						},
					},
					allowedKinds: []gatewayapi_v1.Kind{"HTTPRoute"},
					ready:        true,
				},
			},
			want: []int{0},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
				source: &KubernetesCache{
					gateway: &gatewayapi_v1.Gateway{
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      "contour",
							Namespace: "projectcontour",
						},
					},
				},
			}

			rsu := &status.RouteStatusUpdate{}
			rpsu := rsu.StatusUpdateFor(tc.routeParentRef)

			got := processor.getListenersForRouteParentRef(
				tc.routeParentRef,
				tc.routeNamespace,
				gatewayapi_v1.Kind(tc.routeKind),
				tc.listeners,
				map[string]int{},
				rpsu)

			var want map[string]*listenerInfo
			if len(tc.want) > 0 {
				want = map[string]*listenerInfo{}
				for _, i := range tc.want {
					listener := tc.listeners[i]
					want[string(listener.listener.Name)] = listener
				}
			}

			assert.Equal(t, want, got)
		})
	}
}

func TestSortRoutes(t *testing.T) {
	time1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	time2 := time.Date(2022, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	time3 := time.Date(2023, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	tests := []struct {
		name     string
		m        map[types.NamespacedName]*gatewayapi_v1.HTTPRoute
		expected []*gatewayapi_v1.HTTPRoute
	}{
		{
			name: "3 httproutes, with different timestamp, earlier one should be first ",
			m: map[types.NamespacedName]*gatewayapi_v1.HTTPRoute{
				{
					Namespace: "ns", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time3),
					},
				},
				{
					Namespace: "ns", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					Namespace: "ns", Name: "name3",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.HTTPRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time3),
					},
				},
			},
		},
		{
			name: "3 httproutes with same creation timestamps, same namespaces, smaller name comes first",
			m: map[types.NamespacedName]*gatewayapi_v1.HTTPRoute{
				{
					Namespace: "ns", Name: "name3",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.HTTPRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
		},
		{
			name: "3 httproutes with same creation timestamp, smaller namespaces comes first",
			m: map[types.NamespacedName]*gatewayapi_v1.HTTPRoute{
				{
					Namespace: "ns3", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns2", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns1", Name: "name3",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.HTTPRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
		},
		{
			name: "mixed order, two with same creation timestamp, two with same name",
			m: map[types.NamespacedName]*gatewayapi_v1.HTTPRoute{
				{
					Namespace: "ns1", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					Namespace: "ns2", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns1", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
			},
			expected: []*gatewayapi_v1.HTTPRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
			},
		},
		{
			name: "same name, same timestamp, different namespace",
			m: map[types.NamespacedName]*gatewayapi_v1.HTTPRoute{
				{
					Namespace: "ns3", Name: "name",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns2", Name: "name",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns1", Name: "name",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.HTTPRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := sortHTTPRoutes(tc.m)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func TestSortGRPCRoutes(t *testing.T) {
	time1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	time2 := time.Date(2022, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	time3 := time.Date(2023, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	tests := []struct {
		name     string
		m        map[types.NamespacedName]*gatewayapi_v1.GRPCRoute
		expected []*gatewayapi_v1.GRPCRoute
	}{
		{
			name: "3 grpcroutes, with different timestamp, earlier one should be first ",
			m: map[types.NamespacedName]*gatewayapi_v1.GRPCRoute{
				{
					Namespace: "ns", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time3),
					},
				},
				{
					Namespace: "ns", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					Namespace: "ns", Name: "name3",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.GRPCRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time3),
					},
				},
			},
		},
		{
			name: "3 grpcroutes with same creation timestamps, same namespaces, smaller name comes first",
			m: map[types.NamespacedName]*gatewayapi_v1.GRPCRoute{
				{
					Namespace: "ns", Name: "name3",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.GRPCRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
		},
		{
			name: "3 grpcroutes with same creation timestamp, smaller namespaces comes first",
			m: map[types.NamespacedName]*gatewayapi_v1.GRPCRoute{
				{
					Namespace: "ns3", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns2", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns1", Name: "name3",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.GRPCRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name3",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
		},
		{
			name: "mixed order, two with same creation timestamp, two with same name",
			m: map[types.NamespacedName]*gatewayapi_v1.GRPCRoute{
				{
					Namespace: "ns1", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					Namespace: "ns2", Name: "name2",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns1", Name: "name1",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
			},
			expected: []*gatewayapi_v1.GRPCRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name1",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name2",
						CreationTimestamp: meta_v1.NewTime(time2),
					},
				},
			},
		},
		{
			name: "same name, same timestamp, different namespace",
			m: map[types.NamespacedName]*gatewayapi_v1.GRPCRoute{
				{
					Namespace: "ns3", Name: "name",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns2", Name: "name",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					Namespace: "ns1", Name: "name",
				}: {
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
			expected: []*gatewayapi_v1.GRPCRoute{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns1",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns2",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace:         "ns3",
						Name:              "name",
						CreationTimestamp: meta_v1.NewTime(time1),
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := sortGRPCRoutes(tc.m)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func TestHasConflictRoute(t *testing.T) {
	host1 := "test.projectcontour.io"
	host2 := "test1.projectcontour.io"
	hosts := sets.New(
		host1,
		host2,
	)
	listener := &listenerInfo{
		listener: gatewayapi_v1.Listener{
			Name: "l1",
			AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
				Namespaces: &gatewayapi_v1.RouteNamespaces{
					From: ptr.To(gatewayapi_v1.NamespacesFromSame),
				},
			},
		},
		dagListenerName: "l1",
		allowedKinds:    []gatewayapi_v1.Kind{KindHTTPRoute},
		ready:           true,
	}
	tlsListener := &listenerInfo{
		listener: gatewayapi_v1.Listener{
			Name: "ltls",
			AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
				Namespaces: &gatewayapi_v1.RouteNamespaces{
					From: ptr.To(gatewayapi_v1.NamespacesFromSame),
				},
			},
		},
		allowedKinds:    []gatewayapi_v1.Kind{KindHTTPRoute},
		ready:           true,
		dagListenerName: "ltls",
		tlsSecret:       &Secret{},
	}
	tests := []struct {
		name             string
		existingRoutes   []*Route
		routes           []*Route
		listener         *listenerInfo
		expectedConflict bool
	}{
		{
			name: "There are 2 existing httproute, the 3rd route to add doesn't have conflict, listen doesn't have tls, no conflict expected",
			existingRoutes: []*Route{
				{
					Name:               "route1",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               KindHTTPRoute,
					Name:               "route2",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "version", Value: "2", MatchType: "exact", Invert: false},
					},
				},
			},
			routes: []*Route{
				{
					Kind:               KindHTTPRoute,
					Name:               "route3",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "e-tag", Value: "abc", MatchType: "contains", Invert: true},
					},
				},
			},
			listener: listener,
		},
		{
			name: "There are 2 existing grpcroute, the 3rd route to add doesn't have conflict, listen doesn't have tls, no conflict expected",
			existingRoutes: []*Route{
				{
					Name:               "route1",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               KindGRPCRoute,
					Name:               "route2",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "version", Value: "2", MatchType: "exact", Invert: false},
					},
				},
			},
			routes: []*Route{
				{
					Kind:               KindGRPCRoute,
					Name:               "route3",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "e-tag", Value: "abc", MatchType: "contains", Invert: true},
					},
				},
			},
			listener: listener,
		},
		{
			name: "There are 2 existing route, the 3rd route to add doesn't have conflict, listen has tls, no conflict expected",
			existingRoutes: []*Route{
				{
					Name:               "route1",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
				{
					Kind:               KindHTTPRoute,
					Name:               "route2",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "version", Value: "2", MatchType: "exact", Invert: false},
					},
				},
			},
			routes: []*Route{
				{
					Kind:               KindHTTPRoute,
					Name:               "route3",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "e-tag", Value: "abc", MatchType: "contains", Invert: true},
					},
				},
			},
			listener: tlsListener,
		},
		{
			name: "There are 2 existing route, the 3rd route to add is conflict with 2nd route, listen doesn't have tls, expect conflicts",
			existingRoutes: []*Route{
				{
					Name:               "route1",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path1"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: ":authority", MatchType: HeaderMatchTypeRegex, Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?"},
					},
				},
				{
					Kind:               KindHTTPRoute,
					Name:               "route2",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "version", Value: "2", MatchType: "exact", Invert: false},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			routes: []*Route{
				{
					Kind:               KindHTTPRoute,
					Name:               "route3",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "version", Value: "2", MatchType: "exact", Invert: false},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			listener:         listener,
			expectedConflict: true,
		},
		{
			name: "There are 1 existing route, the 2nd route to add is conflict with 1st route, listen has tls, expect conflicts",
			existingRoutes: []*Route{
				{
					Kind:               KindHTTPRoute,
					Name:               "route1",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "version", Value: "2", MatchType: "exact", Invert: false},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			routes: []*Route{
				{
					Kind:               KindHTTPRoute,
					Name:               "route2",
					Namespace:          "default",
					PathMatchCondition: prefixSegment("/path2"),
					HeaderMatchConditions: []HeaderMatchCondition{
						{Name: "version", Value: "2", MatchType: "exact", Invert: false},
					},
					QueryParamMatchConditions: []QueryParamMatchCondition{
						{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
					},
				},
			},
			listener:         tlsListener,
			expectedConflict: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
				dag: &DAG{
					Listeners: map[string]*Listener{
						tlsListener.dagListenerName: {
							svhostsByName: map[string]*SecureVirtualHost{
								host1: {
									VirtualHost: VirtualHost{
										Name:   host1,
										Routes: make(map[string]*Route),
									},
								},
							},
						},
						listener.dagListenerName: {
							vhostsByName: map[string]*VirtualHost{
								host2: {
									Routes: make(map[string]*Route),
								},
							},
						},
					},
				},
			}
			for host := range hosts {
				for _, route := range tc.existingRoutes {
					switch {
					case tc.listener.tlsSecret != nil:
						svhost := processor.dag.EnsureSecureVirtualHost(tc.listener.dagListenerName, host)
						svhost.Secret = listener.tlsSecret
						svhost.AddRoute(route)
					default:
						vhost := processor.dag.EnsureVirtualHost(tc.listener.dagListenerName, host)
						vhost.AddRoute(route)
					}
				}
			}

			res := processor.hasConflictRoute(tc.listener, hosts, tc.routes)
			assert.Equal(t, tc.expectedConflict, res)
		})
	}
}
