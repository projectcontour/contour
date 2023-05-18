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
package v3

import (
	"fmt"
	"path"
	"testing"

	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// projectcontour/contour#172. Updating an object from
//
//	apiVersion: networking/v1
//	kind: Ingress
//	metadata:
//	  name: kuard
//	spec:
//	  defaultBackend:
//	    service:
//	      name: kuard
//	      port:
//	        number: 80
//
// to
//
//	apiVersion: networking/v1
//	kind: Ingress
//	metadata:
//	  name: kuard
//	spec:
//	  rules:
//	  - http:
//	      paths:
//	      - path: /testing
//	        backend:
//	          service:
//	            name: kuard
//	            port:
//	              number: 80
//
// fails to update the virtualhost cache.
func TestEditIngress(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// add default/kuard to translator.
	old := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(old)

	// check that it's been translated correctly.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*", &envoy_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routecluster("default/kuard/80/da39a3ee5e"),
				}),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})

	// update old to new
	rh.OnUpdate(old, &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/testing",
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	})

	// check that ingress_http has been updated.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*", &envoy_route_v3.Route{
					Match:  routePrefix("/testing"),
					Action: routecluster("default/kuard/80/da39a3ee5e"),
				}),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	})
}

// projectcontour/contour#101
// The path /hello should point to default/hello/80 on "*"
//
//	apiVersion: networking/v1
//	kind: Ingress
//	metadata:
//	  name: hello
//	spec:
//	  rules:
//	  - http:
//		 paths:
//	      - path: /hello
//	        backend:
//	          service:
//	            name: hello
//	            port:
//	              number: 80
func TestIngressPathRouteWithoutHost(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("hello").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// add default/hello to translator.
	rh.OnAdd(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/hello",
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	})

	// check that it's been translated correctly.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/hello"),
						Action: routecluster("default/hello/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	})
}

func TestEditIngressInPlace(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path: "/",
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "wowie",
									Port: networking_v1.ServiceBackendPort{Name: "http"},
								},
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	s1 := fixture.NewService("wowie").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	s2 := fixture.NewService("kerpow").
		WithPorts(v1.ServicePort{Name: "http", Port: 9000, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s2)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	})

	// i2 is like i1 but adds a second route
	i2 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("default/hello"),
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *featuretests.IngressBackend(s1),
						}, {
							Path:    "/whoop",
							Backend: *featuretests.IngressBackend(s2),
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i1, i2)
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "3",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/whoop"),
						Action: routecluster("default/kerpow/9000/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "3",
	})

	// i3 is like i2, but adds the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	i3 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("default/hello", map[string]string{
			"ingress.kubernetes.io/force-ssl-redirect": "true"}),
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *featuretests.IngressBackend(s1),
						}, {
							Path:    "/whoop",
							Backend: *featuretests.IngressBackend(s2),
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i2, i3)
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "4",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/whoop"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "4",
	})

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello-kitty",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	// i4 is the same as i3, and includes a TLS spec object to enable ingress_https routes
	// i3 is like i2, but adds the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	i4 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true"},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"hello.example.com"},
				SecretName: "hello-kitty",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *featuretests.IngressBackend(s1),
						}, {
							Path:    "/whoop",
							Backend: *featuretests.IngressBackend(s2),
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i3, i4)
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "5",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("hello.example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/whoop"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/hello.example.com",
				envoy_v3.VirtualHost("hello.example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/whoop"),
						Action: routecluster("default/kerpow/9000/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/wowie/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "5",
	})
}

