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

package dag

import (
	"fmt"
	"testing"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDAGStatus(t *testing.T) {
	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ssl-cert",
			Namespace: "roots",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	sec2 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-ssl-cert",
			Namespace: "heptio-contour",
		},
		Type: v1.SecretTypeTLS,
		Data: sec1.Data,
	}

	fallbackSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "roots",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: sec1.Namespace,
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

	s4 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "home",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s5 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s6 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s7 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     12345678,
			}},
		},
	}

	s8 := &v1.Service{
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

	s9 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: s1.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}

	s11 := &v1.Service{
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

	s12 := &v1.Service{
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

	// ir1 is a valid ingressroute
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      s4.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: s4.Name,
					Port: 8080,
				}},
			}, {
				Match: "/prefix",
				Delegate: &ingressroutev1.Delegate{
					Name: "delegated",
				}},
			},
		},
	}

	// ir2 is invalid because it contains a service with negative port
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: s4.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: s4.Name,
					Port: -80,
				}},
			}},
		},
	}

	// ir3 is invalid because it lives outside the roots namespace
	ir3 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foobar",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir4 is invalid because its match prefix does not match its parent's (ir1)
	ir4 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/doesnotmatch",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir6 is invalid because it delegates to itself, producing a cycle
	ir6 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "self",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "self",
				},
			}},
		},
	}

	// ir7 delegates to ir8, which is invalid because it delegates back to ir7
	ir7 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "child",
				},
			}},
		},
	}

	ir8 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "child",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "child",
				},
			}},
		},
	}

	// ir9 is invalid because it has a route that both delegates and has a list of services
	ir9 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "child",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// ir10 delegates to ir11 and ir 12.
	ir10 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "validChild",
				},
			}, {
				Match: "/bar",
				Delegate: &ingressroutev1.Delegate{
					Name: "invalidChild",
				},
			}},
		},
	}

	ir11 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "validChild",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	// ir12 is invalid because it contains an invalid port
	ir12 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/bar",
				Services: []ingressroutev1.Service{{
					Name: "foo3",
					Port: 12345678,
				}},
			}},
		},
	}

	// ir13 is invalid because it does not specify and FQDN
	ir13 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// ir14 delegates tp ir15 but it is invalid because it is missing fqdn
	ir14 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "validChild",
				},
			}},
		},
	}

	// ir15 is invalid because it contains a wildcarded fqdn
	ir15 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.*.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir16 is invalid because it references an invalid service
	ir16 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "invalid",
					Port: 8080,
				}},
			}},
		},
	}

	ir17 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// ir18 reuses the fqdn used in ir17
	ir18 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-example",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}
	ir20 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Delegate: &ingressroutev1.Delegate{
					Name:      "blog",
					Namespace: "marketing",
				},
			}},
		},
	}

	ir21 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &ingressroutev1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "green",
					Port: 80,
				}},
			}},
		},
	}

	ir22 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Delegate: &ingressroutev1.Delegate{
					Name:      "blog",
					Namespace: "marketing",
				},
			}},
		},
	}

	ir23 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: "green",
					Port: 80,
				}},
			}},
		},
	}

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: s9.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "example.com",
				IngressRuleValue: ingressrulevalue(backend(s9.Name, intstr.FromInt(80))),
			}},
		},
	}

	ir24 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: s9.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "example.com",
				TLS: &ingressroutev1.TLS{
					SecretName: sec1.Name,
				},
			},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: s9.Name,
					Port: 80,
				}},
			},
		},
	}

	ir25 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample-app",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "127.0.0.1.nip.io",
				TLS: &ingressroutev1.TLS{
					SecretName: sec2.Namespace + "/" + sec2.Name,
				},
			},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			},
		},
	}

	ir26 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &ingressroutev1.TLS{
					SecretName: sec2.Namespace + "/" + sec2.Name,
				},
			},
			Routes: []ingressroutev1.Route{{
				Services: []ingressroutev1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			}},
		},
	}

	s10 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-passthrough",
			Namespace: "roots",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "https",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(443),
			}, {
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(80),
			}},
		},
	}

	ir27 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: s10.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &ingressroutev1.TLS{
					Passthrough: true,
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: s10.Name,
					Port: 80, // proxy non secure traffic to port 80
				}},
			}},
			TCPProxy: &ingressroutev1.TCPProxy{
				Services: []ingressroutev1.Service{{
					Name: s10.Name,
					Port: 443, // ssl passthrough to secure port
				}},
			},
		},
	}

	ir28 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: s10.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &ingressroutev1.TLS{
					SecretName: "heptio-contour/ssl-cert", // not delegated
				},
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Services: []ingressroutev1.Service{{
					Name: s10.Name,
					Port: 80,
				}},
			}},
		},
	}

	ir29 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site1",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "site1.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/",
				Delegate: &ingressroutev1.Delegate{
					Name:      "www",
					Namespace: s1.Namespace,
				},
			}},
		},
	}

	ir30 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site2",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &ingressroutev1.VirtualHost{
				Fqdn: "site2.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name:      "www",
					Namespace: s1.Namespace,
				},
			}},
		},
	}

	ir31 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: s1.Namespace,
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy1 is a valid proxy
	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy2 is invalid because it contains a service with negative port
	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	// proxy3 is invalid because it lives outside the roots namespace
	proxy3 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foobar",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy6 is invalid because it delegates to itself, producing a cycle
	proxy6 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "self",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "self",
				Namespace: "roots",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "green",
					Port: 80,
				}},
			}},
		},
	}

	// proxy7 delegates to proxy8, which is invalid because proxy8 delegates back to proxy8
	proxy7 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxy8 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			Includes: []projcontour.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxy11 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validChild",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy13 is invalid because it does not specify and FQDN
	proxy13 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy14 delegates tp ir15 but it is invalid because it is missing fqdn
	proxy14 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{},
			Includes: []projcontour.Include{{
				Name:      "validChild",
				Namespace: "roots",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	// proxy15 is invalid because it contains a wildcarded fqdn
	proxy15 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.*.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy16 is invalid because it references an invalid service
	proxy16 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "invalid",
					Port: 8080,
				}},
			}},
		},
	}

	proxy17 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy18 reuses the fqdn used in proxy17
	proxy18 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-example",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy19 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &projcontour.TLS{
					SecretName: sec2.Namespace + "/" + sec2.Name,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			}},
		},
	}

	proxy20 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &projcontour.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Includes: []projcontour.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxy21 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: s8.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &projcontour.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s8.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxy22 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Includes: []projcontour.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxy23 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: s8.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s8.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxy24 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: s8.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s8.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxy25 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      proxy24.Name,
				Namespace: proxy24.Namespace,
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
				}},
			}},
		},
	}

	proxy26 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s1.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}
	proxy27 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s1.Name,
					Port:   8080,
					Mirror: true,
				}, {
					Name:   s1.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}
	// proxy28 is a proxy with duplicated route condition headers
	proxy28 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}, {
					Header: &projcontour.HeaderCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy29 is a proxy with duplicated invalid include condition headers
	proxy29 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "delegated",
				Namespace: "roots",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}, {
					Header: &projcontour.HeaderCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
			}},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}
	proxy30 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}
	// proxy31 is a proxy with duplicated valid route condition headers
	proxy31 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}, {
					Header: &projcontour.HeaderCondition{
						Name:     "x-header",
						NotExact: "abc",
					},
				}, {
					Header: &projcontour.HeaderCondition{
						Name:     "x-header",
						NotExact: "1234",
					},
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}
	proxy32 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy33 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []projcontour.Condition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
			}},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}
	proxy34 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "teama",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy35 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{
					{
						Prefix: "api",
					},
				},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy36 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []projcontour.Condition{
					{
						Prefix: "api",
					},
				},
			}},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// invalid because tcpproxy both includes another httpproxy
	// and has a list of services.
	proxy37 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Include: &projcontour.TCPProxyInclude{
					Name:      "foo",
					Namespace: "roots",
				},
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// Invalid because tcpproxy neither includes another httpproxy
	// nor has a list of services.
	proxy37a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &projcontour.TCPProxy{},
		},
	}

	// proxy38 is invalid when combined with proxy39 as the latter
	// is a root httpproxy.
	proxy38 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Include: &projcontour.TCPProxyInclude{
					Name:      "foo",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy39 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy40 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy41 is a proxy with conflicting include conditions
	proxy41 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy42 is a proxy with conflicting include header conditions
	proxy42 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []projcontour.Condition{{
					Header: &projcontour.HeaderCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []projcontour.Condition{{
					Header: &projcontour.HeaderCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy43 is a proxy with conflicting include header conditions
	proxy43 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
					Header: &projcontour.HeaderCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
					Header: &projcontour.HeaderCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy41a is a child of proxy41
	proxy41a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "blogteama",
			Name:      "teama",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
				}},
				Services: []projcontour.Service{{
					Name: s11.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy41a is a child of proxy41
	proxy41b := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "blogteamb",
			Name:      "teamb",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/blog",
				}},
				Services: []projcontour.Service{{
					Name: s12.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy44's include is missing
	proxy44 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "child",
			}},
		},
	}

	proxy45 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tcp-proxy-service",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: "not-found",
					Port: 8080,
				}},
			},
		},
	}

	proxy46 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tls",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy47 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Name,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{Name: "missing", Port: 9000},
				},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy48root := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Include: &projcontour.TCPProxyInclude{
					Name:      "child",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy48rootplural := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: sec1.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				IncludesDeprecated: &projcontour.TCPProxyInclude{
					Name:      "child",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy48child := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// issue 2309, each route must have at least one service
	proxy49 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-service",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "missing-service.example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/",
				}},
				Services: nil, // missing
			}},
		},
	}

	fallbackCertificate := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	fallbackCertificateWithClientValidation := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
					ClientValidation: &projcontour.DownstreamValidation{
						CACertificate: "something",
					},
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	tests := map[string]struct {
		objs                []interface{}
		fallbackCertificate *k8s.FullName
		want                map[k8s.FullName]Status
	}{
		"valid ingressroute": {
			objs: []interface{}{ir1, s4},
			want: map[k8s.FullName]Status{
				{Name: ir1.Name, Namespace: ir1.Namespace}: {Object: ir1, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
			},
		},
		"invalid port in service": {
			objs: []interface{}{ir2},
			want: map[k8s.FullName]Status{
				{Name: ir2.Name, Namespace: ir2.Namespace}: {Object: ir2, Status: "invalid", Description: `route "/foo": service "home": port must be in the range 1-65535`, Vhost: "example.com"},
			},
		},
		"root ingressroute outside of roots namespace": {
			objs: []interface{}{ir3},
			want: map[k8s.FullName]Status{
				{Name: ir3.Name, Namespace: ir3.Namespace}: {Object: ir3, Status: "invalid", Description: "root IngressRoute cannot be defined in this namespace"},
			},
		},
		"delegated route's match prefix does not match parent's prefix": {
			objs: []interface{}{ir1, ir4, s4},
			want: map[k8s.FullName]Status{
				{Name: ir1.Name, Namespace: ir1.Namespace}: {Object: ir1, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Name: ir4.Name, Namespace: ir4.Namespace}: {Object: ir4, Status: "invalid", Description: `the path prefix "/doesnotmatch" does not match the parent's path prefix "/prefix"`},
			},
		},
		"root ingressroute does not specify FQDN": {
			objs: []interface{}{ir13},
			want: map[k8s.FullName]Status{
				{Name: ir13.Name, Namespace: ir13.Namespace}: {Object: ir13, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
			},
		},
		"self-edge produces a cycle": {
			objs: []interface{}{ir6},
			want: map[k8s.FullName]Status{
				{Name: ir6.Name, Namespace: ir6.Namespace}: {
					Object:      ir6,
					Status:      "invalid",
					Description: "root ingressroute cannot delegate to another root ingressroute",
					Vhost:       "example.com",
				},
			},
		},
		"child delegates to parent, producing a cycle": {
			objs: []interface{}{ir7, ir8},
			want: map[k8s.FullName]Status{
				{Name: ir7.Name, Namespace: ir7.Namespace}: {
					Object:      ir7,
					Status:      "valid",
					Description: "valid IngressRoute",
					Vhost:       "example.com",
				},
				{Name: ir8.Name, Namespace: ir8.Namespace}: {
					Object:      ir8,
					Status:      "invalid",
					Description: "route creates a delegation cycle: roots/parent -> roots/child -> roots/child",
				},
			},
		},
		"route has a list of services and also delegates": {
			objs: []interface{}{ir9},
			want: map[k8s.FullName]Status{
				{Name: ir9.Name, Namespace: ir9.Namespace}: {Object: ir9, Status: "invalid", Description: `route "/foo": cannot specify services and delegate in the same route`, Vhost: "example.com"},
			},
		},
		"ingressroute is an orphaned route": {
			objs: []interface{}{ir8},
			want: map[k8s.FullName]Status{
				{Name: ir8.Name, Namespace: ir8.Namespace}: {Object: ir8, Status: "orphaned", Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"},
			},
		},
		"ingressroute delegates to multiple ingressroutes, one is invalid": {
			objs: []interface{}{ir10, ir11, ir12, s6, s7},
			want: map[k8s.FullName]Status{
				{Name: ir11.Name, Namespace: ir11.Namespace}: {Object: ir11, Status: "valid", Description: "valid IngressRoute"},
				{Name: ir12.Name, Namespace: ir12.Namespace}: {Object: ir12, Status: "invalid", Description: `route "/bar": service "foo3": port must be in the range 1-65535`},
				{Name: ir10.Name, Namespace: ir10.Namespace}: {Object: ir10, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
			},
		},
		"invalid parent orphans children": {
			objs: []interface{}{ir14, ir11},
			want: map[k8s.FullName]Status{
				{Name: ir14.Name, Namespace: ir14.Namespace}: {Object: ir14, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Name: ir11.Name, Namespace: ir11.Namespace}: {Object: ir11, Status: "orphaned", Description: "this IngressRoute is not part of a delegation chain from a root IngressRoute"},
			},
		},
		"multi-parent children is not orphaned when one of the parents is invalid": {
			objs: []interface{}{ir14, ir11, ir10, s5, s6},
			want: map[k8s.FullName]Status{
				{Name: ir14.Name, Namespace: ir14.Namespace}: {Object: ir14, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Name: ir11.Name, Namespace: ir11.Namespace}: {Object: ir11, Status: "valid", Description: "valid IngressRoute"},
				{Name: ir10.Name, Namespace: ir10.Namespace}: {Object: ir10, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
			},
		},
		"invalid FQDN contains wildcard": {
			objs: []interface{}{ir15},
			want: map[k8s.FullName]Status{
				{Name: ir15.Name, Namespace: ir15.Namespace}: {Object: ir15, Status: "invalid", Description: `Spec.VirtualHost.Fqdn "example.*.com" cannot use wildcards`, Vhost: "example.*.com"},
			},
		},
		"missing service shows invalid status": {
			objs: []interface{}{ir16},
			want: map[k8s.FullName]Status{
				{Name: ir16.Name, Namespace: ir16.Namespace}: {
					Object:      ir16,
					Status:      "invalid",
					Description: `Service [invalid:8080] is invalid or missing`,
					Vhost:       ir16.Spec.VirtualHost.Fqdn,
				},
			},
		},
		"insert ingressroute": {
			objs: []interface{}{s1, ir17},
			want: map[k8s.FullName]Status{
				{Name: ir17.Name, Namespace: ir17.Namespace}: {
					Object:      ir17,
					Status:      k8s.StatusValid,
					Description: "valid IngressRoute",
					Vhost:       "example.com",
				},
			},
		},
		"insert conflicting ingressroutes due to fqdn reuse": {
			objs: []interface{}{ir17, ir18},
			want: map[k8s.FullName]Status{
				{Name: ir17.Name, Namespace: ir17.Namespace}: {
					Object:      ir17,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "example.com" is used in multiple IngressRoutes: roots/example-com, roots/other-example`,
					Vhost:       "example.com",
				},
				{Name: ir18.Name, Namespace: ir18.Namespace}: {
					Object:      ir18,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "example.com" is used in multiple IngressRoutes: roots/example-com, roots/other-example`,
					Vhost:       "example.com",
				},
			},
		},
		"root ingress delegating to another root": {
			objs: []interface{}{ir20, ir21},
			want: map[k8s.FullName]Status{
				{Name: ir20.Name, Namespace: ir20.Namespace}: {
					Object:      ir20,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "blog.containersteve.com" is used in multiple IngressRoutes: marketing/blog, roots/root-blog`,
					Vhost:       "blog.containersteve.com",
				},
				{Name: ir21.Name, Namespace: ir21.Namespace}: {
					Object:      ir21,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "blog.containersteve.com" is used in multiple IngressRoutes: marketing/blog, roots/root-blog`,
					Vhost:       "blog.containersteve.com",
				},
			},
		},
		"root ingress delegating to another root w/ different hostname": {
			objs: []interface{}{ir22, ir23, s8},
			want: map[k8s.FullName]Status{
				{Name: ir22.Name, Namespace: ir22.Namespace}: {
					Object:      ir22,
					Status:      k8s.StatusInvalid,
					Description: "root ingressroute cannot delegate to another root ingressroute",
					Vhost:       "blog.containersteve.com",
				},
				{Name: ir23.Name, Namespace: ir23.Namespace}: {
					Object:      ir23,
					Status:      k8s.StatusValid,
					Description: `valid IngressRoute`,
					Vhost:       "www.containersteve.com",
				},
			},
		},
		// issue 1399
		"service shared across ingress and ingressroute tcpproxy": {
			objs: []interface{}{
				sec1, s9, i1, ir24,
			},
			want: map[k8s.FullName]Status{
				{Name: ir24.Name, Namespace: ir24.Namespace}: {
					Object:      ir24,
					Status:      k8s.StatusValid,
					Description: `valid IngressRoute`,
					Vhost:       "example.com",
				},
			},
		},
		// issue 1347
		"check status set when tcpproxy combined with tls delegation failure": {
			objs: []interface{}{
				sec2,
				ir25,
			},
			want: map[k8s.FullName]Status{
				{Name: ir25.Name, Namespace: ir25.Namespace}: {
					Object:      ir25,
					Status:      k8s.StatusInvalid,
					Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.ToFullName(sec2)),
					Vhost:       ir25.Spec.VirtualHost.Fqdn,
				},
			},
		},
		// issue 1348
		"check status set when routes combined with tls delegation failure": {
			objs: []interface{}{
				sec2,
				ir26,
			},
			want: map[k8s.FullName]Status{
				{Name: ir26.Name, Namespace: ir26.Namespace}: {
					Object:      ir26,
					Status:      k8s.StatusInvalid,
					Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.ToFullName(sec2)),
					Vhost:       ir26.Spec.VirtualHost.Fqdn,
				},
			},
		},
		// issue 1348
		"check status set when httpproxy routes combined with tls delegation failure": {
			objs: []interface{}{
				sec2,
				proxy19,
			},
			want: map[k8s.FullName]Status{
				{Name: proxy19.Name, Namespace: proxy19.Namespace}: {
					Object:      proxy19,
					Status:      k8s.StatusInvalid,
					Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.ToFullName(sec2)),
					Vhost:       proxy19.Spec.VirtualHost.Fqdn,
				},
			},
		},
		// issue 910
		"non tls routes can be combined with tcp proxy": {
			objs: []interface{}{
				s10,
				ir27,
			},
			want: map[k8s.FullName]Status{
				{Name: ir27.Name, Namespace: ir27.Namespace}: {
					Object:      ir27,
					Status:      k8s.StatusValid,
					Description: `valid IngressRoute`,
					Vhost:       ir27.Spec.VirtualHost.Fqdn,
				},
			},
		},
		// issue 1452
		"ingressroute with missing secret delegation should be invalid": {
			objs: []interface{}{
				s10,
				ir28,
			},
			want: map[k8s.FullName]Status{
				{Name: ir28.Name, Namespace: ir28.Namespace}: {
					Object:      ir28,
					Status:      k8s.StatusInvalid,
					Description: "Spec.VirtualHost.TLS Secret \"heptio-contour/ssl-cert\" is invalid: Secret not found",
					Vhost:       ir28.Spec.VirtualHost.Fqdn,
				},
			},
		},
		"two root ingressroutes delegated to the same object should not conflict on hostname": {
			objs: []interface{}{
				s1, ir29, ir30, ir31,
			},
			want: map[k8s.FullName]Status{
				{Name: ir29.Name, Namespace: ir29.Namespace}: {
					Object:      ir29,
					Status:      "valid",
					Description: "valid IngressRoute",
					Vhost:       "site1.com",
				},
				{Name: ir30.Name, Namespace: ir30.Namespace}: {
					Object:      ir30,
					Status:      "valid",
					Description: "valid IngressRoute",
					Vhost:       "site2.com",
				},
				{Name: ir31.Name, Namespace: ir31.Namespace}: {
					Object:      ir31,
					Status:      "valid",
					Description: "valid IngressRoute",
				},
			},
		},
		"valid proxy": {
			objs: []interface{}{proxy1, s4},
			want: map[k8s.FullName]Status{
				{Name: proxy1.Name, Namespace: proxy1.Namespace}: {Object: proxy1, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
			},
		},
		"proxy invalid port in service": {
			objs: []interface{}{proxy2},
			want: map[k8s.FullName]Status{
				{Name: proxy2.Name, Namespace: proxy2.Namespace}: {Object: proxy2, Status: "invalid", Description: `service "home": port must be in the range 1-65535`, Vhost: "example.com"},
			},
		},
		"root proxy outside of roots namespace": {
			objs: []interface{}{proxy3},
			want: map[k8s.FullName]Status{
				{Name: proxy3.Name, Namespace: proxy3.Namespace}: {Object: proxy3, Status: "invalid", Description: "root HTTPProxy cannot be defined in this namespace"},
			},
		},
		"root proxy does not specify FQDN": {
			objs: []interface{}{proxy13},
			want: map[k8s.FullName]Status{
				{Name: proxy13.Name, Namespace: proxy13.Namespace}: {Object: proxy13, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
			},
		},
		"proxy self-edge produces a cycle": {
			objs: []interface{}{proxy6, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy6.Name, Namespace: proxy6.Namespace}: {
					Object:      proxy6,
					Status:      "invalid",
					Description: "root httpproxy cannot delegate to another root httpproxy",
					Vhost:       "example.com",
				},
			},
		},
		"proxy child delegates to parent, producing a cycle": {
			objs: []interface{}{proxy7, proxy8},
			want: map[k8s.FullName]Status{
				{Name: proxy7.Name, Namespace: proxy7.Namespace}: {
					Object:      proxy7,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "example.com",
				},
				{Name: proxy8.Name, Namespace: proxy8.Namespace}: {
					Object:      proxy8,
					Status:      "invalid",
					Description: "include creates a delegation cycle: roots/parent -> roots/child -> roots/child",
				},
			},
		},
		"proxy orphaned route": {
			objs: []interface{}{proxy8},
			want: map[k8s.FullName]Status{
				{Name: proxy8.Name, Namespace: proxy8.Namespace}: {Object: proxy8, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy"},
			},
		},
		"proxy invalid parent orphans children": {
			objs: []interface{}{proxy14, proxy11},
			want: map[k8s.FullName]Status{
				{Name: proxy14.Name, Namespace: proxy14.Namespace}: {Object: proxy14, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Name: proxy11.Name, Namespace: proxy11.Namespace}: {Object: proxy11, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy"},
			},
		},
		"proxy invalid FQDN contains wildcard": {
			objs: []interface{}{proxy15},
			want: map[k8s.FullName]Status{
				{Name: proxy15.Name, Namespace: proxy15.Namespace}: {Object: proxy15, Status: "invalid", Description: `Spec.VirtualHost.Fqdn "example.*.com" cannot use wildcards`, Vhost: "example.*.com"},
			},
		},
		"proxy missing service shows invalid status": {
			objs: []interface{}{proxy16},
			want: map[k8s.FullName]Status{
				{Name: proxy16.Name, Namespace: proxy16.Namespace}: {
					Object:      proxy16,
					Status:      "invalid",
					Description: `Service [invalid:8080] is invalid or missing`,
					Vhost:       proxy16.Spec.VirtualHost.Fqdn,
				},
			},
		},
		"insert conflicting proxies due to fqdn reuse": {
			objs: []interface{}{proxy17, proxy18},
			want: map[k8s.FullName]Status{
				{Name: proxy17.Name, Namespace: proxy17.Namespace}: {
					Object:      proxy17,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`,
					Vhost:       "example.com",
				},
				{Name: proxy18.Name, Namespace: proxy18.Namespace}: {
					Object:      proxy18,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`,
					Vhost:       "example.com",
				},
			},
		},
		"root proxy delegating to another root": {
			objs: []interface{}{proxy20, proxy21},
			want: map[k8s.FullName]Status{
				{Name: proxy20.Name, Namespace: proxy20.Namespace}: {
					Object:      proxy20,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`,
					Vhost:       "blog.containersteve.com",
				},
				{Name: proxy21.Name, Namespace: proxy21.Namespace}: {
					Object:      proxy21,
					Status:      k8s.StatusInvalid,
					Description: `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`,
					Vhost:       "blog.containersteve.com",
				},
			},
		},
		"root proxy delegating to another root w/ different hostname": {
			objs: []interface{}{proxy22, proxy23, s8},
			want: map[k8s.FullName]Status{
				{Name: proxy22.Name, Namespace: proxy22.Namespace}: {
					Object:      proxy22,
					Status:      k8s.StatusInvalid,
					Description: "root httpproxy cannot delegate to another root httpproxy",
					Vhost:       "blog.containersteve.com",
				},
				{Name: proxy23.Name, Namespace: proxy23.Namespace}: {
					Object:      proxy23,
					Status:      k8s.StatusValid,
					Description: `valid HTTPProxy`,
					Vhost:       "www.containersteve.com",
				},
			},
		},
		"proxy delegate to another": {
			objs: []interface{}{proxy24, proxy25, s1, s8},
			want: map[k8s.FullName]Status{
				{Name: proxy24.Name, Namespace: proxy24.Namespace}: {
					Object:      proxy24,
					Status:      "valid",
					Description: "valid HTTPProxy",
				},
				{Name: proxy25.Name, Namespace: proxy25.Namespace}: {
					Object:      proxy25,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "example.com",
				},
			},
		},
		"proxy with mirror": {
			objs: []interface{}{proxy26, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy26.Name, Namespace: proxy26.Namespace}: {
					Object:      proxy26,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "example.com",
				},
			},
		},
		"proxy with two mirrors": {
			objs: []interface{}{proxy27, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy27.Name, Namespace: proxy27.Namespace}: {
					Object:      proxy27,
					Status:      "invalid",
					Description: "only one service per route may be nominated as mirror",
					Vhost:       "example.com",
				},
			},
		},
		"proxy with two prefix conditions on route": {
			objs: []interface{}{proxy32, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy32.Name, Namespace: proxy32.Namespace}: {
					Object:      proxy32,
					Status:      "invalid",
					Description: "route: more than one prefix is not allowed in a condition block",
					Vhost:       "example.com",
				},
			},
		},
		"proxy with two prefix conditions as an include": {
			objs: []interface{}{proxy33, proxy34, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy33.Name, Namespace: proxy33.Namespace}: {
					Object:      proxy33,
					Status:      "invalid",
					Description: "include: more than one prefix is not allowed in a condition block",
					Vhost:       "example.com",
				}, {Name: proxy34.Name, Namespace: proxy34.Namespace}: {
					Object:      proxy34,
					Status:      "orphaned",
					Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				},
			},
		},
		"proxy with prefix conditions on route that does not start with slash": {
			objs: []interface{}{proxy35, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy35.Name, Namespace: proxy35.Namespace}: {
					Object:      proxy35,
					Status:      "invalid",
					Description: "route: prefix conditions must start with /, api was supplied",
					Vhost:       "example.com",
				},
			},
		},
		"proxy with include prefix that does not start with slash": {
			objs: []interface{}{proxy36, proxy34, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy36.Name, Namespace: proxy36.Namespace}: {
					Object:      proxy36,
					Status:      "invalid",
					Description: "include: prefix conditions must start with /, api was supplied",
					Vhost:       "example.com",
				}, {Name: proxy34.Name, Namespace: proxy34.Namespace}: {
					Object:      proxy34,
					Status:      "orphaned",
					Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				},
			},
		},
		"duplicate route condition headers": {
			objs: []interface{}{proxy28, s4},
			want: map[k8s.FullName]Status{
				{Name: proxy28.Name, Namespace: proxy28.Namespace}: {Object: proxy28, Status: "invalid", Description: "cannot specify duplicate header 'exact match' conditions in the same route", Vhost: "example.com"},
			},
		},
		"duplicate valid route condition headers": {
			objs: []interface{}{proxy31, s4},
			want: map[k8s.FullName]Status{
				{Name: proxy31.Name, Namespace: proxy31.Namespace}: {Object: proxy31, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
			},
		},
		"duplicate include condition headers": {
			objs: []interface{}{proxy29, proxy30, s4},
			want: map[k8s.FullName]Status{
				{Name: proxy29.Name, Namespace: proxy29.Namespace}: {Object: proxy29, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
				{Name: proxy30.Name, Namespace: proxy30.Namespace}: {Object: proxy30, Status: "invalid", Description: "cannot specify duplicate header 'exact match' conditions in the same route", Vhost: ""},
			},
		},
		"duplicate path conditions on an include": {
			objs: []interface{}{proxy41, proxy41a, proxy41b, s4, s11, s12},
			want: map[k8s.FullName]Status{
				{Name: proxy41.Name, Namespace: proxy41.Namespace}:   {Object: proxy41, Status: "invalid", Description: "duplicate conditions defined on an include", Vhost: "example.com"},
				{Name: proxy41a.Name, Namespace: proxy41a.Namespace}: {Object: proxy41a, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
				{Name: proxy41b.Name, Namespace: proxy41b.Namespace}: {Object: proxy41b, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
			},
		},
		"duplicate header conditions on an include": {
			objs: []interface{}{proxy42, proxy41a, proxy41b, s4, s11, s12},
			want: map[k8s.FullName]Status{
				{Name: proxy42.Name, Namespace: proxy42.Namespace}:   {Object: proxy42, Status: "invalid", Description: "duplicate conditions defined on an include", Vhost: "example.com"},
				{Name: proxy41a.Name, Namespace: proxy41a.Namespace}: {Object: proxy41a, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
				{Name: proxy41b.Name, Namespace: proxy41b.Namespace}: {Object: proxy41b, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
			},
		},
		"duplicate header+path conditions on an include": {
			objs: []interface{}{proxy43, proxy41a, proxy41b, s4, s11, s12},
			want: map[k8s.FullName]Status{
				{Name: proxy43.Name, Namespace: proxy43.Namespace}:   {Object: proxy43, Status: "invalid", Description: "duplicate conditions defined on an include", Vhost: "example.com"},
				{Name: proxy41a.Name, Namespace: proxy41a.Namespace}: {Object: proxy41a, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
				{Name: proxy41b.Name, Namespace: proxy41b.Namespace}: {Object: proxy41b, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
			},
		},
		"httpproxy with invalid tcpproxy": {
			objs: []interface{}{proxy37, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy37.Name, Namespace: proxy37.Namespace}: {
					Object:      proxy37,
					Status:      "invalid",
					Description: "tcpproxy: cannot specify services and include in the same httpproxy",
					Vhost:       "passthrough.example.com",
				},
			},
		},
		"httpproxy with empty tcpproxy": {
			objs: []interface{}{proxy37a, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy37a.Name, Namespace: proxy37a.Namespace}: {
					Object:      proxy37a,
					Status:      "invalid",
					Description: "tcpproxy: either services or inclusion must be specified",
					Vhost:       "passthrough.example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy w/ missing include": {
			objs: []interface{}{proxy38, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy38.Name, Namespace: proxy38.Namespace}: {
					Object:      proxy38,
					Status:      "invalid",
					Description: "tcpproxy: include roots/foo not found",
					Vhost:       "passthrough.example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy w/ includes another root": {
			objs: []interface{}{proxy38, proxy39, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy38.Name, Namespace: proxy38.Namespace}: {
					Object:      proxy38,
					Status:      "invalid",
					Description: "root httpproxy cannot delegate to another root httpproxy",
					Vhost:       "passthrough.example.com",
				},
				{Name: proxy39.Name, Namespace: proxy39.Namespace}: {
					Object:      proxy39,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "www.example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy w/ includes valid child": {
			objs: []interface{}{proxy38, proxy40, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy38.Name, Namespace: proxy38.Namespace}: {
					Object:      proxy38,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "passthrough.example.com",
				},
				{Name: proxy40.Name, Namespace: proxy40.Namespace}: {
					Object:      proxy40,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "passthrough.example.com",
				},
			},
		},
		"httpproxy w/ missing include": {
			objs: []interface{}{proxy44, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy44.Name, Namespace: proxy44.Namespace}: {
					Object:      proxy44,
					Status:      "invalid",
					Description: "include roots/child not found",
					Vhost:       "example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy w/ missing service": {
			objs: []interface{}{proxy45},
			want: map[k8s.FullName]Status{
				{Name: proxy45.Name, Namespace: proxy45.Namespace}: {
					Object:      proxy45,
					Status:      "invalid",
					Description: "tcpproxy: service roots/not-found/8080: not found",
					Vhost:       "tcpproxy.example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy missing tls": {
			objs: []interface{}{proxy46},
			want: map[k8s.FullName]Status{
				{Name: proxy46.Name, Namespace: proxy46.Namespace}: {
					Object:      proxy46,
					Status:      "invalid",
					Description: "tcpproxy: missing tls.passthrough or tls.secretName",
					Vhost:       "tcpproxy.example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy missing service": {
			objs: []interface{}{sec1, s1, proxy47},
			want: map[k8s.FullName]Status{
				{Name: proxy47.Name, Namespace: proxy47.Namespace}: {
					Object:      proxy47,
					Status:      "invalid",
					Description: "Service [missing:9000] is invalid or missing",
					Vhost:       "tcpproxy.example.com",
				},
			},
		},
		"ingressroute first, then identical httpproxy": {
			objs: []interface{}{ir1, proxy1, s4},
			want: map[k8s.FullName]Status{
				{Name: ir1.Name, Namespace: ir1.Namespace}:       {Object: ir1, Status: "valid", Description: "valid IngressRoute", Vhost: "example.com"},
				{Name: proxy1.Name, Namespace: proxy1.Namespace}: {Object: proxy1, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
			},
		},
		"valid HTTPProxy.TCPProxy": {
			objs: []interface{}{proxy48root, proxy48child, s1, sec1},
			want: map[k8s.FullName]Status{
				{Name: proxy48root.Name, Namespace: proxy48root.Namespace}:   {Object: proxy48root, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
				{Name: proxy48child.Name, Namespace: proxy48child.Namespace}: {Object: proxy48child, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
			},
		},
		"valid HTTPProxy.TCPProxy - plural": {
			objs: []interface{}{proxy48rootplural, proxy48child, s1, sec1},
			want: map[k8s.FullName]Status{
				{Name: proxy48rootplural.Name, Namespace: proxy48rootplural.Namespace}: {Object: proxy48rootplural, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
				{Name: proxy48child.Name, Namespace: proxy48child.Namespace}:           {Object: proxy48child, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
			},
		},
		// issue 2309, each route must have at least one service
		"invalid HTTPProxy due to empty route.service": {
			objs: []interface{}{proxy49, s1},
			want: map[k8s.FullName]Status{
				{Name: proxy49.Name, Namespace: proxy49.Namespace}: {
					Object:      proxy49,
					Status:      "invalid",
					Description: "route.services must have at least one entry",
					Vhost:       "missing-service.example.com",
				},
			},
		},
		"invalid fallback certificate passed to contour": {
			fallbackCertificate: &k8s.FullName{
				Name:      "invalid",
				Namespace: "invalid",
			},
			objs: []interface{}{fallbackCertificate, fallbackSecret, sec1, s4},
			want: map[k8s.FullName]Status{
				{Name: fallbackCertificate.Name, Namespace: fallbackCertificate.Namespace}: {Object: fallbackCertificate, Status: "invalid", Description: "Spec.Virtualhost.TLS fallback certificate Secret \"invalid/invalid\" is invalid: Secret not found", Vhost: "example.com"},
			},
		},
		"fallback certificate requested but cert not configured in contour": {
			objs: []interface{}{fallbackCertificate, fallbackSecret, sec1, s4},
			want: map[k8s.FullName]Status{
				{Name: fallbackCertificate.Name, Namespace: fallbackCertificate.Namespace}: {Object: fallbackCertificate, Status: "invalid", Description: "Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file", Vhost: "example.com"},
			},
		},
		"fallback certificate requested and clientValidation also configured": {
			objs: []interface{}{fallbackCertificateWithClientValidation, fallbackSecret, sec1, s4},
			want: map[k8s.FullName]Status{
				{Name: fallbackCertificateWithClientValidation.Name, Namespace: fallbackCertificateWithClientValidation.Namespace}: {Object: fallbackCertificateWithClientValidation, Status: "invalid", Description: "Spec.Virtualhost.TLS fallback & client validation are incompatible together", Vhost: "example.com"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				FallbackCertificate: tc.fallbackCertificate,
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    testLogger(t),
				},
			}
			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()
			got := dag.Statuses()
			assert.Equal(t, tc.want, got)
		})
	}
}
