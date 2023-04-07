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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/internal/status"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestDAGStatus(t *testing.T) {

	type testcase struct {
		objs                []interface{}
		fallbackCertificate *types.NamespacedName
		want                map[types.NamespacedName]contour_api_v1.DetailedCondition
	}

	run := func(t *testing.T, desc string, tc testcase) {
		t.Helper()
		t.Run(desc, func(t *testing.T) {
			t.Helper()
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{
						FallbackCertificate: tc.fallbackCertificate,
					},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
				},
			}
			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()
			t.Logf("%#v\n", dag.StatusCache)

			got := make(map[types.NamespacedName]contour_api_v1.DetailedCondition)
			for _, pu := range dag.StatusCache.GetProxyUpdates() {
				got[pu.Fullname] = *pu.Conditions[status.ValidCondition]
			}

			assert.Equal(t, tc.want, got)
		})
	}

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
	run(t, "root proxy does not specify FQDN", testcase{
		objs: []interface{}{proxyNoFQDN},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().WithGeneration(proxyNoFQDN.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
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

	run(t, "valid proxy", testcase{
		objs: []interface{}{proxyValidHomeService, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidHomeService.Name, Namespace: proxyValidHomeService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidHomeService.Generation).
				Valid(),
		},
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

	run(t, "proxy has multiple includes, one is invalid", testcase{
		objs: []interface{}{proxyMultiIncludeOneInvalid, proxyChildValidFoo2, proxyChildInvalidBadPort, fixture.ServiceRootsFoo2, fixture.ServiceRootsFoo3InvalidPort},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildValidFoo2.Generation).
				Valid(),
			{Name: proxyChildInvalidBadPort.Name, Namespace: proxyChildInvalidBadPort.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildInvalidBadPort.Generation).
				WithError(contour_api_v1.ConditionTypeServiceError, "ServicePortInvalid", `service "foo3": port must be in the range 1-65535`),
			{Name: proxyMultiIncludeOneInvalid.Name, Namespace: proxyMultiIncludeOneInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultiIncludeOneInvalid.Generation).
				Valid(),
		},
	})

	run(t, "multi-parent child is not orphaned when one of the parents is invalid", testcase{
		objs: []interface{}{proxyNoFQDN, proxyChildValidFoo2, proxyIncludeValidChild, fixture.ServiceRootsKuard, fixture.ServiceRootsFoo2},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyNoFQDN.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildValidFoo2.Generation).
				Valid(),
			{Name: proxyIncludeValidChild.Name, Namespace: proxyIncludeValidChild.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludeValidChild.Generation).
				Valid(),
		},
	})

	ingressSharedService := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: fixture.ServiceRootsNginx.Namespace,
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: fixture.SecretRootsCert.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "example.com",
				IngressRuleValue: ingressrulev1value(backendv1(fixture.ServiceRootsNginx.Name, intstr.FromInt(80))),
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
	run(t, "service shared across ingress and httpproxy tcpproxy", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert, fixture.ServiceRootsNginx, ingressSharedService, proxyTCPSharedService,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPSharedService.Name, Namespace: proxyTCPSharedService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyTCPSharedService.Generation).
				Valid(),
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
	run(t, "tcpproxy with tls delegation failure", testcase{
		objs: []interface{}{
			fixture.SecretProjectContourCert,
			proxyDelegatedTCPTLS,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyDelegatedTCPTLS.Name, Namespace: proxyDelegatedTCPTLS.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyDelegatedTCPTLS.Generation).
				WithError(contour_api_v1.ConditionTypeTLSError, "DelegationNotPermitted", `Spec.VirtualHost.TLS Secret "projectcontour/default-ssl-cert" certificate delegation not permitted`),
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
	run(t, "routes with tls delegation failure", testcase{
		objs: []interface{}{
			fixture.SecretProjectContourCert,
			proxyDelegatedTLS,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyDelegatedTLS.Name, Namespace: proxyDelegatedTLS.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyDelegatedTCPTLS.Generation).
				WithError(contour_api_v1.ConditionTypeTLSError, "DelegationNotPermitted", `Spec.VirtualHost.TLS Secret "projectcontour/default-ssl-cert" certificate delegation not permitted`),
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
	run(t, "non tls routes can be combined with tcp proxy", testcase{
		objs: []interface{}{
			serviceTLSPassthrough,
			proxyPassthroughProxyNonSecure,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyPassthroughProxyNonSecure.Name, Namespace: proxyPassthroughProxyNonSecure.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyPassthroughProxyNonSecure.Generation).
				Valid(),
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

	run(t, "two root httpproxies with different hostnames delegated to the same object are valid", testcase{
		objs: []interface{}{
			fixture.ServiceRootsKuard, proxyMultipleIncludersSite1, proxyMultipleIncludersSite2, proxyMultiIncludeChild,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyMultipleIncludersSite1.Name, Namespace: proxyMultipleIncludersSite1.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultipleIncludersSite1.Generation).
				Valid(),
			{Name: proxyMultipleIncludersSite2.Name, Namespace: proxyMultipleIncludersSite2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultipleIncludersSite2.Generation).
				Valid(),
			{Name: proxyMultiIncludeChild.Name, Namespace: proxyMultiIncludeChild.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultiIncludeChild.Generation).
				Valid(),
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

	run(t, "invalid port in service", testcase{
		objs: []interface{}{proxyInvalidNegativePortHomeService},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidNegativePortHomeService.Name, Namespace: proxyInvalidNegativePortHomeService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidNegativePortHomeService.Generation).
				WithError(contour_api_v1.ConditionTypeServiceError, "ServicePortInvalid", `service "home": port must be in the range 1-65535`),
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

	run(t, "root proxy outside of roots namespace", testcase{
		objs: []interface{}{proxyInvalidOutsideRootNamespace},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidOutsideRootNamespace.Name, Namespace: proxyInvalidOutsideRootNamespace.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidNegativePortHomeService.Generation).
				WithError(contour_api_v1.ConditionTypeRootNamespaceError, "RootProxyNotAllowedInNamespace", "root HTTPProxy cannot be defined in this namespace"),
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
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy self-edge produces a cycle", testcase{
		objs: []interface{}{proxyInvalidIncludeCycle, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidIncludeCycle.Name, Namespace: proxyInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidIncludeCycle.Generation).
				WithError(contour_api_v1.ConditionTypeIncludeError, "RootIncludesRoot", "root httpproxy cannot include another root httpproxy"),
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

	run(t, "proxy child delegates to itself, producing a cycle", testcase{
		objs: []interface{}{proxyIncludesProxyWithIncludeCycle, proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyIncludesProxyWithIncludeCycle.Name, Namespace: proxyIncludesProxyWithIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludesProxyWithIncludeCycle.Generation).Valid(),
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildInvalidIncludeCycle.Generation).
				WithError(contour_api_v1.ConditionTypeIncludeError, "IncludeCreatesCycle", "include creates an include cycle: roots/parent -> roots/child -> roots/child"),
		},
	})

	run(t, "proxy orphaned route", testcase{
		objs: []interface{}{proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildInvalidIncludeCycle.Generation).
				Orphaned(),
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

	run(t, "proxy invalid parent orphans child", testcase{
		objs: []interface{}{proxyNotRootIncludeRootProxy, proxyIncludedChildValid},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyNotRootIncludeRootProxy.Name, Namespace: proxyNotRootIncludeRootProxy.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyNotRootIncludeRootProxy.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
			{Name: proxyIncludedChildValid.Name, Namespace: proxyIncludedChildValid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildValid.Generation).
				Orphaned(),
		},
	})

	// singleNameFQDN is valid
	singleNameFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example",
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

	run(t, "proxy valid single FQDN", testcase{
		objs: []interface{}{singleNameFQDN, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: singleNameFQDN.Name, Namespace: singleNameFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(singleNameFQDN.Generation).
				Valid(),
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

	run(t, "proxy missing service is invalid", testcase{
		objs: []interface{}{proxyInvalidServiceInvalid},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidServiceInvalid.Name, Namespace: proxyInvalidServiceInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidServiceInvalid.Generation).
				WithError(contour_api_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "roots/invalid" not found`),
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

	run(t, "proxy with service missing port is invalid", testcase{
		objs: []interface{}{proxyInvalidServicePortInvalid, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidServicePortInvalid.Name, Namespace: proxyInvalidServicePortInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidServiceInvalid.Generation).
				WithError(contour_api_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: port "9999" on service "roots/home" not matched`),
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

	proxyValidReuseCaseExampleCom := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "case-example",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "EXAMPLE.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "conflicting proxies due to fqdn reuse", testcase{
		objs: []interface{}{proxyValidExampleCom, proxyValidReuseExampleCom},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidExampleCom.Name, Namespace: proxyValidExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidExampleCom.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`),
			{Name: proxyValidReuseExampleCom.Name, Namespace: proxyValidReuseExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidReuseExampleCom.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`),
		},
	})

	run(t, "conflicting proxies due to fqdn reuse with uppercase/lowercase", testcase{
		objs: []interface{}{proxyValidExampleCom, proxyValidReuseCaseExampleCom},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidExampleCom.Name, Namespace: proxyValidExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidExampleCom.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/case-example, roots/example-com`),
			{Name: proxyValidReuseCaseExampleCom.Name, Namespace: proxyValidReuseCaseExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidReuseCaseExampleCom.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/case-example, roots/example-com`),
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

	run(t, "root proxy including another root", testcase{
		objs: []interface{}{proxyRootIncludesRoot, proxyRootIncludedByRoot},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyRootIncludesRoot.Name, Namespace: proxyRootIncludesRoot.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludesRoot.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`),
			{Name: proxyRootIncludedByRoot.Name, Namespace: proxyRootIncludedByRoot.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludedByRoot.Generation).
				WithError(contour_api_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`),
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

	run(t, "root proxy including another root w/ different hostname", testcase{
		objs: []interface{}{proxyIncludesRootDifferentFQDN, proxyRootIncludedByRootDiffFQDN, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyIncludesRootDifferentFQDN.Name, Namespace: proxyIncludesRootDifferentFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludesRootDifferentFQDN.Generation).
				WithError(contour_api_v1.ConditionTypeIncludeError, "RootIncludesRoot", "root httpproxy cannot include another root httpproxy"),
			{Name: proxyRootIncludedByRootDiffFQDN.Name, Namespace: proxyRootIncludedByRootDiffFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludedByRootDiffFQDN.Generation).
				Valid(),
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

	run(t, "proxy includes another", testcase{
		objs: []interface{}{proxyValidIncludeBlogMarketing, proxyRootValidIncludesBlogMarketing, fixture.ServiceRootsKuard, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidIncludeBlogMarketing.Name, Namespace: proxyValidIncludeBlogMarketing.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidIncludeBlogMarketing.Generation).
				Valid(),
			{Name: proxyRootValidIncludesBlogMarketing.Name, Namespace: proxyRootValidIncludesBlogMarketing.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootValidIncludesBlogMarketing.Generation).
				Valid(),
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

	run(t, "proxy with mirror", testcase{
		objs: []interface{}{proxyValidWithMirror, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidWithMirror.Name, Namespace: proxyValidWithMirror.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidWithMirror.Generation).
				Valid(),
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

	run(t, "proxy with two mirrors", testcase{
		objs: []interface{}{proxyInvalidTwoMirrors, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidTwoMirrors.Name, Namespace: proxyInvalidTwoMirrors.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidTwoMirrors.Generation).
				WithError(contour_api_v1.ConditionTypeServiceError, "OnlyOneMirror", "only one service per route may be nominated as mirror"),
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

	run(t, "duplicate route condition headers", testcase{
		objs: []interface{}{proxyInvalidDuplicateMatchConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateMatchConditionHeaders.Name, Namespace: proxyInvalidDuplicateMatchConditionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateMatchConditionHeaders.Generation).
				WithError(contour_api_v1.ConditionTypeRouteError, "HeaderMatchConditionsNotValid", "cannot specify duplicate header 'exact match' conditions in the same route"),
		},
	})

	proxyInvalidDuplicateMatchConditionQueryParameters := &contour_api_v1.HTTPProxy{
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
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "abc",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param",
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

	run(t, "duplicate route condition query parameters", testcase{
		objs: []interface{}{proxyInvalidDuplicateMatchConditionQueryParameters, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateMatchConditionQueryParameters.Name, Namespace: proxyInvalidDuplicateMatchConditionQueryParameters.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateMatchConditionQueryParameters.Generation).
				WithError(contour_api_v1.ConditionTypeRouteError, "QueryParameterMatchConditionsNotValid", "cannot specify duplicate query parameter 'exact match' conditions in the same route"),
		},
	})

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

	run(t, "duplicate include condition headers", testcase{
		objs: []interface{}{proxyInvalidDuplicateIncludeCondtionHeaders, proxyValidDelegatedRoots, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateIncludeCondtionHeaders.Name,
				Namespace: proxyInvalidDuplicateIncludeCondtionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateIncludeCondtionHeaders.Generation).WithError(contour_api_v1.ConditionTypeRouteError, "HeaderMatchConditionsNotValid", "cannot specify duplicate header 'exact match' conditions in the same route"),
			{Name: proxyValidDelegatedRoots.Name,
				Namespace: proxyValidDelegatedRoots.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidDelegatedRoots.Generation).Orphaned(),
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

	run(t, "duplicate valid route condition headers", testcase{
		objs: []interface{}{proxyInvalidRouteConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidRouteConditionHeaders.Name, Namespace: proxyInvalidRouteConditionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidRouteConditionHeaders.Generation).Valid(),
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

	run(t, "proxy with two prefix conditions on route", testcase{
		objs: []interface{}{proxyInvalidMultiplePrefixes, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidMultiplePrefixes.Name, Namespace: proxyInvalidMultiplePrefixes.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidMultiplePrefixes.Generation).
				WithError(contour_api_v1.ConditionTypeRouteError, "PathMatchConditionsNotValid", "route: more than one prefix is not allowed in a condition block"),
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

	run(t, "proxy with two prefix conditions orphans include", testcase{
		objs: []interface{}{proxyInvalidTwoPrefixesWithInclude, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidTwoPrefixesWithInclude.Name, Namespace: proxyInvalidTwoPrefixesWithInclude.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidTwoPrefixesWithInclude.Generation).
				WithError(contour_api_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid", "include: more than one prefix is not allowed in a condition block"),
			{Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidChildTeamA.Generation).
				Orphaned(),
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

	run(t, "proxy with prefix conditions on route that does not start with slash", testcase{
		objs: []interface{}{proxyInvalidPrefixNoSlash, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidPrefixNoSlash.Name, Namespace: proxyInvalidPrefixNoSlash.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidPrefixNoSlash.Generation).
				WithError(contour_api_v1.ConditionTypeRouteError, "PathMatchConditionsNotValid", "route: prefix conditions must start with /, api was supplied"),
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

	run(t, "proxy with include prefix that does not start with slash", testcase{
		objs: []interface{}{proxyInvalidIncludePrefixNoSlash, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidIncludePrefixNoSlash.Name, Namespace: proxyInvalidIncludePrefixNoSlash.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid", "include: prefix conditions must start with /, api was supplied"),
			{Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: fixture.NewValidCondition().
				Orphaned(),
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

	run(t, "tcpproxy cannot specify services and include", testcase{
		objs: []interface{}{proxyInvalidTCPProxyIncludeAndService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidTCPProxyIncludeAndService.Name, Namespace: proxyInvalidTCPProxyIncludeAndService.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTCPProxyError, "NoServicesAndInclude", "cannot specify services and include in the same httpproxy"),
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

	run(t, "tcpproxy empty", testcase{
		objs: []interface{}{proxyTCPNoServiceOrInclusion, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPNoServiceOrInclusion.Name, Namespace: proxyTCPNoServiceOrInclusion.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTCPProxyError, "NothingDefined", "either services or inclusion must be specified"),
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

	run(t, "tcpproxy w/ missing include", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTCPProxyIncludeError, "IncludeNotFound", "include roots/foo not found"),
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

	run(t, "tcpproxy includes another root", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, proxyValidTCPRoot, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTCPProxyIncludeError, "RootIncludesRoot", "root httpproxy cannot include another root httpproxy"),
			{Name: proxyValidTCPRoot.Name, Namespace: proxyValidTCPRoot.Namespace}: fixture.NewValidCondition().Valid(),
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

	run(t, "tcpproxy includes valid child", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, proxyTCPValidChildFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}:     fixture.NewValidCondition().Valid(),
			{Name: proxyTCPValidChildFoo.Name, Namespace: proxyTCPValidChildFoo.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	proxyInvalidConflictingIncludeConditionsSimple := &contour_api_v1.HTTPProxy{
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
			Namespace: "teama",
			Name:      "blogteama",
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
			Namespace: "teamb",
			Name:      "blogteamb",
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

	run(t, "duplicate path conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictingIncludeConditionsSimple, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(), // Orphaned because the include pointing to this condition is a duplicate so the route is not programmed.
			{Name: proxyInvalidConflictingIncludeConditionsSimple.Name,
				Namespace: proxyInvalidConflictingIncludeConditionsSimple.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyIncludeConditionsEmpty := &contour_api_v1.HTTPProxy{
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
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
			}},
		},
	}

	run(t, "empty include conditions", testcase{
		objs: []interface{}{proxyIncludeConditionsEmpty, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyIncludeConditionsEmpty.Name,
				Namespace: proxyIncludeConditionsEmpty.Namespace}: fixture.NewValidCondition().
				Valid(),
		},
	})

	proxyIncludeConditionsPrefixRoot := &contour_api_v1.HTTPProxy{
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
					Prefix: "/",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	run(t, "multiple prefix / include conditions", testcase{
		objs: []interface{}{proxyIncludeConditionsPrefixRoot, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyIncludeConditionsPrefixRoot.Name,
				Namespace: proxyIncludeConditionsPrefixRoot.Namespace}: fixture.NewValidCondition().
				Valid(),
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
					Prefix: "/somethingelse",
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

	run(t, "duplicate path conditions on an include not consecutive", testcase{
		objs: []interface{}{proxyInvalidConflictingIncludeConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{Name: proxyInvalidConflictingIncludeConditions.Name,
				Namespace: proxyInvalidConflictingIncludeConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
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
						Name:     "x-other-header",
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

	run(t, "duplicate header conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include preceding an invalid one.
			{Name: proxyInvalidConflictHeaderConditions.Name,
				Namespace: proxyInvalidConflictHeaderConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyInvalidDuplicateMultiHeaderConditions := &contour_api_v1.HTTPProxy{
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
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-another-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-another-header",
						Contains: "abc",
					},
				}, {
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

	run(t, "duplicate header conditions on an include mismatched order", testcase{
		objs: []interface{}{proxyInvalidDuplicateMultiHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(),
			{Name: proxyInvalidDuplicateMultiHeaderConditions.Name,
				Namespace: proxyInvalidDuplicateMultiHeaderConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
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

	run(t, "duplicate header+path conditions on an include", testcase{
		objs: []interface{}{proxyInvalidDuplicateHeaderAndPathConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(),
			{Name: proxyInvalidDuplicateHeaderAndPathConditions.Name,
				Namespace: proxyInvalidDuplicateHeaderAndPathConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyInvalidConflictQueryConditions := &contour_api_v1.HTTPProxy{
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
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foo",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param-2",
						Exact: "bar",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:       "param-3",
						Exact:      "bar",
						IgnoreCase: true,
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foooo",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foo",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param-2",
						Exact: "bar",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param-2",
						Exact: "bar",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foo",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foooo",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:       "param-3",
						Exact:      "bar",
						IgnoreCase: true,
					},
				}},
			}},
		},
	}

	run(t, "duplicate query param conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictQueryConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidConflictQueryConditions.Name,
				Namespace: proxyInvalidConflictQueryConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include preceding an invalid one.
		},
	})

	proxyInvalidConflictQueryHeaderConditions := &contour_api_v1.HTTPProxy{
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
						Name:  "x-header",
						Exact: "foo",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "foo",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:   "param-other",
						Prefix: "bar",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header-other",
						Contains: "foo",
					},
				}},
			}},
		},
	}

	run(t, "duplicate query param+header conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictQueryHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidConflictQueryHeaderConditions.Name,
				Namespace: proxyInvalidConflictQueryHeaderConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),   // Valid since there is a valid include.
		},
	})

	proxyValidQueryHeaderConditions := &contour_api_v1.HTTPProxy{
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
						Name:  "x-header",
						Exact: "foo",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "foo",
					},
				}, {
					QueryParameter: &contour_api_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}},
			}},
		},
	}

	run(t, "query param+header conditions on an include should not be duplicate", testcase{
		objs: []interface{}{proxyValidQueryHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyValidQueryHeaderConditions.Name,
				Namespace: proxyValidQueryHeaderConditions.Namespace}: fixture.NewValidCondition().
				Valid(),
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

	run(t, "httpproxy w/ missing include", testcase{
		objs: []interface{}{proxyInvalidMissingInclude, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidMissingInclude.Name, Namespace: proxyInvalidMissingInclude.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "IncludeNotFound", "include roots/child not found"),
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

	run(t, "httpproxy w/ tcpproxy w/ missing service", testcase{
		objs: []interface{}{proxyTCPInvalidMissingService},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPInvalidMissingService.Name, Namespace: proxyTCPInvalidMissingService.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTCPProxyError, "ServiceUnresolvedReference", `Spec.TCPProxy unresolved service reference: service "roots/not-found" not found`),
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

	run(t, "httpproxy w/ tcpproxy w/ service missing port", testcase{
		objs: []interface{}{proxyTCPInvalidPortNotMatched, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPInvalidPortNotMatched.Name, Namespace: proxyTCPInvalidPortNotMatched.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTCPProxyError, "ServiceUnresolvedReference", `Spec.TCPProxy unresolved service reference: port "9999" on service "roots/kuard" not matched`),
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

	run(t, "httpproxy w/ tcpproxy missing tls", testcase{
		objs: []interface{}{proxyTCPInvalidMissingTLS},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPInvalidMissingTLS.Name, Namespace: proxyTCPInvalidMissingTLS.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTCPProxyError, "TLSMustBeConfigured", "Spec.TCPProxy requires that either Spec.TLS.Passthrough or Spec.TLS.SecretName be set"),
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

	run(t, "httpproxy w/ tcpproxy missing service", testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyInvalidMissingServiceWithTCPProxy},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidMissingServiceWithTCPProxy.Name, Namespace: proxyInvalidMissingServiceWithTCPProxy.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "roots/missing" not found`),
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

	run(t, "tcpproxy route unmatched service port", testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyRoutePortNotMatchedWithTCP},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyRoutePortNotMatchedWithTCP.Name, Namespace: proxyRoutePortNotMatchedWithTCP.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: port "9999" on service "roots/kuard" not matched`),
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

	run(t, "valid HTTPProxy.TCPProxy - plural", testcase{
		objs: []interface{}{proxyTCPValidIncludesChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPValidIncludesChild.Name,
				Namespace: proxyTCPValidIncludesChild.Namespace}: fixture.NewValidCondition().Valid(),
			{Name: proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	run(t, "valid HTTPProxy.TCPProxy", testcase{
		objs: []interface{}{proxyTCPValidIncludeChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPValidIncludeChild.Name,
				Namespace: proxyTCPValidIncludeChild.Namespace}: fixture.NewValidCondition().Valid(),
			{Name: proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace}: fixture.NewValidCondition().Valid(),
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

	run(t, "No routeAction specified is invalid", testcase{
		objs: []interface{}{proxyInvalidNoServices, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidNoServices.Name, Namespace: proxyInvalidNoServices.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeRouteError, "RouteActionCountNotValid", "must set exactly one of route.services or route.requestRedirectPolicy or route.directResponsePolicy"),
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

	run(t, "invalid fallback certificate passed to contour", testcase{
		fallbackCertificate: &types.NamespacedName{
			Name:      "invalid",
			Namespace: "invalid",
		},
		objs: []interface{}{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "FallbackNotValid", `Spec.Virtualhost.TLS Secret "invalid/invalid" fallback certificate is invalid: Secret not found`),
		},
	})

	run(t, "fallback certificate requested but cert not configured in contour", testcase{
		objs: []interface{}{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "FallbackNotPresent", "Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file"),
		},
	})

	fallbackCertificateWithClientValidationNoCA := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:       "ssl-cert",
					ClientValidation: &contour_api_v1.DownstreamValidation{},
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

	run(t, "clientValidation missing CA", testcase{
		objs: []interface{}{fallbackCertificateWithClientValidationNoCA, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: fallbackCertificateWithClientValidationNoCA.Name,
				Namespace: fallbackCertificateWithClientValidationNoCA.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "ClientValidationInvalid", "Spec.VirtualHost.TLS client validation is invalid: CA Secret must be specified"),
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

	run(t, "fallback certificate requested and clientValidation also configured", testcase{
		objs: []interface{}{fallbackCertificateWithClientValidation, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: fallbackCertificateWithClientValidation.Name,
				Namespace: fallbackCertificateWithClientValidation.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client validation are incompatible"),
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

	run(t, "passthrough and client auth are incompatible tlsPassthroughAndValidation", testcase{
		objs: []interface{}{fixture.SecretRootsCert, tlsPassthroughAndValidation},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: tlsPassthroughAndValidation.Name, Namespace: tlsPassthroughAndValidation.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.VirtualHost.TLS passthrough cannot be combined with tls.clientValidation"),
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

	run(t, "tcpproxy with TLS passthrough and secret name both specified", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert,
			tlsPassthroughAndSecretName,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "TLSConfigNotValid", "Spec.VirtualHost.TLS: both Passthrough and SecretName were specified"),
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

	run(t, "httpproxy w/ tcpproxy with neither TLS passthrough nor secret name specified", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert,
			tlsNoPassthroughOrSecretName,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "TLSConfigNotValid", "Spec.VirtualHost.TLS: neither Passthrough nor SecretName were specified"),
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

	run(t, "proxy with no routes, includes, or tcpproxy is invalid", testcase{
		objs: []interface{}{emptyProxy},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: emptyProxy.Name, Namespace: emptyProxy.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeSpecError, "NothingDefined", "HTTPProxy.Spec must have at least one Route, Include, or a TCPProxy"),
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

	run(t, "responseHeadersPolicy, Host header invalid on Service", testcase{
		objs: []interface{}{invalidResponseHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: invalidResponseHeadersPolicyService.Name, Namespace: invalidResponseHeadersPolicyService.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeServiceError, "ResponseHeadersPolicyInvalid", `rewriting "Host" header is not supported on response headers`),
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

	run(t, "responseHeadersPolicy, Host header invalid on Route", testcase{
		objs: []interface{}{invalidResponseHeadersPolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: invalidResponseHeadersPolicyRoute.Name, Namespace: invalidResponseHeadersPolicyRoute.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeRouteError, "ResponseHeaderPolicyInvalid", `rewriting "Host" header is not supported on response headers`),
		},
	})

	duplicateCookieRewritePolicyRoute := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidCRPRoute",
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
				CookieRewritePolicies: []contour_api_v1.CookieRewritePolicy{
					{
						Name:   "a-cookie",
						Secure: ref.To(true),
					},
					{
						Name:     "a-cookie",
						SameSite: ref.To("Lax"),
					},
				},
			}},
		},
	}

	run(t, "cookieRewritePolicies, duplicate cookie names on route", testcase{
		objs: []interface{}{duplicateCookieRewritePolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: duplicateCookieRewritePolicyRoute.Name, Namespace: duplicateCookieRewritePolicyRoute.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `duplicate cookie rewrite rule for cookie "a-cookie" on route cookie rewrite rules`),
		},
	})

	duplicateCookieRewritePolicyService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidCRPService",
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
						CookieRewritePolicies: []contour_api_v1.CookieRewritePolicy{
							{
								Name:   "a-cookie",
								Secure: ref.To(true),
							},
							{
								Name:     "a-cookie",
								SameSite: ref.To("Lax"),
							},
						},
					},
				},
			}},
		},
	}

	run(t, "cookieRewritePolicies, duplicate cookie names on service", testcase{
		objs: []interface{}{duplicateCookieRewritePolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: duplicateCookieRewritePolicyService.Name, Namespace: duplicateCookieRewritePolicyService.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `duplicate cookie rewrite rule for cookie "a-cookie" on service cookie rewrite rules`),
		},
	})

	emptyCookieRewritePolicyRoute := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidCRPRoute",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				CookieRewritePolicies: []contour_api_v1.CookieRewritePolicy{
					{
						Name: "a-cookie",
					},
				},
				Services: []contour_api_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
					},
				},
			}},
		},
	}

	run(t, "cookieRewritePolicies, empty cookie rewrite on route", testcase{
		objs: []interface{}{emptyCookieRewritePolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: emptyCookieRewritePolicyRoute.Name, Namespace: emptyCookieRewritePolicyRoute.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `no attributes rewritten for cookie "a-cookie" on route cookie rewrite rules`),
		},
	})

	emptyCookieRewritePolicyService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidCRPService",
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
						CookieRewritePolicies: []contour_api_v1.CookieRewritePolicy{
							{
								Name: "a-cookie",
							},
						},
					},
				},
			}},
		},
	}

	run(t, "cookieRewritePolicies, empty cookie rewrite on service", testcase{
		objs: []interface{}{emptyCookieRewritePolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: emptyCookieRewritePolicyService.Name, Namespace: emptyCookieRewritePolicyService.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `no attributes rewritten for cookie "a-cookie" on service cookie rewrite rules`),
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

	run(t, "fallback and client auth is invalid", testcase{
		objs: []interface{}{fixture.SecretRootsCert, proxyAuthFallback},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyAuthFallback.Name, Namespace: proxyAuthFallback.Namespace}: fixture.NewValidCondition().WithGeneration(proxyAuthFallback.Generation).
				WithError(contour_api_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client authorization are incompatible"),
		},
	})

	proxyAuthHTTP := fixture.NewProxy("roots/http").
		WithSpec(contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "invalid.com",
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

	run(t, "plain HTTP vhost and client auth is invalid", testcase{
		objs: []interface{}{fixture.SecretRootsCert, proxyAuthHTTP},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyAuthHTTP): fixture.NewValidCondition().WithGeneration(proxyAuthHTTP.Generation).
				WithError(contour_api_v1.ConditionTypeAuthError, "AuthNotPermitted", "Spec.VirtualHost.Authorization.ExtensionServiceRef can only be defined for root HTTPProxies that terminate TLS"),
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

	run(t, "proxy with invalid response timeout value is invalid", testcase{
		objs: []interface{}{invalidResponseTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{
				Name:      invalidResponseTimeout.Name,
				Namespace: invalidResponseTimeout.Namespace,
			}: fixture.NewValidCondition().WithError(contour_api_v1.ConditionTypeRouteError, "TimeoutPolicyNotValid",
				`route.timeoutPolicy failed to parse: error parsing response timeout: unable to parse timeout string "invalid-val": time: invalid duration "invalid-val"`),
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

	run(t, "proxy with invalid idle timeout value is invalid", testcase{
		objs: []interface{}{invalidIdleTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{
				Name:      invalidIdleTimeout.Name,
				Namespace: invalidIdleTimeout.Namespace,
			}: fixture.NewValidCondition().WithError(contour_api_v1.ConditionTypeRouteError, "TimeoutPolicyNotValid",
				`route.timeoutPolicy failed to parse: error parsing idle timeout: unable to parse timeout string "invalid-val": time: invalid duration "invalid-val"`),
		},
	})

	// issue 3197: Fallback and passthrough HTTPProxy directive should emit a config error
	tlsPassthroughAndFallback := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				TLS: &contour_api_v1.TLS{
					Passthrough:               true,
					EnableFallbackCertificate: true,
				},
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

	run(t, "TLS with passthrough and fallback cert enabled is invalid", testcase{
		objs: []interface{}{tlsPassthroughAndFallback, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: tlsPassthroughAndFallback.Name, Namespace: tlsPassthroughAndFallback.Namespace}: fixture.NewValidCondition().
				WithGeneration(tlsPassthroughAndFallback.Generation).WithError(
				contour_api_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures",
				`Spec.VirtualHost.TLS: both Passthrough and enableFallbackCertificate were specified`,
			),
		},
	})
	tlsPassthrough := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				TLS: &contour_api_v1.TLS{
					Passthrough:               true,
					EnableFallbackCertificate: false,
				},
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

	run(t, "valid TLS passthrough", testcase{
		objs: []interface{}{tlsPassthrough, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: tlsPassthrough.Name, Namespace: tlsPassthrough.Namespace}: fixture.NewValidCondition().
				WithGeneration(tlsPassthrough.Generation).
				Valid(),
		},
	})

	multipleRouteAction := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "multipleRouteAction",
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
				DirectResponsePolicy: &contour_api_v1.HTTPDirectResponsePolicy{
					StatusCode: 200,
					Body:       "success",
				},
			}},
		},
	}
	run(t, "Selecting more than one routeAction is invalid", testcase{
		objs: []interface{}{multipleRouteAction},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: multipleRouteAction.Name, Namespace: multipleRouteAction.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeRouteError, "RouteActionCountNotValid",
					"must set exactly one of route.services or route.requestRedirectPolicy or route.directResponsePolicy"),
		},
	})

	invalidAllowOrigin := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-alloworigin",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				CORSPolicy: &contour_api_v1.CORSPolicy{
					AllowOrigin:  []string{"example-2.com", "**"},
					AllowMethods: []contour_api_v1.CORSHeaderValue{"GET"},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
							Port: 8080,
						},
					},
				},
			},
		},
	}

	run(t, "proxy with invalid allow origin is invalid", testcase{
		objs: []interface{}{invalidAllowOrigin, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{
				Name:      invalidAllowOrigin.Name,
				Namespace: invalidAllowOrigin.Namespace,
			}: fixture.NewValidCondition().WithError(contour_api_v1.ConditionTypeCORSError, "PolicyDidNotParse",
				`Spec.VirtualHost.CORSPolicy: invalid allowed origin "**": allowed origin is invalid exact match and invalid regex match`),
		},
	})

	jwtVerificationValidProxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-valid-proxy",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name:      "provider-1",
						Issuer:    "jwt.example.com",
						Audiences: []string{"foo", "bar"},
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "10s",
							CacheDuration: "1h",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification valid proxy", testcase{
		objs: []interface{}{
			jwtVerificationValidProxy,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationValidProxy): fixture.NewValidCondition().Valid(),
		},
	})

	jwtVerificationDuplicateProviders := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-duplicate-provider-names",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification duplicate provider names", testcase{
		objs: []interface{}{
			jwtVerificationDuplicateProviders,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationDuplicateProviders): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"DuplicateProviderName",
					"Spec.VirtualHost.JWTProviders is invalid: duplicate name provider-1",
				),
		},
	})

	jwtVerificationMultipleDefaults := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-multiple-defaults",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name:    "provider-1",
						Default: true,
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
					{
						Name:    "provider-2",
						Default: true,
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification multiple default providers", testcase{
		objs: []interface{}{
			jwtVerificationMultipleDefaults,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationMultipleDefaults): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"MultipleDefaultProvidersSpecified",
					"Spec.VirtualHost.JWTProviders is invalid: at most one provider can be set as the default",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSURI := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-uri",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: ":/invalid-uri",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS URI", testcase{
		objs: []interface{}{
			jwtVerificationInvalidRemoteJWKSURI,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSURI): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSURIInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.URI is invalid: parse \":/invalid-uri\": missing protocol scheme",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSScheme := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-scheme",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "ftp://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS scheme", testcase{
		objs: []interface{}{
			jwtVerificationInvalidRemoteJWKSScheme,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSScheme): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSSchemeInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.URI has invalid scheme \"ftp\", must be http or https",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSTimeout := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-timeout",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI:     "http://jwt.example.com/jwks.json",
							Timeout: "invalid-timeout-string",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS timeout", testcase{
		objs: []interface{}{
			jwtVerificationInvalidRemoteJWKSTimeout,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSTimeout): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSTimeoutInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.Timeout is invalid: time: invalid duration \"invalid-timeout-string\"",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSCacheDuration := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-cache-duration",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI:           "http://jwt.example.com/jwks.json",
							CacheDuration: "invalid-duration-string",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS cache duration", testcase{
		objs: []interface{}{
			jwtVerificationInvalidRemoteJWKSCacheDuration,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSCacheDuration): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSCacheDurationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.CacheDuration is invalid: time: invalid duration \"invalid-duration-string\"",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSDNSLookupFamily := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-dns-lookup-family",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI:             "http://jwt.example.com/jwks.json",
							DNSLookupFamily: "v7",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS DNS lookup family", testcase{
		objs: []interface{}{
			jwtVerificationInvalidRemoteJWKSDNSLookupFamily,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSDNSLookupFamily): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSDNSLookupFamilyInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.DNSLookupFamily has an invalid value \"v7\", must be auto, all, v4 or v6",
				),
		},
	})

	jwtVerificationNoProvidersRouteHasRef := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-no-providers-route-has-ref",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{Require: "provider-1"},
				},
			},
		},
	}

	run(t, "JWT verification no providers defined, route has provider ref", testcase{
		objs: []interface{}{
			jwtVerificationNoProvidersRouteHasRef,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationNoProvidersRouteHasRef): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"JWTProviderNotDefined",
					"Route references an undefined JWT provider \"provider-1\"",
				),
		},
	})

	jwtVerificationRouteReferencesNonexistentProvider := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-route-references-nonexistent-provider",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "http://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{Require: "nonexistent-provider"},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification route references nonexistent provider", testcase{
		objs: []interface{}{
			jwtVerificationRouteReferencesNonexistentProvider,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationRouteReferencesNonexistentProvider): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"JWTProviderNotDefined",
					"Route references an undefined JWT provider \"nonexistent-provider\"",
				),
		},
	})

	jwtVerificationInvalidTLSNotConfigured := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-tls-not-configured",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid TLS not configured", testcase{
		objs: []interface{}{
			jwtVerificationInvalidTLSNotConfigured,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidTLSNotConfigured): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"JWTVerificationNotPermitted",
					"Spec.VirtualHost.JWTProviders can only be defined for root HTTPProxies that terminate TLS",
				),
		},
	})

	jwtVerificationInvalidTLSPassthroughConfigured := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-tls-passthrough-configured",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid TLS passthrough configured", testcase{
		objs: []interface{}{
			jwtVerificationInvalidTLSPassthroughConfigured,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidTLSPassthroughConfigured): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"JWTVerificationNotPermitted",
					"Spec.VirtualHost.JWTProviders can only be defined for root HTTPProxies that terminate TLS",
				),
		},
	})

	jwtVerificationInvalidTLSFallbackConfigured := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-tls-fallback-configured",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					EnableFallbackCertificate: true,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid TLS fallback configured", testcase{
		objs: []interface{}{
			jwtVerificationInvalidTLSFallbackConfigured,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidTLSFallbackConfigured): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"JWTVerificationNotPermitted",
					"Spec.VirtualHost.JWTProviders can only be defined for root HTTPProxies that terminate TLS",
				),
		},
	})

	jwtVerificationInvalidRequireAndDisabledSpecified := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-require-and-disabled-specified",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{
						Require:  "provider-1",
						Disabled: true,
					},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid route specifies both requires and disabled", testcase{
		objs: []interface{}{
			jwtVerificationInvalidRequireAndDisabledSpecified,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRequireAndDisabledSpecified): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"InvalidJWTVerificationPolicy",
					"route's JWT verification policy cannot specify both require and disabled",
				),
		},
	})

	jwtVerificationUpstreamValidationForHTTPJWKS := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-for-http-jwks",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "http://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_api_v1.UpstreamValidation{
								CACertificate: "foo",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation specified for HTTP JWKS", testcase{
		objs: []interface{}{
			jwtVerificationUpstreamValidationForHTTPJWKS,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationForHTTPJWKS): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSUpstreamValidationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation must not be specified when URI scheme is http.",
				),
		},
	})

	jwtVerificationUpstreamValidationCACertDoesNotExist := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-cacert-does-not-exist",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_api_v1.UpstreamValidation{
								CACertificate: "nonexistent",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation CA cert does not exist", testcase{
		objs: []interface{}{
			jwtVerificationUpstreamValidationCACertDoesNotExist,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationCACertDoesNotExist): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSUpstreamValidationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation is invalid: invalid CA Secret \"roots/nonexistent\": Secret not found",
				),
		},
	})

	jwksInvalidCACert := &v1.Secret{
		ObjectMeta: fixture.ObjectMeta("roots/cacert"),
		Type:       v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"wrong-key": []byte(fixture.CERTIFICATE),
		},
	}

	jwtVerificationUpstreamValidationCACertInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-cacert-invalid",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_api_v1.UpstreamValidation{
								CACertificate: "cacert",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation CA cert invalid", testcase{
		objs: []interface{}{
			jwtVerificationUpstreamValidationCACertInvalid,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
			jwksInvalidCACert,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationCACertInvalid): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSUpstreamValidationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation is invalid: invalid CA Secret \"roots/cacert\": empty \"ca.crt\" key",
				),
		},
	})

	jwksCACertDifferentNamespace := &v1.Secret{
		ObjectMeta: fixture.ObjectMeta("default/cacert"),
		Data: map[string][]byte{
			"ca.crt": []byte(fixture.CERTIFICATE),
		},
	}

	jwtVerificationUpstreamValidationCACertNotDelegated := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-cacert-not-delegated",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_api_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_api_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_api_v1.UpstreamValidation{
								CACertificate: "default/cacert",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_api_v1.Route{
				{
					JWTVerificationPolicy: &contour_api_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_api_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation CA cert not delegated", testcase{
		objs: []interface{}{
			jwtVerificationUpstreamValidationCACertNotDelegated,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
			jwksCACertDifferentNamespace,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationCACertNotDelegated): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSCACertificateNotDelegated",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation.CACertificate Secret \"default/cacert\" is not configured for certificate delegation",
				),
		},
	})

	// proxyWithInvalidSlowStartWindow is invalid because it has invalid window size syntax.
	proxyWithInvalidSlowStartWindow := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "slow-start-invalid-window",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
					SlowStartPolicy: &contour_api_v1.SlowStartPolicy{
						Window: "invalid",
					},
				}},
			}},
		},
	}

	// proxyWithInvalidSlowStartAggression is invalid because it has invalid aggression syntax.
	proxyWithInvalidSlowStartAggression := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "slow-start-invalid-aggression",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
					SlowStartPolicy: &contour_api_v1.SlowStartPolicy{
						Window:     "5s",
						Aggression: "invalid",
					},
				}},
			}},
		},
	}

	// proxyWithInvalidSlowStartLBStrategy is invalid because route has LB strategy that does not support slow start.
	proxyWithInvalidSlowStartLBStrategy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "slow-start-invalid-lb-strategy",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_api_v1.Route{{
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: LoadBalancerPolicyCookie,
				},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
					SlowStartPolicy: &contour_api_v1.SlowStartPolicy{
						Window: "5s",
					},
				}},
			}},
		},
	}

	run(t, "Slow start with invalid window syntax", testcase{
		objs: []interface{}{
			proxyWithInvalidSlowStartWindow,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyWithInvalidSlowStartWindow): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeServiceError,
					"SlowStartInvalid",
					"error parsing window: time: invalid duration \"invalid\" on slow start",
				),
		},
	})

	run(t, "Slow start with invalid aggression syntax", testcase{
		objs: []interface{}{
			proxyWithInvalidSlowStartAggression,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyWithInvalidSlowStartAggression): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeServiceError,
					"SlowStartInvalid",
					"error parsing aggression: \"invalid\" is not a decimal number on slow start",
				),
		},
	})

	run(t, "Slow start with load balancer strategy that does not support slow start", testcase{
		objs: []interface{}{
			proxyWithInvalidSlowStartLBStrategy,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyWithInvalidSlowStartLBStrategy): fixture.NewValidCondition().
				WithError(
					contour_api_v1.ConditionTypeServiceError,
					"SlowStartInvalid",
					"slow start is only supported with RoundRobin or WeightedLeastRequest load balancer strategy",
				),
		},
	})
}

