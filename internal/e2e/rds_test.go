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

// End to ends tests for translator to grpc operations.
package e2e

import (
	"context"
	"fmt"
	"path"
	"sort"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes/any"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// routeResources returns the given routes as a slice of any.Any, appropriately sorted.
func routeResources(t *testing.T, routes ...*v2.RouteConfiguration) []*any.Any {
	sort.Stable(sorter.For(routes))
	return resources(t, protobuf.AsMessages(routes)...)
}

// heptio/contour#172. Updating an object from
//
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: kuard
// spec:
//   backend:
//     serviceName: kuard
//     servicePort: 80
//
// to
//
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: kuard
// spec:
//   rules:
//   - http:
//       paths:
//       - path: /testing
//         backend:
//           serviceName: kuard
//           servicePort: 80
//
// fails to update the virtualhost cache.
func TestEditIngress(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	meta := metav1.ObjectMeta{Name: "kuard", Namespace: "default"}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// add default/kuard to translator.
	old := &v1beta1.Ingress{
		ObjectMeta: meta,
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(old)

	// check that it's been translated correctly.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*", &envoy_api_v2_route.Route{
					Match:  routePrefix("/"),
					Action: routecluster("default/kuard/80/da39a3ee5e"),
				}),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))

	// update old to new
	rh.OnUpdate(old, &v1beta1.Ingress{
		ObjectMeta: meta,
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/testing",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	// check that ingress_http has been updated.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*", &envoy_api_v2_route.Route{
					Match:  routePrefix("/testing"),
					Action: routecluster("default/kuard/80/da39a3ee5e"),
				}),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc))
}

// heptio/contour#101
// The path /hello should point to default/hello/80 on "*"
//
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: hello
// spec:
//   rules:
//   - http:
// 	 paths:
//       - path: /hello
//         backend:
//           serviceName: hello
//           servicePort: 80
func TestIngressPathRouteWithoutHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// add default/hello to translator.
	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
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
	})

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// check that it's been translated correctly.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/hello"),
						Action: routecluster("default/hello/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc))
}

func TestEditIngressInPlace(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wowie",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kerpow",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       9000,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s2)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc))

	// i2 is like i1 but adds a second route
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromInt(80),
							},
						}, {
							Path: "/whoop",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kerpow",
								ServicePort: intstr.FromInt(9000),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i1, i2)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/whoop"),
						Action: routecluster("default/kerpow/9000/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "3",
	}, streamRDS(t, cc))

	// i3 is like i2, but adds the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true"},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromInt(80),
							},
						}, {
							Path: "/whoop",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kerpow",
								ServicePort: intstr.FromInt(9000),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i2, i3)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "4",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/whoop"),
						Action: envoy.UpgradeHTTPS(),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: envoy.UpgradeHTTPS(),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "4",
	}, streamRDS(t, cc))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello-kitty",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	// i4 is the same as i3, and includes a TLS spec object to enable ingress_https routes
	// i3 is like i2, but adds the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true"},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"hello.example.com"},
				SecretName: "hello-kitty",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromInt(80),
							},
						}, {
							Path: "/whoop",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kerpow",
								ServicePort: intstr.FromInt(9000),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i3, i4)
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "5",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("hello.example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/whoop"),
						Action: envoy.UpgradeHTTPS(),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: envoy.UpgradeHTTPS(),
					},
				),
			),
			envoy.RouteConfiguration("https/hello.example.com",
				envoy.VirtualHost("hello.example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/whoop"),
						Action: routecluster("default/kerpow/9000/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "5",
	}, streamRDS(t, cc))
}

// contour#250 ingress.kubernetes.io/force-ssl-redirect: "true" should apply
// per route, not per vhost.
func TestSSLRedirectOverlay(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// i1 is a stock ingress with force-ssl-redirect on the / route
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "app-service",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-service",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	// i2 is an overlay to add the let's encrypt handler.
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: v1beta1.IngressBackend{
								ServiceName: "challenge-service",
								ServicePort: intstr.FromInt(8009),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "challenge-service",
			Namespace: "nginx-ingress",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8009,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	assertRDS(t, cc, "5", virtualhosts(
		envoy.VirtualHost("example.com",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
				Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
			},
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"), // match all
				Action: envoy.UpgradeHTTPS(),
			},
		),
	), virtualhosts(
		envoy.VirtualHost("example.com",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
				Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
			},
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"), // match all
				Action: routecluster("default/app-service/8080/da39a3ee5e"),
			},
		),
	))
}

