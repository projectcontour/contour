// Copyright © 2017 Heptio
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

package contour

import (
	"reflect"
	"sort"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTranslatorCacheOnAddIngress(t *testing.T) {
	tests := map[string]struct {
		i             v1beta1.Ingress
		wantIngresses map[metadata]*v1beta1.Ingress
		wantVhosts    map[string]map[metadata]*v1beta1.Ingress
	}{
		"add default ingress": {
			i: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: backend("simple", intstr.FromInt(80)),
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("simple", intstr.FromInt(80)),
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"*": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Backend: backend("simple", intstr.FromInt(80)),
						},
					},
				},
			},
		},
		"add default rule ingress": {
			i: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"*": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
			},
		},
		"add default and path default ingress": {
			i: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: backend("simple", intstr.FromInt(80)),
					Rules: []v1beta1.IngressRule{{
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path: "/hello",
									Backend: v1beta1.IngressBackend{
										ServiceName: "hello",
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("simple", intstr.FromInt(80)),
						Rules: []v1beta1.IngressRule{{
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Path: "/hello",
										Backend: v1beta1.IngressBackend{
											ServiceName: "hello", ServicePort: intstr.FromInt(80)},
									}},
								},
							},
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"*": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{Backend: backend("simple", intstr.FromInt(80)), Rules: []v1beta1.IngressRule{{IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{Paths: []v1beta1.HTTPIngressPath{{
								Path: "/hello", Backend: v1beta1.IngressBackend{
									ServiceName: "hello", ServicePort: intstr.FromInt(80)},
							}},
							},
						}}},
						},
					},
				},
			},
		},
		"add default and host ingress": {
			i: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: backend("simple", intstr.FromInt(80)),
					Rules: []v1beta1.IngressRule{{
						Host: "hello.example.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Backend: v1beta1.IngressBackend{
										ServiceName: "hello",
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Backend: backend("simple", intstr.FromInt(80)),
						Rules: []v1beta1.IngressRule{{
							Host: "hello.example.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Backend: v1beta1.IngressBackend{
											ServiceName: "hello",
											ServicePort: intstr.FromInt(80),
										},
									}},
								},
							},
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"*": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Backend: backend("simple", intstr.FromInt(80)),
							Rules: []v1beta1.IngressRule{{
								Host: "hello.example.com",
								IngressRuleValue: v1beta1.IngressRuleValue{
									HTTP: &v1beta1.HTTPIngressRuleValue{
										Paths: []v1beta1.HTTPIngressPath{{
											Backend: v1beta1.IngressBackend{
												ServiceName: "hello",
												ServicePort: intstr.FromInt(80),
											},
										}},
									},
								},
							}},
						},
					},
				},
				"hello.example.com": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Backend: backend("simple", intstr.FromInt(80)),
							Rules: []v1beta1.IngressRule{{
								Host: "hello.example.com",
								IngressRuleValue: v1beta1.IngressRuleValue{
									HTTP: &v1beta1.HTTPIngressRuleValue{
										Paths: []v1beta1.HTTPIngressPath{{
											Backend: v1beta1.IngressBackend{
												ServiceName: "hello",
												ServicePort: intstr.FromInt(80),
											},
										}},
									},
								},
							}},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var c translatorCache
			c.OnAdd(&tc.i)
			if !reflect.DeepEqual(tc.wantIngresses, c.ingresses) {
				t.Errorf("want:\n%v\n got:\n%v", tc.wantIngresses, c.ingresses)
			}
			if !reflect.DeepEqual(tc.wantVhosts, c.vhosts) {
				t.Fatalf("want:\n%v\n got:\n%v", tc.wantVhosts, c.vhosts)
			}
		})
	}
}