func validGatewayStatusUpdate(listenerName string, kind gatewayapi_v1beta1.Kind, attachedRoutes int) []*status.GatewayStatusUpdate {
	// This applies to tests that the listener doesn't have allowed kind configured
	// hence the wanted allowed kind is determined by the listener protocol only.
	var supportedKinds []gatewayapi_v1beta1.RouteGroupKind

	if kind == "HTTPRoute" || kind == "GRPCRoute" {
		supportedKinds = append(supportedKinds, gatewayapi_v1beta1.RouteGroupKind{
			Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
			Kind:  "HTTPRoute",
		})
		supportedKinds = append(supportedKinds, gatewayapi_v1beta1.RouteGroupKind{
			Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
			Kind:  "GRPCRoute",
		})
	} else {
		supportedKinds = append(supportedKinds, gatewayapi_v1beta1.RouteGroupKind{
			Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
			Kind:  kind,
		})
	}

	return []*status.GatewayStatusUpdate{
		{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionTrue,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonProgrammed),
					Message: status.MessageValidGateway,
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				listenerName: {
					Name:           gatewayapi_v1beta1.SectionName(listenerName),
					AttachedRoutes: int32(attachedRoutes),
					SupportedKinds: supportedKinds,
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
							Message: "Valid listener",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		},
	}
}

