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

// End to ends tests for translator to grpc operations.
package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/generated/clientset/versioned/fake"
	"github.com/heptio/contour/internal/k8s"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// heptio/contour#172. Updating an object from
//
// apiVersion: extensions/v1beta1
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
// apiVersion: extensions/v1beta1
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []route.Route{{
						Match:  prefixmatch("/"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					}},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []route.Route{{
						Match:  prefixmatch("/testing"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					}},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc))
}

// heptio/contour#101
// The path /hello should point to default/hello/80 on "*"
//
// apiVersion: extensions/v1beta1
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []route.Route{{
						Match:  prefixmatch("/hello"),
						Action: routecluster("default/hello/80/da39a3ee5e"),
					}},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
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

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: []string{"hello.example.com", "hello.example.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					}},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: []string{"hello.example.com", "hello.example.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/whoop"),
						Action: routecluster("default/kerpow/9000/da39a3ee5e"),
					}, {
						Match:  prefixmatch("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					}},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: []string{"hello.example.com", "hello.example.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/whoop"),
						Action: redirecthttps(),
					}, {
						Match:  prefixmatch("/"),
						Action: redirecthttps(),
					}},
				}}}),
			any(t, &v2.RouteConfiguration{Name: "ingress_https"}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello-kitty",
			Namespace: "default",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: []string{"hello.example.com", "hello.example.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/whoop"),
						Action: redirecthttps(),
					}, {
						Match:  prefixmatch("/"),
						Action: redirecthttps(),
					}},
				}}}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: []string{"hello.example.com", "hello.example.com:443"},
					Routes: []route.Route{{
						Match:  prefixmatch("/whoop"),
						Action: routecluster("default/kerpow/9000/da39a3ee5e"),
					}, {
						Match:  prefixmatch("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					}},
				}}}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc))
}

// contour#164: backend request timeout support
func TestRequestTimeout(t *testing.T) {
	const (
		durationInfinite  = time.Duration(0)
		duration10Minutes = 10 * time.Minute
	)

	rh, cc, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// i1 is a simple ingress bound to the default vhost.
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: routecluster("default/backend/80/da39a3ee5e"),
		}},
	}}, nil)

	// i2 adds an _invalid_ timeout, which we interpret as _infinite_.
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/request-timeout": "600", // not valid
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnUpdate(i1, i2)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: clustertimeout("default/backend/80/da39a3ee5e", durationInfinite),
		}},
	}}, nil)

	// i3 corrects i2 to use a proper duration
	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/request-timeout": "600s", // 10 * time.Minute
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnUpdate(i2, i3)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: clustertimeout("default/backend/80/da39a3ee5e", duration10Minutes),
		}},
	}}, nil)

	// i4 updates i3 to explicitly request infinite timeout
	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/request-timeout": "infinity",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnUpdate(i3, i4)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: clustertimeout("default/backend/80/da39a3ee5e", durationInfinite),
		}},
	}}, nil)
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	assertRDS(t, cc, []route.VirtualHost{{ // ingress_http
		Name:    "example.com",
		Domains: []string{"example.com", "example.com:80"},
		Routes: []route.Route{{
			Match:  prefixmatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
			Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
		}, {
			Match:  prefixmatch("/"), // match all
			Action: redirecthttps(),
		}},
	}}, []route.VirtualHost{{ // ingress_https
		Name:    "example.com",
		Domains: []string{"example.com", "example.com:443"},
		Routes: []route.Route{{
			Match:  prefixmatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
			Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
		}, {
			Match:  prefixmatch("/"), // match all
			Action: routecluster("default/app-service/8080/da39a3ee5e"),
		}},
	}})
}

// issue #257: editing default ingress did not remove original default route
func TestIssue257(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// apiVersion: extensions/v1beta1
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

	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: routecluster("default/kuard/80/da39a3ee5e"),
		}},
	}}, nil)

	// apiVersion: extensions/v1beta1
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

	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "kuard.db.gd-ms.com",
		Domains: []string{"kuard.db.gd-ms.com", "kuard.db.gd-ms.com:80"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: routecluster("default/kuard/80/da39a3ee5e"),
		}},
	}}, nil)
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
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

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{ // ingress_http
					Name:    "example.com",
					Domains: []string{"example.com", "example.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
					}, {
						Match:  prefixmatch("/"), // match all
						Action: redirecthttps(),
					}},
				}},
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc, "ingress_http"))

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []route.VirtualHost{{ // ingress_https
					Name:    "example.com",
					Domains: []string{"example.com", "example.com:443"},
					Routes: []route.Route{{
						Match:  prefixmatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
					}, {
						Match:  prefixmatch("/"), // match all
						Action: routecluster("default/app-service/8080/da39a3ee5e"),
					}},
				}},
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc, "ingress_https"))
}