func TestTranslatorAddIngressRoute(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*Translator)
		route         *ingressroutev1.IngressRoute
		ingress_http  []proto.Message
		ingress_https []proto.Message
	}{
		{
			name: "default backend",
			route: &ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "",
					},
					Routes: []ingressroutev1.Route{
						ingressroutev1.Route{
							Match: "/",
							Services: []ingressroutev1.Service{
								ingressroutev1.Service{
									Name: "backend",
									Port: 80,
								},
							},
						},
					},
				},
			},
			ingress_http: []proto.Message{
				&route.VirtualHost{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []route.Route{{
						Match: prefixmatch("/"),
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name: "default/backend/80",
												Weight: &types.UInt32Value{
													Value: uint32(100),
												},
											},
										},
									},
								},
							},
						},
					}},
				},
			},
			ingress_https: []proto.Message{},
		},
		{
			name: "basic hostname",
			route: &ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "foo.bar",
					},
					Routes: []ingressroutev1.Route{
						ingressroutev1.Route{
							Match: "/",
							Services: []ingressroutev1.Service{
								ingressroutev1.Service{
									Name: "backend",
									Port: 80,
								},
							},
						},
					},
				},
			},
			ingress_http: []proto.Message{
				&route.VirtualHost{
					Name:    "foo.bar",
					Domains: []string{"foo.bar", "foo.bar:80"},
					Routes: []route.Route{{
						Match: prefixmatch("/"),
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name: "default/backend/80",
												Weight: &types.UInt32Value{
													Value: uint32(100),
												},
											},
										},
									},
								},
							},
						},
					}},
				},
			},
			ingress_https: []proto.Message{},
		},
		{
			name: "basic path",
			route: &ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "foo.bar",
					},
					Routes: []ingressroutev1.Route{
						ingressroutev1.Route{
							Match: "/zed",
							Services: []ingressroutev1.Service{
								ingressroutev1.Service{
									Name: "backend",
									Port: 80,
								},
							},
						},
					},
				},
			},
			ingress_http: []proto.Message{
				&route.VirtualHost{
					Name:    "foo.bar",
					Domains: []string{"foo.bar", "foo.bar:80"},
					Routes: []route.Route{{
						Match: prefixmatch("/zed"),
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name: "default/backend/80",
												Weight: &types.UInt32Value{
													Value: uint32(100),
												},
											},
										},
									},
								},
							},
						},
					}},
				},
			},
			ingress_https: []proto.Message{},
		},
		{
			name: "multiple routes",
			route: &ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "foo.bar",
					},
					Routes: []ingressroutev1.Route{
						ingressroutev1.Route{
							Match: "/zed",
							Services: []ingressroutev1.Service{
								ingressroutev1.Service{
									Name: "backend",
									Port: 80,
								},
							},
						},
						ingressroutev1.Route{
							Match: "/",
							Services: []ingressroutev1.Service{
								ingressroutev1.Service{
									Name: "backendtwo",
									Port: 80,
								},
							},
						},
					},
				},
			},
			ingress_http: []proto.Message{
				&route.VirtualHost{
					Name:    "foo.bar",
					Domains: []string{"foo.bar", "foo.bar:80"},
					Routes: []route.Route{{
						Match: prefixmatch("/zed"),
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name: "default/backend/80",
												Weight: &types.UInt32Value{
													Value: uint32(100),
												},
											},
										},
									},
								},
							},
						},
					},
						{
							Match: prefixmatch("/"),
							Action: &route.Route_Route{
								Route: &route.RouteAction{
									ClusterSpecifier: &route.RouteAction_WeightedClusters{
										WeightedClusters: &route.WeightedCluster{
											Clusters: []*route.WeightedCluster_ClusterWeight{
												{
													Name: "default/backendtwo/80",
													Weight: &types.UInt32Value{
														Value: uint32(100),
													},
												},
											},
										},
									},
								},
							},
						}},
				},
			},
			ingress_https: []proto.Message{},
		},
	}

	log := testLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			if tc.setup != nil {
				tc.setup(tr)
			}
			tr.OnAdd(tc.route)
			got := contents(&tr.VirtualHostCache.HTTP)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_http, got) {
				t.Fatalf("(ingress_http) want:\n%v\n got:\n%v", tc.ingress_http, got)
			}

			got = contents(&tr.VirtualHostCache.HTTPS)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_https, got) {
				t.Fatalf("(ingress_https) want:\n%v\n got:\n%v", tc.ingress_https, got)
			}
		})
	}
}