func TestGatewayAPIHTTPRouteDAGStatus(t *testing.T) {
	type testcase struct {
		objs                    []interface{}
		gateway                 *gatewayapi_v1beta1.Gateway
		wantRouteConditions     []*status.RouteStatusUpdate
		wantGatewayStatusUpdate []*status.GatewayStatusUpdate
	}

	run := func(t *testing.T, desc string, tc testcase) {
		t.Helper()
		t.Run(desc, func(t *testing.T) {
			t.Helper()
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    fixture.NewTestLogger(t),
					gatewayclass: &gatewayapi_v1beta1.GatewayClass{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1beta1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1beta1.GatewayClassStatus{
							Conditions: []metav1.Condition{
								{
									Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
					gateway: tc.gateway,
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
				},
			}

			// Set a default gateway if not defined by a test
			if tc.gateway == nil {
				builder.Source.gateway = &gatewayapi_v1beta1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1beta1.GatewaySpec{
						Listeners: []gatewayapi_v1beta1.Listener{{
							Name:     "http",
							Port:     80,
							Protocol: gatewayapi_v1beta1.HTTPProtocolType,
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
								},
							},
						}},
					},
				}
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()
			gotRouteUpdates := dag.StatusCache.GetRouteUpdates()
			gotGatewayUpdates := dag.StatusCache.GetGatewayUpdates()

			ops := []cmp.Option{
				cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "TransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Resource"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "ExistingConditions"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j metav1.Condition) bool {
					return i.Message < j.Message
				}),
				cmpopts.SortSlices(func(i, j *status.RouteStatusUpdate) bool {
					return i.FullName.String() < j.FullName.String()
				}),
			}

			// Since we're using a single static GatewayClass,
			// set the expected controller string here for all
			// test cases.
			for _, u := range tc.wantRouteConditions {
				u.GatewayController = builder.Source.gatewayclass.Spec.ControllerName

				for _, rps := range u.RouteParentStatuses {
					rps.ControllerName = builder.Source.gatewayclass.Spec.ControllerName
				}
			}

			if diff := cmp.Diff(tc.wantRouteConditions, gotRouteUpdates, ops...); diff != "" {
				t.Fatalf("expected route status: %v, got %v", tc.wantRouteConditions, diff)
			}

			if diff := cmp.Diff(tc.wantGatewayStatusUpdate, gotGatewayUpdates, ops...); diff != "" {
				t.Fatalf("expected gateway status: %v, got %v", tc.wantGatewayStatusUpdate, diff)
			}
		})
	}

	kuardService := &v1.Service{
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

	kuardService2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard2",
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

	kuardService3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard3",
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

	run(t, "simple httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "simple httproute with backendref namespace matching route's explicitly specified", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi_v1beta1.BackendObjectReference{
										Kind:      ref.To(gatewayapi_v1beta1.Kind("Service")),
										Namespace: ref.To(gatewayapi_v1beta1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1beta1.ObjectName(kuardService.Name),
										Port:      ref.To(gatewayapi_v1beta1.PortNumber(8080)),
									},
									Weight: ref.To(int32(1)),
								},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{{
				ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
				Conditions: []metav1.Condition{
					routeResolvedRefsCondition(),
					routeAcceptedHTTPRouteCondition(),
				},
			}},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "multiple httproutes", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-2",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
				RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []metav1.Condition{
							routeResolvedRefsCondition(),
							routeAcceptedHTTPRouteCondition(),
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-2"},
				RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []metav1.Condition{
							routeResolvedRefsCondition(),
							routeAcceptedHTTPRouteCondition(),
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 2),
	})

	run(t, "prefix path match not starting with '/' for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("doesnt-start-with-slash"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeAcceptedHTTPRouteCondition(),
						routeResolvedRefsCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must start with '/'.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})
	run(t, "exact path match not starting with '/' for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchExact),
								Value: ref.To("doesnt-start-with-slash"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must start with '/'.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "prefix path match with consecutive '/' characters for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("/foo///bar"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must not contain consecutive '/' characters.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "exact path match with consecutive '/' characters for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchExact),
								Value: ref.To("//foo/bar"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must not contain consecutive '/' characters.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "invalid path match type for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchType("UNKNOWN")), // <---- unknown type to break the test
								Value: ref.To("/"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "regular expression match not yet supported for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchRegularExpression, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type and Exact match type are supported.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "RegularExpression header match not yet supported for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("/"),
							},
							Headers: []gatewayapi_v1beta1.HTTPHeaderMatch{
								{
									Type:  ref.To(gatewayapi_v1beta1.HeaderMatchRegularExpression), // <---- RegularExpression type not yet supported
									Name:  gatewayapi_v1beta1.HTTPHeaderName("foo"),
									Value: "bar",
								},
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "HTTPRoute.Spec.Rules.Matches.Headers: Only Exact match type is supported",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "RegularExpression query param match not supported for httproute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("/"),
							},
							QueryParams: []gatewayapi_v1beta1.HTTPQueryParamMatch{
								{
									Type:  ref.To(gatewayapi_v1beta1.QueryParamMatchRegularExpression), // <---- RegularExpression type not yet supported
									Name:  "param-1",
									Value: "value-1",
								},
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "HTTPRoute.Spec.Rules.Matches.QueryParams: Only Exact match type is supported",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "spec.rules.backendRef.name not specified", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi_v1beta1.BackendObjectReference{
										Kind: ref.To(gatewayapi_v1beta1.Kind("Service")),
										Port: ref.To(gatewayapi_v1beta1.PortNumber(8080)),
									},
								},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "Spec.Rules.BackendRef.Name must be specified",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "spec.rules.backendRef.serviceName invalid on two matches", testcase{
		objs: []interface{}{
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("/"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("invalid-one", 8080, 1),
					}, {
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("/blog"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("invalid-two", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonBackendNotFound),
							Message: "service \"invalid-one\" is invalid: service \"default/invalid-one\" not found, service \"invalid-two\" is invalid: service \"default/invalid-two\" not found",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "spec.rules.backendRef.port not specified", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi_v1beta1.BackendObjectReference{
										Kind: ref.To(gatewayapi_v1beta1.Kind("Service")),
										Name: "kuard",
									},
								},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "Spec.Rules.BackendRef.Port must be specified",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "spec.rules.backendRefs not specified", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "At least one Spec.Rules.BackendRef must be specified.",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "spec.rules.backendRef.namespace does not match route", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi_v1beta1.BackendObjectReference{
										Kind:      ref.To(gatewayapi_v1beta1.Kind("Service")),
										Namespace: ref.To(gatewayapi_v1beta1.Namespace("some-other-namespace")),
										Name:      "service",
										Port:      ref.To(gatewayapi_v1beta1.PortNumber(8080)),
									},
								},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.Rules.BackendRef.Namespace must match the route's namespace or be covered by a ReferenceGrant",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	// BEGIN TLS CertificateRef + ReferenceGrant tests
	run(t, "Gateway references TLS cert in different namespace, with valid ReferenceGrant", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1beta1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1beta1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("https", "HTTPRoute", 0),
	})

	run(t, "Gateway references TLS cert in different namespace, with no ReferenceGrant", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with valid ReferenceGrant (secret-specific)", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1beta1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1beta1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
						Name: ref.To(gatewayapi_v1beta1.ObjectName("secret")),
					}},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("https", "HTTPRoute", 0),
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (policy in wrong namespace)", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "wrong-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1beta1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1beta1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong From namespace)", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1beta1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1beta1.Namespace("wrong-namespace"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong From kind)", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1beta1.GroupName,
						Kind:      "WrongKind",
						Namespace: gatewayapi_v1beta1.Namespace("projectontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong To kind)", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1beta1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1beta1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "WrongKind",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong secret name)", testcase{
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				GatewayClassName: gatewayapi_v1beta1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []interface{}{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1beta1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1beta1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
						Name: ref.To(gatewayapi_v1beta1.ObjectName("wrong-name")),
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	// END TLS CertificateRef + ReferenceGrant tests

	run(t, "spec.rules.hostname: invalid wildcard", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"*.*.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "spec.rules.hostname: invalid hostname", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"#projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "spec.rules.hostname: invalid hostname, ip address", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"1.2.3.4",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid hostname \"1.2.3.4\": must be a DNS name, not an IP address",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 0),
	})

	run(t, "two HTTP listeners, route's hostname intersects with one of them", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("*.projectcontour.io")),
					},
					{
						Name:     "listener-2",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("specific.hostname.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
					gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1beta1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1beta1.SectionName("listener-1"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
								Message: "Valid listener",
							},
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
								Message: "Listener accepted",
							},
						},
					},
					"listener-2": {
						Name:           gatewayapi_v1beta1.SectionName("listener-2"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
								Message: "Valid listener",
							},
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
								Message: "Listener accepted",
							},
						},
					},
				},
			},
		},
	})

	run(t, "two HTTP listeners, route's hostname intersects with neither of them", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.randomdomain.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("*.projectcontour.io")),
					},
					{
						Name:     "listener-2",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("specific.hostname.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
					gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1beta1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1beta1.SectionName("listener-1"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
								Message: "Valid listener",
							},
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
								Message: "Listener accepted",
							},
						},
					},
					"listener-2": {
						Name:           gatewayapi_v1beta1.SectionName("listener-2"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
								Message: "Valid listener",
							},
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
								Message: "Listener accepted",
							},
						},
					},
				},
			},
		},
	})

	run(t, "HTTP listener, route's parent ref sectionname does not match", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 0)},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 0),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
					gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1beta1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1beta1.SectionName("listener-1"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
								Message: "Valid listener",
							},
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
								Message: "Listener accepted",
							},
						},
					},
				},
			},
		},
	})

	run(t, "HTTP listener, route's parent ref port does not match", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "", 443)},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "", 443),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
					gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1beta1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1beta1.SectionName("listener-1"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
								Message: "Valid listener",
							},
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
								Message: "Listener accepted",
							},
						},
					},
				},
			},
		},
	})

	run(t, "HTTP listener, route's parent ref section name and port both must match", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 80)},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-2",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 443)},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-3",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 80)},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
				RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 80),
						Conditions: []metav1.Condition{
							routeResolvedRefsCondition(),
							{
								Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
								Status:  contour_api_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingParent),
								Message: "No listeners match this parent ref",
							},
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-2"},
				RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 443),
						Conditions: []metav1.Condition{
							routeResolvedRefsCondition(),
							{
								Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
								Status:  contour_api_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingParent),
								Message: "No listeners match this parent ref",
							},
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-3"},
				RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 80),
						Conditions: []metav1.Condition{
							routeResolvedRefsCondition(),
							routeAcceptedHTTPRouteCondition(),
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
					gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1beta1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1beta1.SectionName("listener-1"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
								Message: "Valid listener",
							},
							{
								Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
								Status:  metav1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
								Message: "Listener accepted",
							},
						},
					},
				},
			},
		},
	})

	run(t, "HTTPRoute: backendrefs still validated when route not accepted", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 81)},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("invalid", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: ref.To(gatewayapi_v1beta1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
				RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 81),
						Conditions: []metav1.Condition{
							{
								Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
								Status:  contour_api_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1beta1.RouteReasonBackendNotFound),
								Message: "service \"invalid\" is invalid: service \"default/invalid\" not found",
							},
							{
								Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
								Status:  contour_api_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingParent),
								Message: "No listeners match this parent ref",
							},
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("listener-1", "HTTPRoute", 0),
	})

	run(t, "More than one RequestMirror filters in HTTPRoute.Spec.Rules.Filters", testcase{
		objs: []interface{}{
			kuardService,
			kuardService2,
			kuardService3,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
							},
						}, {
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("kuard3", 8080),
							}},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid RequestMirror filter due to unspecified backendRef.name", testcase{
		objs: []interface{}{
			kuardService,
			kuardService2,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1beta1.BackendObjectReference{
									Group: ref.To(gatewayapi_v1beta1.Group("")),
									Kind:  ref.To(gatewayapi_v1beta1.Kind("Service")),
									Port:  ref.To(gatewayapi_v1beta1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "Spec.Rules.Filters.RequestMirror.BackendRef.Name must be specified",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid RequestMirror filter due to unspecified backendRef.port", testcase{
		objs: []interface{}{
			kuardService,
			kuardService2,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1beta1.BackendObjectReference{
									Group: ref.To(gatewayapi_v1beta1.Group("")),
									Kind:  ref.To(gatewayapi_v1beta1.Kind("Service")),
									Name:  gatewayapi_v1beta1.ObjectName("kuard2"),
								},
							},
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "Spec.Rules.Filters.RequestMirror.BackendRef.Port must be specified",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid RequestMirror filter due to invalid backendRef.name on two matches", testcase{
		objs: []interface{}{
			kuardService,
			kuardService2,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("/"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("invalid-one", 8080),
							},
						}},
					}, {
						BackendRefs: gatewayapi.HTTPBackendRef("kuard2", 8080, 1),
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  ref.To(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: ref.To("/blog"),
							},
						}},
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("invalid-two", 8080),
							},
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonBackendNotFound),
							Message: "service \"invalid-one\" is invalid: service \"default/invalid-one\" not found, service \"invalid-two\" is invalid: service \"default/invalid-two\" not found",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid RequestMirror filter due to unmatched backendRef.namespace", testcase{
		objs: []interface{}{
			kuardService,
			kuardService2,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1beta1.BackendObjectReference{
									Group:     ref.To(gatewayapi_v1beta1.Group("")),
									Kind:      ref.To(gatewayapi_v1beta1.Kind("Service")),
									Namespace: ref.To(gatewayapi_v1beta1.Namespace("some-other-namespace")),
									Name:      gatewayapi_v1beta1.ObjectName("kuard2"),
									Port:      ref.To(gatewayapi_v1beta1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonRefNotPermitted),
							Message: "Spec.Rules.Filters.RequestMirror.BackendRef.Namespace must match the route's namespace or be covered by a ReferenceGrant",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "HTTPRouteFilterRequestMirror not yet supported for httproute backendref", testcase{
		objs: []interface{}{

			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
									Type: gatewayapi_v1beta1.HTTPRouteFilterRequestMirror, // HTTPRouteFilterRequestMirror is not supported yet.
								}},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "HTTPRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier and ResponseHeaderModifier type is supported.",
						},
						routeResolvedRefsCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "HTTPRouteFilterURLRewrite with custom HTTPPathModifierType is not supported", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gatewayapi_v1beta1.HTTPURLRewriteFilter{
								Path: &gatewayapi_v1beta1.HTTPPathModifier{
									Type: "custom",
								},
							},
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "HTTPRoute.Spec.Rules.Filters.URLRewrite.Path.Type: invalid type \"custom\": only ReplacePrefixMatch and ReplaceFullPath are supported.",
						},
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid RequestHeaderModifier due to duplicated headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
								Set: []gatewayapi_v1beta1.HTTPHeader{
									{Name: "custom", Value: "duplicated"},
									{Name: "Custom", Value: "duplicated"},
								},
							},
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "duplicate header addition: \"Custom\" on request headers",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid RequestHeaderModifier after forward due to invalid headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
									Type: gatewayapi_v1beta1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
										Set: []gatewayapi_v1beta1.HTTPHeader{
											{Name: "!invalid-header", Value: "foo"},
										},
									},
								}},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid set header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on request headers",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid ResponseHeaderModifier due to duplicated headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: gatewayapi_v1beta1.HTTPRouteFilterResponseHeaderModifier,
							ResponseHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
								Set: []gatewayapi_v1beta1.HTTPHeader{
									{Name: "custom", Value: "duplicated"},
									{Name: "Custom", Value: "duplicated"},
								},
							},
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "duplicate header addition: \"Custom\" on response headers",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "Invalid ResponseHeaderModifier on backend due to invalid headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1beta1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
									Type: gatewayapi_v1beta1.HTTPRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
										Set: []gatewayapi_v1beta1.HTTPHeader{
											{Name: "!invalid-header", Value: "foo"},
										},
									},
								}},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid set header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on response headers",
						},
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "custom filter type is not supported", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1beta1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1beta1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1beta1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1beta1.HTTPRouteFilter{{
							Type: "custom-filter",
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "HTTPRoute.Spec.Rules.Filters: invalid type \"custom-filter\": only RequestHeaderModifier, ResponseHeaderModifier, RequestRedirect, RequestMirror and URLRewrite are supported.",
						},
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "HTTPRoute", 1),
	})

	run(t, "gateway.spec.addresses results in invalid gateway", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Addresses: []gatewayapi_v1beta1.GatewayAddress{{
					Value: "1.2.3.4",
				}},
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1beta1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  metav1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonAddressNotAssigned),
					Message: "None of the addresses in Spec.Addresses have been assigned to the Gateway",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonProgrammed),
							Message: "Valid listener",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "invalid allowedroutes API group results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1beta1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Kinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: ref.To(gatewayapi_v1beta1.Group("invalid-group")),
								Kind:  "HTTPRoute",
							},
						},
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidRouteKinds),
							Message: "Group \"invalid-group\" is not supported, group must be \"gateway.networking.k8s.io\"",
						},
					},
				},
			},
		}},
	})

	run(t, "invalid allowedroutes API kind results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1beta1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Kinds: []gatewayapi_v1beta1.RouteGroupKind{
							{Kind: "FooRoute"},
						},
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidRouteKinds),
							Message: "Kind \"FooRoute\" is not supported, kind must be \"HTTPRoute\" or \"TLSRoute\" or \"GRPCRoute\"",
						},
					},
				},
			},
		}},
	})

	run(t, "allowedroute of TLSRoute on a non-TLS listener results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1beta1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Kinds: []gatewayapi_v1beta1.RouteGroupKind{
							{Kind: "TLSRoute"},
						},
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidRouteKinds),
							Message: "TLSRoutes are incompatible with listener protocol \"HTTP\"",
						},
					},
				},
			},
		}},
	})

	run(t, "TLS certificate ref to a non-secret on an HTTPS listener results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							{
								Group: ref.To(gatewayapi_v1beta1.Group("invalid-group")),
								Kind:  ref.To(gatewayapi_v1beta1.Kind("NotASecret")),
								Name:  "foo",
							},
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"foo\" must contain a reference to a core.Secret",
						},
					},
				},
			},
		}},
	})

	run(t, "nonexistent TLS certificate ref on an HTTPS listener results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("nonexistent-secret", "projectcontour"),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"nonexistent-secret\" referent is invalid: Secret not found",
						},
					},
				},
			},
		}},
	})

	run(t, "invalid listener protocol results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: "invalid",
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonUnsupportedProtocol),
							Message: "Listener protocol \"invalid\" is unsupported, must be one of HTTP, HTTPS or TLS",
						},
					},
				},
			},
		}},
	})

	run(t, "HTTPS listener without TLS defined results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS is required when protocol is \"HTTPS\".",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "TLS listener without TLS defined results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "TLSRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS is required when protocol is \"TLS\".",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "TLS Passthrough listener with a TLS certificate ref defined results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModePassthrough),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("tlscert", "projectcontour"),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "TLSRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS.CertificateRefs cannot be defined when Listener.TLS.Mode is \"Passthrough\".",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "TLS listener with TLS.Mode=Terminate results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("tlscert", "projectcontour"),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "TLSRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS.Mode must be \"Passthrough\" when protocol is \"TLS\".",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "HTTPS listener with TLS.Mode=Passthrough results in a listener condition", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1beta1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: ref.To(gatewayapi_v1beta1.TLSModePassthrough),
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS.Mode must be \"Terminate\" when protocol is \"HTTPS\".",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "Listener with FromNamespaces=Selector, no selector specified", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From:     ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
								Selector: nil,
							},
						},
					}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.AllowedRoutes.Namespaces.Selector is required when Listener.AllowedRoutes.Namespaces.From is set to \"Selector\".",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "Listener with FromNamespaces=Selector, invalid selector (can't specify values with Exists operator)", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
								Selector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{{
										Key:      "something",
										Operator: metav1.LabelSelectorOpExists,
										Values:   []string{"error"},
									}},
								},
							},
						},
					}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Error parsing Listener.AllowedRoutes.Namespaces.Selector: values: Invalid value: []string{\"error\"}: values set must be empty for exists and does not exist.",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

	run(t, "Listener with FromNamespaces=Selector, invalid selector (must specify MatchLabels and/or MatchExpressions)", testcase{
		objs: []interface{}{},
		gateway: &gatewayapi_v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1beta1.GatewaySpec{
				Listeners: []gatewayapi_v1beta1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From:     ref.To(gatewayapi_v1beta1.NamespacesFromSelector),
								Selector: &metav1.LabelSelector{},
							},
						},
					}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1beta1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionProgrammed),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ref.To(gatewayapi_v1beta1.Group(gatewayapi_v1beta1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.AllowedRoutes.Namespaces.Selector must specify at least one MatchLabel or MatchExpression.",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionAccepted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonAccepted),
							Message: "Listener accepted",
						},
					},
				},
			},
		}},
	})

}

