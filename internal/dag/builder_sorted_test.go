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

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOriginalRouting(t *testing.T) {
	proxyObject := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "unique.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_api_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Regex: "/fizzbuzz.*",
					}},
					Services: []contour_api_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Exact: "/foobar",
					}},
					Services: []contour_api_v1.Service{{
						Name: "kuarder",
						Port: 8080,
					}},
				},
			},
		},
	}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	// s2 is like s1 but with a different name
	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	tests := []struct {
		name            string
		objs            []any
		want            []*Listener
		ingressReqHp    *HeadersPolicy
		ingressRespHp   *HeadersPolicy
		httpProxyReqHp  *HeadersPolicy
		httpProxyRespHp *HeadersPolicy
		wantErr         error
	}{
		{
			name: "VHost should be marked as needs to be sorted",
			objs: []any{
				proxyObject, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("unique.com", true,
							&Route{
								PathMatchCondition: prefixString("/"),
								Clusters:           clusters(service(s1)),
							},
							&Route{
								PathMatchCondition: regex("/fizzbuzz.*"),
								Clusters:           clusters(service(s1)),
							},
							&Route{
								PathMatchCondition: exact("/foobar"),
								Clusters:           clusters(service(s2)),
							},
						),
					),
				},
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					FieldLogger: fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger:           fixture.NewTestLogger(t),
						RequestHeadersPolicy:  tc.ingressReqHp,
						ResponseHeadersPolicy: tc.ingressRespHp,
					},
					&HTTPProxyProcessor{
						ShouldSortRoutes:      true,
						RequestHeadersPolicy:  tc.httpProxyReqHp,
						ResponseHeadersPolicy: tc.httpProxyRespHp,
					},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			for _, l := range dag.Listeners {
				got[l.Port] = l
			}

			want := make(map[int]*Listener)
			for _, l := range tc.want {
				want[l.Port] = l
			}
			assert.Equal(t, want, got)
		})
	}
}