// contour#250 ingress.kubernetes.io/force-ssl-redirect: "true" should apply
// per route, not per vhost.
func TestSSLRedirectOverlay(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("app-service").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// i1 is a stock ingress with force-ssl-redirect on the / route
	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *featuretests.IngressBackend(s1),
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
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	s2 := fixture.NewService("nginx-ingress/challenge-service").
		WithPorts(v1.ServicePort{Name: "http", Port: 8009, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s2)

	// i2 is an overlay to add the let's encrypt handler.
	i2 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: *featuretests.IngressBackend(s2),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	assertRDS(t, c, "5", virtualhosts(
		envoy_v3.VirtualHost("example.com",
			&envoy_route_v3.Route{
				Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
				Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
			},
			&envoy_route_v3.Route{
				Match:  routePrefix("/"), // match all
				Action: envoy_v3.UpgradeHTTPS(),
			},
		),
	), virtualhosts(
		envoy_v3.VirtualHost("example.com",
			&envoy_route_v3.Route{
				Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
				Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
			},
			&envoy_route_v3.Route{
				Match:  routePrefix("/"), // match all
				Action: routecluster("default/app-service/8080/da39a3ee5e"),
			},
		),
	))
}

func TestInvalidCertInIngress(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// Create an invalid TLS secret
	secret := &v1.Secret{
		ObjectMeta: fixture.ObjectMeta("example-tls"),
		Type:       "kubernetes.io/tls",
		Data:       featuretests.Secretdata("wrong", featuretests.RSA_PRIVATE_KEY),
	}
	rh.OnAdd(secret)

	// Create a service
	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// Create an ingress that uses the invalid secret
	rh.OnAdd(&networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("kuard-ing"),
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.io"},
				SecretName: "example-tls",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.io",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	})

	assertRDS(t, c, "1", virtualhosts(
		envoy_v3.VirtualHost("kuard.io",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	// Correct the secret
	rh.OnUpdate(secret, &v1.Secret{
		ObjectMeta: fixture.ObjectMeta("example-tls"),
		Type:       "kubernetes.io/tls",
		Data:       featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	assertRDS(t, c, "2", virtualhosts(
		envoy_v3.VirtualHost("kuard.io",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), virtualhosts(
		envoy_v3.VirtualHost("kuard.io",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	))
}

// issue #257: editing default ingress did not remove original default route
func TestIssue257(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	//	apiVersion: networking/v1
	//	kind: Ingress
	//	metadata:
	//	  name: kuard-ing
	//	  labels:
	//	    app: kuard
	//	  annotations:
	//	    kubernetes.io/ingress.class: contour
	//	spec:
	//	  defaultBackend:
	//	    service:
	//	      name: kuard
	//	      port:
	//	        number: 80
	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"kubernetes.io/ingress.class": "contour",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)

	assertRDS(t, c, "2", virtualhosts(
		envoy_v3.VirtualHost("*",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)

	//	apiVersion: networking/v1
	//	kind: Ingress
	//	metadata:
	//	  name: kuard-ing
	//	  labhls:
	//	    app: kuard
	//	  annotations:
	//	    kubernetes.io/ingress.class: contour
	//	spec:
	//	 rules:
	//	 - host: kuard.db.gd-ms.com
	//	   http:
	//	     paths:
	//	     - backend:
	//	        service:
	//	          name: kuard
	//	          port:
	//	            number: 80
	//	       path: /
	i2 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"kubernetes.io/ingress.class": "contour",
		}),
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.db.gd-ms.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i1, i2)

	assertRDS(t, c, "3", virtualhosts(
		envoy_v3.VirtualHost("kuard.db.gd-ms.com",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/80/da39a3ee5e"),
			},
		),
	), nil)
}

func TestRDSFilter(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("app-service").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// i1 is a stock ingress with force-ssl-redirect on the / route
	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *featuretests.IngressBackend(s1),
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
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	s2 := fixture.NewService("nginx-ingress/challenge-service").
		WithPorts(v1.ServicePort{Name: "http", Port: 8009, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s2)

	// i2 is an overlay to add the let's encrypt handler.
	i2 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: *featuretests.IngressBackend(s2),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "5",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"), // match all
						Action: envoy_v3.UpgradeHTTPS(),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/example.com",
				envoy_v3.VirtualHost("example.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action: routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/app-service/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "5",
	})

}

// issue 404
func TestDefaultBackendDoesNotOverwriteNamedHost(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(
			v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			v1.ServicePort{Name: "alt", Port: 8080, TargetPort: intstr.FromInt(8080)},
		),
	)

	rh.OnAdd(fixture.NewService("test-gui").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: &networking_v1.IngressBackend{
				Service: &networking_v1.IngressServiceBackend{
					Name: "kuard",
					Port: networking_v1.ServiceBackendPort{Number: 80},
				},
			},
			Rules: []networking_v1.IngressRule{{
				Host: "test-gui",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path: "/",
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "test-gui",
									Port: networking_v1.ServiceBackendPort{Number: 80},
								},
							},
						}},
					},
				},
			}, {
				// Empty host.
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path: "/kuard",
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "kuard",
									Port: networking_v1.ServiceBackendPort{Number: 8080},
								},
							},
						}},
					},
				},
			}},
		},
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/kuard"),
						Action: routecluster("default/kuard/8080/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
				envoy_v3.VirtualHost("test-gui",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/test-gui/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

func TestDefaultBackendIsOverriddenByNoHostIngressRule(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(
			v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			v1.ServicePort{Name: "alt", Port: 8080, TargetPort: intstr.FromInt(8080)},
		),
	)

	rh.OnAdd(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			DefaultBackend: &networking_v1.IngressBackend{
				Service: &networking_v1.IngressServiceBackend{
					Name: "kuard",
					Port: networking_v1.ServiceBackendPort{Number: 80},
				},
			},
			Rules: []networking_v1.IngressRule{{
				// Empty host.
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{
							{
								// This conflicts with the default backend and
								// should override it.
								Path: "/",
								Backend: networking_v1.IngressBackend{
									Service: &networking_v1.IngressServiceBackend{
										Name: "kuard",
										Port: networking_v1.ServiceBackendPort{Number: 8080},
									},
								},
							},
						},
					},
				},
			}},
		},
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/kuard/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