func TestGatewayAPITLSRouteDAGStatus(t *testing.T) {

	type testcase struct {
		objs                    []interface{}
		gateway                 *gatewayapi_v1beta1.Gateway
		wantRouteConditions     []*status.RouteStatusUpdate
		wantGatewayStatusUpdate []*status.GatewayStatusUpdate
	}

	run := func(t *testing.T, desc string, tc testcase) {
		t.Helper()
		t.Run(desc, func(t *testing.T) {
			t.Helper()
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    fixture.NewTestLogger(t),
					gateway:        tc.gateway,
					gatewayclass: &gatewayapi_v1beta1.GatewayClass{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1beta1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1beta1.GatewayClassStatus{
							Conditions: []metav1.Condition{
								{
									Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
				},
			}

			// Add a default cert to be used in tests with TLS.
			builder.Source.Insert(fixture.SecretProjectContourCert)

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()
			gotRouteUpdates := dag.StatusCache.GetRouteUpdates()
			gotGatewayUpdates := dag.StatusCache.GetGatewayUpdates()

			ops := []cmp.Option{
				cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "TransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Resource"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "ExistingConditions"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j metav1.Condition) bool {
					return i.Message < j.Message
				}),
			}

			// Since we're using a single static GatewayClass,
			// set the expected controller string here for all
			// test cases.
			for _, u := range tc.wantRouteConditions {
				u.GatewayController = builder.Source.gatewayclass.Spec.ControllerName

				for _, rps := range u.RouteParentStatuses {
					rps.ControllerName = builder.Source.gatewayclass.Spec.ControllerName
				}
			}

			if diff := cmp.Diff(tc.wantRouteConditions, gotRouteUpdates, ops...); diff != "" {
				t.Fatalf("expected route status: %v, got %v", tc.wantRouteConditions, diff)
			}

			if diff := cmp.Diff(tc.wantGatewayStatusUpdate, gotGatewayUpdates, ops...); diff != "" {
				t.Fatalf("expected gateway status: %v, got %v", tc.wantGatewayStatusUpdate, diff)
			}

		})
	}

	gw := &gatewayapi_v1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1beta1.GatewaySpec{
			Listeners: []gatewayapi_v1beta1.Listener{{
				Name:     "tls-passthrough",
				Port:     443,
				Protocol: gatewayapi_v1beta1.TLSProtocolType,
				TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
					Mode: ref.To(gatewayapi_v1beta1.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
					Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
						From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	kuardService := &v1.Service{
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

	run(t, "TLSRoute: spec.rules.backendRef.name not specified", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayapi_v1alpha2.BackendRef{
							{
								BackendObjectReference: gatewayapi_v1alpha2.BackendObjectReference{
									Kind: ref.To(gatewayapi_v1beta1.Kind("Service")),
									Port: ref.To(gatewayapi_v1beta1.PortNumber(8080)),
								},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "Spec.Rules.BackendRef.Name must be specified",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: spec.rules.backendRef.name invalid on two matches", testcase{
		gateway: gw,
		objs: []interface{}{
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{
						{BackendRefs: gatewayapi.TLSRouteBackendRef("invalid-one", 8080, nil)},
						{BackendRefs: gatewayapi.TLSRouteBackendRef("invalid-two", 8080, nil)},
					},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonBackendNotFound),
							Message: "service \"invalid-one\" is invalid: service \"default/invalid-one\" not found, service \"invalid-two\" is invalid: service \"default/invalid-two\" not found",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: spec.rules.backendRef.port not specified", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayapi_v1alpha2.BackendRef{
							{
								BackendObjectReference: gatewayapi_v1alpha2.BackendObjectReference{
									Kind: ref.To(gatewayapi_v1beta1.Kind("Service")),
									Name: "kuard",
								},
							},
						},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "Spec.Rules.BackendRef.Port must be specified",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: spec.rules.backendRefs not specified", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{
						{}, // rule with no backend refs
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "At least one Spec.Rules.BackendRef must be specified.",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: spec.rules.hostname: invalid wildcard", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"*.*.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: spec.rules.hostname: invalid hostname", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"#projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: spec.rules.hostname: invalid hostname, ip address", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"1.2.3.4"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid hostname \"1.2.3.4\": must be a DNS name, not an IP address",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: spec.rules.backendRefs has 0 weight", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							gatewayapi.GatewayParentRef(gw.Namespace, gw.Name),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef(kuardService.Name, 8080, ref.To(int32(0))),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(status.ConditionValidBackendRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonAllBackendRefsHaveZeroWeights),
							Message: "At least one Spec.Rules.BackendRef must have a non-zero weight.",
						},
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})

	run(t, "TLSRoute: backendrefs still validated when route not accepted", testcase{
		gateway: gw,
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							// Wrong port.
							gatewayapi.GatewayListenerParentRef(gw.Namespace, gw.Name, "tls-passthrough", 444),
						},
					},
					Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("invalid-one", 8080, nil),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef(gw.Namespace, gw.Name, "tls-passthrough", 444),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonBackendNotFound),
							Message: "service \"invalid-one\" is invalid: service \"default/invalid-one\" not found",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), "TLSRoute", 0),
	})
}