func TestTranslatorCacheOnUpdateIngress(t *testing.T) {
	tests := map[string]struct {
		c              translatorCache
		oldObj, newObj v1beta1.Ingress
		wantIngresses  map[metadata]*v1beta1.Ingress
		wantVhosts     map[string]map[metadata]*v1beta1.Ingress
	}{
		"update default ingress": {
			c: translatorCache{
				ingresses: map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Backend: backend("simple", intstr.FromInt(80)),
						},
					},
				},
				vhosts: map[string]map[metadata]*v1beta1.Ingress{
					"*": map[metadata]*v1beta1.Ingress{
						metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: v1beta1.IngressSpec{
								Backend: backend("simple", intstr.FromInt(80)),
							},
						},
					},
				},
			},
			oldObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: backend("simple", intstr.FromInt(80)),
				},
			},
			newObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"*": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
			},
		},
		"update default with host ingress": {
			c: translatorCache{
				ingresses: map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
				vhosts: map[string]map[metadata]*v1beta1.Ingress{
					"*": map[metadata]*v1beta1.Ingress{
						metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: v1beta1.IngressSpec{
								Rules: []v1beta1.IngressRule{{
									IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
								}},
							},
						},
					},
				},
			},
			oldObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			newObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "hello.example.com",
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							Host:             "hello.example.com",
							IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"hello.example.com": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								Host:             "hello.example.com",
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
			},
		},
		"update host ingress to default": {
			c: translatorCache{
				ingresses: map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								Host:             "hello.example.com",
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
				vhosts: map[string]map[metadata]*v1beta1.Ingress{
					"hello.example.com": map[metadata]*v1beta1.Ingress{
						metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: v1beta1.IngressSpec{
								Rules: []v1beta1.IngressRule{{
									Host:             "hello.example.com",
									IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
								}},
							},
						},
					},
				},
			},
			oldObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "hello.example.com",
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			newObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"*": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
			},
		},
		"update rename host ingress": {
			c: translatorCache{
				ingresses: map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								Host:             "hello.example.com",
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
				vhosts: map[string]map[metadata]*v1beta1.Ingress{
					"hello.example.com": map[metadata]*v1beta1.Ingress{
						metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: v1beta1.IngressSpec{
								Rules: []v1beta1.IngressRule{{
									Host:             "hello.example.com",
									IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
								}},
							},
						},
					},
				},
			},
			oldObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "hello.example.com",
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			newObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "goodbye.example.com",
						IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							Host:             "goodbye.example.com",
							IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"goodbye.example.com": map[metadata]*v1beta1.Ingress{
					metadata{name: "simple", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								Host:             "goodbye.example.com",
								IngressRuleValue: ingressrulevalue(backend("simple", intstr.FromInt(80))),
							}},
						},
					},
				},
			},
		},
		"move rename default ingress to named vhost without renaming object": { // issue 257
			c: translatorCache{
				ingresses: map[metadata]*v1beta1.Ingress{
					metadata{name: "kuard-ing", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard-ing",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Backend: &v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						},
					},
				},
				vhosts: map[string]map[metadata]*v1beta1.Ingress{
					"*": map[metadata]*v1beta1.Ingress{
						metadata{name: "kuard-ing", namespace: "default"}: &v1beta1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "kuard-ing",
								Namespace: "default",
							},
							Spec: v1beta1.IngressSpec{
								Backend: &v1beta1.IngressBackend{
									ServiceName: "kuard",
									ServicePort: intstr.FromInt(80),
								},
							},
						},
					},
				},
			},
			oldObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard-ing",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: &v1beta1.IngressBackend{
						ServiceName: "kuard",
						ServicePort: intstr.FromInt(80),
					},
				},
			},
			newObj: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard-ing",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "kuard.db.gd-ms.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path: "/",
									Backend: v1beta1.IngressBackend{
										ServiceName: "kuard",
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			},
			wantIngresses: map[metadata]*v1beta1.Ingress{
				metadata{name: "kuard-ing", namespace: "default"}: &v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard-ing",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						Rules: []v1beta1.IngressRule{{
							Host: "kuard.db.gd-ms.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{{
										Path: "/",
										Backend: v1beta1.IngressBackend{
											ServiceName: "kuard",
											ServicePort: intstr.FromInt(80),
										},
									}},
								},
							},
						}},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*v1beta1.Ingress{
				"kuard.db.gd-ms.com": map[metadata]*v1beta1.Ingress{
					metadata{name: "kuard-ing", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard-ing",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Rules: []v1beta1.IngressRule{{
								Host: "kuard.db.gd-ms.com",
								IngressRuleValue: v1beta1.IngressRuleValue{
									HTTP: &v1beta1.HTTPIngressRuleValue{
										Paths: []v1beta1.HTTPIngressPath{{
											Path: "/",
											Backend: v1beta1.IngressBackend{
												ServiceName: "kuard",
												ServicePort: intstr.FromInt(80),
											},
										}},
									},
								},
							}},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.c.OnUpdate(&tc.oldObj, &tc.newObj)
			if !reflect.DeepEqual(tc.wantIngresses, tc.c.ingresses) {
				t.Errorf("ingresses want:\n%v\n got:\n%v", tc.wantIngresses, tc.c.ingresses)
			}
			if !reflect.DeepEqual(tc.wantVhosts, tc.c.vhosts) {
				t.Fatalf("vhosts want:\n%+v\n got:\n%+v", tc.wantVhosts, tc.c.vhosts)
			}
		})
	}
}