// Test DAGAdapter.IngressClass setting works, this could be done
// in LDS or RDS, or even CDS, but this test mirrors the place it's
// tested in internal/contour/route_test.go
func TestRDSIngressClassAnnotation(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		b.Source.IngressClassNames = []string{"linkerd"}
	})
	defer done()

	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	i1 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"projectcontour.io/ingress.class": "linkerd",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, c, "1", virtualhosts(
		envoy_v3.VirtualHost("*",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/8080/da39a3ee5e"),
			},
		),
	), nil)

	i2 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"kubernetes.io/ingress.class": "contour",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i1, i2)
	assertRDS(t, c, "2", nil, nil)

	i3 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"projectcontour.io/ingress.class": "contour",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i2, i3)
	assertRDS(t, c, "2", nil, nil)

	i4 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"kubernetes.io/ingress.class": "linkerd",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i3, i4)
	assertRDS(t, c, "3", virtualhosts(
		envoy_v3.VirtualHost("*",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/8080/da39a3ee5e"),
			},
		),
	), nil)

	i5 := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMetaWithAnnotations("kuard-ing", map[string]string{
			"projectcontour.io/ingress.class": "linkerd",
		}),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
		},
	}
	rh.OnUpdate(i4, i5)
	assertRDS(t, c, "4", virtualhosts(
		envoy_v3.VirtualHost("*",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/kuard/8080/da39a3ee5e"),
			},
		),
	), nil)

	rh.OnUpdate(i5, i3)
	assertRDS(t, c, "5", nil, nil)
}

