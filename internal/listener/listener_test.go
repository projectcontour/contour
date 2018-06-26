// Copyright © 2018 Heptio
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

package listener

import (
	"reflect"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestListenerVisit(t *testing.T) {
	tests := map[string]struct {
		objs []interface{}
		want map[string]*v2.Listener
	}{
		"nothing": {
			objs: nil,
			want: map[string]*v2.Listener{},
		},
		"one http only ingress": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
			},
			want: map[string]*v2.Listener{
				ENVOY_HTTP_LISTENER: &v2.Listener{
					Name:    ENVOY_HTTP_LISTENER,
					Address: socketaddress("0.0.0.0", 8080),
					FilterChains: []listener.FilterChain{
						filterchain(false, httpfilter(ENVOY_HTTP_LISTENER, DEFAULT_HTTP_ACCESS_LOG)),
					},
				},
			},
		},
		"one http only ingressroute": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
			},
			want: map[string]*v2.Listener{
				ENVOY_HTTP_LISTENER: &v2.Listener{
					Name:    ENVOY_HTTP_LISTENER,
					Address: socketaddress("0.0.0.0", 8080),
					FilterChains: []listener.FilterChain{
						filterchain(false, httpfilter(ENVOY_HTTP_LISTENER, DEFAULT_HTTP_ACCESS_LOG)),
					},
				},
			},
		},
		"simple ingress with secret": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
			},
			want: map[string]*v2.Listener{
				ENVOY_HTTP_LISTENER: &v2.Listener{
					Name:    ENVOY_HTTP_LISTENER,
					Address: socketaddress("0.0.0.0", 8080),
					FilterChains: []listener.FilterChain{
						filterchain(false, httpfilter(ENVOY_HTTP_LISTENER, DEFAULT_HTTP_ACCESS_LOG)),
					},
				},
				ENVOY_HTTPS_LISTENER: &v2.Listener{
					Name:    ENVOY_HTTPS_LISTENER,
					Address: socketaddress("0.0.0.0", 8443),
					FilterChains: []listener.FilterChain{{
						FilterChainMatch: &listener.FilterChainMatch{
							SniDomains: []string{"whatever.example.com"},
						},
						TlsContext: tlscontext(secretdata("certificate", "key"), auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
						Filters: []listener.Filter{
							httpfilter(ENVOY_HTTPS_LISTENER, DEFAULT_HTTPS_ACCESS_LOG),
						},
					}},
				},
			},
		},
		"simple ingress with missing secret": {
			objs: []interface{}{
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "missing",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
			},
			want: map[string]*v2.Listener{
				ENVOY_HTTP_LISTENER: &v2.Listener{
					Name:    ENVOY_HTTP_LISTENER,
					Address: socketaddress("0.0.0.0", 8080),
					FilterChains: []listener.FilterChain{
						filterchain(false, httpfilter(ENVOY_HTTP_LISTENER, DEFAULT_HTTP_ACCESS_LOG)),
					},
				},
			},
		},
		"simple ingressroute with secret": {
			objs: []interface{}{
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &ingressroutev1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("certificate", "key"),
				},
			},
			want: map[string]*v2.Listener{
				ENVOY_HTTP_LISTENER: &v2.Listener{
					Name:    ENVOY_HTTP_LISTENER,
					Address: socketaddress("0.0.0.0", 8080),
					FilterChains: []listener.FilterChain{
						filterchain(false, httpfilter(ENVOY_HTTP_LISTENER, DEFAULT_HTTP_ACCESS_LOG)),
					},
				},
				ENVOY_HTTPS_LISTENER: &v2.Listener{
					Name:    ENVOY_HTTPS_LISTENER,
					Address: socketaddress("0.0.0.0", 8443),
					FilterChains: []listener.FilterChain{{
						FilterChainMatch: &listener.FilterChainMatch{
							SniDomains: []string{"www.example.com"},
						},
						TlsContext: tlscontext(secretdata("certificate", "key"), auth.TlsParameters_TLSv1_1, "h2", "http/1.1"),
						Filters: []listener.Filter{
							httpfilter(ENVOY_HTTPS_LISTENER, DEFAULT_HTTPS_ACCESS_LOG),
						},
					}},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var d dag.DAG
			for _, o := range tc.objs {
				d.Insert(o)
			}
			d.Recompute()
			v := Visitor{
				DAG: &d,
			}
			got := v.Visit()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%+v\ngot:\n%+v", tc.want, got)
			}
		})
	}
}

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
	}
}