func TestInvalidCertInIngress(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// Create an invalid TLS secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata("wrong", RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

	// Create a service
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	// Create an ingress that uses the invalid secret
	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "kuard-ing", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.io"},
				SecretName: "example-tls",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.io",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("kuard.io",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	// Correct the secret
	rh.OnUpdate(secret, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	assertRDS(t, cc, "2", virtualhosts(
		envoy.VirtualHost("kuard.io",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), virtualhosts(
		envoy.VirtualHost("kuard.io",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	))
}

// issue #257: editing default ingress did not remove original default route
func TestIssue257(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// apiVersion: networking/v1beta1
	// kind: Ingress
	// metadata:
	//   name: kuard-ing
	//   labels:
	//     app: kuard
	//   annotations:
	//     kubernetes.io/ingress.class: contour
	// spec:
	//   backend:
	//     serviceName: kuard
	//     servicePort: 80
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(i1)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	assertRDS(t, cc, "2", virtualhosts(
		envoy.VirtualHost("*",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	// apiVersion: networking/v1beta1
	// kind: Ingress
	// metadata:
	//   name: kuard-ing
	//   labhls:
	//     app: kuard
	//   annotations:
	//     kubernetes.io/ingress.class: contour
	// spec:
	//  rules:
	//  - host: kuard.db.gd-ms.com
	//    http:
	//      paths:
	//      - backend:
	//         serviceName: kuard
	//         servicePort: 80
	//        path: /
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
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
	}
	rh.OnUpdate(i1, i2)

	assertRDS(t, cc, "3", virtualhosts(
		envoy.VirtualHost("kuard.db.gd-ms.com",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)
}

func TestRDSFilter(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// i1 is a stock ingress with force-ssl-redirect on the / route
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "app-service",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-service",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// i2 is an overlay to add the let's encrypt handler.
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: v1beta1.IngressBackend{
								ServiceName: "challenge-service",
								ServicePort: intstr.FromInt(8009),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "challenge-service",
			Namespace: "nginx-ingress",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8009,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s2)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "5",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"), // match all
						Action: envoy.UpgradeHTTPS(),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "5",
	}, streamRDS(t, cc, "ingress_http"))

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "5",
		Resources: routeResources(t,
			envoy.RouteConfiguration("https/example.com",
				envoy.VirtualHost("example.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/app-service/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "5",
	}, streamRDS(t, cc, "https/example.com"))
}

// issue 404
func TestDefaultBackendDoesNotOverwriteNamedHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}, {
				Name:       "alt",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gui",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},

			Rules: []v1beta1.IngressRule{{
				Host: "test-gui",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "test-gui",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}, {
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/kuard",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	})

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/kuard"),
						Action: routecluster("default/kuard/8080/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
				envoy.VirtualHost("test-gui",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/test-gui/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc, "ingress_http"))
}

// Test DAGAdapter.IngressClass setting works, this could be done
// in LDS or RDS, or even CDS, but this test mirrors the place it's
// tested in internal/contour/route_test.go
func TestRDSIngressClassAnnotation(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.IngressClass = "linkerd"
	})
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("*",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/8080/da39a3ee5e"),
			},
		),
	), nil)

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i1, i2)
	assertRDS(t, cc, "2", nil, nil)

	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i2, i3)
	assertRDS(t, cc, "2", nil, nil)

	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i3, i4)
	assertRDS(t, cc, "3", virtualhosts(
		envoy.VirtualHost("*",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/8080/da39a3ee5e"),
			},
		),
	), nil)

	i5 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i4, i5)
	assertRDS(t, cc, "4", virtualhosts(
		envoy.VirtualHost("*",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/8080/da39a3ee5e"),
			},
		),
	), nil)

	rh.OnUpdate(i5, i3)
	assertRDS(t, cc, "5", nil, nil)
}

// issue 523, check for data races caused by accidentally
// sorting the contents of an RDS entry's virtualhost list.
func TestRDSAssertNoDataRaceDuringInsertAndStream(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	stop := make(chan struct{})

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	go func() {
		for i := 0; i < 100; i++ {
			rh.OnAdd(&projcontour.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("simple-%d", i),
					Namespace: "default",
				},
				Spec: projcontour.HTTPProxySpec{
					VirtualHost: &projcontour.VirtualHost{Fqdn: fmt.Sprintf("example-%d.com", i)},
					Routes: []projcontour.Route{{
						Conditions: []projcontour.MatchCondition{{
							Prefix: "/",
						}},
						Services: []projcontour.Service{{
							Name: "kuard",
							Port: 80,
						}},
					}},
				},
			})
		}
		close(stop)
	}()

	for {
		select {
		case <-stop:
			return
		default:
			streamRDS(t, cc)
		}
	}
}