func TestTranslatorCacheOnUpdateIngressRoute(t *testing.T) {
	tests := map[string]struct {
		c              translatorCache
		oldObj, newObj ingressroutev1.IngressRoute
		wantRoutes     map[metadata]*ingressroutev1.IngressRoute
		wantVhosts     map[string]map[metadata]*ingressroutev1.IngressRoute
	}{
		"ingressroute update default ingress": {
			c: translatorCache{
				routes: map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
								},
							},
						},
					},
				},
				vhostroutes: map[string]map[metadata]*ingressroutev1.IngressRoute{
					"*": map[metadata]*ingressroutev1.IngressRoute{
						metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: ingressroutev1.IngressRouteSpec{
								Routes: []ingressroutev1.Route{
									{
										Services: []ingressroutev1.Service{
											{
												Name: "simple",
												Port: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			oldObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			newObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			wantRoutes: map[metadata]*ingressroutev1.IngressRoute{
				metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "simple",
										Port: 80,
									},
								},
							},
						},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*ingressroutev1.IngressRoute{
				"*": map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"ingressroute update default with host ingress": {
			c: translatorCache{
				routes: map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
								},
							},
						},
					},
				},
				vhostroutes: map[string]map[metadata]*ingressroutev1.IngressRoute{
					"*": map[metadata]*ingressroutev1.IngressRoute{
						metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: ingressroutev1.IngressRouteSpec{
								Routes: []ingressroutev1.Route{
									{
										Services: []ingressroutev1.Service{
											{
												Name: "simple",
												Port: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			oldObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			newObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "hello.example.com",
					},
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			wantRoutes: map[metadata]*ingressroutev1.IngressRoute{
				metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "hello.example.com",
						},
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "simple",
										Port: 80,
									},
								},
							},
						},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*ingressroutev1.IngressRoute{
				"hello.example.com": map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							VirtualHost: &ingressroutev1.VirtualHost{
								Fqdn: "hello.example.com",
							},
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"ingressroute update host ingress to default": {
			c: translatorCache{
				routes: map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							VirtualHost: &ingressroutev1.VirtualHost{
								Fqdn: "hello.example.com",
							},
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
								},
							},
						},
					},
				},
				vhostroutes: map[string]map[metadata]*ingressroutev1.IngressRoute{
					"hello.example.com": map[metadata]*ingressroutev1.IngressRoute{
						metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: ingressroutev1.IngressRouteSpec{
								VirtualHost: &ingressroutev1.VirtualHost{
									Fqdn: "hello.example.com",
								},
								Routes: []ingressroutev1.Route{
									{
										Services: []ingressroutev1.Service{
											{
												Name: "simple",
												Port: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			oldObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "hello.example.com",
					},
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			newObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			wantRoutes: map[metadata]*ingressroutev1.IngressRoute{
				metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "simple",
										Port: 80,
									},
								},
							},
						},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*ingressroutev1.IngressRoute{
				"*": map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"ingressroute update rename host ingress": {
			c: translatorCache{
				routes: map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							VirtualHost: &ingressroutev1.VirtualHost{
								Fqdn: "hello.example.com",
							},
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
								},
							},
						},
					},
				},
				vhostroutes: map[string]map[metadata]*ingressroutev1.IngressRoute{
					"hello.example.com": map[metadata]*ingressroutev1.IngressRoute{
						metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "simple",
								Namespace: "default",
							},
							Spec: ingressroutev1.IngressRouteSpec{
								VirtualHost: &ingressroutev1.VirtualHost{
									Fqdn: "hello.example.com",
								},
								Routes: []ingressroutev1.Route{
									{
										Services: []ingressroutev1.Service{
											{
												Name: "simple",
												Port: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			oldObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "hello.example.com",
					},
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			newObj: ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "goodbye.example.com",
					},
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "simple",
									Port: 80,
								},
							},
						},
					},
				},
			},
			wantRoutes: map[metadata]*ingressroutev1.IngressRoute{
				metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "goodbye.example.com",
						},
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "simple",
										Port: 80,
									},
								},
							},
						},
					},
				},
			},
			wantVhosts: map[string]map[metadata]*ingressroutev1.IngressRoute{
				"goodbye.example.com": map[metadata]*ingressroutev1.IngressRoute{
					metadata{name: "simple", namespace: "default"}: &ingressroutev1.IngressRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "simple",
							Namespace: "default",
						},
						Spec: ingressroutev1.IngressRouteSpec{
							VirtualHost: &ingressroutev1.VirtualHost{
								Fqdn: "goodbye.example.com",
							},
							Routes: []ingressroutev1.Route{
								{
									Services: []ingressroutev1.Service{
										{
											Name: "simple",
											Port: 80,
										},
									},
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
			tc.c.OnUpdate(&tc.oldObj, &tc.newObj)
			if !reflect.DeepEqual(tc.wantRoutes, tc.c.routes) {
				t.Errorf("routes want:\n%v\n got:\n%v", tc.wantRoutes, tc.c.routes)
			}
			if !reflect.DeepEqual(tc.wantVhosts, tc.c.vhostroutes) {
				t.Fatalf("vhosts want:\n%+v\n got:\n%+v", tc.wantVhosts, tc.c.vhostroutes)
			}
		})
	}
}