func TestWebsocketIngress(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
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

	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/websocket-routes": "/",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "websocket.hello.world",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "ws",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "websocket.hello.world",
		Domains: []string{"websocket.hello.world", "websocket.hello.world:80"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: websocketroute("default/ws/80/da39a3ee5e"),
		}},
	}}, nil)
}

func TestWebsocketIngressRoute(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
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

	rh.OnAdd(&ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "websocket.hello.world"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}, {
				Match:            "/ws-1",
				EnableWebsockets: true,
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}, {
				Match:            "/ws-2",
				EnableWebsockets: true,
				Services: []ingressroutev1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "websocket.hello.world",
		Domains: []string{"websocket.hello.world", "websocket.hello.world:80"},
		Routes: []route.Route{{
			Match:  prefixmatch("/ws-2"),
			Action: websocketroute("default/ws/80/da39a3ee5e"),
		}, {
			Match:  prefixmatch("/ws-1"),
			Action: websocketroute("default/ws/80/da39a3ee5e"),
		}, {
			Match:  prefixmatch("/"), // match all
			Action: routecluster("default/ws/80/da39a3ee5e"),
		}},
	}}, nil)
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

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []route.Route{{
						Match:  prefixmatch("/kuard"),
						Action: routecluster("default/kuard/8080/da39a3ee5e"),
					}, {
						Match:  prefixmatch("/"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					}},
				}, {
					Name:    "test-gui",
					Domains: []string{"test-gui", "test-gui:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/"),
						Action: routecluster("default/test-gui/80/da39a3ee5e"),
					}},
				}},
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc, "ingress_http"))
}

func TestRDSIngressRouteInsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressRouteRootNamespaces = []string{"roots"}
		reh.Notifier.(*contour.CacheHandler).IngressRouteStatus = &k8s.IngressRouteStatus{
			Client: fake.NewSimpleClientset(),
		}
	})
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "roots",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	// ir1 is an ingressroute that is in the root namespaces
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// add ingressroute
	rh.OnAdd(ir1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "example.com",
					Domains: []string{"example.com", "example.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/"),
						Action: routecluster("roots/kuard/8080/da39a3ee5e"),
					}},
				}},
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc, "ingress_http"))
}

func TestRDSIngressRouteOutsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressRouteRootNamespaces = []string{"roots"}
		reh.Notifier.(*contour.CacheHandler).IngressRouteStatus = &k8s.IngressRouteStatus{
			Client: fake.NewSimpleClientset(),
		}
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

	// ir1 is an ingressroute that is not in the root namespaces
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "example.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// add ingressroute
	rh.OnAdd(ir1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc, "ingress_http"))
}

// Test DAGAdapter.IngressClass setting works, this could be done
// in LDS or RDS, or even CDS, but this test mirrors the place it's
// tested in internal/contour/route_test.go
func TestRDSIngressClass(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressClass = "linkerd"
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
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"),
			Action: routecluster("default/kuard/8080/da39a3ee5e"),
		}},
	}}, nil)

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
	assertRDS(t, cc, nil, nil)

	i3 := &v1beta1.Ingress{
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
	rh.OnUpdate(i2, i3)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"),
			Action: routecluster("default/kuard/8080/da39a3ee5e"),
		}},
	}}, nil)

	rh.OnUpdate(i3, i2)
	assertRDS(t, cc, nil, nil)
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
			rh.OnAdd(&ingressroutev1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("simple-%d", i),
					Namespace: "default",
				},
				Spec: ingressroutev1.IngressRouteSpec{
					VirtualHost: &ingressroutev1.VirtualHost{Fqdn: fmt.Sprintf("example-%d.com", i)},
					Routes: []ingressroutev1.Route{{
						Match: "/",
						Services: []ingressroutev1.Service{{
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
// apiVersion: extensions/v1beta1
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

	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: []string{"test2.test.com", "test2.test.com:80"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: routecluster("default/network-test/9001/da39a3ee5e"),
		}},
	}}, nil)
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

	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/a",
				Services: []ingressroutev1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90, // ignored
				}},
			}},
		},
	}

	rh.OnAdd(ir1)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: []string{"test2.test.com", "test2.test.com:80"},
		Routes: []route.Route{{
			Match:  prefixmatch("/a"), // match all
			Action: routecluster("default/kuard/80/da39a3ee5e"),
		}},
	}}, nil)

	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []ingressroutev1.Route{{
				Match: "/a",
				Services: []ingressroutev1.Service{{
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

	rh.OnUpdate(ir1, ir2)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: []string{"test2.test.com", "test2.test.com:80"},
		Routes: []route.Route{{
			Match: prefixmatch("/a"), // match all
			Action: routeweightedcluster(
				weightedcluster{"default/kuard/80/da39a3ee5e", 60},
				weightedcluster{"default/kuard/80/da39a3ee5e", 90},
			),
		}},
	}}, nil)
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/a",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(ir1)

	// check that ingress_http has been updated.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: []string{"test2.test.com", "test2.test.com:80"},
					Routes: []route.Route{{
						Match:  prefixmatch("/a"),
						Action: redirecthttps(),
					}},
				}}}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: []string{"test2.test.com", "test2.test.com:443"},
					Routes: []route.Route{{
						Match:  prefixmatch("/a"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					}},
				}}}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
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
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match:          "/insecure",
				PermitInsecure: true,
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}, {
				Match: "/secure",
				Services: []ingressroutev1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(ir1)

	// check that ingress_http has been updated.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: []string{"test2.test.com", "test2.test.com:80"},
					Routes: []route.Route{
						{
							Match:  prefixmatch("/secure"),
							Action: redirecthttps(),
						}, {
							Match:  prefixmatch("/insecure"),
							Action: routecluster("default/kuard/80/da39a3ee5e"),
						},
					},
				}}}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: []string{"test2.test.com", "test2.test.com:443"},
					Routes: []route.Route{
						{
							Match:  prefixmatch("/secure"),
							Action: routecluster("default/svc2/80/da39a3ee5e"),
						}, {
							Match:  prefixmatch("/insecure"),
							Action: routecluster("default/kuard/80/da39a3ee5e"),
						},
					},
				}}}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc))
}