// issue 606: spec.rules.host without a http key causes panic.
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: test-ingress3
// spec:
//   rules:
//   - host: test1.test.com
//   - host: test2.test.com
//     http:
//       paths:
//       - backend:
//           serviceName: network-test
//           servicePort: 9001
//         path: /
//
// note: this test caused a panic in dag.Builder, but testing the
// context of RDS is a good place to start.
func TestRDSIngressSpecMissingHTTPKey(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress3",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "test1.test.com",
			}, {
				Host: "test2.test.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "network-test",
								ServicePort: intstr.FromInt(9001),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "network-test",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       9001,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	assertRDS(t, cc, "2", virtualhosts(
		envoy.VirtualHost("test2.test.com",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/network-test/9001/da39a3ee5e"),
			},
		),
	), nil)
}

func TestRouteWithAServiceWeight(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []projcontour.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90, // ignored
				}},
			}},
		},
	}

	rh.OnAdd(p1)
	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("test2.test.com",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/a"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	p2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []projcontour.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	}

	rh.OnUpdate(p1, p2)
	assertRDS(t, cc, "2", virtualhosts(
		envoy.VirtualHost("test2.test.com",
			&envoy_api_v2_route.Route{
				Match: routePrefix("/a"),
				Action: routeweightedcluster(
					weightedcluster{"default/kuard/80/da39a3ee5e", 60},
					weightedcluster{"default/kuard/80/da39a3ee5e", 90}),
			},
		),
	), nil)
}

func TestRouteWithTLS(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &projcontour.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(p1)

	// check that ingress_http has been updated.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Action: envoy.UpgradeHTTPS(),
						Match:  routePrefix("/a"),
					},
				),
			),
			envoy.RouteConfiguration("https/test2.test.com",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/a"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))
}

func TestRouteWithTLS_InsecurePaths(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
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

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &projcontour.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/insecure",
				}},
				PermitInsecure: true,
				Services: []projcontour.Service{{Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/secure",
				}},
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(p1)

	// check that ingress_http has been updated.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: envoy.UpgradeHTTPS(),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("https/test2.test.com",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))
}

func TestRouteWithTLS_InsecurePaths_DisablePermitInsecureTrue(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.DisablePermitInsecure = true
	})

	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
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

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &projcontour.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/insecure",
				}},
				PermitInsecure: true,
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/secure",
				}},
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(p1)

	// check that ingress_http has been updated.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: envoy.UpgradeHTTPS(),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: envoy.UpgradeHTTPS(),
					},
				),
			),
			envoy.RouteConfiguration("https/test2.test.com",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))
}

// issue 1234, assert that RoutePrefix and RouteRegex work as expected
func TestRoutePrefixRouteRegex(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	meta := metav1.ObjectMeta{Name: "kuard", Namespace: "default"}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// add default/kuard to translator.
	old := &v1beta1.Ingress{
		ObjectMeta: meta,
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/[^/]+/invoices(/.*|/?)", // issue 1243
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(old)

	// check that it's been translated correctly.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("*",
					&envoy_api_v2_route.Route{
						Match:  routeRegex("/[^/]+/invoices(/.*|/?)"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))
}

func assertRDS(t *testing.T, cc *grpc.ClientConn, versioninfo string, ingress_http, ingress_https []*envoy_api_v2_route.VirtualHost) {
	t.Helper()

	routes := []*v2.RouteConfiguration{
		envoy.RouteConfiguration("ingress_http", ingress_http...),
	}

	for _, vh := range ingress_https {
		routes = append(routes,
			envoy.RouteConfiguration(path.Join("https", vh.Name), vh))
	}

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: versioninfo,
		Resources:   routeResources(t, routes...),
		TypeUrl:     routeType,
		Nonce:       versioninfo,
	}, streamRDS(t, cc))
}

func streamRDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewRouteDiscoveryServiceClient(cc)
	st, err := rds.StreamRoutes(context.TODO())
	check(t, err)
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       routeType,
		ResourceNames: rn,
	})
}

type weightedcluster struct {
	name   string
	weight uint32
}

func routeRegex(regex string, headers ...dag.HeaderMatchCondition) *envoy_api_v2_route.RouteMatch {
	return envoy.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.RegexMatchCondition{
			Regex: regex,
		},
		HeaderMatchConditions: headers,
	})
}

func routePrefix(prefix string, headers ...dag.HeaderMatchCondition) *envoy_api_v2_route.RouteMatch {
	return envoy.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
		HeaderMatchConditions: headers,
	})
}

