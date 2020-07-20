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

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDAGStatus(t *testing.T) {
	secretRootsNS := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ssl-cert",
			Namespace: "roots",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	secretContourNS := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-ssl-cert",
			Namespace: "projectcontour",
		},
		Type: v1.SecretTypeTLS,
		Data: secretRootsNS.Data,
	}

	fallbackSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "roots",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	serviceKuard := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: secretRootsNS.Namespace,
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

	serviceHome := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "home",
			Namespace: serviceKuard.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	serviceFoo2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: serviceKuard.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	serviceFoo3InvalidPort := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: serviceKuard.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     12345678,
			}},
		},
	}

	serviceGreenMarketing := &v1.Service{
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

	serviceNginx := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: serviceKuard.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}

	sericeKuardTeamA := &v1.Service{
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

	serviceKuardTeamB := &v1.Service{
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

	proxyMultiIncludeOneInvalid := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "validChild",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "invalidChild",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxyIncludeValidChild := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parentvalidchild",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "validChild",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxyChildValidFoo2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "validChild",
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

	proxyChildInvalidBadPort := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "foo3",
					Port: 12345678,
				}},
			}},
		},
	}

	ingressSharedService := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: serviceNginx.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: secretRootsNS.Name,
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "example.com",
				IngressRuleValue: ingressrulevalue(backend(serviceNginx.Name, intstr.FromInt(80))),
			}},
		},
	}

	proxyTCPSharedService := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: serviceNginx.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName: secretRootsNS.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: serviceNginx.Name,
					Port: 80,
				}},
			},
		},
	}

	proxyDelegatedTCPTLS := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &projcontour.TLS{
					SecretName: secretContourNS.Namespace + "/" + secretContourNS.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			},
		},
	}

	proxyDelegatedTLS := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &projcontour.TLS{
					SecretName: secretContourNS.Namespace + "/" + secretContourNS.Name,
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

	proxyPassthroughProxyNonSecure := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: s10.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
				},
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/",
				}},
				Services: []projcontour.Service{{
					Name: s10.Name,
					Port: 80, // proxy non secure traffic to port 80
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: s10.Name,
					Port: 443, // ssl passthrough to secure port
				}},
			},
		},
	}

	proxyMultipleIncludersSite1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site1",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "site1.com",
			},
			Includes: []projcontour.Include{{
				Name:      "www",
				Namespace: serviceKuard.Namespace,
			}},
		},
	}

	proxyMultipleIncludersSite2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site2",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "site2.com",
			},
			Includes: []projcontour.Include{{
				Name:      "www",
				Namespace: serviceKuard.Namespace,
			}},
		},
	}

	proxyMultiIncludeChild := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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

	// proxyNoFQDN is invalid because it does not specify and FQDN
	proxyNoFQDN := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "invalid",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy16a is invalid because it references an invalid port on a service
	proxy16a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 9999,
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
				Conditions: []projcontour.MatchCondition{{
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
					SecretName: secretContourNS.Namespace + "/" + secretContourNS.Name,
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxy21 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: serviceGreenMarketing.Namespace,
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
					Name: serviceGreenMarketing.Name,
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxy23 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: serviceGreenMarketing.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: serviceGreenMarketing.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxyBlogMarketing := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: serviceGreenMarketing.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: serviceGreenMarketing.Name,
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
				Name:      proxyBlogMarketing.Name,
				Namespace: proxyBlogMarketing.Namespace,
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
		},
	}

	proxy26 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}, {
					Name: serviceKuard.Name,
					Port: 8080,
				}, {
					Name:   serviceKuard.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}
	proxy27 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}, {
					Name:   serviceKuard.Name,
					Port:   8080,
					Mirror: true,
				}, {
					Name:   serviceKuard.Name,
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &projcontour.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &projcontour.HeaderMatchCondition{
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &projcontour.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &projcontour.HeaderMatchCondition{
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &projcontour.HeaderMatchCondition{
						Name:     "x-header",
						NotExact: "abc",
					},
				}, {
					Header: &projcontour.HeaderMatchCondition{
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
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy33 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []projcontour.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
			}},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
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
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy35 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{
					{
						Prefix: "api",
					},
				},
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy36 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []projcontour.MatchCondition{
					{
						Prefix: "api",
					},
				},
			}},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
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
					Name: serviceKuard.Name,
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
					Namespace: serviceKuard.Namespace,
				},
			},
		},
	}

	proxy39 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: serviceKuard.Namespace,
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
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy40 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
					Header: &projcontour.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
					Header: &projcontour.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []projcontour.Service{{
					Name: sericeKuardTeamA.Name,
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
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []projcontour.Service{{
					Name: serviceKuardTeamB.Name,
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
			Namespace: serviceKuard.Namespace,
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

	proxy45a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-proxy-service-missing-port",
			Namespace: serviceKuard.Namespace,
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
					Name: serviceKuard.Name,
					Port: 9999,
				}},
			},
		},
	}

	proxy46 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tls",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy47 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: secretRootsNS.Name,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{Name: "missing", Port: 9000},
				},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy47a := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service-port",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: secretRootsNS.Name,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{Name: serviceKuard.Name, Port: 9999},
				},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy48root := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: secretRootsNS.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Include: &projcontour.TCPProxyInclude{
					Name:      "child",
					Namespace: serviceKuard.Namespace,
				},
			},
		},
	}

	proxy48rootplural := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: secretRootsNS.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				IncludesDeprecated: &projcontour.TCPProxyInclude{
					Name:      "child",
					Namespace: serviceKuard.Namespace,
				},
			},
		},
	}

	proxy48child := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: serviceKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	// issue 2309, each route must have at least one service
	proxy49 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-service",
			Namespace: serviceKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "missing-service.example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
				Conditions: []projcontour.MatchCondition{{
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
		"proxy has multiple includes, one is invalid": {
			objs: []interface{}{proxyMultiIncludeOneInvalid, proxyChildValidFoo2, proxyChildInvalidBadPort, serviceFoo2, serviceFoo3InvalidPort},
			want: map[k8s.FullName]Status{
				{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}:                 {Object: proxyChildValidFoo2, Status: "valid", Description: "valid HTTPProxy"},
				{Name: proxyChildInvalidBadPort.Name, Namespace: proxyChildInvalidBadPort.Namespace}:       {Object: proxyChildInvalidBadPort, Status: "invalid", Description: `service "foo3": port must be in the range 1-65535`},
				{Name: proxyMultiIncludeOneInvalid.Name, Namespace: proxyMultiIncludeOneInvalid.Namespace}: {Object: proxyMultiIncludeOneInvalid, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
			},
		},
		"multi-parent children is not orphaned when one of the parents is invalid": {
			objs: []interface{}{proxyNoFQDN, proxyChildValidFoo2, proxyIncludeValidChild, serviceKuard, serviceFoo2},
			want: map[k8s.FullName]Status{
				{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}:                       {Object: proxyNoFQDN, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
				{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}:       {Object: proxyChildValidFoo2, Status: "valid", Description: "valid HTTPProxy"},
				{Name: proxyIncludeValidChild.Name, Namespace: proxyIncludeValidChild.Namespace}: {Object: proxyIncludeValidChild, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
			},
		},
		// issue 1399
		"service shared across ingress and httpproxy tcpproxy": {
			objs: []interface{}{
				secretRootsNS, serviceNginx, ingressSharedService, proxyTCPSharedService,
			},
			want: map[k8s.FullName]Status{
				{Name: proxyTCPSharedService.Name, Namespace: proxyTCPSharedService.Namespace}: {
					Object:      proxyTCPSharedService,
					Status:      k8s.StatusValid,
					Description: `valid HTTPProxy`,
					Vhost:       "example.com",
				},
			},
		},
		// issue 1347
		"check status set when tcpproxy combined with tls delegation failure": {
			objs: []interface{}{
				secretContourNS,
				proxyDelegatedTCPTLS,
			},
			want: map[k8s.FullName]Status{
				{Name: proxyDelegatedTCPTLS.Name, Namespace: proxyDelegatedTCPTLS.Namespace}: {
					Object:      proxyDelegatedTCPTLS,
					Status:      k8s.StatusInvalid,
					Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.ToFullName(secretContourNS)),
					Vhost:       proxyDelegatedTCPTLS.Spec.VirtualHost.Fqdn,
				},
			},
		},
		// issue 1348
		"check status set when routes combined with tls delegation failure": {
			objs: []interface{}{
				secretContourNS,
				proxyDelegatedTLS,
			},
			want: map[k8s.FullName]Status{
				{Name: proxyDelegatedTLS.Name, Namespace: proxyDelegatedTLS.Namespace}: {
					Object:      proxyDelegatedTLS,
					Status:      k8s.StatusInvalid,
					Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.ToFullName(secretContourNS)),
					Vhost:       proxyDelegatedTLS.Spec.VirtualHost.Fqdn,
				},
			},
		},
		// issue 1348
		"check status set when httpproxy routes combined with tls delegation failure": {
			objs: []interface{}{
				secretContourNS,
				proxy19,
			},
			want: map[k8s.FullName]Status{
				{Name: proxy19.Name, Namespace: proxy19.Namespace}: {
					Object:      proxy19,
					Status:      k8s.StatusInvalid,
					Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.ToFullName(secretContourNS)),
					Vhost:       proxy19.Spec.VirtualHost.Fqdn,
				},
			},
		},
		// issue 910
		"non tls routes can be combined with tcp proxy": {
			objs: []interface{}{
				s10,
				proxyPassthroughProxyNonSecure,
			},
			want: map[k8s.FullName]Status{
				{Name: proxyPassthroughProxyNonSecure.Name, Namespace: proxyPassthroughProxyNonSecure.Namespace}: {
					Object:      proxyPassthroughProxyNonSecure,
					Status:      k8s.StatusValid,
					Description: `valid HTTPProxy`,
					Vhost:       proxyPassthroughProxyNonSecure.Spec.VirtualHost.Fqdn,
				},
			},
		},
		"two root httpproxies delegated to the same object should not conflict on hostname": {
			objs: []interface{}{
				serviceKuard, proxyMultipleIncludersSite1, proxyMultipleIncludersSite2, proxyMultiIncludeChild,
			},
			want: map[k8s.FullName]Status{
				{Name: proxyMultipleIncludersSite1.Name, Namespace: proxyMultipleIncludersSite1.Namespace}: {
					Object:      proxyMultipleIncludersSite1,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "site1.com",
				},
				{Name: proxyMultipleIncludersSite2.Name, Namespace: proxyMultipleIncludersSite2.Namespace}: {
					Object:      proxyMultipleIncludersSite2,
					Status:      "valid",
					Description: "valid HTTPProxy",
					Vhost:       "site2.com",
				},
				{Name: proxyMultiIncludeChild.Name, Namespace: proxyMultiIncludeChild.Namespace}: {
					Object:      proxyMultiIncludeChild,
					Status:      "valid",
					Description: "valid HTTPProxy",
				},
			},
		},
		"valid proxy": {
			objs: []interface{}{proxy1, serviceHome},
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
			objs: []interface{}{proxyNoFQDN},
			want: map[k8s.FullName]Status{
				{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: {Object: proxyNoFQDN, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
			},
		},
		"proxy self-edge produces a cycle": {
			objs: []interface{}{proxy6, serviceKuard},
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
					Description: `Spec.Routes unresolved service reference: service "roots/invalid" not found`,
					Vhost:       proxy16.Spec.VirtualHost.Fqdn,
				},
			},
		},
		"proxy with service missing port shows invalid status": {
			objs: []interface{}{proxy16a, serviceHome},
			want: map[k8s.FullName]Status{
				{Name: proxy16a.Name, Namespace: proxy16a.Namespace}: {
					Object:      proxy16a,
					Status:      "invalid",
					Description: `Spec.Routes unresolved service reference: port "9999" on service "roots/home" not matched`,
					Vhost:       proxy16a.Spec.VirtualHost.Fqdn,
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
		"root proxy including another root": {
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
		"root proxy including another root w/ different hostname": {
			objs: []interface{}{proxy22, proxy23, serviceGreenMarketing},
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
		"proxy includes another": {
			objs: []interface{}{proxyBlogMarketing, proxy25, serviceKuard, serviceGreenMarketing},
			want: map[k8s.FullName]Status{
				{Name: proxyBlogMarketing.Name, Namespace: proxyBlogMarketing.Namespace}: {
					Object:      proxyBlogMarketing,
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
			objs: []interface{}{proxy26, serviceKuard},
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
			objs: []interface{}{proxy27, serviceKuard},
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
			objs: []interface{}{proxy32, serviceKuard},
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
			objs: []interface{}{proxy33, proxy34, serviceKuard},
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
			objs: []interface{}{proxy35, serviceKuard},
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
			objs: []interface{}{proxy36, proxy34, serviceKuard},
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
			objs: []interface{}{proxy28, serviceHome},
			want: map[k8s.FullName]Status{
				{Name: proxy28.Name, Namespace: proxy28.Namespace}: {Object: proxy28, Status: "invalid", Description: "cannot specify duplicate header 'exact match' conditions in the same route", Vhost: "example.com"},
			},
		},
		"duplicate valid route condition headers": {
			objs: []interface{}{proxy31, serviceHome},
			want: map[k8s.FullName]Status{
				{Name: proxy31.Name, Namespace: proxy31.Namespace}: {Object: proxy31, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
			},
		},
		"duplicate include condition headers": {
			objs: []interface{}{proxy29, proxy30, serviceHome},
			want: map[k8s.FullName]Status{
				{Name: proxy29.Name, Namespace: proxy29.Namespace}: {Object: proxy29, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
				{Name: proxy30.Name, Namespace: proxy30.Namespace}: {Object: proxy30, Status: "invalid", Description: "cannot specify duplicate header 'exact match' conditions in the same route", Vhost: ""},
			},
		},
		"duplicate path conditions on an include": {
			objs: []interface{}{proxy41, proxy41a, proxy41b, serviceHome, sericeKuardTeamA, serviceKuardTeamB},
			want: map[k8s.FullName]Status{
				{Name: proxy41.Name, Namespace: proxy41.Namespace}:   {Object: proxy41, Status: "invalid", Description: "duplicate conditions defined on an include", Vhost: "example.com"},
				{Name: proxy41a.Name, Namespace: proxy41a.Namespace}: {Object: proxy41a, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
				{Name: proxy41b.Name, Namespace: proxy41b.Namespace}: {Object: proxy41b, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
			},
		},
		"duplicate header conditions on an include": {
			objs: []interface{}{proxy42, proxy41a, proxy41b, serviceHome, sericeKuardTeamA, serviceKuardTeamB},
			want: map[k8s.FullName]Status{
				{Name: proxy42.Name, Namespace: proxy42.Namespace}:   {Object: proxy42, Status: "invalid", Description: "duplicate conditions defined on an include", Vhost: "example.com"},
				{Name: proxy41a.Name, Namespace: proxy41a.Namespace}: {Object: proxy41a, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
				{Name: proxy41b.Name, Namespace: proxy41b.Namespace}: {Object: proxy41b, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
			},
		},
		"duplicate header+path conditions on an include": {
			objs: []interface{}{proxy43, proxy41a, proxy41b, serviceHome, sericeKuardTeamA, serviceKuardTeamB},
			want: map[k8s.FullName]Status{
				{Name: proxy43.Name, Namespace: proxy43.Namespace}:   {Object: proxy43, Status: "invalid", Description: "duplicate conditions defined on an include", Vhost: "example.com"},
				{Name: proxy41a.Name, Namespace: proxy41a.Namespace}: {Object: proxy41a, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
				{Name: proxy41b.Name, Namespace: proxy41b.Namespace}: {Object: proxy41b, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy", Vhost: ""},
			},
		},
		"httpproxy with invalid tcpproxy": {
			objs: []interface{}{proxy37, serviceKuard},
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
			objs: []interface{}{proxy37a, serviceKuard},
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
			objs: []interface{}{proxy38, serviceKuard},
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
			objs: []interface{}{proxy38, proxy39, serviceKuard},
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
			objs: []interface{}{proxy38, proxy40, serviceKuard},
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
			objs: []interface{}{proxy44, serviceKuard},
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
					Description: `Spec.TCPProxy unresolved service reference: service "roots/not-found" not found`,
					Vhost:       "tcpproxy.example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy w/ service missing port": {
			objs: []interface{}{proxy45a, serviceKuard},
			want: map[k8s.FullName]Status{
				{Name: proxy45a.Name, Namespace: proxy45a.Namespace}: {
					Object:      proxy45a,
					Status:      "invalid",
					Description: `Spec.TCPProxy unresolved service reference: port "9999" on service "roots/kuard" not matched`,
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
			objs: []interface{}{secretRootsNS, serviceKuard, proxy47},
			want: map[k8s.FullName]Status{
				{Name: proxy47.Name, Namespace: proxy47.Namespace}: {
					Object:      proxy47,
					Status:      "invalid",
					Description: `Spec.Routes unresolved service reference: service "roots/missing" not found`,
					Vhost:       "tcpproxy.example.com",
				},
			},
		},
		"httpproxy w/ tcpproxy missing service port": {
			objs: []interface{}{secretRootsNS, serviceKuard, proxy47a},
			want: map[k8s.FullName]Status{
				{Name: proxy47a.Name, Namespace: proxy47a.Namespace}: {
					Object:      proxy47a,
					Status:      "invalid",
					Description: `Spec.Routes unresolved service reference: port "9999" on service "roots/kuard" not matched`,
					Vhost:       "tcpproxy.example.com",
				},
			},
		},
		"valid HTTPProxy.TCPProxy": {
			objs: []interface{}{proxy48root, proxy48child, serviceKuard, secretRootsNS},
			want: map[k8s.FullName]Status{
				{Name: proxy48root.Name, Namespace: proxy48root.Namespace}:   {Object: proxy48root, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
				{Name: proxy48child.Name, Namespace: proxy48child.Namespace}: {Object: proxy48child, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
			},
		},
		"valid HTTPProxy.TCPProxy - plural": {
			objs: []interface{}{proxy48rootplural, proxy48child, serviceKuard, secretRootsNS},
			want: map[k8s.FullName]Status{
				{Name: proxy48rootplural.Name, Namespace: proxy48rootplural.Namespace}: {Object: proxy48rootplural, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
				{Name: proxy48child.Name, Namespace: proxy48child.Namespace}:           {Object: proxy48child, Status: "valid", Description: "valid HTTPProxy", Vhost: "tcpproxy.example.com"},
			},
		},
		// issue 2309, each route must have at least one service
		"invalid HTTPProxy due to empty route.service": {
			objs: []interface{}{proxy49, serviceKuard},
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
			objs: []interface{}{fallbackCertificate, fallbackSecret, secretRootsNS, serviceHome},
			want: map[k8s.FullName]Status{
				{Name: fallbackCertificate.Name, Namespace: fallbackCertificate.Namespace}: {Object: fallbackCertificate, Status: "invalid", Description: "Spec.Virtualhost.TLS Secret \"invalid/invalid\" fallback certificate is invalid: Secret not found", Vhost: "example.com"},
			},
		},
		"fallback certificate requested but cert not configured in contour": {
			objs: []interface{}{fallbackCertificate, fallbackSecret, secretRootsNS, serviceHome},
			want: map[k8s.FullName]Status{
				{Name: fallbackCertificate.Name, Namespace: fallbackCertificate.Namespace}: {Object: fallbackCertificate, Status: "invalid", Description: "Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file", Vhost: "example.com"},
			},
		},
		"fallback certificate requested and clientValidation also configured": {
			objs: []interface{}{fallbackCertificateWithClientValidation, fallbackSecret, secretRootsNS, serviceHome},
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
