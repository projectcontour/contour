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

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDAGStatus(t *testing.T) {

	type testcase struct {
		objs                []interface{}
		fallbackCertificate *types.NamespacedName
		want                map[types.NamespacedName]projcontour.DetailedCondition
	}

	run := func(desc string, tc testcase) {
		t.Run(desc, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{
						FallbackCertificate: tc.fallbackCertificate,
					},
					&ListenerProcessor{},
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

	// Common test fixtures (used across more than one test)

	// proxyNoFQDN is invalid because it does not specify and FQDN
	proxyNoFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "parent",
			Generation: 23,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// Tests using common fixtures
	run("root proxy does not specify FQDN", testcase{
		objs: []interface{}{proxyNoFQDN},
		want: map[types.NamespacedName]projcontour.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().WithGeneration(23).
				WithError("VirtualHostError", "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
		},
	})

	// Simple Valid HTTPProxy
	proxyValidHomeService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("valid proxy", testcase{
		objs: []interface{}{proxyValidHomeService, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]projcontour.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().WithGeneration(24).
				WithError("VirtualHostError", "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
		},

		// map[types.NamespacedName]Status{
		// 	{Name: proxyValidHomeService.Name, Namespace: proxyValidHomeService.Namespace}: {Object: proxyValidHomeService, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
		// },
	})

	// Multiple Includes, one invalid
	proxyMultiIncludeOneInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "parent",
			Generation: 45,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "validChild",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "invalidChild",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxyIncludeValidChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parentvalidchild",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "validChild",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxyChildValidFoo2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "validChild",
			Generation: 1,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	proxyChildInvalidBadPort := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "foo3",
					Port: 12345678,
				}},
			}},
		},
	}

	run("proxy has multiple includes, one is invalid", testcase{
		objs: []interface{}{proxyMultiIncludeOneInvalid, proxyChildValidFoo2, proxyChildInvalidBadPort, fixture.ServiceRootsFoo2, fixture.ServiceRootsFoo3InvalidPort},
		want: map[types.NamespacedName]projcontour.DetailedCondition{
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}:                 fixture.NewValidCondition().WithGeneration(proxyChildValidFoo2.Generation).Valid(),
			{Name: proxyChildInvalidBadPort.Name, Namespace: proxyChildInvalidBadPort.Namespace}:       fixture.NewValidCondition().WithGeneration(proxyChildInvalidBadPort.Generation).WithError("ServiceError", "ServicePortInvalid", `service "foo3": port must be in the range 1-65535`),
			{Name: proxyMultiIncludeOneInvalid.Name, Namespace: proxyMultiIncludeOneInvalid.Namespace}: fixture.NewValidCondition().WithGeneration(proxyMultiIncludeOneInvalid.Generation).Valid(),
		},
	})

	run("multi-parent child is not orphaned when one of the parents is invalid", testcase{
		objs: []interface{}{proxyNoFQDN, proxyChildValidFoo2, proxyIncludeValidChild, fixture.ServiceRootsKuard, fixture.ServiceRootsFoo2},
		want: map[types.NamespacedName]projcontour.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}:                       fixture.NewValidCondition().WithGeneration(proxyNoFQDN.Generation).WithError("VirtualHostError", "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}:       fixture.NewValidCondition().WithGeneration(proxyChildValidFoo2.Generation).Valid(),
			{Name: proxyIncludeValidChild.Name, Namespace: proxyIncludeValidChild.Namespace}: fixture.NewValidCondition().WithGeneration(proxyIncludeValidChild.Generation).Valid(),
		},
	})

	ingressSharedService := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: fixture.ServiceRootsNginx.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: fixture.SecretRootsCert.Name,
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "example.com",
				IngressRuleValue: ingressrulevalue(backend(fixture.ServiceRootsNginx.Name, intstr.FromInt(80))),
			}},
		},
	}

	proxyTCPSharedService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: fixture.ServiceRootsNginx.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsNginx.Name,
					Port: 80,
				}},
			},
		},
	}

	// issue 1399
	run("service shared across ingress and httpproxy tcpproxy", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert, fixture.ServiceRootsNginx, ingressSharedService, proxyTCPSharedService,
		},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPSharedService.Name, Namespace: proxyTCPSharedService.Namespace}: {
				Object:      proxyTCPSharedService,
				Status:      k8s.StatusValid,
				Description: `valid HTTPProxy`,
				Vhost:       "example.com",
			},
		},
	})

	proxyDelegatedTCPTLS := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			},
		},
	}

	// issue 1347
	run("tcpproxy with tls delegation failure", testcase{
		objs: []interface{}{
			fixture.SecretProjectContourCert,
			proxyDelegatedTCPTLS,
		},
		want: map[types.NamespacedName]Status{
			{Name: proxyDelegatedTCPTLS.Name, Namespace: proxyDelegatedTCPTLS.Namespace}: {
				Object:      proxyDelegatedTCPTLS,
				Status:      k8s.StatusInvalid,
				Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.NamespacedNameOf(fixture.SecretProjectContourCert)),
				Vhost:       proxyDelegatedTCPTLS.Spec.VirtualHost.Fqdn,
			},
		},
	})

	proxyDelegatedTLS := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			}},
		},
	}

	// issue 1348
	run("routes with tls delegation failure", testcase{
		objs: []interface{}{
			fixture.SecretProjectContourCert,
			proxyDelegatedTLS,
		},
		want: map[types.NamespacedName]Status{
			{Name: proxyDelegatedTLS.Name, Namespace: proxyDelegatedTLS.Namespace}: {
				Object:      proxyDelegatedTLS,
				Status:      k8s.StatusInvalid,
				Description: fmt.Sprintf("Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", k8s.NamespacedNameOf(fixture.SecretProjectContourCert)),
				Vhost:       proxyDelegatedTLS.Spec.VirtualHost.Fqdn,
			},
		},
	})

	serviceTLSPassthrough := &v1.Service{
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

	proxyPassthroughProxyNonSecure := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: serviceTLSPassthrough.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: serviceTLSPassthrough.Name,
					Port: 80, // proxy non secure traffic to port 80
				}},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: serviceTLSPassthrough.Name,
					Port: 443, // ssl passthrough to secure port
				}},
			},
		},
	}

	// issue 910
	run("non tls routes can be combined with tcp proxy", testcase{
		objs: []interface{}{
			serviceTLSPassthrough,
			proxyPassthroughProxyNonSecure,
		},
		want: map[types.NamespacedName]Status{
			{Name: proxyPassthroughProxyNonSecure.Name, Namespace: proxyPassthroughProxyNonSecure.Namespace}: {
				Object:      proxyPassthroughProxyNonSecure,
				Status:      k8s.StatusValid,
				Description: `valid HTTPProxy`,
				Vhost:       proxyPassthroughProxyNonSecure.Spec.VirtualHost.Fqdn,
			},
		},
	})

	proxyMultipleIncludersSite1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site1",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "site1.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultipleIncludersSite2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site2",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "site2.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultiIncludeChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run("two root httpproxies with different hostnames delegated to the same object are valid", testcase{
		objs: []interface{}{
			fixture.ServiceRootsKuard, proxyMultipleIncludersSite1, proxyMultipleIncludersSite2, proxyMultiIncludeChild,
		},
		want: map[types.NamespacedName]Status{
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
	})

	// proxyInvalidNegativePortHomeService is invalid because it contains a service with negative port
	proxyInvalidNegativePortHomeService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	run("invalid port in service", testcase{
		objs: []interface{}{proxyInvalidNegativePortHomeService},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidNegativePortHomeService.Name, Namespace: proxyInvalidNegativePortHomeService.Namespace}: {Object: proxyInvalidNegativePortHomeService, Status: "invalid", Description: `service "home": port must be in the range 1-65535`, Vhost: "example.com"},
		},
	})

	// proxyInvalidOutsideRootNamespace is invalid because it lives outside the roots namespace
	proxyInvalidOutsideRootNamespace := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foobar",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("root proxy outside of roots namespace", testcase{
		objs: []interface{}{proxyInvalidOutsideRootNamespace},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidOutsideRootNamespace.Name, Namespace: proxyInvalidOutsideRootNamespace.Namespace}: {Object: proxyInvalidOutsideRootNamespace, Status: "invalid", Description: "root HTTPProxy cannot be defined in this namespace"},
		},
	})

	// proxyInvalidIncludeCycle is invalid because it delegates to itself, producing a cycle
	proxyInvalidIncludeCycle := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "self",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "self",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "green",
					Port: 80,
				}},
			}},
		},
	}

	run("proxy self-edge produces a cycle", testcase{
		objs: []interface{}{proxyInvalidIncludeCycle, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidIncludeCycle.Name, Namespace: proxyInvalidIncludeCycle.Namespace}: {
				Object:      proxyInvalidIncludeCycle,
				Status:      "invalid",
				Description: "root httpproxy cannot delegate to another root httpproxy",
				Vhost:       "example.com",
			},
		},
	})

	// proxyIncludesProxyWithIncludeCycle delegates to proxy8, which is invalid because proxy8 delegates back to proxy8
	proxyIncludesProxyWithIncludeCycle := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxyIncludedChildInvalidIncludeCycle := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	run("proxy child delegates to parent, producing a cycle", testcase{
		objs: []interface{}{proxyIncludesProxyWithIncludeCycle, proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]Status{
			{Name: proxyIncludesProxyWithIncludeCycle.Name, Namespace: proxyIncludesProxyWithIncludeCycle.Namespace}: {
				Object:      proxyIncludesProxyWithIncludeCycle,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "example.com",
			},
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: {
				Object:      proxyIncludedChildInvalidIncludeCycle,
				Status:      "invalid",
				Description: "include creates a delegation cycle: roots/parent -> roots/child -> roots/child",
			},
		},
	})

	run("proxy orphaned route", testcase{
		objs: []interface{}{proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]Status{
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: {Object: proxyIncludedChildInvalidIncludeCycle, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy"},
		},
	})

	proxyIncludedChildValid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validChild",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	// proxyNotRootIncludeRootProxy delegates to proxyWildCardFQDN but it is invalid because it is missing fqdn
	proxyNotRootIncludeRootProxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{},
			Includes: []contour_api_v1.Include{{
				Name:      "validChild",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	run("proxy invalid parent orphans child", testcase{
		objs: []interface{}{proxyNotRootIncludeRootProxy, proxyIncludedChildValid},
		want: map[types.NamespacedName]Status{
			{Name: proxyNotRootIncludeRootProxy.Name, Namespace: proxyNotRootIncludeRootProxy.Namespace}: {Object: proxyNotRootIncludeRootProxy, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
			{Name: proxyIncludedChildValid.Name, Namespace: proxyIncludedChildValid.Namespace}:           {Object: proxyIncludedChildValid, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy"},
		},
	})

	// proxyWildCardFQDN is invalid because it contains a wildcarded fqdn
	proxyWildCardFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.*.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("proxy invalid FQDN contains wildcard", testcase{
		objs: []interface{}{proxyWildCardFQDN},
		want: map[types.NamespacedName]Status{
			{Name: proxyWildCardFQDN.Name, Namespace: proxyWildCardFQDN.Namespace}: {Object: proxyWildCardFQDN, Status: "invalid", Description: `Spec.VirtualHost.Fqdn "example.*.com" cannot use wildcards`, Vhost: "example.*.com"},
		},
	})

	// proxyInvalidServiceInvalid is invalid because it references an invalid service
	proxyInvalidServiceInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "invalid",
					Port: 8080,
				}},
			}},
		},
	}

	run("proxy missing service is invalid", testcase{
		objs: []interface{}{proxyInvalidServiceInvalid},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidServiceInvalid.Name, Namespace: proxyInvalidServiceInvalid.Namespace}: {
				Object:      proxyInvalidServiceInvalid,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: service "roots/invalid" not found`,
				Vhost:       proxyInvalidServiceInvalid.Spec.VirtualHost.Fqdn,
			},
		},
	})

	// proxyInvalidServicePortInvalid is invalid because it references an invalid port on a service
	proxyInvalidServicePortInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 9999,
				}},
			}},
		},
	}

	run("proxy with service missing port is invalid", testcase{
		objs: []interface{}{proxyInvalidServicePortInvalid, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidServicePortInvalid.Name, Namespace: proxyInvalidServicePortInvalid.Namespace}: {
				Object:      proxyInvalidServicePortInvalid,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: port "9999" on service "roots/home" not matched`,
				Vhost:       proxyInvalidServicePortInvalid.Spec.VirtualHost.Fqdn,
			},
		},
	})

	proxyValidExampleCom := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidReuseExampleCom := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-example",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run("conflicting proxies due to fqdn reuse", testcase{
		objs: []interface{}{proxyValidExampleCom, proxyValidReuseExampleCom},
		want: map[types.NamespacedName]Status{
			{Name: proxyValidExampleCom.Name, Namespace: proxyValidExampleCom.Namespace}: {
				Object:      proxyValidExampleCom,
				Status:      k8s.StatusInvalid,
				Description: `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`,
				Vhost:       "example.com",
			},
			{Name: proxyValidReuseExampleCom.Name, Namespace: proxyValidReuseExampleCom.Namespace}: {
				Object:      proxyValidReuseExampleCom,
				Status:      k8s.StatusInvalid,
				Description: `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`,
				Vhost:       "example.com",
			},
		},
	})

	proxyRootIncludesRoot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxyRootIncludedByRoot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	run("root proxy including another root", testcase{
		objs: []interface{}{proxyRootIncludesRoot, proxyRootIncludedByRoot},
		want: map[types.NamespacedName]Status{
			{Name: proxyRootIncludesRoot.Name, Namespace: proxyRootIncludesRoot.Namespace}: {
				Object:      proxyRootIncludesRoot,
				Status:      k8s.StatusInvalid,
				Description: `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`,
				Vhost:       "blog.containersteve.com",
			},
			{Name: proxyRootIncludedByRoot.Name, Namespace: proxyRootIncludedByRoot.Namespace}: {
				Object:      proxyRootIncludedByRoot,
				Status:      k8s.StatusInvalid,
				Description: `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`,
				Vhost:       "blog.containersteve.com",
			},
		},
	})

	proxyIncludesRootDifferentFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxyRootIncludedByRootDiffFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	run("root proxy including another root w/ different hostname", testcase{
		objs: []interface{}{proxyIncludesRootDifferentFQDN, proxyRootIncludedByRootDiffFQDN, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]Status{
			{Name: proxyIncludesRootDifferentFQDN.Name, Namespace: proxyIncludesRootDifferentFQDN.Namespace}: {
				Object:      proxyIncludesRootDifferentFQDN,
				Status:      k8s.StatusInvalid,
				Description: "root httpproxy cannot delegate to another root httpproxy",
				Vhost:       "blog.containersteve.com",
			},
			{Name: proxyRootIncludedByRootDiffFQDN.Name, Namespace: proxyRootIncludedByRootDiffFQDN.Namespace}: {
				Object:      proxyRootIncludedByRootDiffFQDN,
				Status:      k8s.StatusValid,
				Description: `valid HTTPProxy`,
				Vhost:       "www.containersteve.com",
			},
		},
	})

	proxyValidIncludeBlogMarketing := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxyRootValidIncludesBlogMarketing := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      proxyValidIncludeBlogMarketing.Name,
				Namespace: proxyValidIncludeBlogMarketing.Namespace,
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
		},
	}

	run("proxy includes another", testcase{
		objs: []interface{}{proxyValidIncludeBlogMarketing, proxyRootValidIncludesBlogMarketing, fixture.ServiceRootsKuard, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]Status{
			{Name: proxyValidIncludeBlogMarketing.Name, Namespace: proxyValidIncludeBlogMarketing.Namespace}: {
				Object:      proxyValidIncludeBlogMarketing,
				Status:      "valid",
				Description: "valid HTTPProxy",
			},
			{Name: proxyRootValidIncludesBlogMarketing.Name, Namespace: proxyRootValidIncludesBlogMarketing.Namespace}: {
				Object:      proxyRootValidIncludesBlogMarketing,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "example.com",
			},
		},
	})

	proxyValidWithMirror := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}, {
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}, {
					Name:   fixture.ServiceRootsKuard.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	run("proxy with mirror", testcase{
		objs: []interface{}{proxyValidWithMirror, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyValidWithMirror.Name, Namespace: proxyValidWithMirror.Namespace}: {
				Object:      proxyValidWithMirror,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "example.com",
			},
		},
	})

	proxyInvalidTwoMirrors := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}, {
					Name:   fixture.ServiceRootsKuard.Name,
					Port:   8080,
					Mirror: true,
				}, {
					Name:   fixture.ServiceRootsKuard.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	run("proxy with two mirrors", testcase{
		objs: []interface{}{proxyInvalidTwoMirrors, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidTwoMirrors.Name, Namespace: proxyInvalidTwoMirrors.Namespace}: {
				Object:      proxyInvalidTwoMirrors,
				Status:      "invalid",
				Description: "only one service per route may be nominated as mirror",
				Vhost:       "example.com",
			},
		},
	})

	proxyInvalidDuplicateMatchConditionHeaders := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("duplicate route condition headers", testcase{
		objs: []interface{}{proxyInvalidDuplicateMatchConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidDuplicateMatchConditionHeaders.Name, Namespace: proxyInvalidDuplicateMatchConditionHeaders.Namespace}: {
				Object: proxyInvalidDuplicateMatchConditionHeaders,
				Status: "invalid", Description: "cannot specify duplicate header 'exact match' conditions in the same route",
				Vhost: "example.com",
			},
		},
	})

	proxyInvalidDuplicateIncludeCondtionHeaders := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "delegated",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}
	proxyValidDelegatedRoots := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("duplicate include condition headers", testcase{
		objs: []interface{}{proxyInvalidDuplicateIncludeCondtionHeaders, proxyValidDelegatedRoots, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidDuplicateIncludeCondtionHeaders.Name,
				Namespace: proxyInvalidDuplicateIncludeCondtionHeaders.Namespace}: {
				Object: proxyInvalidDuplicateIncludeCondtionHeaders,
				Status: "valid", Description: "valid HTTPProxy",
				Vhost: "example.com",
			},
			{Name: proxyValidDelegatedRoots.Name,
				Namespace: proxyValidDelegatedRoots.Namespace}: {
				Object:      proxyValidDelegatedRoots,
				Status:      "invalid",
				Description: "cannot specify duplicate header 'exact match' conditions in the same route",
				Vhost:       ""},
		},
	})

	proxyInvalidRouteConditionHeaders := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						NotExact: "abc",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						NotExact: "1234",
					},
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("duplicate valid route condition headers", testcase{
		objs: []interface{}{proxyInvalidRouteConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidRouteConditionHeaders.Name, Namespace: proxyInvalidRouteConditionHeaders.Namespace}: {
				Object: proxyInvalidRouteConditionHeaders,
				Status: "valid", Description: "valid HTTPProxy",
				Vhost: "example.com",
			},
		},
	})

	proxyInvalidMultiplePrefixes := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run("proxy with two prefix conditions on route", testcase{
		objs: []interface{}{proxyInvalidMultiplePrefixes, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidMultiplePrefixes.Name, Namespace: proxyInvalidMultiplePrefixes.Namespace}: {
				Object:      proxyInvalidMultiplePrefixes,
				Status:      "invalid",
				Description: "route: more than one prefix is not allowed in a condition block",
				Vhost:       "example.com",
			},
		},
	})

	proxyInvalidTwoPrefixesWithInclude := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidChildTeamA := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "teama",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run("proxy with two prefix conditions orphans include", testcase{
		objs: []interface{}{proxyInvalidTwoPrefixesWithInclude, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidTwoPrefixesWithInclude.Name, Namespace: proxyInvalidTwoPrefixesWithInclude.Namespace}: {
				Object:      proxyInvalidTwoPrefixesWithInclude,
				Status:      "invalid",
				Description: "include: more than one prefix is not allowed in a condition block",
				Vhost:       "example.com",
			}, {Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: {
				Object:      proxyValidChildTeamA,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
			},
		},
	})

	proxyInvalidPrefixNoSlash := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "api",
					},
				},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run("proxy with prefix conditions on route that does not start with slash", testcase{
		objs: []interface{}{proxyInvalidPrefixNoSlash, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidPrefixNoSlash.Name, Namespace: proxyInvalidPrefixNoSlash.Namespace}: {
				Object:      proxyInvalidPrefixNoSlash,
				Status:      "invalid",
				Description: "route: prefix conditions must start with /, api was supplied",
				Vhost:       "example.com",
			},
		},
	})

	proxyInvalidIncludePrefixNoSlash := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "api",
					},
				},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run("proxy with include prefix that does not start with slash", testcase{
		objs: []interface{}{proxyInvalidIncludePrefixNoSlash, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidIncludePrefixNoSlash.Name, Namespace: proxyInvalidIncludePrefixNoSlash.Namespace}: {
				Object:      proxyInvalidIncludePrefixNoSlash,
				Status:      "invalid",
				Description: "include: prefix conditions must start with /, api was supplied",
				Vhost:       "example.com",
			}, {Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: {
				Object:      proxyValidChildTeamA,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
			},
		},
	})

	proxyInvalidTCPProxyIncludeAndService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Include: &contour_api_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: "roots",
				},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run("tcpproxy cannot specify services and include", testcase{
		objs: []interface{}{proxyInvalidTCPProxyIncludeAndService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidTCPProxyIncludeAndService.Name, Namespace: proxyInvalidTCPProxyIncludeAndService.Namespace}: {
				Object:      proxyInvalidTCPProxyIncludeAndService,
				Status:      "invalid",
				Description: "tcpproxy: cannot specify services and include in the same httpproxy",
				Vhost:       "passthrough.example.com",
			},
		},
	})

	proxyTCPNoServiceOrInclusion := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run("tcpproxy empty", testcase{
		objs: []interface{}{proxyTCPNoServiceOrInclusion, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPNoServiceOrInclusion.Name, Namespace: proxyTCPNoServiceOrInclusion.Namespace}: {
				Object:      proxyTCPNoServiceOrInclusion,
				Status:      "invalid",
				Description: "tcpproxy: either services or inclusion must be specified",
				Vhost:       "passthrough.example.com",
			},
		},
	})

	proxyTCPIncludesFoo := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Include: &contour_api_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	run("tcpproxy w/ missing include", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: {
				Object:      proxyTCPIncludesFoo,
				Status:      "invalid",
				Description: "tcpproxy: include roots/foo not found",
				Vhost:       "passthrough.example.com",
			},
		},
	})

	proxyValidTCPRoot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run("tcpproxy includes another root", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, proxyValidTCPRoot, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: {
				Object:      proxyTCPIncludesFoo,
				Status:      "invalid",
				Description: "root httpproxy cannot delegate to another root httpproxy",
				Vhost:       "passthrough.example.com",
			},
			{Name: proxyValidTCPRoot.Name, Namespace: proxyValidTCPRoot.Namespace}: {
				Object:      proxyValidTCPRoot,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "www.example.com",
			},
		},
	})

	proxyTCPValidChildFoo := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run("tcpproxy includes valid child", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, proxyTCPValidChildFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: {
				Object:      proxyTCPIncludesFoo,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "passthrough.example.com",
			},
			{Name: proxyTCPValidChildFoo.Name, Namespace: proxyTCPValidChildFoo.Namespace}: {
				Object:      proxyTCPValidChildFoo,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "passthrough.example.com",
			},
		},
	})

	proxyInvalidConflictingIncludeConditions := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
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
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidBlogTeamA := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "blogteama",
			Name:      "teama",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceTeamAKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidBlogTeamB := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "blogteamb",
			Name:      "teamb",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceTeamBKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run("duplicate path conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictingIncludeConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidConflictingIncludeConditions.Name,
				Namespace: proxyInvalidConflictingIncludeConditions.Namespace}: {
				Object:      proxyInvalidConflictingIncludeConditions,
				Status:      "invalid",
				Description: "duplicate conditions defined on an include",
				Vhost:       "example.com",
			},
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: {
				Object:      proxyValidBlogTeamA,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				Vhost:       "",
			},
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: {
				Object:      proxyValidBlogTeamB,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				Vhost:       "",
			},
		},
	})

	proxyInvalidConflictHeaderConditions := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{{
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
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
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("duplicate header conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidConflictHeaderConditions.Name,
				Namespace: proxyInvalidConflictHeaderConditions.Namespace}: {
				Object:      proxyInvalidConflictHeaderConditions,
				Status:      "invalid",
				Description: "duplicate conditions defined on an include",
				Vhost:       "example.com",
			},
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: {
				Object:      proxyValidBlogTeamA,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				Vhost:       "",
			},
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: {
				Object:      proxyValidBlogTeamB,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				Vhost:       "",
			},
		},
	})

	proxyInvalidDuplicateHeaderAndPathConditions := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
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
				Name:      "blogteamb",
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
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("duplicate header+path conditions on an include", testcase{
		objs: []interface{}{proxyInvalidDuplicateHeaderAndPathConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidDuplicateHeaderAndPathConditions.Name,
				Namespace: proxyInvalidDuplicateHeaderAndPathConditions.Namespace}: {
				Object:      proxyInvalidDuplicateHeaderAndPathConditions,
				Status:      "invalid",
				Description: "duplicate conditions defined on an include",
				Vhost:       "example.com",
			},
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: {
				Object:      proxyValidBlogTeamA,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				Vhost:       "",
			},
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: {
				Object:      proxyValidBlogTeamB,
				Status:      "orphaned",
				Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy",
				Vhost:       "",
			},
		},
	})

	proxyInvalidMissingInclude := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "child",
			}},
		},
	}

	run("httpproxy w/ missing include", testcase{
		objs: []interface{}{proxyInvalidMissingInclude, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidMissingInclude.Name, Namespace: proxyInvalidMissingInclude.Namespace}: {
				Object:      proxyInvalidMissingInclude,
				Status:      "invalid",
				Description: "include roots/child not found",
				Vhost:       "example.com",
			},
		},
	})

	proxyTCPInvalidMissingService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tcp-proxy-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: "not-found",
					Port: 8080,
				}},
			},
		},
	}

	run("httpproxy w/ tcpproxy w/ missing service", testcase{
		objs: []interface{}{proxyTCPInvalidMissingService},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPInvalidMissingService.Name, Namespace: proxyTCPInvalidMissingService.Namespace}: {
				Object:      proxyTCPInvalidMissingService,
				Status:      "invalid",
				Description: `Spec.TCPProxy unresolved service reference: service "roots/not-found" not found`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	proxyTCPInvalidPortNotMatched := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-proxy-service-missing-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 9999,
				}},
			},
		},
	}

	run("httpproxy w/ tcpproxy w/ service missing port", testcase{
		objs: []interface{}{proxyTCPInvalidPortNotMatched, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPInvalidPortNotMatched.Name, Namespace: proxyTCPInvalidPortNotMatched.Namespace}: {
				Object:      proxyTCPInvalidPortNotMatched,
				Status:      "invalid",
				Description: `Spec.TCPProxy unresolved service reference: port "9999" on service "roots/kuard" not matched`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	proxyTCPInvalidMissingTLS := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tls",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run("httpproxy w/ tcpproxy missing tls", testcase{
		objs: []interface{}{proxyTCPInvalidMissingTLS},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPInvalidMissingTLS.Name, Namespace: proxyTCPInvalidMissingTLS.Namespace}: {
				Object:      proxyTCPInvalidMissingTLS,
				Status:      "invalid",
				Description: "Spec.TCPProxy requires that either Spec.TLS.Passthrough or Spec.TLS.SecretName be set",
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	proxyInvalidMissingServiceWithTCPProxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{Name: "missing", Port: 9000},
				},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run("httpproxy w/ tcpproxy missing service", testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyInvalidMissingServiceWithTCPProxy},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidMissingServiceWithTCPProxy.Name, Namespace: proxyInvalidMissingServiceWithTCPProxy.Namespace}: {
				Object:      proxyInvalidMissingServiceWithTCPProxy,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: service "roots/missing" not found`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	proxyRoutePortNotMatchedWithTCP := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{Name: fixture.ServiceRootsKuard.Name, Port: 9999},
				},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run("tcpproxy route unmatched service port", testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyRoutePortNotMatchedWithTCP},
		want: map[types.NamespacedName]Status{
			{Name: proxyRoutePortNotMatchedWithTCP.Name, Namespace: proxyRoutePortNotMatchedWithTCP.Namespace}: {
				Object:      proxyRoutePortNotMatchedWithTCP,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: port "9999" on service "roots/kuard" not matched`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	proxyTCPValidIncludeChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Include: &contour_api_v1.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidIncludesChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				IncludesDeprecated: &contour_api_v1.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run("valid HTTPProxy.TCPProxy - plural", testcase{
		objs: []interface{}{proxyTCPValidIncludesChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPValidIncludesChild.Name,
				Namespace: proxyTCPValidIncludesChild.Namespace}: {
				Object:      proxyTCPValidIncludesChild,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "tcpproxy.example.com",
			},
			{Name: proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace}: {
				Object:      proxyTCPValidChild,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	run("valid HTTPProxy.TCPProxy", testcase{
		objs: []interface{}{proxyTCPValidIncludeChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPValidIncludeChild.Name,
				Namespace: proxyTCPValidIncludeChild.Namespace}: {
				Object:      proxyTCPValidIncludeChild,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "tcpproxy.example.com",
			},
			{Name: proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace}: {
				Object:      proxyTCPValidChild,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "tcpproxy.example.com"},
		},
	})

	// issue 2309
	proxyInvalidNoServices := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "missing-service.example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: nil, // missing
			}},
		},
	}

	run("invalid HTTPProxy due to empty route.service", testcase{
		objs: []interface{}{proxyInvalidNoServices, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidNoServices.Name, Namespace: proxyInvalidNoServices.Namespace}: {
				Object:      proxyInvalidNoServices,
				Status:      "invalid",
				Description: "route.services must have at least one entry",
				Vhost:       "missing-service.example.com",
			},
		},
	})

	fallbackCertificate := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("invalid fallback certificate passed to contour", testcase{
		fallbackCertificate: &types.NamespacedName{
			Name:      "invalid",
			Namespace: "invalid",
		},
		objs: []interface{}{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace}: {
				Object:      fallbackCertificate,
				Status:      "invalid",
				Description: "Spec.Virtualhost.TLS Secret \"invalid/invalid\" fallback certificate is invalid: Secret not found",
				Vhost:       "example.com",
			},
		},
	})

	run("fallback certificate requested but cert not configured in contour", testcase{
		objs: []interface{}{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace}: {
				Object:      fallbackCertificate,
				Status:      "invalid",
				Description: "Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file",
				Vhost:       "example.com",
			},
		},
	})

	fallbackCertificateWithClientValidation := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: "something",
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run("fallback certificate requested and clientValidation also configured", testcase{
		objs: []interface{}{fallbackCertificateWithClientValidation, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: fallbackCertificateWithClientValidation.Name,
				Namespace: fallbackCertificateWithClientValidation.Namespace}: {
				Object:      fallbackCertificateWithClientValidation,
				Status:      "invalid",
				Description: "Spec.Virtualhost.TLS fallback & client validation are incompatible",
				Vhost:       "example.com",
			},
		},
	})

	tlsPassthroughAndValidation := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: "aCAcert",
					},
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run("passthrough and client auth are incompatible tlsPassthroughAndValidation", testcase{
		objs: []interface{}{fixture.SecretRootsCert, tlsPassthroughAndValidation},
		want: map[types.NamespacedName]Status{
			{Name: tlsPassthroughAndValidation.Name, Namespace: tlsPassthroughAndValidation.Namespace}: {
				Object:      tlsPassthroughAndValidation,
				Status:      "invalid",
				Description: "Spec.VirtualHost.TLS passthrough cannot be combined with tls.clientValidation",
				Vhost:       tlsPassthroughAndValidation.Spec.VirtualHost.Fqdn,
			},
		},
	})

	tlsPassthroughAndSecretName := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
					SecretName:  fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run("tcpproxy with TLS passthrough and secret name both specified", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert,
			tlsPassthroughAndSecretName,
		},
		want: map[types.NamespacedName]Status{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: {
				Object:      tlsPassthroughAndSecretName,
				Status:      "invalid",
				Description: "Spec.VirtualHost.TLS: both Passthrough and SecretName were specified",
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	tlsNoPassthroughOrSecretName := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: false,
					SecretName:  "",
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run("httpproxy w/ tcpproxy with neither TLS passthrough nor secret name specified", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert,
			tlsNoPassthroughOrSecretName,
		},
		want: map[types.NamespacedName]Status{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: {
				Object:      tlsNoPassthroughOrSecretName,
				Status:      "invalid",
				Description: "Spec.VirtualHost.TLS: neither Passthrough nor SecretName were specified",
				Vhost:       "tcpproxy.example.com",
			},
		},
	})

	emptyProxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
		},
	}

	run("proxy with no routes, includes, or tcpproxy is invalid", testcase{
		objs: []interface{}{emptyProxy},
		want: map[types.NamespacedName]Status{
			{Name: emptyProxy.Name, Namespace: emptyProxy.Namespace}: {
				Object:      emptyProxy,
				Status:      "invalid",
				Description: "HTTPProxy.Spec must have at least one Route, Include, or a TCPProxy",
				Vhost:       emptyProxy.Spec.VirtualHost.Fqdn,
			},
		},
	})

	invalidRequestHeadersPolicyService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
							Set: []contour_api_v1.HeaderValue{{
								Name:  "Host",
								Value: "external.com",
							}},
						},
					},
				},
			}},
		},
	}

	run("requestHeadersPolicy, Host header invalid on Service", testcase{
		objs: []interface{}{invalidRequestHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: invalidRequestHeadersPolicyService.Name, Namespace: invalidRequestHeadersPolicyService.Namespace}: {
				Object:      invalidRequestHeadersPolicyService,
				Status:      "invalid",
				Description: "rewriting \"Host\" header is not supported on a service",
				Vhost:       invalidRequestHeadersPolicyService.Spec.VirtualHost.Fqdn,
			},
		},
	})

	invalidResponseHeadersPolicyService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
							Set: []contour_api_v1.HeaderValue{{
								Name:  "Host",
								Value: "external.com",
							}},
						},
					},
				},
			}},
		},
	}

	run("responseHeadersPolicy, Host header invalid on Service", testcase{
		objs: []interface{}{invalidResponseHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: invalidResponseHeadersPolicyService.Name, Namespace: invalidResponseHeadersPolicyService.Namespace}: {
				Object:      invalidResponseHeadersPolicyService,
				Status:      "invalid",
				Description: "rewriting \"Host\" header is not supported on response headers",
				Vhost:       invalidResponseHeadersPolicyService.Spec.VirtualHost.Fqdn,
			},
		},
	})

	invalidResponseHeadersPolicyRoute := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPRoute",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
					},
				},
				ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "external.com",
					}},
				},
			}},
		},
	}

	run("responseHeadersPolicy, Host header invalid on Route", testcase{
		objs: []interface{}{invalidResponseHeadersPolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: invalidResponseHeadersPolicyRoute.Name, Namespace: invalidResponseHeadersPolicyRoute.Namespace}: {
				Object:      invalidResponseHeadersPolicyRoute,
				Status:      "invalid",
				Description: "rewriting \"Host\" header is not supported on response headers",
				Vhost:       invalidResponseHeadersPolicyRoute.Spec.VirtualHost.Fqdn,
			},
		},
	})

	proxyAuthFallback := fixture.NewProxy("roots/fallback-incompat").
		WithSpec(contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "invalid.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
				Authorization: &contour_api_v1.AuthorizationServer{
					ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
						Namespace: "auth",
						Name:      "extension",
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	run("fallback and client auth is invalid", testcase{
		objs: []interface{}{fixture.SecretRootsCert, proxyAuthFallback},
		want: map[types.NamespacedName]Status{
			{Name: proxyAuthFallback.Name, Namespace: proxyAuthFallback.Namespace}: {
				Object:      proxyAuthFallback,
				Status:      "invalid",
				Description: "Spec.Virtualhost.TLS fallback & client authorization are incompatible",
				Vhost:       proxyAuthFallback.Spec.VirtualHost.Fqdn,
			},
		},
	})

	invalidResponseTimeout := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
						Response: "invalid-val",
					},
				},
			},
		},
	}

	run("proxy with invalid response timeout value is invalid", testcase{
		objs: []interface{}{invalidResponseTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{
				Name:      invalidResponseTimeout.Name,
				Namespace: invalidResponseTimeout.Namespace,
			}: {
				Object:      invalidResponseTimeout,
				Status:      "invalid",
				Description: "route.timeoutPolicy failed to parse: error parsing response timeout: unable to parse timeout string \"invalid-val\": time: invalid duration \"invalid-val\"",
				Vhost:       invalidResponseTimeout.Spec.VirtualHost.Fqdn,
			},
		},
	})

	invalidIdleTimeout := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
						Idle: "invalid-val",
					},
				},
			},
		},
	}

	run("proxy with invalid idle timeout value is invalid", testcase{
		objs: []interface{}{invalidIdleTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{
				Name:      invalidIdleTimeout.Name,
				Namespace: invalidIdleTimeout.Namespace,
			}: {
				Object:      invalidIdleTimeout,
				Status:      "invalid",
				Description: "route.timeoutPolicy failed to parse: error parsing idle timeout: unable to parse timeout string \"invalid-val\": time: invalid duration \"invalid-val\"",
				Vhost:       invalidIdleTimeout.Spec.VirtualHost.Fqdn,
			},
		},
	})

}