func TestTranslatorCacheOnDeleteIngress(t *testing.T) {
	tests := map[string]struct {
		c             translatorCache
		i             v1beta1.Ingress
		wantIngresses map[metadata]*v1beta1.Ingress
		wantVhosts    map[string]map[metadata]*v1beta1.Ingress
	}{
		"remove default ingress": {
			c: translatorCache{
				ingresses: map[metadata]*v1beta1.Ingress{
					metadata{name: "kuard-ing", namespace: "default"}: &v1beta1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kuard-ing",
							Namespace: "default",
						},
						Spec: v1beta1.IngressSpec{
							Backend: &v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						},
					},
				},
				vhosts: map[string]map[metadata]*v1beta1.Ingress{
					"*": map[metadata]*v1beta1.Ingress{
						metadata{name: "kuard-ing", namespace: "default"}: &v1beta1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "kuard-ing",
								Namespace: "default",
							},
							Spec: v1beta1.IngressSpec{
								Backend: &v1beta1.IngressBackend{
									ServiceName: "kuard",
									ServicePort: intstr.FromInt(80),
								},
							},
						},
					},
				},
			},
			i: v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard-ing",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: &v1beta1.IngressBackend{
						ServiceName: "kuard",
						ServicePort: intstr.FromInt(80),
					},
				},
			},

			wantIngresses: map[metadata]*v1beta1.Ingress{},
			wantVhosts:    map[string]map[metadata]*v1beta1.Ingress{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.c.OnDelete(&tc.i)
			if !reflect.DeepEqual(tc.wantIngresses, tc.c.ingresses) {
				t.Errorf("want:\n%v\n got:\n%v", tc.wantIngresses, tc.c.ingresses)
			}
			if !reflect.DeepEqual(tc.wantVhosts, tc.c.vhosts) {
				t.Fatalf("want:\n%v\n got:\n%v", tc.wantVhosts, tc.c.vhosts)
			}
		})
	}
}

