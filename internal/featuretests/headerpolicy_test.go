// Copyright © 2019 VMware
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

package featuretests

import (
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"

	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestHeaderPolicy_ReplaceHeader_HTTProxy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc1",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name:  "Host",
						Value: "goodbye.planet",
					}},
				},
			}},
		},
	})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/svc1/80/da39a3ee5e", "goodbye.planet"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	// Non-Host header
	rh.OnAdd(&projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name:  "x-header",
						Value: "goodbye.planet",
					}},
				},
			}},
		},
	})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
						RequestHeadersToAdd: []*envoy_api_v2_core.HeaderValueOption{{
							Header: &envoy_api_v2_core.HeaderValue{
								Key:   "X-Header",
								Value: "goodbye.planet",
							},
							Append: &wrappers.BoolValue{
								Value: false,
							},
						}},
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	// Empty value for replaceHeader in HeadersPolicy
	rh.OnAdd(&projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "hello.world"},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "svc1",
					Port: 80,
				}},
				RequestHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name: "Host",
					}},
				},
			}},
		},
	})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "externalname",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "https,443",
			},
		},
		Spec: v1.ServiceSpec{
			ExternalName: "goodbye.planet",
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     443,
				Name:     "https",
			}},
		},
	})

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	// Proxy with SNI
	rh.OnAdd(&projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "hello.world",
				TLS:  &projcontour.TLS{SecretName: "foo"},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "externalname",
					Port: 443,
				}},
				RequestHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name:  "Host",
						Value: "goodbye.planet",
					}},
				},
			}},
		},
	})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match: routePrefix("/"),
						Action: &envoy_api_v2_route.Route_Redirect{
							Redirect: &envoy_api_v2_route.RedirectAction{
								SchemeRewriteSpecifier: &envoy_api_v2_route.RedirectAction_HttpsRedirect{
									HttpsRedirect: true,
								},
							},
						},
					}),
			),
			envoy.RouteConfiguration("ingress_https",
				envoy.VirtualHost("hello.world",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routeHostRewrite("default/externalname/443/da39a3ee5e", "goodbye.planet"),
					},
				)),
		),
		TypeUrl: routeType,
	})

	c.Request(clusterType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/externalname/443/da39a3ee5e", "default/externalname/https", "default_externalname_443"), nil, "goodbye.planet", ""),
		),
		TypeUrl: clusterType,
	})
}