// issue 523, check for data races caused by accidentally
// sorting the contents of an RDS entry's virtualhost list.
func TestRDSAssertNoDataRaceDuringInsertAndStream(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	stop := make(chan struct{})

	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	go func() {
		for i := 0; i < 100; i++ {
			rh.OnAdd(&contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("simple-%d", i),
					Namespace: "default",
				},
				Spec: contour_api_v1.HTTPProxySpec{
					VirtualHost: &contour_api_v1.VirtualHost{Fqdn: fmt.Sprintf("example-%d.com", i)},
					Routes: []contour_api_v1.Route{{
						Conditions: []contour_api_v1.MatchCondition{{
							Prefix: "/",
						}},
						Services: []contour_api_v1.Service{{
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
			c.Request(routeType)
		}
	}
}

// issue 606: spec.rules.host without a http key causes panic.
//
//	apiVersion: networking/v1
//	kind: Ingress
//	metadata:
//	  name: test-ingress3
//	spec:
//	  rules:
//	  - host: test1.test.com
//	  - host: test2.test.com
//	    http:
//	      paths:
//	      - backend:
//	          service:
//	            name: network-test
//	            port:
//	              number: 9001
//	        path: /
//
// note: this test caused a panic in dag.Builder, but testing the
// context of RDS is a good place to start.
func TestRDSIngressSpecMissingHTTPKey(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("network-test").
		WithPorts(v1.ServicePort{Name: "http", Port: 9001, TargetPort: intstr.FromInt(8080)})

	i1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress3",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "test1.test.com",
			}, {
				Host: "test2.test.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(s1)

	assertRDS(t, c, "2", virtualhosts(
		envoy_v3.VirtualHost("test2.test.com",
			&envoy_route_v3.Route{
				Match:  routePrefix("/"),
				Action: routecluster("default/network-test/9001/da39a3ee5e"),
			},
		),
	), nil)
}

func TestRouteWithTLS(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	p1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(p1)

	// check that ingress_http has been updated.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Action: envoy_v3.UpgradeHTTPS(),
						Match:  routePrefix("/a"),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/test2.test.com",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/a"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

func TestRouteWithTLS_InsecurePaths(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	p1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/insecure",
				}},
				PermitInsecure: true,
				Services: []contour_api_v1.Service{{Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/secure",
				}},
				Services: []contour_api_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(p1)

	// check that ingress_http has been updated.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/test2.test.com",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

func TestRouteWithTLS_InsecurePaths_DisablePermitInsecureTrue(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		b.Processors = []dag.Processor{
			&dag.ListenerProcessor{},
			&dag.IngressProcessor{},
			&dag.HTTPProxyProcessor{
				DisablePermitInsecure: true,
			},
		}
	})

	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	p1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/insecure",
				}},
				PermitInsecure: true,
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/secure",
				}},
				Services: []contour_api_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(p1)

	// check that ingress_http has been updated.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/test2.test.com",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

// issue 1234, assert that RoutePrefix and RouteRegex work as expected
func TestRoutePrefixRouteRegex(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	meta := metav1.ObjectMeta{Name: "kuard", Namespace: "default"}

	s1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	// add default/kuard to translator.
	old := &networking_v1.Ingress{
		ObjectMeta: meta,
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/[^/]+/invoices(/.*|/?)", // issue 1243
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(old)

	// check that it's been translated correctly.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routeRegex("/[^/]+/invoices(/.*|/?)"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})

	invalid := &networking_v1.Ingress{
		ObjectMeta: meta,
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(s1),
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "^\\/(?!\\/)(.*?)",
							Backend: *featuretests.IngressBackend(s1),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(invalid)

	// check that it's been translated correctly.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("*",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

func assertRDS(t *testing.T, c *Contour, versioninfo string, ingressHTTP, ingressHTTPS []*envoy_route_v3.VirtualHost) {
	t.Helper()

	routes := []*envoy_route_v3.RouteConfiguration{
		envoy_v3.RouteConfiguration("ingress_http", ingressHTTP...),
	}

	for _, vh := range ingressHTTPS {
		routes = append(routes,
			envoy_v3.RouteConfiguration(path.Join("https", vh.Name), vh))
	}

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: versioninfo,
		Resources:   routeResources(t, routes...),
		TypeUrl:     routeType,
		Nonce:       versioninfo,
	})
}

func routeRegex(regex string, headers ...dag.HeaderMatchCondition) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.RegexMatchCondition{
			Regex: regex,
		},
		HeaderMatchConditions: headers,
	})
}