func routecluster(cluster string) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func routeweightedcluster(clusters ...weightedcluster) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
				WeightedClusters: weightedclusters(clusters),
			},
		},
	}
}

func weightedclusters(clusters []weightedcluster) *envoy_api_v2_route.WeightedCluster {
	var wc envoy_api_v2_route.WeightedCluster
	var total uint32
	for _, c := range clusters {
		total += c.weight
		wc.Clusters = append(wc.Clusters, &envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: protobuf.UInt32(c.weight),
		})
	}
	wc.TotalWeight = protobuf.UInt32(total)
	return &wc
}

func service(ns, name string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

func TestHTTPProxyRouteWithAServiceWeight(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []projcontour.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90, // ignored
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)
	assertRDS(t, cc, "1", virtualhosts(
		envoy.VirtualHost("test2.test.com",
			&envoy_api_v2_route.Route{
				Match:  routePrefix("/a"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []projcontour.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []projcontour.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	}

	rh.OnUpdate(proxy1, proxy2)
	assertRDS(t, cc, "2", virtualhosts(
		envoy.VirtualHost("test2.test.com",
			&envoy_api_v2_route.Route{
				Match: routePrefix("/a"),
				Action: routeweightedcluster(
					weightedcluster{"default/kuard/80/da39a3ee5e", 60},
					weightedcluster{"default/kuard/80/da39a3ee5e", 90}),
			},
		),
	), nil)
}

func TestHTTPProxyRouteWithTLS(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &projcontour.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []projcontour.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)

	// check that ingress_http has been updated.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/a"),
						Action: envoy.UpgradeHTTPS(),
					},
				),
			),
			envoy.RouteConfiguration("https/test2.test.com",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/a"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))
}

func TestHTTPProxyRouteWithTLS_InsecurePaths(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
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

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &projcontour.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []projcontour.Route{{
				Conditions:     conditions(prefixCondition("/insecure")),
				PermitInsecure: true,
				Services: []projcontour.Service{{Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: conditions(prefixCondition("/secure")),
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)

	// check that ingress_http has been updated.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: envoy.UpgradeHTTPS(),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
			envoy.RouteConfiguration("https/test2.test.com",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))
}

func TestHTTPProxyRouteWithTLS_InsecurePaths_DisablePermitInsecureTrue(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.EventHandler) {
		reh.DisablePermitInsecure = true
	})

	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
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

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
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

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	})

	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &projcontour.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []projcontour.Route{{
				Conditions:     conditions(prefixCondition("/insecure")),
				PermitInsecure: true,
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: conditions(prefixCondition("/secure")),
				Services: []projcontour.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)

	// check that ingress_http has been updated.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: envoy.UpgradeHTTPS(),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: envoy.UpgradeHTTPS(),
					},
				),
			),
			envoy.RouteConfiguration("https/test2.test.com",
				envoy.VirtualHost("test2.test.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	}, streamRDS(t, cc))
}

func TestRDSHTTPProxyRootCannotDelegateToAnotherRoot(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "green",
			Namespace: "marketing",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}
	rh.OnAdd(svc1)

	child := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: svc1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: svc1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(child)

	root := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "default",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Includes: []projcontour.Include{{
				Name:      child.Name,
				Namespace: child.Namespace,
			}},
		},
	}
	rh.OnAdd(root)

	// verify that child's route is present because while it is not possible to
	// delegate to it, it can host www.containersteve.com.
	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("www.containersteve.com",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: routecluster("marketing/green/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc))
}

func TestRDSHTTPProxyDuplicateIncludeConditions(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc1)

	svc2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "teama",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc2)

	svc3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "teamb",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(svc3)

	proxyRoot := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root",
			Namespace: svc1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
					Header: &projcontour.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteama",
				Namespace: "teamb",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
					Header: &projcontour.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: svc1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyChildA := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blogteama",
			Namespace: "teama",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: svc2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyChildB := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blogteamb",
			Namespace: "teamb",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: svc3.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(proxyRoot)
	rh.OnAdd(proxyChildA)
	rh.OnAdd(proxyChildB)

	assert.Equal(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc))
}

func virtualhosts(v ...*envoy_api_v2_route.VirtualHost) []*envoy_api_v2_route.VirtualHost { return v }

func conditions(c ...projcontour.MatchCondition) []projcontour.MatchCondition { return c }

func prefixCondition(prefix string) projcontour.MatchCondition {
	return projcontour.MatchCondition{
		Prefix: prefix,
	}
}