func TestGatewayAPIGRPCRouteDAGStatus(t *testing.T) {
	type testcase struct {
		objs                    []interface{}
		gateway                 *gatewayapi_v1beta1.Gateway
		wantRouteConditions     []*status.RouteStatusUpdate
		wantGatewayStatusUpdate []*status.GatewayStatusUpdate
	}

	run := func(t *testing.T, desc string, tc testcase) {
		t.Helper()
		t.Run(desc, func(t *testing.T) {
			t.Helper()
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    fixture.NewTestLogger(t),
					gatewayclass: &gatewayapi_v1beta1.GatewayClass{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1beta1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1beta1.GatewayClassStatus{
							Conditions: []metav1.Condition{
								{
									Type:   string(gatewayapi_v1beta1.GatewayClassConditionStatusAccepted),
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
					gateway: tc.gateway,
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
				},
			}

			// Set a default gateway if not defined by a test
			if tc.gateway == nil {
				builder.Source.gateway = &gatewayapi_v1beta1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1beta1.GatewaySpec{
						Listeners: []gatewayapi_v1beta1.Listener{{
							Name:     "http",
							Port:     80,
							Protocol: gatewayapi_v1beta1.HTTPProtocolType,
							AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
								Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
									From: ref.To(gatewayapi_v1beta1.NamespacesFromAll),
								},
							},
						}},
					},
				}
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()
			gotRouteUpdates := dag.StatusCache.GetRouteUpdates()
			gotGatewayUpdates := dag.StatusCache.GetGatewayUpdates()

			ops := []cmp.Option{
				cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "TransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Resource"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "ExistingConditions"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j metav1.Condition) bool {
					return i.Message < j.Message
				}),
				cmpopts.SortSlices(func(i, j *status.RouteStatusUpdate) bool {
					return i.FullName.String() < j.FullName.String()
				}),
			}

			// Since we're using a single static GatewayClass,
			// set the expected controller string here for all
			// test cases.
			for _, u := range tc.wantRouteConditions {
				u.GatewayController = builder.Source.gatewayclass.Spec.ControllerName

				for _, rps := range u.RouteParentStatuses {
					rps.ControllerName = builder.Source.gatewayclass.Spec.ControllerName
				}
			}

			if diff := cmp.Diff(tc.wantRouteConditions, gotRouteUpdates, ops...); diff != "" {
				t.Fatalf("expected route status: %v, got %v", tc.wantRouteConditions, diff)
			}

			if diff := cmp.Diff(tc.wantGatewayStatusUpdate, gotGatewayUpdates, ops...); diff != "" {
				t.Fatalf("expected gateway status: %v, got %v", tc.wantGatewayStatusUpdate, diff)
			}
		})
	}

	kuardService := &v1.Service{
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

	kuardService2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard2",
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

	kuardService3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard3",
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

	run(t, "simple grpcroute", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

	run(t, "grpcroute: regular expression method match type is not yet supported", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						// RegularExpression type not yet supported
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchRegularExpression, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "GRPCRoute.Spec.Rules.Matches.Method: Only Exact match type is supported.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 0),
	})

	run(t, "grpcroute: method match must have Service configured", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidMethodMatch),
							Message: "GRPCRoute.Spec.Rules.Matches.Method: Both Service and Method need be configured.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 0),
	})

	run(t, "grpcroute: method match must have Method configured", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", ""),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidMethodMatch),
							Message: "GRPCRoute.Spec.Rules.Matches.Method: Both Service and Method need be configured.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 0),
	})

	run(t, "grpcroute: regular expression header match is not yet supported", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: &gatewayapi_v1alpha2.GRPCMethodMatch{
								Type:    ref.To(gatewayapi_v1alpha2.GRPCMethodMatchExact),
								Service: ref.To("come.example.service"),
								Method:  ref.To("Login"),
							},
							Headers: []gatewayapi_v1alpha2.GRPCHeaderMatch{
								{
									// <---- RegularExpression type not yet supported
									Type:  ref.To(gatewayapi_v1beta1.HeaderMatchRegularExpression),
									Name:  gatewayapi_v1alpha2.GRPCHeaderName("foo"),
									Value: "bar",
								},
							},
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "GRPCRoute.Spec.Rules.Matches.Headers: Only Exact match type is supported",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 0),
	})

	run(t, "grpcroute: invalid RequestHeaderModifier due to duplicated headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1alpha2.GRPCRouteFilter{{
							Type: gatewayapi_v1alpha2.GRPCRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
								Set: []gatewayapi_v1beta1.HTTPHeader{
									{Name: "custom", Value: "duplicated"},
									{Name: "Custom", Value: "duplicated"},
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "duplicate header addition: \"Custom\" on request headers",
						},
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

	run(t, "grpcroute: invalid ResponseHeaderModifier due to invalid headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1alpha2.GRPCRouteFilter{{
							Type: gatewayapi_v1alpha2.GRPCRouteFilterResponseHeaderModifier,
							ResponseHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
								Add: []gatewayapi_v1beta1.HTTPHeader{
									{Name: "!invalid-header", Value: "foo"},
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid add header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on response headers",
						},
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

	run(t, "grpcroute: more than one RequestMirror filters in GRPCRoute.Spec.Rules.Filters", testcase{
		objs: []interface{}{
			kuardService,
			kuardService2,
			kuardService3,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1alpha2.GRPCRouteFilter{{
							Type: gatewayapi_v1alpha2.GRPCRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
							},
						}, {
							Type: gatewayapi_v1alpha2.GRPCRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("kuard3", 8080),
							}},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

	run(t, "grpcroute: invalid RequestMirror filter due to unspecified backendRef.name", testcase{
		objs: []interface{}{
			kuardService,
			kuardService2,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1alpha2.GRPCRouteFilter{{
							Type: gatewayapi_v1alpha2.GRPCRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1beta1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1beta1.BackendObjectReference{
									Group: ref.To(gatewayapi_v1beta1.Group("")),
									Kind:  ref.To(gatewayapi_v1beta1.Kind("Service")),
									Port:  ref.To(gatewayapi_v1beta1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "Spec.Rules.Filters.RequestMirror.BackendRef.Name must be specified",
						},
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

	run(t, "grpcroute: custom filter type is not supported", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1alpha2.GRPCRouteFilter{{
							Type: "custom-filter",
						}},
					}},
				},
			}},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonUnsupportedValue),
							Message: "GRPCRoute.Spec.Rules.Filters: invalid type \"custom-filter\": only RequestHeaderModifier, ResponseHeaderModifier and RequestMirror are supported.",
						},
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

	run(t, "grpcroute: at lease one backend need to be specified", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: []gatewayapi_v1alpha2.GRPCBackendRef{}},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "At least one Spec.Rules.BackendRef must be specified.",
						},
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 0),
	})

	run(t, "grpcroute: still validate backendrefs when not accepted", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1alpha2.ParentReference{
							// Wrong port.
							gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http", 900),
						},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("invalid", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http", 900),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonBackendNotFound),
							Message: "service \"invalid\" is invalid: service \"default/invalid\" not found",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 0),
	})

	run(t, "grpcroute: invalid RequestHeaderModifier on backend due to duplicated headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: []gatewayapi_v1alpha2.GRPCBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1alpha2.GRPCRouteFilter{{
									Type: gatewayapi_v1alpha2.GRPCRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
										Set: []gatewayapi_v1beta1.HTTPHeader{
											{Name: "custom", Value: "duplicated"},
											{Name: "Custom", Value: "duplicated"},
										},
									},
								}},
							},
						}},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "duplicate header addition: \"Custom\" on request headers",
						},
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

	run(t, "grpcroute: invalid ResponseHeaderModifier on backend due to invalid headers", testcase{
		objs: []interface{}{
			kuardService,
			&gatewayapi_v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1beta1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1beta1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha2.GRPCRouteRule{{
						Matches: []gatewayapi_v1alpha2.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1alpha2.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: []gatewayapi_v1alpha2.GRPCBackendRef{
							{
								BackendRef: gatewayapi_v1beta1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1alpha2.GRPCRouteFilter{{
									Type: gatewayapi_v1alpha2.GRPCRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1beta1.HTTPHeaderFilter{
										Set: []gatewayapi_v1beta1.HTTPHeader{
											{Name: "!invalid-header", Value: "foo"},
										},
									},
								}},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1beta1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []metav1.Condition{
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
							Status:  contour_api_v1.ConditionFalse,
							Reason:  string(status.ReasonDegraded),
							Message: "invalid set header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on response headers",
						},
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", "GRPCRoute", 1),
	})

}

func gatewayAcceptedCondition() metav1.Condition {
	return metav1.Condition{
		Type:    string(gatewayapi_v1beta1.GatewayConditionAccepted),
		Status:  contour_api_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1beta1.GatewayReasonAccepted),
		Message: "Gateway is accepted",
	}
}

func routeResolvedRefsCondition() metav1.Condition {
	return metav1.Condition{
		Type:    string(gatewayapi_v1beta1.RouteConditionResolvedRefs),
		Status:  contour_api_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1beta1.RouteReasonResolvedRefs),
		Message: "References resolved",
	}
}

func routeAcceptedHTTPRouteCondition() metav1.Condition {
	return metav1.Condition{
		Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
		Status:  contour_api_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
		Message: "Accepted HTTPRoute",
	}
}

func routeAcceptedGRPCRouteCondition() metav1.Condition {
	return metav1.Condition{
		Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
		Status:  contour_api_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
		Message: "Accepted GRPCRoute",
	}
}