// issue 665, support for retry-on, num-retries, and per-try-timeout annotations.
func TestRouteRetryAnnotations(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"contour.heptio.com/retry-on":        "50x,gateway-error",
				"contour.heptio.com/num-retries":     "7",
				"contour.heptio.com/per-try-timeout": "120ms",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, []route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []route.Route{{
			Match:  prefixmatch("/"), // match all
			Action: routeretry("default/backend/80/da39a3ee5e", "50x,gateway-error", 7, 120*time.Millisecond),
		}},
	}}, nil)
}

func assertRDS(t *testing.T, cc *grpc.ClientConn, ingress_http, ingress_https []route.VirtualHost) {
	t.Helper()
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.RouteConfiguration{
				Name:         "ingress_http",
				VirtualHosts: ingress_http,
			}),
			any(t, &v2.RouteConfiguration{
				Name:         "ingress_https",
				VirtualHosts: ingress_https,
			}),
		},
		TypeUrl: routeType,
		Nonce:   "0",
	}, streamRDS(t, cc))
}

func streamRDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewRouteDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	st, err := rds.StreamRoutes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       routeType,
		ResourceNames: rn,
	})
}

func prefixmatch(prefix string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

type weightedcluster struct {
	name   string
	weight uint32
}

func routecluster(cluster string) *route.Route_Route {
	return &route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func routeweightedcluster(first weightedcluster, rest ...weightedcluster) *route.Route_Route {
	return &route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: weightedclusters(append([]weightedcluster{first}, rest...)),
			},
		},
	}
}

func weightedclusters(clusters []weightedcluster) *route.WeightedCluster {
	var wc route.WeightedCluster
	total := uint32(0)
	for _, c := range clusters {
		total += c.weight
		wc.Clusters = append(wc.Clusters, &route.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: &types.UInt32Value{Value: c.weight},
		})
	}
	wc.TotalWeight = &types.UInt32Value{
		Value: total,
	}
	return &wc
}

func websocketroute(c string) *route.Route_Route {
	cl := routecluster(c)
	cl.Route.UseWebsocket = &types.BoolValue{Value: true}
	return cl
}

func clustertimeout(c string, timeout time.Duration) *route.Route_Route {
	cl := routecluster(c)
	cl.Route.Timeout = &timeout
	return cl
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

// redirecthttps returns a 301 redirect to the HTTPS scheme.
func redirecthttps() *route.Route_Redirect {
	return &route.Route_Redirect{
		Redirect: &route.RedirectAction{
			HttpsRedirect: true,
		},
	}
}

func routeretry(cluster string, retryOn string, numRetries uint32, perTryTimeout time.Duration) *route.Route_Route {
	r := routecluster(cluster)
	r.Route.RetryPolicy = &route.RouteAction_RetryPolicy{
		RetryOn: retryOn,
	}
	if numRetries > 0 {
		r.Route.RetryPolicy.NumRetries = &types.UInt32Value{Value: numRetries}
	}
	if perTryTimeout > 0 {
		r.Route.RetryPolicy.PerTryTimeout = &perTryTimeout
	}
	return r
}