func TestTranslatorRemoveIngressRoute(t *testing.T) {
	tests := map[string]struct {
		setup         func(*Translator)
		route         *ingressroutev1.IngressRoute
		ingress_http  []proto.Message
		ingress_https []proto.Message
	}{
		"remove existing": {
			setup: func(tr *Translator) {
				tr.OnAdd(&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "httpbin.org",
						},
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "peter",
										Port: 80,
									},
								},
							},
						},
					},
				})
			},
			route: &ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "httpbin.org",
					},
					Routes: []ingressroutev1.Route{
						{
							Services: []ingressroutev1.Service{
								{
									Name: "peter",
									Port: 80,
								},
							},
						},
					},
				},
			},
			ingress_http:  []proto.Message{},
			ingress_https: []proto.Message{},
		},
		"remove different": {
			setup: func(tr *Translator) {
				tr.OnAdd(&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "httpbin.org",
						},
						Routes: []ingressroutev1.Route{
							{
								Match: "/",
								Services: []ingressroutev1.Service{
									{
										Name: "peter",
										Port: 80,
									},
								},
							},
						},
					},
				})
			},
			route: &ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "different",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "httpbin.org",
					},
					Routes: []ingressroutev1.Route{
						{
							Match: "/",
							Services: []ingressroutev1.Service{
								{
									Name: "peter",
									Port: 80,
								},
							},
						},
					},
				},
			},
			ingress_http: []proto.Message{
				&route.VirtualHost{
					Name:    "httpbin.org",
					Domains: []string{"httpbin.org", "httpbin.org:80"},
					Routes: []route.Route{{
						Match: prefixmatch("/"),
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name: "default/peter/80",
												Weight: &types.UInt32Value{
													Value: uint32(100),
												},
											},
										},
									},
								},
							},
						},
					}},
				},
			},
			ingress_https: []proto.Message{},
		},
		"remove non existant": {
			setup: func(*Translator) {},
			route: &ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{
						Fqdn: "httpbin.org",
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
			ingress_http:  []proto.Message{},
			ingress_https: []proto.Message{},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tc.setup(tr)
			tr.OnDelete(tc.route)

			got := contents(&tr.VirtualHostCache.HTTP)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_http, got) {
				t.Fatalf("(ingress_http) want:\n%v\n got:\n%v", tc.ingress_http, got)
			}

			got = contents(&tr.VirtualHostCache.HTTPS)
			sort.Stable(virtualHostsByName(got))
			if !reflect.DeepEqual(tc.ingress_https, got) {
				t.Fatalf("(ingress_https) want:\n%v\n got:\n%v", tc.ingress_https, got)
			}
		})
	}
}