func routecluster(cluster string) *envoy_route_v3.Route_Route {
	return &envoy_route_v3.Route_Route{
		Route: &envoy_route_v3.RouteAction{
			ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func TestHTTPProxyRouteWithTLS(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	proxy1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: conditions(prefixCondition("/a")),
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)

	// check that ingress_http has been updated.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/a"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/test2.test.com",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/a"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

func TestHTTPProxyRouteWithTLS_InsecurePaths(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	proxy1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions:     conditions(prefixCondition("/insecure")),
				PermitInsecure: true,
				Services: []contour_api_v1.Service{{Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: conditions(prefixCondition("/secure")),
				Services: []contour_api_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)

	// check that ingress_http has been updated.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/test2.test.com",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

func TestHTTPProxyRouteWithTLS_InsecurePaths_DisablePermitInsecureTrue(t *testing.T) {
	rh, c, done := setup(t, func(b *dag.Builder) {
		b.Processors = []dag.Processor{
			&dag.ListenerProcessor{},
			&dag.IngressProcessor{},
			&dag.HTTPProxyProcessor{
				DisablePermitInsecure: true,
			},
		}
	})

	defer done()

	rh.OnAdd(fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	})

	proxy1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions:     conditions(prefixCondition("/insecure")),
				PermitInsecure: true,
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: conditions(prefixCondition("/secure")),
				Services: []contour_api_v1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(proxy1)

	// check that ingress_http has been updated.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: envoy_v3.UpgradeHTTPS(),
					},
				),
			),
			envoy_v3.RouteConfiguration("https/test2.test.com",
				envoy_v3.VirtualHost("test2.test.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/secure"),
						Action: routecluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/insecure"),
						Action: routecluster("default/kuard/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "1",
	})
}

func TestRDSHTTPProxyRootCannotDelegateToAnotherRoot(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc1 := fixture.NewService("marketing/green").
		WithPorts(v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(svc1)

	child := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: svc1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: svc1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(child)

	root := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      child.Name,
				Namespace: child.Namespace,
			}},
		},
	}
	rh.OnAdd(root)

	// verify that child's route is present because while it is not possible to
	// delegate to it, it can host www.containersteve.com.
	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("www.containersteve.com",
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("marketing/green/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	})
}

func TestRDSHTTPProxyDuplicateIncludeConditions(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc1 := fixture.NewService("kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc1)

	svc2 := fixture.NewService("teama/kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc2)

	svc3 := fixture.NewService("teamb/kuard").
		WithPorts(v1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(svc3)

	proxyRoot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root",
			Namespace: svc1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteama",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: svc1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyChildA := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blogteama",
			Namespace: "teama",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: svc2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyChildB := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blogteamb",
			Namespace: "teamb",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: svc3.Name,
					Port: 8080,
				}},
			}},
		},
	}
	rh.OnAdd(proxyRoot)
	rh.OnAdd(proxyChildA)
	rh.OnAdd(proxyChildB)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		VersionInfo: "2",
		Resources: routeResources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("example.com",
					&envoy_route_v3.Route{
						Match: routePrefixWithHeaderConditions("/blog", dag.HeaderMatchCondition{
							Name:      "x-header",
							Value:     "abc",
							MatchType: "contains",
							Invert:    false,
						}),
						Action: routecluster("teama/kuard/8080/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routecluster("default/kuard/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
		Nonce:   "2",
	})
}

func virtualhosts(v ...*envoy_route_v3.VirtualHost) []*envoy_route_v3.VirtualHost { return v }

func conditions(c ...contour_api_v1.MatchCondition) []contour_api_v1.MatchCondition { return c }

func prefixCondition(prefix string) contour_api_v1.MatchCondition {
	return contour_api_v1.MatchCondition{
		Prefix: prefix,
	}
}
