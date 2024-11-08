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
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/status"
)

func TestDAGStatus(t *testing.T) {
	type testcase struct {
		objs                []any
		fallbackCertificate *types.NamespacedName
		want                map[types.NamespacedName]contour_v1.DetailedCondition
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

			got := make(map[types.NamespacedName]contour_v1.DetailedCondition)
			for _, pu := range dag.StatusCache.GetProxyUpdates() {
				got[pu.Fullname] = *pu.Conditions[status.ValidCondition]
			}

			assert.Equal(t, tc.want, got)
		})
	}

	// proxyNoFQDN is invalid because it does not specify and FQDN
	proxyNoFQDN := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "parent",
			Generation: 23,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// Tests using common fixtures
	run(t, "root proxy does not specify FQDN", testcase{
		objs: []any{proxyNoFQDN},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().WithGeneration(proxyNoFQDN.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
		},
	})

	// Simple Valid HTTPProxy
	proxyValidHomeService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "valid proxy", testcase{
		objs: []any{proxyValidHomeService, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidHomeService.Name, Namespace: proxyValidHomeService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidHomeService.Generation).
				Valid(),
		},
	})

	// Multiple Includes, one invalid
	proxyMultiIncludeOneInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "parent",
			Generation: 45,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "validChild",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "invalidChild",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxyIncludeValidChild := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "parentvalidchild",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "validChild",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxyChildValidFoo2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "validChild",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	proxyChildInvalidBadPort := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "foo3",
					Port: 12345678,
				}},
			}},
		},
	}

	run(t, "proxy has multiple includes, one is invalid", testcase{
		objs: []any{proxyMultiIncludeOneInvalid, proxyChildValidFoo2, proxyChildInvalidBadPort, fixture.ServiceRootsFoo2, fixture.ServiceRootsFoo3InvalidPort},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildValidFoo2.Generation).
				Valid(),
			{Name: proxyChildInvalidBadPort.Name, Namespace: proxyChildInvalidBadPort.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildInvalidBadPort.Generation).
				WithError(contour_v1.ConditionTypeServiceError, "ServicePortInvalid", `service "foo3": port must be in the range 1-65535`),
			{Name: proxyMultiIncludeOneInvalid.Name, Namespace: proxyMultiIncludeOneInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultiIncludeOneInvalid.Generation).
				Valid(),
		},
	})

	run(t, "multi-parent child is not orphaned when one of the parents is invalid", testcase{
		objs: []any{proxyNoFQDN, proxyChildValidFoo2, proxyIncludeValidChild, fixture.ServiceRootsKuard, fixture.ServiceRootsFoo2},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyNoFQDN.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildValidFoo2.Generation).
				Valid(),
			{Name: proxyIncludeValidChild.Name, Namespace: proxyIncludeValidChild.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludeValidChild.Generation).
				Valid(),
		},
	})

	// Exact match condition in include match conditions, invalid
	proxyExactIncludeInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "invalid-parent",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "exact-invalid.com",
			},
			Includes: []contour_v1.Include{{
				Name: "child1",
				Conditions: []contour_v1.MatchCondition{{
					Exact: "/foo",
				}},
			}, {
				Name: "child2",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	// Exact match condition in include match conditions, invalid
	proxyExactMatchValid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "valid-parent",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "exact-valid.com",
			},
			Includes: []contour_v1.Include{{
				Name: "child1",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "child2",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxyExactIncludeChild1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "child1",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Exact: "/exact",
				}},
				Services: []contour_v1.Service{{
					Name: "foo1",
					Port: 8080,
				}},
			}},
		},
	}

	proxyExactIncludeChild2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "child2",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy has exact match condition in include match conditions, should be invalid", testcase{
		objs: []any{proxyExactIncludeInvalid, proxyExactMatchValid, proxyExactIncludeChild1, proxyExactIncludeChild2, fixture.ServiceRootsFoo1, fixture.ServiceRootsFoo2},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyExactIncludeChild1.Name, Namespace: proxyExactIncludeChild1.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyExactIncludeChild1.Generation).
				Valid(),
			{Name: proxyExactIncludeChild2.Name, Namespace: proxyExactIncludeChild2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyExactIncludeChild2.Generation).
				Valid(),
			{Name: proxyExactIncludeInvalid.Name, Namespace: proxyExactIncludeInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyExactIncludeInvalid.Generation).
				WithError(contour_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid", `include: exact conditions are not allowed in includes block`),
			{Name: proxyExactMatchValid.Name, Namespace: proxyExactMatchValid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyExactMatchValid.Generation).
				Valid(),
		},
	})

	ingressSharedService := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
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

	proxyTCPSharedService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "nginx",
			Namespace: fixture.ServiceRootsNginx.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsNginx.Name,
					Port: 80,
				}},
			},
		},
	}

	// issue 1399
	run(t, "service shared across ingress and httpproxy tcpproxy", testcase{
		objs: []any{
			fixture.SecretRootsCert, fixture.ServiceRootsNginx, ingressSharedService, proxyTCPSharedService,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPSharedService.Name, Namespace: proxyTCPSharedService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyTCPSharedService.Generation).
				Valid(),
		},
	})

	proxyDelegatedTCPTLS := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			},
		},
	}

	// issue 1347
	run(t, "tcpproxy with tls delegation failure", testcase{
		objs: []any{
			fixture.SecretProjectContourCert,
			proxyDelegatedTCPTLS,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyDelegatedTCPTLS.Name, Namespace: proxyDelegatedTCPTLS.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyDelegatedTCPTLS.Generation).
				WithError(contour_v1.ConditionTypeTLSError, "DelegationNotPermitted", `Spec.VirtualHost.TLS Secret "projectcontour/default-ssl-cert" certificate delegation not permitted`),
		},
	})

	proxyDelegatedTLS := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			}},
		},
	}

	// issue 1348
	run(t, "routes with tls delegation failure", testcase{
		objs: []any{
			fixture.SecretProjectContourCert,
			proxyDelegatedTLS,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyDelegatedTLS.Name, Namespace: proxyDelegatedTLS.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyDelegatedTCPTLS.Generation).
				WithError(contour_v1.ConditionTypeTLSError, "DelegationNotPermitted", `Spec.VirtualHost.TLS Secret "projectcontour/default-ssl-cert" certificate delegation not permitted`),
		},
	})

	serviceTLSPassthrough := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "tls-passthrough",
			Namespace: "roots",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("https", "TCP", 443, 443), makeServicePort("http", "TCP", 80, 80)},
		},
	}

	proxyPassthroughProxyNonSecure := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: serviceTLSPassthrough.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: serviceTLSPassthrough.Name,
					Port: 80, // proxy non secure traffic to port 80
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: serviceTLSPassthrough.Name,
					Port: 443, // ssl passthrough to secure port
				}},
			},
		},
	}

	// issue 910
	run(t, "non tls routes can be combined with tcp proxy", testcase{
		objs: []any{
			serviceTLSPassthrough,
			proxyPassthroughProxyNonSecure,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyPassthroughProxyNonSecure.Name, Namespace: proxyPassthroughProxyNonSecure.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyPassthroughProxyNonSecure.Generation).
				Valid(),
		},
	})

	proxyMultipleIncludersSite1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "site1",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "site1.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultipleIncludersSite2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "site2",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "site2.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultiIncludeChild := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "two root httpproxies with different hostnames delegated to the same object are valid", testcase{
		objs: []any{
			fixture.ServiceRootsKuard, proxyMultipleIncludersSite1, proxyMultipleIncludersSite2, proxyMultiIncludeChild,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
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
	proxyInvalidNegativePortHomeService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	run(t, "invalid port in service", testcase{
		objs: []any{proxyInvalidNegativePortHomeService},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidNegativePortHomeService.Name, Namespace: proxyInvalidNegativePortHomeService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidNegativePortHomeService.Generation).
				WithError(contour_v1.ConditionTypeServiceError, "ServicePortInvalid", `service "home": port must be in the range 1-65535`),
		},
	})

	// proxyInvalidOutsideRootNamespace is invalid because it lives outside the roots namespace
	proxyInvalidOutsideRootNamespace := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foobar",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "root proxy outside of roots namespace", testcase{
		objs: []any{proxyInvalidOutsideRootNamespace},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidOutsideRootNamespace.Name, Namespace: proxyInvalidOutsideRootNamespace.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidNegativePortHomeService.Generation).
				WithError(contour_v1.ConditionTypeRootNamespaceError, "RootProxyNotAllowedInNamespace", "root HTTPProxy cannot be defined in this namespace"),
		},
	})

	// proxyInvalidIncludeCycle is invalid because it delegates to itself, producing a cycle
	proxyInvalidIncludeCycle := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "self",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "self",
				Namespace: "roots",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy self-edge produces a cycle", testcase{
		objs: []any{proxyInvalidIncludeCycle, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidIncludeCycle.Name, Namespace: proxyInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidIncludeCycle.Generation).
				WithError(contour_v1.ConditionTypeIncludeError, "RootIncludesRoot", fmt.Sprintf("root httpproxy cannot include another root httpproxy (%s/%s)", proxyInvalidIncludeCycle.Namespace, proxyInvalidIncludeCycle.Name)),
		},
	})

	// proxyIncludesProxyWithIncludeCycle delegates to proxy8, which is invalid because proxy8 delegates back to proxy8
	proxyIncludesProxyWithIncludeCycle := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "parent",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxyIncludedChildInvalidIncludeCycle := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "child",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			Includes: []contour_v1.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	run(t, "proxy child delegates to itself, producing a cycle", testcase{
		objs: []any{proxyIncludesProxyWithIncludeCycle, proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyIncludesProxyWithIncludeCycle.Name, Namespace: proxyIncludesProxyWithIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludesProxyWithIncludeCycle.Generation).Valid(),
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildInvalidIncludeCycle.Generation).
				WithError(contour_v1.ConditionTypeIncludeError, "IncludeCreatesCycle", "include creates an include cycle: roots/parent -> roots/child -> roots/child"),
		},
	})

	run(t, "proxy orphaned route", testcase{
		objs: []any{proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildInvalidIncludeCycle.Generation).
				Orphaned(),
		},
	})

	proxyIncludedChildValid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "validChild",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	// proxyNotRootIncludeRootProxy delegates to proxyWildCardFQDN but it is invalid because it is missing fqdn
	proxyNotRootIncludeRootProxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{},
			Includes: []contour_v1.Include{{
				Name:      "validChild",
				Namespace: "roots",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	run(t, "proxy invalid parent orphans child", testcase{
		objs: []any{proxyNotRootIncludeRootProxy, proxyIncludedChildValid},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyNotRootIncludeRootProxy.Name, Namespace: proxyNotRootIncludeRootProxy.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyNotRootIncludeRootProxy.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
			{Name: proxyIncludedChildValid.Name, Namespace: proxyIncludedChildValid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildValid.Generation).
				Orphaned(),
		},
	})

	// singleNameFQDN is valid
	singleNameFQDN := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy valid single FQDN", testcase{
		objs: []any{singleNameFQDN, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: singleNameFQDN.Name, Namespace: singleNameFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(singleNameFQDN.Generation).
				Valid(),
		},
	})

	// proxyInvalidServiceInvalid is invalid because it references an invalid service
	proxyInvalidServiceInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "invalid",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy missing service is invalid", testcase{
		objs: []any{proxyInvalidServiceInvalid},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidServiceInvalid.Name, Namespace: proxyInvalidServiceInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidServiceInvalid.Generation).
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "roots/invalid" not found`),
		},
	})

	// proxyInvalidServicePortInvalid is invalid because it references an invalid port on a service
	proxyInvalidServicePortInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 9999,
				}},
			}},
		},
	}

	run(t, "proxy with service missing port is invalid", testcase{
		objs: []any{proxyInvalidServicePortInvalid, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidServicePortInvalid.Name, Namespace: proxyInvalidServicePortInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidServiceInvalid.Generation).
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: port "9999" on service "roots/home" not matched`),
		},
	})

	proxyValidExampleCom := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidReuseExampleCom := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "other-example",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidReuseCaseExampleCom := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "case-example",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "EXAMPLE.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "conflicting proxies due to fqdn reuse", testcase{
		objs: []any{proxyValidExampleCom, proxyValidReuseExampleCom},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidExampleCom.Name, Namespace: proxyValidExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidExampleCom.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`),
			{Name: proxyValidReuseExampleCom.Name, Namespace: proxyValidReuseExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidReuseExampleCom.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`),
		},
	})

	run(t, "conflicting proxies due to fqdn reuse with uppercase/lowercase", testcase{
		objs: []any{proxyValidExampleCom, proxyValidReuseCaseExampleCom},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidExampleCom.Name, Namespace: proxyValidExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidExampleCom.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/case-example, roots/example-com`),
			{Name: proxyValidReuseCaseExampleCom.Name, Namespace: proxyValidReuseCaseExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidReuseCaseExampleCom.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/case-example, roots/example-com`),
		},
	})

	proxyRootIncludesRoot := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &contour_v1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Includes: []contour_v1.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxyRootIncludedByRoot := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &contour_v1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	run(t, "root proxy including another root", testcase{
		objs: []any{proxyRootIncludesRoot, proxyRootIncludedByRoot},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyRootIncludesRoot.Name, Namespace: proxyRootIncludesRoot.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludesRoot.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`),
			{Name: proxyRootIncludedByRoot.Name, Namespace: proxyRootIncludedByRoot.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludedByRoot.Generation).
				WithError(contour_v1.ConditionTypeVirtualHostError, "DuplicateVhost", `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`),
		},
	})

	proxyIncludesRootDifferentFQDN := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxyRootIncludedByRootDiffFQDN := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	run(t, "root proxy including another root w/ different hostname", testcase{
		objs: []any{proxyIncludesRootDifferentFQDN, proxyRootIncludedByRootDiffFQDN, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyIncludesRootDifferentFQDN.Name, Namespace: proxyIncludesRootDifferentFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludesRootDifferentFQDN.Generation).
				WithError(contour_v1.ConditionTypeIncludeError, "RootIncludesRoot", fmt.Sprintf("root httpproxy cannot include another root httpproxy (%s/%s)", proxyRootIncludedByRootDiffFQDN.Namespace, proxyRootIncludedByRootDiffFQDN.Name)),
			{Name: proxyRootIncludedByRootDiffFQDN.Name, Namespace: proxyRootIncludedByRootDiffFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludedByRootDiffFQDN.Generation).
				Valid(),
		},
	})

	proxyValidIncludeBlogMarketing := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxyRootValidIncludesBlogMarketing := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      proxyValidIncludeBlogMarketing.Name,
				Namespace: proxyValidIncludeBlogMarketing.Namespace,
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
		},
	}

	run(t, "proxy includes another", testcase{
		objs: []any{proxyValidIncludeBlogMarketing, proxyRootValidIncludesBlogMarketing, fixture.ServiceRootsKuard, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidIncludeBlogMarketing.Name, Namespace: proxyValidIncludeBlogMarketing.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidIncludeBlogMarketing.Generation).
				Valid(),
			{Name: proxyRootValidIncludesBlogMarketing.Name, Namespace: proxyRootValidIncludesBlogMarketing.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootValidIncludesBlogMarketing.Generation).
				Valid(),
		},
	})

	proxyValidWithMirror := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
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
		objs: []any{proxyValidWithMirror, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidWithMirror.Name, Namespace: proxyValidWithMirror.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidWithMirror.Generation).
				Valid(),
		},
	})

	proxyInvalidTwoMirrors := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
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
		objs: []any{proxyInvalidTwoMirrors, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidTwoMirrors.Name, Namespace: proxyInvalidTwoMirrors.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidTwoMirrors.Generation).
				WithError(contour_v1.ConditionTypeServiceError, "OnlyOneMirror", "only one service per route may be nominated as mirror"),
		},
	})

	proxyInvalidDuplicateMatchConditionHeaders := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate route condition headers", testcase{
		objs: []any{proxyInvalidDuplicateMatchConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateMatchConditionHeaders.Name, Namespace: proxyInvalidDuplicateMatchConditionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateMatchConditionHeaders.Generation).
				WithError(contour_v1.ConditionTypeRouteError, "HeaderMatchConditionsNotValid", "cannot specify duplicate header 'exact match' conditions in the same route"),
		},
	})

	proxyInvalidDuplicateMatchConditionQueryParameters := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "abc",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "1234",
					},
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate route condition query parameters", testcase{
		objs: []any{proxyInvalidDuplicateMatchConditionQueryParameters, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateMatchConditionQueryParameters.Name, Namespace: proxyInvalidDuplicateMatchConditionQueryParameters.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateMatchConditionQueryParameters.Generation).
				WithError(contour_v1.ConditionTypeRouteError, "QueryParameterMatchConditionsNotValid", "cannot specify duplicate query parameter 'exact match' conditions in the same route"),
		},
	})

	proxyValidDelegatedRoots := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	proxyInvalidDuplicateIncludeCondtionHeaders := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "delegated",
				Namespace: "roots",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
			}},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate include condition headers", testcase{
		objs: []any{proxyInvalidDuplicateIncludeCondtionHeaders, proxyValidDelegatedRoots, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyInvalidDuplicateIncludeCondtionHeaders.Name,
				Namespace: proxyInvalidDuplicateIncludeCondtionHeaders.Namespace,
			}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateIncludeCondtionHeaders.Generation).WithError(contour_v1.ConditionTypeRouteError, "HeaderMatchConditionsNotValid", "cannot specify duplicate header 'exact match' conditions in the same route"),
			{
				Name:      proxyValidDelegatedRoots.Name,
				Namespace: proxyValidDelegatedRoots.Namespace,
			}: fixture.NewValidCondition().
				WithGeneration(proxyValidDelegatedRoots.Generation).Orphaned(),
		},
	})

	proxyInvalidRouteConditionHeaders := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						NotExact: "abc",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						NotExact: "1234",
					},
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate valid route condition headers", testcase{
		objs: []any{proxyInvalidRouteConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidRouteConditionHeaders.Name, Namespace: proxyInvalidRouteConditionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidRouteConditionHeaders.Generation).Valid(),
		},
	})

	proxyInvalidMultiplePrefixes := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with two prefix conditions on route", testcase{
		objs: []any{proxyInvalidMultiplePrefixes, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidMultiplePrefixes.Name, Namespace: proxyInvalidMultiplePrefixes.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidMultiplePrefixes.Generation).
				WithError(contour_v1.ConditionTypeRouteError, "PathMatchConditionsNotValid", "route: more than one prefix, exact or regex is not allowed in a condition block"),
		},
	})

	proxyInvalidTwoPrefixesWithInclude := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
			}},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidChildTeamA := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "child",
			Namespace: "teama",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with two prefix conditions orphans include", testcase{
		objs: []any{proxyInvalidTwoPrefixesWithInclude, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidTwoPrefixesWithInclude.Name, Namespace: proxyInvalidTwoPrefixesWithInclude.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidTwoPrefixesWithInclude.Generation).
				WithError(contour_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid", "include: more than one prefix, exact or regex is not allowed in a condition block"),
			{Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidChildTeamA.Generation).
				Orphaned(),
		},
	})

	proxyInvalidPrefixNoSlash := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{
					{
						Prefix: "api",
					},
				},
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with prefix conditions on route that does not start with slash", testcase{
		objs: []any{proxyInvalidPrefixNoSlash, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidPrefixNoSlash.Name, Namespace: proxyInvalidPrefixNoSlash.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidPrefixNoSlash.Generation).
				WithError(contour_v1.ConditionTypeRouteError, "PathMatchConditionsNotValid", "route: prefix conditions must start with /, api was supplied"),
		},
	})

	proxyInvalidIncludePrefixNoSlash := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{
					{
						Prefix: "api",
					},
				},
			}},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with include prefix that does not start with slash", testcase{
		objs: []any{proxyInvalidIncludePrefixNoSlash, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidIncludePrefixNoSlash.Name, Namespace: proxyInvalidIncludePrefixNoSlash.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid", "include: prefix conditions must start with /, api was supplied"),
			{Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: fixture.NewValidCondition().
				Orphaned(),
		},
	})

	proxyInvalidTCPProxyIncludeAndService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Include: &contour_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: "roots",
				},
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy cannot specify services and include", testcase{
		objs: []any{proxyInvalidTCPProxyIncludeAndService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidTCPProxyIncludeAndService.Name, Namespace: proxyInvalidTCPProxyIncludeAndService.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTCPProxyError, "NoServicesAndInclude", "cannot specify services and include in the same httpproxy"),
		},
	})

	proxyTCPNoServiceOrInclusion := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{},
		},
	}

	run(t, "tcpproxy empty", testcase{
		objs: []any{proxyTCPNoServiceOrInclusion, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPNoServiceOrInclusion.Name, Namespace: proxyTCPNoServiceOrInclusion.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTCPProxyError, "NothingDefined", "either services or inclusion must be specified"),
		},
	})

	proxyTCPIncludesFoo := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Include: &contour_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	run(t, "tcpproxy w/ missing include", testcase{
		objs: []any{proxyTCPIncludesFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTCPProxyIncludeError, "IncludeNotFound", "include roots/foo not found"),
		},
	})

	proxyValidTCPRoot := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy includes another root", testcase{
		objs: []any{proxyTCPIncludesFoo, proxyValidTCPRoot, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTCPProxyIncludeError, "RootIncludesRoot", fmt.Sprintf("root httpproxy cannot include another root httpproxy (%s/%s)", proxyValidTCPRoot.Namespace, proxyValidTCPRoot.Name)),
			{Name: proxyValidTCPRoot.Name, Namespace: proxyValidTCPRoot.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	proxyTCPValidChildFoo := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy includes valid child", testcase{
		objs: []any{proxyTCPIncludesFoo, proxyTCPValidChildFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}:     fixture.NewValidCondition().Valid(),
			{Name: proxyTCPValidChildFoo.Name, Namespace: proxyTCPValidChildFoo.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	proxyInvalidConflictingIncludeConditionsSimple := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidBlogTeamA := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "teama",
			Name:      "blogteama",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []contour_v1.Service{{
					Name: fixture.ServiceTeamAKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidBlogTeamB := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "teamb",
			Name:      "blogteamb",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []contour_v1.Service{{
					Name: fixture.ServiceTeamBKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate path conditions on an include", testcase{
		objs: []any{proxyInvalidConflictingIncludeConditionsSimple, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(), // Orphaned because the include pointing to this condition is a duplicate so the route is not programmed.
			{
				Name:      proxyInvalidConflictingIncludeConditionsSimple.Name,
				Namespace: proxyInvalidConflictingIncludeConditionsSimple.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyIncludeConditionsEmpty := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
			}},
		},
	}

	run(t, "empty include conditions", testcase{
		objs: []any{proxyIncludeConditionsEmpty, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),
			{
				Name:      proxyIncludeConditionsEmpty.Name,
				Namespace: proxyIncludeConditionsEmpty.Namespace,
			}: fixture.NewValidCondition().
				Valid(),
		},
	})

	proxyIncludeConditionsPrefixRoot := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	run(t, "multiple prefix / include conditions", testcase{
		objs: []any{proxyIncludeConditionsPrefixRoot, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(),
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(),
			{
				Name:      proxyIncludeConditionsPrefixRoot.Name,
				Namespace: proxyIncludeConditionsPrefixRoot.Namespace,
			}: fixture.NewValidCondition().
				Valid(),
		},
	})

	proxyInvalidConflictingIncludeConditions := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/somethingelse",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate path conditions on an include not consecutive", testcase{
		objs: []any{proxyInvalidConflictingIncludeConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyInvalidConflictingIncludeConditions.Name,
				Namespace: proxyInvalidConflictingIncludeConditions.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyInvalidConflictHeaderConditions := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-other-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate header conditions on an include", testcase{
		objs: []any{proxyInvalidConflictHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyInvalidConflictHeaderConditions.Name,
				Namespace: proxyInvalidConflictHeaderConditions.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyInvalidDuplicateMultiHeaderConditions := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-another-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-another-header",
						Contains: "abc",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate header conditions on an include mismatched order", testcase{
		objs: []any{proxyInvalidDuplicateMultiHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace,
			}: fixture.NewValidCondition().
				Orphaned(),
			{
				Name:      proxyInvalidDuplicateMultiHeaderConditions.Name,
				Namespace: proxyInvalidDuplicateMultiHeaderConditions.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyInvalidDuplicateIncludeSamePathDiffHeaders := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				// First valid header matches on path /foo.
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-another-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				// Second valid header matches on path /foo.
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header-other",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteama",
				Namespace: "teama",
				// This match on /foo with same headers as previous should be invalid.
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header-other",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate header conditions on an include with same path", testcase{
		objs: []any{proxyInvalidDuplicateIncludeSamePathDiffHeaders, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyInvalidDuplicateMultiHeaderConditions.Name,
				Namespace: proxyInvalidDuplicateMultiHeaderConditions.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyInvalidDuplicateHeaderAndPathConditions := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate header+path conditions on an include", testcase{
		objs: []any{proxyInvalidDuplicateHeaderAndPathConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace,
			}: fixture.NewValidCondition().
				Orphaned(),
			{
				Name:      proxyInvalidDuplicateHeaderAndPathConditions.Name,
				Namespace: proxyInvalidDuplicateHeaderAndPathConditions.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
		},
	})

	proxyInvalidConflictQueryConditions := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foo",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param-2",
						Exact: "bar",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:       "param-3",
						Exact:      "bar",
						IgnoreCase: true,
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foooo",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foo",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param-2",
						Exact: "bar",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param-2",
						Exact: "bar",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foo",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:   "param-1",
						Prefix: "foooo",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:       "param-3",
						Exact:      "bar",
						IgnoreCase: true,
					},
				}},
			}},
		},
	}

	run(t, "duplicate query param conditions on an include", testcase{
		objs: []any{proxyInvalidConflictQueryConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyInvalidConflictQueryConditions.Name,
				Namespace: proxyInvalidConflictQueryConditions.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{
				Name:      proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
		},
	})

	proxyInvalidConflictQueryHeaderConditions := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "foo",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "foo",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:   "param-other",
						Prefix: "bar",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header-other",
						Contains: "foo",
					},
				}},
			}},
		},
	}

	run(t, "duplicate query param+header conditions on an include", testcase{
		objs: []any{proxyInvalidConflictQueryHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyInvalidConflictQueryHeaderConditions.Name,
				Namespace: proxyInvalidConflictQueryHeaderConditions.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{
				Name:      proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include preceding an invalid one.
			{
				Name:      proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace,
			}: fixture.NewValidCondition().
				Valid(), // Valid since there is a valid include.
		},
	})

	proxyValidQueryHeaderConditions := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "foo",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "foo",
					},
				}, {
					QueryParameter: &contour_v1.QueryParameterMatchCondition{
						Name:  "param",
						Exact: "bar",
					},
				}},
			}},
		},
	}

	run(t, "query param+header conditions on an include should not be duplicate", testcase{
		objs: []any{proxyValidQueryHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace,
			}: fixture.NewValidCondition().
				Valid(),
			{
				Name:      proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace,
			}: fixture.NewValidCondition().
				Valid(),
			{
				Name:      proxyValidQueryHeaderConditions.Name,
				Namespace: proxyValidQueryHeaderConditions.Namespace,
			}: fixture.NewValidCondition().
				Valid(),
		},
	})

	proxyInvalidMissingInclude := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "child",
			}},
		},
	}

	run(t, "httpproxy w/ missing include", testcase{
		objs: []any{proxyInvalidMissingInclude, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidMissingInclude.Name, Namespace: proxyInvalidMissingInclude.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "IncludeNotFound", "include roots/child not found"),
		},
	})

	proxyTCPInvalidMissingService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "missing-tcp-proxy-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: "not-found",
					Port: 8080,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy w/ missing service", testcase{
		objs: []any{proxyTCPInvalidMissingService},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPInvalidMissingService.Name, Namespace: proxyTCPInvalidMissingService.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTCPProxyError, "ServiceUnresolvedReference", `Spec.TCPProxy unresolved service reference: service "roots/not-found" not found`),
		},
	})

	proxyTCPInvalidPortNotMatched := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "tcp-proxy-service-missing-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 9999,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy w/ service missing port", testcase{
		objs: []any{proxyTCPInvalidPortNotMatched, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPInvalidPortNotMatched.Name, Namespace: proxyTCPInvalidPortNotMatched.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTCPProxyError, "ServiceUnresolvedReference", `Spec.TCPProxy unresolved service reference: port "9999" on service "roots/kuard" not matched`),
		},
	})

	proxyTCPInvalidMissingTLS := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "missing-tls",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy missing tls", testcase{
		objs: []any{proxyTCPInvalidMissingTLS},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyTCPInvalidMissingTLS.Name, Namespace: proxyTCPInvalidMissingTLS.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTCPProxyError, "TLSMustBeConfigured", "Spec.TCPProxy requires that either Spec.TLS.Passthrough or Spec.TLS.SecretName be set"),
		},
	})

	proxyInvalidMissingServiceWithTCPProxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "missing-route-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{
					{Name: "missing", Port: 9000},
				},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy missing service", testcase{
		objs: []any{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyInvalidMissingServiceWithTCPProxy},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidMissingServiceWithTCPProxy.Name, Namespace: proxyInvalidMissingServiceWithTCPProxy.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "roots/missing" not found`),
		},
	})

	proxyRoutePortNotMatchedWithTCP := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "missing-route-service-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{
					{Name: fixture.ServiceRootsKuard.Name, Port: 9999},
				},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy route unmatched service port", testcase{
		objs: []any{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyRoutePortNotMatchedWithTCP},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyRoutePortNotMatchedWithTCP.Name, Namespace: proxyRoutePortNotMatchedWithTCP.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: port "9999" on service "roots/kuard" not matched`),
		},
	})

	proxyTCPValidIncludeChild := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Include: &contour_v1.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidIncludesChild := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				IncludesDeprecated: &contour_v1.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidChild := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "child",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "valid HTTPProxy.TCPProxy - plural", testcase{
		objs: []any{proxyTCPValidIncludesChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyTCPValidIncludesChild.Name,
				Namespace: proxyTCPValidIncludesChild.Namespace,
			}: fixture.NewValidCondition().Valid(),
			{
				Name:      proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace,
			}: fixture.NewValidCondition().Valid(),
		},
	})

	run(t, "valid HTTPProxy.TCPProxy", testcase{
		objs: []any{proxyTCPValidIncludeChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      proxyTCPValidIncludeChild.Name,
				Namespace: proxyTCPValidIncludeChild.Namespace,
			}: fixture.NewValidCondition().Valid(),
			{
				Name:      proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace,
			}: fixture.NewValidCondition().Valid(),
		},
	})

	// issue 2309
	proxyInvalidNoServices := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "missing-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "missing-service.example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: nil, // missing
			}},
		},
	}

	run(t, "No routeAction specified is invalid", testcase{
		objs: []any{proxyInvalidNoServices, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyInvalidNoServices.Name, Namespace: proxyInvalidNoServices.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeRouteError, "RouteActionCountNotValid", "must set exactly one of route.services or route.requestRedirectPolicy or route.directResponsePolicy"),
		},
	})

	fallbackCertificate := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	fallbackCertDelegation := &contour_v1.TLSCertificateDelegation{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "non-existing",
			Name:      "fallback-cert-delegation",
		},
		Spec: contour_v1.TLSCertificateDelegationSpec{
			Delegations: []contour_v1.CertificateDelegation{
				{
					SecretName:       "non-existing",
					TargetNamespaces: []string{"roots"},
				},
			},
		},
	}

	run(t, "non-existing fallback certificate passed to contour", testcase{
		fallbackCertificate: &types.NamespacedName{
			Name:      "non-existing",
			Namespace: "non-existing",
		},
		objs: []any{fallbackCertificate, fallbackCertDelegation, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "FallbackNotValid", `Spec.Virtualhost.TLS Secret "non-existing/non-existing" fallback certificate is invalid: Secret not found`),
		},
	})

	run(t, "fallback certificate reference without certificate delegation passed to contour", testcase{
		fallbackCertificate: &types.NamespacedName{
			Name:      "not-delegated",
			Namespace: "not-delegated",
		},
		objs: []any{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "FallbackNotDelegated", `Spec.VirtualHost.TLS Secret "not-delegated/not-delegated" is not configured for certificate delegation`),
		},
	})

	run(t, "fallback certificate requested but cert not configured in contour", testcase{
		objs: []any{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "FallbackNotPresent", "Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file"),
		},
	})

	fallbackCertificateWithClientValidationNoCA := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName:       "ssl-cert",
					ClientValidation: &contour_v1.DownstreamValidation{},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "clientValidation missing CA", testcase{
		objs: []any{fallbackCertificateWithClientValidationNoCA, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificateWithClientValidationNoCA.Name,
				Namespace: fallbackCertificateWithClientValidationNoCA.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "ClientValidationInvalid", "Spec.VirtualHost.TLS client validation is invalid: CA Secret must be specified"),
		},
	})

	fallbackCertificateWithClientValidation := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: "something",
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "fallback certificate requested and clientValidation also configured", testcase{
		objs: []any{fallbackCertificateWithClientValidation, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificateWithClientValidation.Name,
				Namespace: fallbackCertificateWithClientValidation.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client validation are incompatible"),
		},
	})

	tlsPassthroughAndValidation := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: "aCAcert",
					},
				},
			},
			TCPProxy: &contour_v1.TCPProxy{},
		},
	}

	run(t, "passthrough and client auth are incompatible tlsPassthroughAndValidation", testcase{
		objs: []any{fixture.SecretRootsCert, tlsPassthroughAndValidation},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: tlsPassthroughAndValidation.Name, Namespace: tlsPassthroughAndValidation.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.VirtualHost.TLS passthrough cannot be combined with tls.clientValidation"),
		},
	})

	tlsPassthroughAndSecretName := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
					SecretName:  fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{},
		},
	}

	run(t, "tcpproxy with TLS passthrough and secret name both specified", testcase{
		objs: []any{
			fixture.SecretRootsCert,
			tlsPassthroughAndSecretName,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "TLSConfigNotValid", "Spec.VirtualHost.TLS: both Passthrough and SecretName were specified"),
		},
	})

	tlsNoPassthroughOrSecretName := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: false,
					SecretName:  "",
				},
			},
			TCPProxy: &contour_v1.TCPProxy{},
		},
	}

	run(t, "httpproxy w/ tcpproxy with neither TLS passthrough nor secret name specified", testcase{
		objs: []any{
			fixture.SecretRootsCert,
			tlsNoPassthroughOrSecretName,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "TLSConfigNotValid", "Spec.VirtualHost.TLS: neither Passthrough nor SecretName were specified"),
		},
	})

	emptyProxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "empty",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
		},
	}

	run(t, "proxy with no routes, includes, or tcpproxy is invalid", testcase{
		objs: []any{emptyProxy},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: emptyProxy.Name, Namespace: emptyProxy.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeSpecError, "NothingDefined", "HTTPProxy.Spec must have at least one Route, Include, or a TCPProxy"),
		},
	})

	invalidResponseHeadersPolicyService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalidRHPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						ResponseHeadersPolicy: &contour_v1.HeadersPolicy{
							Set: []contour_v1.HeaderValue{{
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
		objs: []any{invalidResponseHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: invalidResponseHeadersPolicyService.Name, Namespace: invalidResponseHeadersPolicyService.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ResponseHeadersPolicyInvalid", `rewriting "Host" header is not supported on response headers`),
		},
	})

	invalidResponseHeadersPolicyRoute := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalidRHPRoute",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
					},
				},
				ResponseHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "external.com",
					}},
				},
			}},
		},
	}

	run(t, "responseHeadersPolicy, Host header invalid on Route", testcase{
		objs: []any{invalidResponseHeadersPolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: invalidResponseHeadersPolicyRoute.Name, Namespace: invalidResponseHeadersPolicyRoute.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeRouteError, "ResponseHeaderPolicyInvalid", `rewriting "Host" header is not supported on response headers`),
		},
	})

	duplicateCookieRewritePolicyRoute := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalidCRPRoute",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
					},
				},
				CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
					{
						Name:   "a-cookie",
						Secure: ptr.To(true),
					},
					{
						Name:     "a-cookie",
						SameSite: ptr.To("Lax"),
					},
				},
			}},
		},
	}

	run(t, "cookieRewritePolicies, duplicate cookie names on route", testcase{
		objs: []any{duplicateCookieRewritePolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: duplicateCookieRewritePolicyRoute.Name, Namespace: duplicateCookieRewritePolicyRoute.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `duplicate cookie rewrite rule for cookie "a-cookie" on route cookie rewrite rules`),
		},
	})

	duplicateCookieRewritePolicyService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalidCRPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
							{
								Name:   "a-cookie",
								Secure: ptr.To(true),
							},
							{
								Name:     "a-cookie",
								SameSite: ptr.To("Lax"),
							},
						},
					},
				},
			}},
		},
	}

	run(t, "cookieRewritePolicies, duplicate cookie names on service", testcase{
		objs: []any{duplicateCookieRewritePolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: duplicateCookieRewritePolicyService.Name, Namespace: duplicateCookieRewritePolicyService.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `duplicate cookie rewrite rule for cookie "a-cookie" on service cookie rewrite rules`),
		},
	})

	emptyCookieRewritePolicyRoute := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalidCRPRoute",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
					{
						Name: "a-cookie",
					},
				},
				Services: []contour_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
					},
				},
			}},
		},
	}

	run(t, "cookieRewritePolicies, empty cookie rewrite on route", testcase{
		objs: []any{emptyCookieRewritePolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: emptyCookieRewritePolicyRoute.Name, Namespace: emptyCookieRewritePolicyRoute.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `no attributes rewritten for cookie "a-cookie" on route cookie rewrite rules`),
		},
	})

	emptyCookieRewritePolicyService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "invalidCRPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
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
		objs: []any{emptyCookieRewritePolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: emptyCookieRewritePolicyService.Name, Namespace: emptyCookieRewritePolicyService.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeRouteError, "CookieRewritePoliciesInvalid", `no attributes rewritten for cookie "a-cookie" on service cookie rewrite rules`),
		},
	})

	proxyAuthFallback := fixture.NewProxy("roots/fallback-incompat").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "invalid.com",
				TLS: &contour_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
				Authorization: &contour_v1.AuthorizationServer{
					ExtensionServiceRef: contour_v1.ExtensionServiceReference{
						Namespace: "auth",
						Name:      "extension",
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	run(t, "fallback and client auth is invalid", testcase{
		objs: []any{fixture.SecretRootsCert, proxyAuthFallback},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: proxyAuthFallback.Name, Namespace: proxyAuthFallback.Namespace}: fixture.NewValidCondition().WithGeneration(proxyAuthFallback.Generation).
				WithError(contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client authorization are incompatible"),
		},
	})

	proxyAuthHTTP := fixture.NewProxy("roots/http").
		WithSpec(contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "invalid.com",
				Authorization: &contour_v1.AuthorizationServer{
					ExtensionServiceRef: contour_v1.ExtensionServiceReference{
						Namespace: "auth",
						Name:      "extension",
					},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	run(t, "plain HTTP vhost and client auth is invalid", testcase{
		objs: []any{fixture.SecretRootsCert, proxyAuthHTTP},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyAuthHTTP): fixture.NewValidCondition().WithGeneration(proxyAuthHTTP.Generation).
				WithError(contour_v1.ConditionTypeAuthError, "AuthNotPermitted", "Spec.VirtualHost.Authorization.ExtensionServiceRef can only be defined for root HTTPProxies that terminate TLS"),
		},
	})

	invalidResponseTimeout := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &contour_v1.TimeoutPolicy{
						Response: "invalid-val",
					},
				},
			},
		},
	}

	run(t, "proxy with invalid response timeout value is invalid", testcase{
		objs: []any{invalidResponseTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      invalidResponseTimeout.Name,
				Namespace: invalidResponseTimeout.Namespace,
			}: fixture.NewValidCondition().WithError(contour_v1.ConditionTypeRouteError, "TimeoutPolicyNotValid",
				`route.timeoutPolicy failed to parse: error parsing response timeout: unable to parse timeout string "invalid-val": time: invalid duration "invalid-val"`),
		},
	})

	invalidIdleTimeout := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &contour_v1.TimeoutPolicy{
						Idle: "invalid-val",
					},
				},
			},
		},
	}

	run(t, "proxy with invalid idle timeout value is invalid", testcase{
		objs: []any{invalidIdleTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      invalidIdleTimeout.Name,
				Namespace: invalidIdleTimeout.Namespace,
			}: fixture.NewValidCondition().WithError(contour_v1.ConditionTypeRouteError, "TimeoutPolicyNotValid",
				`route.timeoutPolicy failed to parse: error parsing idle timeout: unable to parse timeout string "invalid-val": time: invalid duration "invalid-val"`),
		},
	})

	// issue 3197: Fallback and passthrough HTTPProxy directive should emit a config error
	tlsPassthroughAndFallback := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				TLS: &contour_v1.TLS{
					Passthrough:               true,
					EnableFallbackCertificate: true,
				},
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "TLS with passthrough and fallback cert enabled is invalid", testcase{
		objs: []any{tlsPassthroughAndFallback, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: tlsPassthroughAndFallback.Name, Namespace: tlsPassthroughAndFallback.Namespace}: fixture.NewValidCondition().
				WithGeneration(tlsPassthroughAndFallback.Generation).WithError(
				contour_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures",
				`Spec.VirtualHost.TLS: both Passthrough and enableFallbackCertificate were specified`,
			),
		},
	})
	tlsPassthrough := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				TLS: &contour_v1.TLS{
					Passthrough:               true,
					EnableFallbackCertificate: false,
				},
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "valid TLS passthrough", testcase{
		objs: []any{tlsPassthrough, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: tlsPassthrough.Name, Namespace: tlsPassthrough.Namespace}: fixture.NewValidCondition().
				WithGeneration(tlsPassthrough.Generation).
				Valid(),
		},
	})

	multipleRouteAction := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "multipleRouteAction",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
				DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
					StatusCode: 200,
					Body:       "success",
				},
			}},
		},
	}
	run(t, "Selecting more than one routeAction is invalid", testcase{
		objs: []any{multipleRouteAction},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: multipleRouteAction.Name, Namespace: multipleRouteAction.Namespace}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeRouteError, "RouteActionCountNotValid",
					"must set exactly one of route.services or route.requestRedirectPolicy or route.directResponsePolicy"),
		},
	})

	invalidAllowOrigin := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-alloworigin",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				CORSPolicy: &contour_v1.CORSPolicy{
					AllowOrigin:  []string{"example-2.com", "**"},
					AllowMethods: []contour_v1.CORSHeaderValue{"GET"},
				},
			},
			Routes: []contour_v1.Route{
				{
					Services: []contour_v1.Service{
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
		objs: []any{invalidAllowOrigin, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      invalidAllowOrigin.Name,
				Namespace: invalidAllowOrigin.Namespace,
			}: fixture.NewValidCondition().WithError(contour_v1.ConditionTypeCORSError, "PolicyDidNotParse",
				`Spec.VirtualHost.CORSPolicy: invalid allowed origin "**": allowed origin is invalid exact match and invalid regex match`),
		},
	})

	jwtVerificationValidProxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-valid-proxy",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:      "provider-1",
						Issuer:    "jwt.example.com",
						Audiences: []string{"foo", "bar"},
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "https://jwt.example.com/jwks.json",
							Timeout:       "10s",
							CacheDuration: "1h",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification valid proxy", testcase{
		objs: []any{
			jwtVerificationValidProxy,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationValidProxy): fixture.NewValidCondition().Valid(),
		},
	})

	jwtVerificationDuplicateProviders := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-duplicate-provider-names",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification duplicate provider names", testcase{
		objs: []any{
			jwtVerificationDuplicateProviders,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationDuplicateProviders): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"DuplicateProviderName",
					"Spec.VirtualHost.JWTProviders is invalid: duplicate name provider-1",
				),
		},
	})

	jwtVerificationMultipleDefaults := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-multiple-defaults",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name:    "provider-1",
						Default: true,
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
					{
						Name:    "provider-2",
						Default: true,
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification multiple default providers", testcase{
		objs: []any{
			jwtVerificationMultipleDefaults,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationMultipleDefaults): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"MultipleDefaultProvidersSpecified",
					"Spec.VirtualHost.JWTProviders is invalid: at most one provider can be set as the default",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSURI := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-uri",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: ":/invalid-uri",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS URI", testcase{
		objs: []any{
			jwtVerificationInvalidRemoteJWKSURI,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSURI): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSURIInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.URI is invalid: parse \":/invalid-uri\": missing protocol scheme",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSScheme := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-scheme",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "ftp://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS scheme", testcase{
		objs: []any{
			jwtVerificationInvalidRemoteJWKSScheme,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSScheme): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSSchemeInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.URI has invalid scheme \"ftp\", must be http or https",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSTimeout := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-timeout",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:     "http://jwt.example.com/jwks.json",
							Timeout: "invalid-timeout-string",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS timeout", testcase{
		objs: []any{
			jwtVerificationInvalidRemoteJWKSTimeout,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSTimeout): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSTimeoutInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.Timeout is invalid: time: invalid duration \"invalid-timeout-string\"",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSCacheDuration := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-cache-duration",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:           "http://jwt.example.com/jwks.json",
							CacheDuration: "invalid-duration-string",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS cache duration", testcase{
		objs: []any{
			jwtVerificationInvalidRemoteJWKSCacheDuration,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSCacheDuration): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSCacheDurationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.CacheDuration is invalid: time: invalid duration \"invalid-duration-string\"",
				),
		},
	})

	jwtVerificationInvalidRemoteJWKSDNSLookupFamily := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-remote-jwks-dns-lookup-family",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:             "http://jwt.example.com/jwks.json",
							DNSLookupFamily: "v7",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid remote JWKS DNS lookup family", testcase{
		objs: []any{
			jwtVerificationInvalidRemoteJWKSDNSLookupFamily,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRemoteJWKSDNSLookupFamily): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSDNSLookupFamilyInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.DNSLookupFamily has an invalid value \"v7\", must be auto, all, v4 or v6",
				),
		},
	})

	jwtVerificationNoProvidersRouteHasRef := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-no-providers-route-has-ref",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
				},
			},
		},
	}

	run(t, "JWT verification no providers defined, route has provider ref", testcase{
		objs: []any{
			jwtVerificationNoProvidersRouteHasRef,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationNoProvidersRouteHasRef): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"JWTProviderNotDefined",
					"Route references an undefined JWT provider \"provider-1\"",
				),
		},
	})

	jwtVerificationRouteReferencesNonexistentProvider := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-route-references-nonexistent-provider",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "http://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "nonexistent-provider"},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification route references nonexistent provider", testcase{
		objs: []any{
			jwtVerificationRouteReferencesNonexistentProvider,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationRouteReferencesNonexistentProvider): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"JWTProviderNotDefined",
					"Route references an undefined JWT provider \"nonexistent-provider\"",
				),
		},
	})

	jwtVerificationInvalidTLSNotConfigured := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-tls-not-configured",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid TLS not configured", testcase{
		objs: []any{
			jwtVerificationInvalidTLSNotConfigured,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidTLSNotConfigured): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"JWTVerificationNotPermitted",
					"Spec.VirtualHost.JWTProviders can only be defined for root HTTPProxies that terminate TLS",
				),
		},
	})

	jwtVerificationInvalidTLSPassthroughConfigured := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-tls-passthrough-configured",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid TLS passthrough configured", testcase{
		objs: []any{
			jwtVerificationInvalidTLSPassthroughConfigured,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidTLSPassthroughConfigured): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"JWTVerificationNotPermitted",
					"Spec.VirtualHost.JWTProviders can only be defined for root HTTPProxies that terminate TLS",
				),
		},
	})

	jwtVerificationInvalidTLSFallbackConfigured := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-tls-fallback-configured",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					EnableFallbackCertificate: true,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Require: "provider-1"},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid TLS fallback configured", testcase{
		objs: []any{
			jwtVerificationInvalidTLSFallbackConfigured,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidTLSFallbackConfigured): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"JWTVerificationNotPermitted",
					"Spec.VirtualHost.JWTProviders can only be defined for root HTTPProxies that terminate TLS",
				),
		},
	})

	jwtVerificationInvalidRequireAndDisabledSpecified := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-invalid-require-and-disabled-specified",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{
						Require:  "provider-1",
						Disabled: true,
					},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid route specifies both requires and disabled", testcase{
		objs: []any{
			jwtVerificationInvalidRequireAndDisabledSpecified,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationInvalidRequireAndDisabledSpecified): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"InvalidJWTVerificationPolicy",
					"route's JWT verification policy cannot specify both require and disabled",
				),
		},
	})

	jwtVerificationUpstreamValidationForHTTPJWKS := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-for-http-jwks",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "http://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_v1.UpstreamValidation{
								CACertificate: "foo",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation specified for HTTP JWKS", testcase{
		objs: []any{
			jwtVerificationUpstreamValidationForHTTPJWKS,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationForHTTPJWKS): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSUpstreamValidationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation must not be specified when URI scheme is http.",
				),
		},
	})

	jwtVerificationUpstreamValidationCACertDoesNotExist := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-cacert-does-not-exist",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_v1.UpstreamValidation{
								CACertificate: "nonexistent",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation CA cert does not exist", testcase{
		objs: []any{
			jwtVerificationUpstreamValidationCACertDoesNotExist,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationCACertDoesNotExist): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSUpstreamValidationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation is invalid: invalid CA Secret \"roots/nonexistent\": Secret not found",
				),
		},
	})

	jwksInvalidCACert := &core_v1.Secret{
		ObjectMeta: fixture.ObjectMeta("roots/cacert"),
		Type:       core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"wrong-key": []byte(fixture.CERTIFICATE),
		},
	}

	jwtVerificationUpstreamValidationCACertInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-cacert-invalid",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_v1.UpstreamValidation{
								CACertificate: "cacert",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation CA cert invalid", testcase{
		objs: []any{
			jwtVerificationUpstreamValidationCACertInvalid,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
			jwksInvalidCACert,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationCACertInvalid): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSUpstreamValidationInvalid",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation is invalid: invalid CA Secret \"roots/cacert\": empty \"ca.crt\" key",
				),
		},
	})

	jwksCACertDifferentNamespace := &core_v1.Secret{
		ObjectMeta: fixture.ObjectMeta("default/cacert"),
		Data: map[string][]byte{
			"ca.crt": []byte(fixture.CERTIFICATE),
		},
	}

	jwtVerificationUpstreamValidationCACertNotDelegated := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "jwt-verification-upstream-validation-cacert-not-delegated",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
				JWTProviders: []contour_v1.JWTProvider{
					{
						Name: "provider-1",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI: "https://jwt.example.com/jwks.json",
							UpstreamValidation: &contour_v1.UpstreamValidation{
								CACertificate: "default/cacert",
								SubjectName:   "jwt.example.com",
							},
						},
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{
						Require: "provider-1",
					},
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "JWT verification invalid upstream validation CA cert not delegated", testcase{
		objs: []any{
			jwtVerificationUpstreamValidationCACertNotDelegated,
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
			jwksCACertDifferentNamespace,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(jwtVerificationUpstreamValidationCACertNotDelegated): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeJWTVerificationError,
					"RemoteJWKSCACertificateNotDelegated",
					"Spec.VirtualHost.JWTProviders.RemoteJWKS.UpstreamValidation.CACertificate Secret \"default/cacert\" is not configured for certificate delegation",
				),
		},
	})

	ipFilterVirtualHostValidProxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "ip-filter-valid-proxy",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				IPAllowFilterPolicy: []contour_v1.IPFilterPolicy{
					{
						Source: contour_v1.IPFilterSourcePeer,
						CIDR:   "10.8.8.8/0",
					},
					{
						Source: contour_v1.IPFilterSourceRemote,
						CIDR:   "10.8.8.8/0",
					},
				},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "virtualhost ip-filter valid proxy", testcase{
		objs: []any{
			ipFilterVirtualHostValidProxy,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(ipFilterVirtualHostValidProxy): fixture.NewValidCondition().Valid(),
		},
	})

	ipFilterVirtualHostAllowAndDenyInvalidProxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "ip-filter-invalid-allow-and-deny-proxy",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				IPAllowFilterPolicy: []contour_v1.IPFilterPolicy{{
					Source: contour_v1.IPFilterSourcePeer,
					CIDR:   "10.8.8.8/0",
				}},
				IPDenyFilterPolicy: []contour_v1.IPFilterPolicy{{
					Source: contour_v1.IPFilterSourceRemote,
					CIDR:   "10.8.8.8/0",
				}},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "virtualhost ip-filter invalid allow and deny proxy", testcase{
		objs: []any{
			ipFilterVirtualHostAllowAndDenyInvalidProxy,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(ipFilterVirtualHostAllowAndDenyInvalidProxy): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeIPFilterError,
					"IncompatibleIPAddressFilters",
					"Spec.VirtualHost.IPAllowFilterPolicy and Spec.VirtualHost.IPDepnyFilterPolicy cannot both be defined.",
				),
		},
	})

	ipFilterVirtualHostFilterRulesInvalidProxy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "ip-filter-invalid-filter-rules-proxy",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				IPAllowFilterPolicy: []contour_v1.IPFilterPolicy{{
					Source: contour_v1.IPFilterSourcePeer,
					CIDR:   "abcd",
				}},
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{{
						Prefix: "/foo",
					}},
					Services: []contour_v1.Service{{
						Name: "home",
						Port: 8080,
					}},
				},
			},
		},
	}

	run(t, "virtualhost ip-filter invalid filter rules proxy", testcase{
		objs: []any{
			ipFilterVirtualHostFilterRulesInvalidProxy,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(ipFilterVirtualHostFilterRulesInvalidProxy): {
				Condition: contour_v1.Condition{
					Type:    contour_v1.ValidConditionType,
					Status:  contour_v1.ConditionFalse,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []contour_v1.SubCondition{
					{
						Type:    contour_v1.ConditionTypeIPFilterError,
						Status:  contour_v1.ConditionTrue,
						Reason:  "InvalidCIDR",
						Message: "abcd failed to parse: invalid CIDR address: abcd/32",
					},
					{
						Type:    contour_v1.ConditionTypeIPFilterError,
						Status:  contour_v1.ConditionTrue,
						Reason:  "IPFilterPolicyNotValid",
						Message: "Spec.VirtualHost.IPAllowFilterPolicy or Spec.VirtualHost.IPDenyFilterPolicy is invalid: invalid CIDR address: abcd/32",
					},
				},
			},
		},
	})

	// proxyWithInvalidSlowStartWindow is invalid because it has invalid window size syntax.
	proxyWithInvalidSlowStartWindow := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "slow-start-invalid-window",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
					SlowStartPolicy: &contour_v1.SlowStartPolicy{
						Window: "invalid",
					},
				}},
			}},
		},
	}

	// proxyWithInvalidSlowStartAggression is invalid because it has invalid aggression syntax.
	proxyWithInvalidSlowStartAggression := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "slow-start-invalid-aggression",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
					SlowStartPolicy: &contour_v1.SlowStartPolicy{
						Window:     "5s",
						Aggression: "invalid",
					},
				}},
			}},
		},
	}

	// proxyWithInvalidSlowStartLBStrategy is invalid because route has LB strategy that does not support slow start.
	proxyWithInvalidSlowStartLBStrategy := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "slow-start-invalid-lb-strategy",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_v1.Route{{
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: LoadBalancerPolicyCookie,
				},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
					SlowStartPolicy: &contour_v1.SlowStartPolicy{
						Window: "5s",
					},
				}},
			}},
		},
	}

	run(t, "Slow start with invalid window syntax", testcase{
		objs: []any{
			proxyWithInvalidSlowStartWindow,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyWithInvalidSlowStartWindow): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeServiceError,
					"SlowStartInvalid",
					"error parsing window: time: invalid duration \"invalid\" on slow start",
				),
		},
	})

	run(t, "Slow start with invalid aggression syntax", testcase{
		objs: []any{
			proxyWithInvalidSlowStartAggression,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyWithInvalidSlowStartAggression): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeServiceError,
					"SlowStartInvalid",
					"error parsing aggression: \"invalid\" is not a decimal number on slow start",
				),
		},
	})

	run(t, "Slow start with load balancer strategy that does not support slow start", testcase{
		objs: []any{
			proxyWithInvalidSlowStartLBStrategy,
			fixture.ServiceRootsHome,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(proxyWithInvalidSlowStartLBStrategy): fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeServiceError,
					"SlowStartInvalid",
					"slow start is only supported with RoundRobin or WeightedLeastRequest load balancer strategy",
				),
		},
	})

	// Invalid, Regex is in include match condition block
	proxyRegexIncludeInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "regex include invalid",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "regex-invalid.com",
			},
			Includes: []contour_v1.Include{{
				Name: "subproxy1",
				Conditions: []contour_v1.MatchCondition{{
					Regex: "/.*/foo",
				}},
			}, {
				Name: "subproxy2",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	// Valid regex proxy with regex in the sub proxy.
	proxyRegexIncludeValid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "regex include valid",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "regex-valid.com",
			},
			Includes: []contour_v1.Include{{
				Name: "subproxy1",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "subproxy2",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	Subproxy1Regex := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "subproxy1",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Regex: "/.*baz",
				}},
				Services: []contour_v1.Service{{
					Name: "foo1",
					Port: 8080,
				}},
			}},
		},
	}

	Subproxy2Regex := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "subproxy2",
			Generation: 1,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy has regex in the includes block, should be invalid", testcase{
		objs: []any{proxyRegexIncludeInvalid, proxyRegexIncludeValid, Subproxy1Regex, Subproxy2Regex, fixture.ServiceRootsFoo1, fixture.ServiceRootsFoo2},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: Subproxy1Regex.Name, Namespace: Subproxy1Regex.Namespace}: fixture.NewValidCondition().
				WithGeneration(Subproxy1Regex.Generation).
				Valid(),
			{Name: Subproxy2Regex.Name, Namespace: Subproxy2Regex.Namespace}: fixture.NewValidCondition().
				WithGeneration(Subproxy2Regex.Generation).
				Valid(),
			{Name: proxyRegexIncludeInvalid.Name, Namespace: proxyRegexIncludeInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRegexIncludeInvalid.Generation).
				WithError(contour_v1.ConditionTypeIncludeError, "PathMatchConditionsNotValid", `include: regex conditions are not allowed in includes block`),
			{Name: proxyRegexIncludeValid.Name, Namespace: proxyRegexIncludeValid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRegexIncludeValid.Generation).
				Valid(),
		},
	})

	run(t, "HTTPProxy cannot attach to a Gateway with >1 HTTP Listener", testcase{
		objs: []any{
			&gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "http-2",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     81,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			&core_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "kuard",
					Namespace: "roots",
				},
				Spec: core_v1.ServiceSpec{
					Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
				},
			},
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "roots",
					Name:      "kuard-proxy",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "kuard.projectcontour.io",
					},
					Routes: []contour_v1.Route{
						{
							Conditions: []contour_v1.MatchCondition{
								{
									Prefix: "/",
								},
							},
							Services: []contour_v1.Service{
								{
									Name: "kuard",
									Port: 8080,
								},
							},
						},
					},
				},
			},
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Namespace: "roots", Name: "kuard-proxy"}: fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeListenerError,
					"ErrorIdentifyingListener",
					"more than one HTTP listener configured",
				),
		},
	})

	run(t, "HTTPProxy cannot attach to a Gateway with no HTTP Listener", testcase{
		objs: []any{
			&gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     443,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
							},
						},
					},
				},
			},
			&core_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "kuard",
					Namespace: "roots",
				},
				Spec: core_v1.ServiceSpec{
					Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
				},
			},
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "roots",
					Name:      "kuard-proxy",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "kuard.projectcontour.io",
					},
					Routes: []contour_v1.Route{
						{
							Conditions: []contour_v1.MatchCondition{
								{
									Prefix: "/",
								},
							},
							Services: []contour_v1.Service{
								{
									Name: "kuard",
									Port: 8080,
								},
							},
						},
					},
				},
			},
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Namespace: "roots", Name: "kuard-proxy"}: fixture.NewValidCondition().
				WithError(
					contour_v1.ConditionTypeListenerError,
					"ErrorIdentifyingListener",
					"no HTTP listener configured",
				),
		},
	})

	clientValidationWithDelegatedCA := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: "ssl-cert",
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: "delegated/delegated",
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "ClientValidation with CA but missing delegation", testcase{
		objs: []any{
			fixture.SecretRootsCert,
			clientValidationWithDelegatedCA,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "DelegationNotPermitted", `Spec.VirtualHost.TLS CA Secret "delegated/delegated" is invalid: Certificate delegation not permitted`),
		},
	})

	run(t, "ClientValidation with delegated CA but missing secret", testcase{
		objs: []any{
			fixture.SecretRootsCert,
			clientValidationWithDelegatedCA,
			&contour_v1.TLSCertificateDelegation{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "delegated",
					Name:      "ca-cert-delegation",
				},
				Spec: contour_v1.TLSCertificateDelegationSpec{
					Delegations: []contour_v1.CertificateDelegation{
						{
							SecretName:       "delegated",
							TargetNamespaces: []string{"roots"},
						},
					},
				},
			},
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "ClientValidationInvalid", `Spec.VirtualHost.TLS client validation is invalid: invalid CA Secret "delegated/delegated": Secret not found`),
		},
	})

	clientValidationWithDelegatedCRL := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: "ssl-cert",
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             "ca-cert",
						CertificateRevocationList: "delegated/delegated",
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	caCertSecret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "ca-cert",
		},
		Type: core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ca.crt": []byte(fixture.CERTIFICATE),
		},
	}

	run(t, "ClientValidation with CRL but missing delegation", testcase{
		objs: []any{
			fixture.SecretRootsCert,
			caCertSecret,
			clientValidationWithDelegatedCRL,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "DelegationNotPermitted", `Spec.VirtualHost.TLS CRL Secret "delegated/delegated" is invalid: Certificate delegation not permitted`),
		},
	})

	run(t, "ClientValidation with delegated CRL but missing secret", testcase{
		objs: []any{
			fixture.SecretRootsCert,
			caCertSecret,
			clientValidationWithDelegatedCRL,
			&contour_v1.TLSCertificateDelegation{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "delegated",
					Name:      "crl-cert-delegation",
				},
				Spec: contour_v1.TLSCertificateDelegationSpec{
					Delegations: []contour_v1.CertificateDelegation{
						{
							SecretName:       "delegated",
							TargetNamespaces: []string{"roots"},
						},
					},
				},
			},
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{
				Name:      fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace,
			}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeTLSError, "ClientValidationInvalid", `Spec.VirtualHost.TLS client validation is invalid: invalid CRL Secret "delegated/delegated": Secret not found`),
		},
	})

	clientValidationWithDelegatedCAandCRL := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: "ssl-cert",
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             "delegated/delegated",
						CertificateRevocationList: "delegated/delegated",
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	caCertCRLSecret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: "delegated",
			Name:      "delegated",
		},
		Type: core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			CACertificateKey: []byte(fixture.CERTIFICATE),
			CRLKey:           []byte(fixture.CRL),
		},
	}

	run(t, "ClientValidation with delegated CA and CRL", testcase{
		objs: []any{
			fixture.SecretRootsCert,
			fixture.ServiceRootsHome,
			clientValidationWithDelegatedCAandCRL,
			caCertCRLSecret,
			&contour_v1.TLSCertificateDelegation{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "delegated",
					Name:      "ca-crl-cert-delegation",
				},
				Spec: contour_v1.TLSCertificateDelegationSpec{
					Delegations: []contour_v1.CertificateDelegation{
						{
							SecretName:       "delegated",
							TargetNamespaces: []string{"roots"},
						},
					},
				},
			},
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			k8s.NamespacedNameOf(clientValidationWithDelegatedCAandCRL): fixture.NewValidCondition().Valid(),
		},
	})

	tlsProtocolVersion := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				TLS: &contour_v1.TLS{
					MinimumProtocolVersion: "1.3",
					MaximumProtocolVersion: "1.2",
					SecretName:             fixture.SecretRootsCert.Name,
				},
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "valid TLS protocol version", testcase{
		objs: []any{
			ingressSharedService,
			tlsProtocolVersion,
			fixture.ServiceRootsHome,
			fixture.SecretRootsCert,
		},
		want: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: tlsProtocolVersion.Name, Namespace: tlsProtocolVersion.Namespace}: fixture.NewValidCondition().
				WithGeneration(tlsProtocolVersion.Generation).
				WithError(
					contour_v1.ConditionTypeTLSError, "TLSConfigNotValid",
					`Spec.Virtualhost.TLS the minimum protocol version is greater than the maximum protocol version`,
				),
		},
	})
}

func validGatewayStatusUpdate(listenerName string, listenerProtocol gatewayapi_v1.ProtocolType, attachedRoutes int32) []*status.GatewayStatusUpdate {
	var supportedKinds []gatewayapi_v1.RouteGroupKind

	switch listenerProtocol {
	case gatewayapi_v1.HTTPProtocolType, gatewayapi_v1.HTTPSProtocolType:
		supportedKinds = append(supportedKinds,
			gatewayapi_v1.RouteGroupKind{
				Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
				Kind:  KindHTTPRoute,
			},
			gatewayapi_v1.RouteGroupKind{
				Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
				Kind:  KindGRPCRoute,
			},
		)
	case gatewayapi_v1.TLSProtocolType:
		supportedKinds = append(supportedKinds,
			gatewayapi_v1.RouteGroupKind{
				Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
				Kind:  KindTLSRoute,
			},
			gatewayapi_v1.RouteGroupKind{
				Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
				Kind:  KindTCPRoute,
			},
		)
	case gatewayapi_v1.TCPProtocolType:
		supportedKinds = append(supportedKinds,
			gatewayapi_v1.RouteGroupKind{
				Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
				Kind:  KindTCPRoute,
			},
		)
	}

	return []*status.GatewayStatusUpdate{
		{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionTrue,
					Reason:  string(gatewayapi_v1.GatewayReasonProgrammed),
					Message: status.MessageValidGateway,
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				listenerName: {
					Name:           gatewayapi_v1.SectionName(listenerName),
					AttachedRoutes: attachedRoutes,
					SupportedKinds: supportedKinds,
					Conditions:     listenerValidConditions(),
				},
			},
		},
	}
}

func TestGatewayAPIHTTPRouteDAGStatus(t *testing.T) {
	type testcase struct {
		objs                    []any
		gateway                 *gatewayapi_v1.Gateway
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
					gatewayclass: &gatewayapi_v1.GatewayClass{
						TypeMeta: meta_v1.TypeMeta{},
						ObjectMeta: meta_v1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1.GatewayClassStatus{
							Conditions: []meta_v1.Condition{
								{
									Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
									Status: meta_v1.ConditionTrue,
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
				builder.Source.gateway = &gatewayapi_v1.Gateway{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GatewaySpec{
						Listeners: []gatewayapi_v1.Listener{{
							Name:     "http",
							Port:     80,
							Protocol: gatewayapi_v1.HTTPProtocolType,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
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
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "TransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Resource"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "ExistingConditions"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j meta_v1.Condition) bool {
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

	kuardService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard2",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService3 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard3",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService4 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard4",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080, protoK8sH2C)},
		},
	}

	kuardService5 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard5",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{
				makeServicePort("wss", "TCP", 8444, 8444, "kubernetes.io/wss"),
			},
		},
	}

	run(t, "simple httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "simple httproute with backendref namespace matching route's explicitly specified", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{{
				ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
				Conditions: []meta_v1.Condition{
					routeResolvedRefsCondition(),
					routeAcceptedHTTPRouteCondition(),
				},
			}},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "multiple httproutes", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic-2",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						// changed to different matches in case there is conflict with above HTTPRoute
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchExact, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeResolvedRefsCondition(),
							routeAcceptedHTTPRouteCondition(),
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-2"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeResolvedRefsCondition(),
							routeAcceptedHTTPRouteCondition(),
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 2),
	})

	run(t, "3 httproutes, but duplicate match condition between these 3. The 2 rank lower gets Conflict condition ", testcase{
		objs: []any{
			kuardService,
			// first HTTPRoute with oldest creationTimestamp
			&gatewayapi_v1.HTTPRoute{
				TypeMeta: meta_v1.TypeMeta{
					Kind: KindHTTPRoute,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:              "basic-1",
					Namespace:         "default",
					CreationTimestamp: meta_v1.Date(2020, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1.HTTPPathMatch{
									Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
								Headers: []gatewayapi_v1.HTTPHeaderMatch{
									{
										Type:  ptr.To(gatewayapi_v1.HeaderMatchExact),
										Name:  gatewayapi_v1.HTTPHeaderName("foo"),
										Value: "bar",
									},
								},
								QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
									{
										Type:  ptr.To(gatewayapi_v1.QueryParamMatchRegularExpression),
										Name:  "param-1",
										Value: "valid-[a-z]?-[A-Za-z]+-[0=9]+-\\d+",
									},
								},
							},
							{
								Path: &gatewayapi_v1.HTTPPathMatch{
									Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
								Headers: []gatewayapi_v1.HTTPHeaderMatch{
									{
										Type:  ptr.To(gatewayapi_v1.HeaderMatchExact),
										Name:  gatewayapi_v1.HTTPHeaderName("a"),
										Value: "b",
									},
								},
							},
						},

						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			// second HTTPRoute with 2nd oldest creationTimestamp, conflict with 1st HTTPRoute
			&gatewayapi_v1.HTTPRoute{
				TypeMeta: meta_v1.TypeMeta{
					Kind: KindHTTPRoute,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:              "basic-2",
					Namespace:         "default",
					CreationTimestamp: meta_v1.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{
							{
								Path: &gatewayapi_v1.HTTPPathMatch{
									Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
								Headers: []gatewayapi_v1.HTTPHeaderMatch{
									{
										Type:  ptr.To(gatewayapi_v1.HeaderMatchExact),
										Name:  gatewayapi_v1.HTTPHeaderName("a"),
										Value: "b",
									},
								},
							},
						},

						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			// 3rd HTTPRoute with newest creationTimestamp, partially conflict with 1st HTTPRoute
			&gatewayapi_v1.HTTPRoute{
				TypeMeta: meta_v1.TypeMeta{
					Kind: KindHTTPRoute,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:              "basic-3",
					Namespace:         "default",
					CreationTimestamp: meta_v1.Date(2022, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{
						{
							Matches: []gatewayapi_v1.HTTPRouteMatch{
								{
									Path: &gatewayapi_v1.HTTPPathMatch{
										Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
									Headers: []gatewayapi_v1.HTTPHeaderMatch{
										{
											Type:  ptr.To(gatewayapi_v1.HeaderMatchExact),
											Name:  gatewayapi_v1.HTTPHeaderName("foo"),
											Value: "bar",
										},
									},
									QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
										{
											Type:  ptr.To(gatewayapi_v1.QueryParamMatchRegularExpression),
											Name:  "param-1",
											Value: "valid-[a-z]?-[A-Za-z]+-[0=9]+-\\d+",
										},
									},
								},
							},

							BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						},
						{
							Matches: []gatewayapi_v1.HTTPRouteMatch{
								{
									Path: &gatewayapi_v1.HTTPPathMatch{
										Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
										Value: ptr.To("/random"),
									},
									Headers: []gatewayapi_v1.HTTPHeaderMatch{
										{
											Type:  ptr.To(gatewayapi_v1.HeaderMatchExact),
											Name:  gatewayapi_v1.HTTPHeaderName("random"),
											Value: "b",
										},
									},
								},
							},

							BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-1"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeResolvedRefsCondition(),
							routeAcceptedHTTPRouteCondition(),
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-2"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeAcceptedFalse(status.ReasonRouteRuleMatchConflict, fmt.Sprintf(status.MessageRouteRuleMatchConflict, KindHTTPRoute, KindHTTPRoute)),
							routeResolvedRefsCondition(),
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-3"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeAcceptedHTTPRouteCondition(),
							routePartialMatchConflict(status.ReasonRouteRuleMatchPartiallyConflict, fmt.Sprintf(status.MessageRouteRuleMatchPartiallyConflict, KindHTTPRoute, KindHTTPRoute)),
							routeResolvedRefsCondition(),
						},
					},
				},
			},
		},
		// is it ok to show the listeners are attached, just it's not accepted because of the conflict
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 3),
	})

	run(t, "prefix path match not starting with '/' for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("doesnt-start-with-slash"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeAcceptedHTTPRouteCondition(),
						routeResolvedRefsCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must start with '/'.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})
	run(t, "exact path match not starting with '/' for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchExact),
								Value: ptr.To("doesnt-start-with-slash"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must start with '/'.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "regular expression path match with invalid value for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchRegularExpression, "invalid-regex???"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value is invalid for RegularExpression match type.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "prefix path match with consecutive '/' characters for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/foo///bar"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must not contain consecutive '/' characters.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "exact path match with consecutive '/' characters for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchExact),
								Value: ptr.To("//foo/bar"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
						{
							Type:    string(status.ConditionValidMatches),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidPathMatch),
							Message: "Match.Path.Value must not contain consecutive '/' characters.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "invalid path match type for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchType("UNKNOWN")), // <---- unknown type to break the test
								Value: ptr.To("/"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.PathMatch: Only Prefix match type, Exact match type and RegularExpression match type are supported."),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "invalid header match type not supported for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
							Headers: []gatewayapi_v1.HTTPHeaderMatch{
								{
									Type:  ptr.To(gatewayapi_v1.HeaderMatchType("UNKNOWN")), // <---- unknown type to break the test
									Name:  gatewayapi_v1.HTTPHeaderName("foo"),
									Value: "bar",
								},
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.Matches.Headers: Only Exact match type and RegularExpression match type are supported"),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "regular expression header match with invalid value for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
							Headers: []gatewayapi_v1.HTTPHeaderMatch{
								{
									Type:  ptr.To(gatewayapi_v1.HeaderMatchRegularExpression),
									Name:  gatewayapi_v1.HTTPHeaderName("foo"),
									Value: "invalid-regrex\\",
								},
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.Matches.Headers: Invalid value for RegularExpression match type is specified"),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "regular expression query param match with valid value for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
							QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
								{
									Type:  ptr.To(gatewayapi_v1.QueryParamMatchRegularExpression),
									Name:  "param-1",
									Value: "valid-[a-z]?-[A-Za-z]+-[0=9]+-\\d+",
								},
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "regular expression query param match with invalid value for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
							QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
								{
									Type:  ptr.To(gatewayapi_v1.QueryParamMatchRegularExpression),
									Name:  "param-1",
									Value: "invalid-regex????",
								},
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.Matches.QueryParams: Invalid value for RegularExpression match type is specified"),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "query param match with invalid type for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
							QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
								{
									Type:  ptr.To(gatewayapi_v1.QueryParamMatchType("Invalid")),
									Name:  "param-1",
									Value: "invalid query param type",
								},
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.Matches.QueryParams: Only Exact and RegularExpression match types are supported"),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "spec.rules.backendRef.name not specified", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind: ptr.To(gatewayapi_v1.Kind("Service")),
										Port: ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "Spec.Rules.BackendRef.Name must be specified"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "spec.rules.backendRef.serviceName invalid on two matches", testcase{
		objs: []any{
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("invalid-one", 8080, 1),
					}, {
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/blog"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("invalid-two", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(gatewayapi_v1.RouteReasonBackendNotFound, "service \"invalid-one\" is invalid: service \"default/invalid-one\" not found, service \"invalid-two\" is invalid: service \"default/invalid-two\" not found"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "spec.rules.backendRef.port not specified", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind: ptr.To(gatewayapi_v1.Kind("Service")),
										Name: "kuard",
									},
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "Spec.Rules.BackendRef.Port must be specified"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "spec.rules.backendRefs not specified", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified."),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "spec.rules.backendRef.namespace does not match route", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace("some-other-namespace")),
										Name:      "service",
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							gatewayapi_v1.RouteConditionReason(gatewayapi_v1.ListenerReasonRefNotPermitted),
							"Spec.Rules.BackendRef.Namespace must match the route's namespace or be covered by a ReferenceGrant"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	// BEGIN TLS CertificateRef + ReferenceGrant tests
	run(t, "Gateway references TLS cert in different namespace, with valid ReferenceGrant", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("https", gatewayapi_v1.HTTPSProtocolType, 0),
	})

	run(t, "Gateway references TLS cert in different namespace, with no ReferenceGrant", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with valid ReferenceGrant (secret-specific)", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
						Name: ptr.To(gatewayapi_v1.ObjectName("secret")),
					}},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("https", gatewayapi_v1.HTTPSProtocolType, 0),
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (policy in wrong namespace)", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "wrong-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong From namespace)", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1.Namespace("wrong-namespace"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong From kind)", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1.GroupName,
						Kind:      "WrongKind",
						Namespace: gatewayapi_v1.Namespace("projectontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong To kind)", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "WrongKind",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	run(t, "Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong secret name)", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("projectcontour.io/contour"),
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret",
					Namespace: "tls-cert-namespace",
				},
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			&gatewayapi_v1beta1.ReferenceGrant{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
					From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
						Group:     gatewayapi_v1.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1beta1.ReferenceGrantTo{{
						Kind: "Secret",
						Name: ptr.To(gatewayapi_v1.ObjectName("wrong-name")),
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonRefNotPermitted),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferenceGrant",
						},
					},
				},
			},
		}},
	})

	// END TLS CertificateRef + ReferenceGrant tests

	run(t, "spec.rules.hostname: invalid wildcard", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"*.*.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "spec.rules.hostname: invalid hostname", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"#projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "spec.rules.hostname: invalid hostname, ip address", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"1.2.3.4",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "invalid hostname \"1.2.3.4\": must be a DNS name, not an IP address"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "two HTTP listeners, route's hostname intersects with one of them", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					},
					{
						Name:     "listener-2",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("specific.hostname.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
					gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
						Status:  contour_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1.SectionName("listener-1"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: listenerValidConditions(),
					},
					"listener-2": {
						Name:           gatewayapi_v1.SectionName("listener-2"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: listenerValidConditions(),
					},
				},
			},
		},
	})

	run(t, "two HTTP listeners, route's hostname intersects with neither of them", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.randomdomain.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					},
					{
						Name:     "listener-2",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("specific.hostname.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
					gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
						Status:  contour_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1.SectionName("listener-1"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: listenerValidConditions(),
					},
					"listener-2": {
						Name:           gatewayapi_v1.SectionName("listener-2"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: listenerValidConditions(),
					},
				},
			},
		},
	})

	run(t, "HTTP listener, route's parent ref sectionname does not match", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 0)},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 0),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
					gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
						Status:  contour_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1.SectionName("listener-1"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: listenerValidConditions(),
					},
				},
			},
		},
	})

	run(t, "HTTP listener, route's parent ref port does not match", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "", 443)},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "", 443),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
					gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
						Status:  contour_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1.SectionName("listener-1"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: listenerValidConditions(),
					},
				},
			},
		},
	})

	run(t, "HTTP listener, route's parent ref section name and port both must match", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 80)},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic-2",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 443)},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic-3",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 80)},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "nonexistent", 80),
						Conditions: []meta_v1.Condition{
							routeResolvedRefsCondition(),
							{
								Type:    string(gatewayapi_v1.RouteConditionAccepted),
								Status:  contour_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
								Message: "No listeners match this parent ref",
							},
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-2"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 443),
						Conditions: []meta_v1.Condition{
							routeResolvedRefsCondition(),
							{
								Type:    string(gatewayapi_v1.RouteConditionAccepted),
								Status:  contour_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
								Message: "No listeners match this parent ref",
							},
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-3"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 80),
						Conditions: []meta_v1.Condition{
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
				Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
					gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
						Status:  contour_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1.GatewayReasonProgrammed),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1.SectionName("listener-1"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "HTTPRoute",
							},
							{
								Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
								Kind:  "GRPCRoute",
							},
						},
						Conditions: listenerValidConditions(),
					},
				},
			},
		},
	})

	run(t, "HTTPRoute: backendrefs still validated when route not accepted", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 81)},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("invalid", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 81),
						Conditions: []meta_v1.Condition{
							{
								Type:    string(gatewayapi_v1.RouteConditionResolvedRefs),
								Status:  contour_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1.RouteReasonBackendNotFound),
								Message: "service \"invalid\" is invalid: service \"default/invalid\" not found",
							},
							{
								Type:    string(gatewayapi_v1.RouteConditionAccepted),
								Status:  contour_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
								Message: "No listeners match this parent ref",
							},
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("listener-1", gatewayapi_v1.HTTPProtocolType, 0),
	})

	run(t, "More than one RequestMirror filters in HTTPRoute.Spec.Rules.Filters", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			kuardService3,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{
							{
								Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
								RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
									BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
								},
							}, {
								Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
								RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
									BackendRef: gatewayapi.ServiceBackendObjectRef("kuard3", 8080),
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid RequestMirror filter due to unspecified backendRef.name", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1.BackendObjectReference{
									Group: ptr.To(gatewayapi_v1.Group("")),
									Kind:  ptr.To(gatewayapi_v1.Kind("Service")),
									Port:  ptr.To(gatewayapi_v1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "Spec.Rules.Filters.RequestMirror.BackendRef.Name must be specified"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid RequestMirror filter due to unspecified backendRef.port", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1.BackendObjectReference{
									Group: ptr.To(gatewayapi_v1.Group("")),
									Kind:  ptr.To(gatewayapi_v1.Kind("Service")),
									Name:  gatewayapi_v1.ObjectName("kuard2"),
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "Spec.Rules.Filters.RequestMirror.BackendRef.Port must be specified"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid RequestMirror filter due to invalid backendRef.name on two matches", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("invalid-one", 8080),
							},
						}},
					}, {
						BackendRefs: gatewayapi.HTTPBackendRef("kuard2", 8080, 1),
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/blog"),
							},
						}},
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("invalid-two", 8080),
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							gatewayapi_v1.RouteReasonBackendNotFound,
							"service \"invalid-one\" is invalid: service \"default/invalid-one\" not found, service \"invalid-two\" is invalid: service \"default/invalid-two\" not found"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid RequestMirror filter due to unmatched backendRef.namespace", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1.BackendObjectReference{
									Group:     ptr.To(gatewayapi_v1.Group("")),
									Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
									Namespace: ptr.To(gatewayapi_v1.Namespace("some-other-namespace")),
									Name:      gatewayapi_v1.ObjectName("kuard2"),
									Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							gatewayapi_v1.RouteConditionReason(gatewayapi_v1.ListenerReasonRefNotPermitted),
							"Spec.Rules.Filters.RequestMirror.BackendRef.Namespace must match the route's namespace or be covered by a ReferenceGrant"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "HTTPRouteFilterRequestMirror not yet supported for httproute backendref", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1.HTTPRouteFilter{{
									Type: gatewayapi_v1.HTTPRouteFilterRequestMirror, // HTTPRouteFilterRequestMirror is not supported yet.
								}},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier and ResponseHeaderModifier type is supported."),
						routeResolvedRefsCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "HTTPRouteFilterURLRewrite with custom HTTPPathModifierType is not supported", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gatewayapi_v1.HTTPURLRewriteFilter{
								Path: &gatewayapi_v1.HTTPPathModifier{
									Type: "custom",
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.Filters.URLRewrite.Path.Type: invalid type \"custom\": only ReplacePrefixMatch and ReplaceFullPath are supported."),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid RequestHeaderModifier due to duplicated headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "duplicate header addition: \"Custom\" on request headers"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid RequestHeaderModifier after forward due to invalid headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1.HTTPRouteFilter{{
									Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid set header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on request headers"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid ResponseHeaderModifier due to duplicated headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
							ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "duplicate header addition: \"Custom\" on response headers"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "Invalid ResponseHeaderModifier on backend due to invalid headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: []gatewayapi_v1.HTTPBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1.HTTPRouteFilter{{
									Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid set header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on response headers"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "custom filter type is not supported", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.HTTPRouteFilter{{
							Type: "custom-filter",
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.Filters: invalid type \"custom-filter\": only RequestHeaderModifier, ResponseHeaderModifier, RequestRedirect, RequestMirror and URLRewrite are supported."),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "gateway.spec.addresses results in invalid gateway", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Addresses: []gatewayapi_v1.GatewayAddress{{
					Value: "1.2.3.4",
				}},
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  meta_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonAddressNotAssigned),
					Message: "None of the addresses in Spec.Addresses have been assigned to the Gateway",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: listenerValidConditions(),
				},
			},
		}},
	})

	run(t, "invalid allowedroutes API group results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Kinds: []gatewayapi_v1.RouteGroupKind{
							{
								Group: ptr.To(gatewayapi_v1.Group("invalid-group")),
								Kind:  "HTTPRoute",
							},
						},
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonInvalidRouteKinds),
							Message: "Group \"invalid-group\" is not supported, group must be \"gateway.networking.k8s.io\"",
						},
					},
				},
			},
		}},
	})

	run(t, "invalid allowedroutes API kind results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Kinds: []gatewayapi_v1.RouteGroupKind{
							{Kind: "FooRoute"},
						},
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonInvalidRouteKinds),
							Message: "Kind \"FooRoute\" is not supported, kind must be \"HTTPRoute\", \"TLSRoute\", \"GRPCRoute\" or \"TCPRoute\"",
						},
					},
				},
			},
		}},
	})

	run(t, "allowedroute of TLSRoute on a non-TLS listener results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Kinds: []gatewayapi_v1.RouteGroupKind{
							{Kind: "TLSRoute"},
						},
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonInvalidRouteKinds),
							Message: "TLSRoutes are incompatible with listener protocol \"HTTP\"",
						},
					},
				},
			},
		}},
	})

	run(t, "TLS certificate ref to a non-secret on an HTTPS listener results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							{
								Group: ptr.To(gatewayapi_v1.Group("invalid-group")),
								Kind:  ptr.To(gatewayapi_v1.Kind("NotASecret")),
								Name:  "foo",
							},
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"foo\" must contain a reference to a core.Secret",
						},
					},
				},
			},
		}},
	})

	run(t, "nonexistent TLS certificate ref on an HTTPS listener results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("nonexistent-secret", "projectcontour"),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"nonexistent-secret\" referent is invalid: Secret not found",
						},
					},
				},
			},
		}},
	})

	run(t, "invalid listener protocol results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: "invalid",
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1.ListenerConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonUnsupportedProtocol),
							Message: "Listener protocol \"invalid\" is unsupported, must be one of HTTP, HTTPS, TLS, TCP or projectcontour.io/https",
						},
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "HTTPS listener without TLS defined results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS is required when protocol is \"HTTPS\".",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "TLS listener without TLS defined results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1.TLSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TLSRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TCPRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS is required when protocol is \"TLS\".",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "TLS Passthrough listener with a TLS certificate ref defined results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1.TLSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef("tlscert", "projectcontour"),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TLSRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TCPRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS.CertificateRefs cannot be defined when Listener.TLS.Mode is \"Passthrough\".",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "TLS listener with TLS.Mode=Terminate without a certificate ref results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1.TLSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TLSRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TCPRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonInvalid),
							Message: "Listener.TLS.CertificateRefs must contain exactly one entry",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "HTTPS listener with TLS.Mode=Passthrough results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS.Mode must be \"Terminate\" when protocol is \"HTTPS\".",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "Listener with FromNamespaces=Selector, no selector specified", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From:     ptr.To(gatewayapi_v1.NamespacesFromSelector),
								Selector: nil,
							},
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.AllowedRoutes.Namespaces.Selector is required when Listener.AllowedRoutes.Namespaces.From is set to \"Selector\".",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "Listener with FromNamespaces=Selector, invalid selector (can't specify values with Exists operator)", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
								Selector: &meta_v1.LabelSelector{
									MatchExpressions: []meta_v1.LabelSelectorRequirement{{
										Key:      "something",
										Operator: meta_v1.LabelSelectorOpExists,
										Values:   []string{"error"},
									}},
								},
							},
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Error parsing Listener.AllowedRoutes.Namespaces.Selector: values: Invalid value: []string{\"error\"}: values set must be empty for exists and does not exist.",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "Listener with FromNamespaces=Selector, invalid selector (must specify MatchLabels and/or MatchExpressions)", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From:     ptr.To(gatewayapi_v1.NamespacesFromSelector),
								Selector: &meta_v1.LabelSelector{},
							},
						},
					},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "HTTPRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "GRPCRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.AllowedRoutes.Namespaces.Selector must specify at least one MatchLabel or MatchExpression.",
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
				},
			},
		}},
	})

	run(t, "service with supported app protocol: h2c", testcase{
		objs: []any{
			kuardService4,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("kuard4", 8080, 1),
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "service with unsupported app protocol: wss", testcase{
		objs: []any{
			kuardService5,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("kuard5", 8444, 1),
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(gatewayapi_v1.RouteReasonUnsupportedProtocol, "AppProtocol: \"kubernetes.io/wss\" is unsupported"),
						routeAcceptedHTTPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "HTTP listener with invalid AllowedRoute kind referenced by route parent ref", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 80)},
					},
					Hostnames: []gatewayapi_v1.Hostname{"foo.projectcontour.io"},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{
					{
						Name:     "listener-1",
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Kinds: []gatewayapi_v1.RouteGroupKind{
								{Kind: "FooRoute"},
							},
						},
						Hostname: ptr.To(gatewayapi_v1.Hostname("*.projectcontour.io")),
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "listener-1", 80),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNotAllowedByListeners),
							Message: "No listeners included by this parent ref allowed this attachment.",
						},
						routeResolvedRefsCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
					gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
					gatewayapi_v1.GatewayConditionProgrammed: {
						Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
						Status:  contour_v1.ConditionFalse,
						Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
						Message: "Listeners are not valid",
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1.SectionName("listener-1"),
						AttachedRoutes: int32(0),
						Conditions: []meta_v1.Condition{
							{
								Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
								Status:  meta_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1.ListenerReasonInvalidRouteKinds),
								Message: "Kind \"FooRoute\" is not supported, kind must be \"HTTPRoute\", \"TLSRoute\", \"GRPCRoute\" or \"TCPRoute\"",
							},
							{
								Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
								Status:  meta_v1.ConditionFalse,
								Reason:  string(gatewayapi_v1.ListenerReasonInvalid),
								Message: "Invalid listener, see other listener conditions for details",
							},
							listenerAcceptedCondition(),
						},
					},
				},
			},
		},
	})

	run(t, "route rule with timeouts.backendRequest greater than timeouts.request", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
							Timeouts: &gatewayapi_v1.HTTPRouteTimeouts{
								Request:        ptr.To(gatewayapi_v1.Duration("30s")),
								BackendRequest: ptr.To(gatewayapi_v1.Duration("60s")),
							},
						},
					},
				},
			},
		},

		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "HTTPRoute.Spec.Rules.Timeouts.BackendRequest must be less than/equal to HTTPRoute.Spec.Rules.Timeouts.Request when both are specified"),
					},
				},
			},
		}},

		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "route rule with invalid timeouts.backendRequest specified", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{
						{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
							Timeouts: &gatewayapi_v1.HTTPRouteTimeouts{
								BackendRequest: ptr.To(gatewayapi_v1.Duration("invalid")),
							},
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "invalid HTTPRoute.Spec.Rules.Timeouts.BackendRequest: unable to parse timeout string \"invalid\": time: invalid duration \"invalid\""),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "timeouts with invalid request for httproute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.HTTPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.HTTPRouteRule{{
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						Timeouts: &gatewayapi_v1.HTTPRouteTimeouts{
							Request: ptr.To(gatewayapi_v1.Duration("invalid")),
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedFalse(gatewayapi_v1.RouteReasonUnsupportedValue, "invalid HTTPRoute.Spec.Rules.Timeouts.Request: unable to parse timeout string \"invalid\": time: invalid duration \"invalid\""),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})
}

func TestGatewayAPITLSRouteDAGStatus(t *testing.T) {
	type testcase struct {
		objs                    []any
		gateway                 *gatewayapi_v1.Gateway
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
					gatewayclass: &gatewayapi_v1.GatewayClass{
						TypeMeta: meta_v1.TypeMeta{},
						ObjectMeta: meta_v1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1.GatewayClassStatus{
							Conditions: []meta_v1.Condition{
								{
									Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
									Status: meta_v1.ConditionTrue,
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
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "TransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Resource"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "ExistingConditions"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j meta_v1.Condition) bool {
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

	gw := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "tls-passthrough",
				Port:     443,
				Protocol: gatewayapi_v1.TLSProtocolType,
				TLS: &gatewayapi_v1.GatewayTLSConfig{
					Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	kuardService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	run(t, "TLSRoute: spec.rules.backendRef.name not specified", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayapi_v1.BackendRef{
							{
								BackendObjectReference: gatewayapi_v1.BackendObjectReference{
									Kind: ptr.To(gatewayapi_v1.Kind("Service")),
									Port: ptr.To(gatewayapi_v1.PortNumber(8080)),
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "Spec.Rules.BackendRef.Name must be specified"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: spec.rules.backendRef.name invalid on two matches", testcase{
		gateway: gw,
		objs: []any{
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{
						{BackendRefs: gatewayapi.TLSRouteBackendRef("invalid-one", 8080, nil)},
						{BackendRefs: gatewayapi.TLSRouteBackendRef("invalid-two", 8080, nil)},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							gatewayapi_v1.RouteReasonBackendNotFound,
							"service \"invalid-one\" is invalid: service \"default/invalid-one\" not found, service \"invalid-two\" is invalid: service \"default/invalid-two\" not found"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: spec.rules.backendRef.port not specified", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayapi_v1.BackendRef{
							{
								BackendObjectReference: gatewayapi_v1.BackendObjectReference{
									Kind: ptr.To(gatewayapi_v1.Kind("Service")),
									Name: "kuard",
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "Spec.Rules.BackendRef.Port must be specified"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: spec.rules.backendRefs not specified", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{
						{}, // rule with no backend refs
					},
					Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified."),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: spec.rules.hostname: invalid wildcard", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"*.*.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: spec.rules.hostname: invalid hostname", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"#projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: spec.rules.hostname: invalid hostname, ip address", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef("projectcontour", "contour"),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"1.2.3.4"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "invalid hostname \"1.2.3.4\": must be a DNS name, not an IP address"),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingListenerHostname),
							Message: "No intersecting hostnames were found between the listener and the route.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: spec.rules.backendRefs has 0 weight", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayParentRef(gw.Namespace, gw.Name),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef(kuardService.Name, 8080, ptr.To(int32(0))),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(status.ConditionValidBackendRefs),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonAllBackendRefsHaveZeroWeights),
							Message: "At least one Spec.Rules.BackendRef must have a non-zero weight.",
						},
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1.RouteReasonAccepted),
							Message: "Accepted TLSRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 1),
	})

	run(t, "TLSRoute: backendrefs still validated when route not accepted", testcase{
		gateway: gw,
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							// Wrong port.
							gatewayapi.GatewayListenerParentRef(gw.Namespace, gw.Name, "tls-passthrough", 444),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("invalid-one", 8080, nil),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef(gw.Namespace, gw.Name, "tls-passthrough", 444),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.RouteConditionResolvedRefs),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonBackendNotFound),
							Message: "service \"invalid-one\" is invalid: service \"default/invalid-one\" not found",
						},
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate(string(gw.Spec.Listeners[0].Name), gw.Spec.Listeners[0].Protocol, 0),
	})

	run(t, "TLS Listener with invalid TLS mode", testcase{
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1.TLSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeType("invalid-mode")),
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TLSRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							gatewayapi.GatewayListenerParentRef(gw.Namespace, gw.Name, "tls", 443),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
					Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
						BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef(gw.Namespace, gw.Name, "tls", 443),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1.RouteGroupKind{
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TLSRoute",
						},
						{
							Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
							Kind:  "TCPRoute",
						},
					},
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: `Listener.TLS.Mode must be "Terminate" or "Passthrough".`,
						},
						listenerAcceptedCondition(),
						listenerResolvedRefsCondition(),
					},
					AttachedRoutes: 1,
				},
			},
		}},
	})
}

func TestGatewayAPIGRPCRouteDAGStatus(t *testing.T) {
	type testcase struct {
		objs                    []any
		gateway                 *gatewayapi_v1.Gateway
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
					gatewayclass: &gatewayapi_v1.GatewayClass{
						TypeMeta: meta_v1.TypeMeta{},
						ObjectMeta: meta_v1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1.GatewayClassStatus{
							Conditions: []meta_v1.Condition{
								{
									Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
									Status: meta_v1.ConditionTrue,
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
				builder.Source.gateway = &gatewayapi_v1.Gateway{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GatewaySpec{
						Listeners: []gatewayapi_v1.Listener{{
							Name:     "http",
							Port:     80,
							Protocol: gatewayapi_v1.HTTPProtocolType,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
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
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "TransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Resource"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "ExistingConditions"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j meta_v1.Condition) bool {
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

	kuardService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard2",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService3 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard3",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	run(t, "simple grpcroute", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: regular expression method match type is not yet supported", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						// RegularExpression type not yet supported
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchRegularExpression, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonUnsupportedValue),
							Message: "GRPCRoute.Spec.Rules.Matches.Method: Only Exact match type is supported.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: method match must have Service configured", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidMethodMatch),
							Message: "GRPCRoute.Spec.Rules.Matches.Method: Both Service and Method need be configured.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: method match must have Method configured", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", ""),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(status.ReasonInvalidMethodMatch),
							Message: "GRPCRoute.Spec.Rules.Matches.Method: Both Service and Method need be configured.",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: invalid header match type is not supported", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: &gatewayapi_v1.GRPCMethodMatch{
								Type:    ptr.To(gatewayapi_v1.GRPCMethodMatchExact),
								Service: ptr.To("come.example.service"),
								Method:  ptr.To("Login"),
							},
							Headers: []gatewayapi_v1.GRPCHeaderMatch{
								{
									Type:  ptr.To(gatewayapi_v1.GRPCHeaderMatchType("UNKNOWN")), // <---- unknown type to break the test
									Name:  gatewayapi_v1.GRPCHeaderName("foo"),
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonUnsupportedValue),
							Message: "GRPCRoute.Spec.Rules.Matches.Headers: Only Exact match type and RegularExpression match type are supported",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: regular expression header match has invalid value", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: &gatewayapi_v1.GRPCMethodMatch{
								Type:    ptr.To(gatewayapi_v1.GRPCMethodMatchExact),
								Service: ptr.To("come.example.service"),
								Method:  ptr.To("Login"),
							},
							Headers: []gatewayapi_v1.GRPCHeaderMatch{
								{
									Type:  ptr.To(gatewayapi_v1.GRPCHeaderMatchRegularExpression),
									Name:  gatewayapi_v1.GRPCHeaderName("foo"),
									Value: "invalid(-)regex)",
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonUnsupportedValue),
							Message: "GRPCRoute.Spec.Rules.Matches.Headers: Invalid value for RegularExpression match type is specified",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: invalid RequestHeaderModifier due to duplicated headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.GRPCRouteFilter{{
							Type: gatewayapi_v1.GRPCRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "duplicate header addition: \"Custom\" on request headers"),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: invalid ResponseHeaderModifier due to invalid headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.GRPCRouteFilter{{
							Type: gatewayapi_v1.GRPCRouteFilterResponseHeaderModifier,
							ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Add: []gatewayapi_v1.HTTPHeader{
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid add header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on response headers"),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: more than one RequestMirror filters in GRPCRoute.Spec.Rules.Filters", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			kuardService3,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.GRPCRouteFilter{
							{
								Type: gatewayapi_v1.GRPCRouteFilterRequestMirror,
								RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
									BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
								},
							}, {
								Type: gatewayapi_v1.GRPCRouteFilterRequestMirror,
								RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
									BackendRef: gatewayapi.ServiceBackendObjectRef("kuard3", 8080),
								},
							},
						},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: invalid RequestMirror filter due to unspecified backendRef.name", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.GRPCRouteFilter{{
							Type: gatewayapi_v1.GRPCRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi_v1.BackendObjectReference{
									Group: ptr.To(gatewayapi_v1.Group("")),
									Kind:  ptr.To(gatewayapi_v1.Kind("Service")),
									Port:  ptr.To(gatewayapi_v1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "Spec.Rules.Filters.RequestMirror.BackendRef.Name must be specified"),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// This still results in an attached route because it returns a 404.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: custom filter type is not supported", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						Filters: []gatewayapi_v1.GRPCRouteFilter{{
							Type: "custom-filter",
						}},
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonUnsupportedValue),
							Message: "GRPCRoute.Spec.Rules.Filters: invalid type \"custom-filter\": only RequestHeaderModifier, ResponseHeaderModifier and RequestMirror are supported.",
						},
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: at lease one backend need to be specified", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{
						{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
							}},
							BackendRefs: []gatewayapi_v1.GRPCBackendRef{},
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified."),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: still validate backendrefs when not accepted", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{
							// Wrong port.
							gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http", 900),
						},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("invalid", 8080, 1),
					}},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http", 900),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.RouteConditionResolvedRefs),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonBackendNotFound),
							Message: "service \"invalid\" is invalid: service \"default/invalid\" not found",
						},
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  contour_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.RouteReasonNoMatchingParent),
							Message: "No listeners match this parent ref",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 0),
	})

	run(t, "grpcroute: invalid RequestHeaderModifier on backend due to duplicated headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{
						{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
							}},
							BackendRefs: []gatewayapi_v1.GRPCBackendRef{
								{
									BackendRef: gatewayapi_v1.BackendRef{
										BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
									},
									Filters: []gatewayapi_v1.GRPCRouteFilter{{
										Type: gatewayapi_v1.GRPCRouteFilterRequestHeaderModifier,
										RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
											Set: []gatewayapi_v1.HTTPHeader{
												{Name: "custom", Value: "duplicated"},
												{Name: "Custom", Value: "duplicated"},
											},
										},
									}},
								},
							},
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "duplicate header addition: \"Custom\" on request headers"),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: invalid ResponseHeaderModifier on backend due to invalid headers", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1.GRPCRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{{
							Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "com.example.service", "Login"),
						}},
						BackendRefs: []gatewayapi_v1.GRPCBackendRef{
							{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								},
								Filters: []gatewayapi_v1.GRPCRouteFilter{{
									Type: gatewayapi_v1.GRPCRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
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
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(
							status.ReasonDegraded,
							"invalid set header \"!invalid-Header\": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')] on response headers"),
						routeAcceptedGRPCRouteCondition(),
					},
				},
			},
		}},
		// Invalid filters still result in an attached route.
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 1),
	})

	run(t, "grpcroute: 3 grpcroutes, but duplicate match condition between these 3. The 2 out of 3 rank lower gets Conflict condition ", testcase{
		objs: []any{
			kuardService,
			// first GRPCRoute with oldest creationTimestamp
			&gatewayapi_v1.GRPCRoute{
				TypeMeta: meta_v1.TypeMeta{
					Kind: KindGRPCRoute,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:              "basic-1",
					Namespace:         "default",
					CreationTimestamp: meta_v1.Date(2020, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{
							{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "ok.service", "Login"),
							},
							{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "foo.ok.service", "Login"),
							},
						},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			},
			// second GRPCRoute with 2nd oldest creationTimestamp, conflict with 1st GRPCRoute
			&gatewayapi_v1.GRPCRoute{
				TypeMeta: meta_v1.TypeMeta{
					Kind: KindGRPCRoute,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:              "basic-2",
					Namespace:         "default",
					CreationTimestamp: meta_v1.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{{
						Matches: []gatewayapi_v1.GRPCRouteMatch{
							{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "ok.service", "Login"),
							},
						},
						BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
					}},
				},
			},
			// 3rd GRPCRoute with newest creationTimestamp, partially conflict with 1st GRPCRoute
			&gatewayapi_v1.GRPCRoute{
				TypeMeta: meta_v1.TypeMeta{
					Kind: KindGRPCRoute,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:              "basic-3",
					Namespace:         "default",
					CreationTimestamp: meta_v1.Date(2022, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
				},
				Spec: gatewayapi_v1.GRPCRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Hostnames: []gatewayapi_v1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1.GRPCRouteRule{
						{
							Matches: []gatewayapi_v1.GRPCRouteMatch{
								{
									Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "foo.ok.service", "Login"),
								},
							},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						},
						{
							Matches: []gatewayapi_v1.GRPCRouteMatch{
								{
									Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "bar.ok.service", "Logout"),
								},
							},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						},
					},
				},
			},
		},

		wantRouteConditions: []*status.RouteStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-1"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeResolvedRefsCondition(),
							routeAcceptedGRPCRouteCondition(),
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-2"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeAcceptedFalse(status.ReasonRouteRuleMatchConflict, fmt.Sprintf(status.MessageRouteRuleMatchConflict, KindGRPCRoute, KindGRPCRoute)),
							routeResolvedRefsCondition(),
						},
					},
				},
			},
			{
				FullName: types.NamespacedName{Namespace: "default", Name: "basic-3"},
				RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
					{
						ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
						Conditions: []meta_v1.Condition{
							routeAcceptedGRPCRouteCondition(),
							routePartialMatchConflict(status.ReasonRouteRuleMatchPartiallyConflict, fmt.Sprintf(status.MessageRouteRuleMatchPartiallyConflict, KindGRPCRoute, KindGRPCRoute)),
							routeResolvedRefsCondition(),
						},
					},
				},
			},
		},
		// is it ok to show the listeners are attached, just it's not accepted because of the conflict
		wantGatewayStatusUpdate: validGatewayStatusUpdate("http", gatewayapi_v1.HTTPProtocolType, 3),
	})
}

func TestGatewayAPITCPRouteDAGStatus(t *testing.T) {
	type testcase struct {
		objs                    []any
		gateway                 *gatewayapi_v1.Gateway
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
					gatewayclass: &gatewayapi_v1.GatewayClass{
						TypeMeta: meta_v1.TypeMeta{},
						ObjectMeta: meta_v1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1.GatewayClassStatus{
							Conditions: []meta_v1.Condition{
								{
									Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
									Status: meta_v1.ConditionTrue,
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
				builder.Source.gateway = &gatewayapi_v1.Gateway{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GatewaySpec{
						Listeners: []gatewayapi_v1.Listener{{
							Name:     "tcp",
							Port:     10000,
							Protocol: gatewayapi_v1.TCPProtocolType,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
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
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "TransitionTime"),
				cmpopts.IgnoreFields(status.RouteStatusUpdate{}, "Resource"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "ExistingConditions"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.GatewayStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j meta_v1.Condition) bool {
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

	kuardService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard2",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	run(t, "allowedroute of TCPRoute on a non-TCP listener results in a listener condition", testcase{
		objs: []any{},
		gateway: &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "contour",
				Namespace: "projectcontour",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				Listeners: []gatewayapi_v1.Listener{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Kinds: []gatewayapi_v1.RouteGroupKind{
							{Kind: "TCPRoute"},
						},
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1.GatewayConditionType]meta_v1.Condition{
				gatewayapi_v1.GatewayConditionAccepted: gatewayAcceptedCondition(),
				gatewayapi_v1.GatewayConditionProgrammed: {
					Type:    string(gatewayapi_v1.GatewayConditionProgrammed),
					Status:  contour_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1.ListenerStatus{
				"http": {
					Name:           "http",
					SupportedKinds: nil,
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
							Status:  meta_v1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						listenerAcceptedCondition(),
						{
							Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1.ListenerReasonInvalidRouteKinds),
							Message: "TCPRoutes are incompatible with listener protocol \"HTTP\"",
						},
					},
				},
			},
		}},
	})

	run(t, "TCPRoute with more than one rule", testcase{
		objs: []any{
			kuardService,
			kuardService2,
			&gatewayapi_v1alpha2.TCPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TCPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Rules: []gatewayapi_v1alpha2.TCPRouteRule{
						{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, ptr.To(int32(1))),
						},
						{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard2", 8080, ptr.To(int32(1))),
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						routeResolvedRefsCondition(),
						{
							Type:    string(gatewayapi_v1.RouteConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  "InvalidRouteRules",
							Message: "TCPRoute must have only a single rule defined",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("tcp", gatewayapi_v1.TCPProtocolType, 1),
	})
	run(t, "TCPRoute with rule with no backends", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TCPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TCPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Rules: []gatewayapi_v1alpha2.TCPRouteRule{
						{},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(status.ReasonDegraded, "At least one Spec.Rules.BackendRef must be specified."),
						routeAcceptedTCPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("tcp", gatewayapi_v1.TCPProtocolType, 1),
	})
	run(t, "TCPRoute with rule with ref to nonexistent backend", testcase{
		objs: []any{
			kuardService,
			&gatewayapi_v1alpha2.TCPRoute{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "default",
				},
				Spec: gatewayapi_v1alpha2.TCPRouteSpec{
					CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
						ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
					},
					Rules: []gatewayapi_v1alpha2.TCPRouteRule{
						{
							BackendRefs: gatewayapi.TLSRouteBackendRef("nonexistent", 8080, ptr.To(int32(1))),
						},
					},
				},
			},
		},
		wantRouteConditions: []*status.RouteStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "default", Name: "basic"},
			RouteParentStatuses: []*gatewayapi_v1.RouteParentStatus{
				{
					ParentRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						resolvedRefsFalse(gatewayapi_v1.RouteReasonBackendNotFound, `service "nonexistent" is invalid: service "default/nonexistent" not found`),
						routeAcceptedTCPRouteCondition(),
					},
				},
			},
		}},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("tcp", gatewayapi_v1.TCPProtocolType, 1),
	})
}

func TestGatewayAPIBackendTLSPolicyDAGStatus(t *testing.T) {
	type testcase struct {
		objs                           []any
		gateway                        *gatewayapi_v1.Gateway
		wantBackendTLSPolicyConditions []*status.BackendTLSPolicyStatusUpdate
	}

	run := func(t *testing.T, desc string, tc testcase) {
		t.Helper()
		t.Run(desc, func(t *testing.T) {
			t.Helper()
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    fixture.NewTestLogger(t),
					gatewayclass: &gatewayapi_v1.GatewayClass{
						TypeMeta: meta_v1.TypeMeta{},
						ObjectMeta: meta_v1.ObjectMeta{
							Name: "test-gc",
						},
						Spec: gatewayapi_v1.GatewayClassSpec{
							ControllerName: "projectcontour.io/contour",
						},
						Status: gatewayapi_v1.GatewayClassStatus{
							Conditions: []meta_v1.Condition{
								{
									Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
									Status: meta_v1.ConditionTrue,
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
				builder.Source.gateway = &gatewayapi_v1.Gateway{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GatewaySpec{
						Listeners: []gatewayapi_v1.Listener{{
							Name:     "http",
							Port:     80,
							Protocol: gatewayapi_v1.HTTPProtocolType,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
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
			gotBackendTLSPolicyUpdates := dag.StatusCache.GetBackendTLSPolicyUpdates()

			ops := []cmp.Option{
				cmpopts.IgnoreFields(meta_v1.Condition{}, "LastTransitionTime"),
				cmpopts.IgnoreFields(status.BackendTLSPolicyStatusUpdate{}, "GatewayRef"),
				cmpopts.IgnoreFields(status.BackendTLSPolicyStatusUpdate{}, "Generation"),
				cmpopts.IgnoreFields(status.BackendTLSPolicyStatusUpdate{}, "TransitionTime"),
				cmpopts.SortSlices(func(i, j meta_v1.Condition) bool {
					return i.Message < j.Message
				}),
				cmpopts.SortSlices(func(i, j *status.BackendTLSPolicyStatusUpdate) bool {
					return i.FullName.String() < j.FullName.String()
				}),
			}

			// Since we're using a single static GatewayClass,
			// set the expected controller string here for all
			// test cases.
			for _, u := range tc.wantBackendTLSPolicyConditions {
				u.GatewayController = builder.Source.gatewayclass.Spec.ControllerName

				for _, pas := range u.PolicyAncestorStatuses {
					pas.ControllerName = builder.Source.gatewayclass.Spec.ControllerName
				}
			}

			if diff := cmp.Diff(tc.wantBackendTLSPolicyConditions, gotBackendTLSPolicyUpdates, ops...); diff != "" {
				t.Fatalf("expected backend tls policy status: %v, got %v", tc.wantBackendTLSPolicyConditions, diff)
			}
		})
	}

	tlsService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "tlssvc",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("https", "TCP", 443, 8443)},
		},
	}

	configMapCert1 := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ca",
			Namespace: "projectcontour",
		},
		Data: map[string]string{
			CACertificateKey: fixture.CERTIFICATE,
		},
	}

	run(t, "simple httproute with backendtlspolicy", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "ConfigMap",
							Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
						}},
						Hostname: "example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonAccepted),
							Message: "Accepted BackendTLSPolicy",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with a targetref that cannot be found does not set any conditions", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "nonexistent",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "ConfigMap",
							Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
						}},
						Hostname: "example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: nil,
	})

	run(t, "backendtlspolicy with unsupported cacertref", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "Invalid",
							Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
						}},
						Hostname: "example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "BackendTLSPolicy.Spec.Validation.CACertificateRef.Kind \"Invalid\" is unsupported. Only ConfigMap or Secret Kind is supported.",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with missing configmap certref", testcase{
		objs: []any{
			tlsService,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "ConfigMap",
							Name: gatewayapi_v1.ObjectName("missing"),
						}},
						Hostname: "example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "Could not find CACertificateRef ConfigMap: projectcontour/missing",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with missing secret certref", testcase{
		objs: []any{
			tlsService,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "Secret",
							Name: gatewayapi_v1.ObjectName("missing"),
						}},
						Hostname: "example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "Could not find CACertificateRef Secret: projectcontour/missing",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with multiple cacertref that are a mix of valid and invalid", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{
							{
								Kind: "Invalid",
								Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
							},
							{
								Kind: "ConfigMap",
								Name: gatewayapi_v1.ObjectName("missing"),
							},
							{
								Kind: "ConfigMap",
								Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
							},
						},
						Hostname: "example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "BackendTLSPolicy.Spec.Validation.CACertificateRef.Kind \"Invalid\" is unsupported. Only ConfigMap or Secret Kind is supported., Could not find CACertificateRef ConfigMap: projectcontour/missing",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with wellknowncacerts set", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						WellKnownCACertificates: ptr.To(gatewayapi_v1alpha3.WellKnownCACertificatesSystem),
						Hostname:                "example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "BackendTLSPolicy.Spec.Validation.WellKnownCACertificates is unsupported.",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with malformed hostname", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "ConfigMap",
							Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
						}},
						Hostname: "-bad-hostname.example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "BackendTLSPolicy.Spec.Validation.Hostname \"-bad-hostname.example.com\" is invalid. Hostname must be a valid RFC 1123 fully qualified domain name. Wildcard domains and numeric IP addresses are not allowed",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with unsupported wildcard hostname", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "ConfigMap",
							Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
						}},
						Hostname: "*.example.com",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "BackendTLSPolicy.Spec.Validation.Hostname \"*.example.com\" is invalid. Hostname must be a valid RFC 1123 fully qualified domain name. Wildcard domains and numeric IP addresses are not allowed",
						},
					},
				},
			},
		}},
	})

	run(t, "backendtlspolicy with unsupported numeric ip as hostname", testcase{
		objs: []any{
			tlsService,
			configMapCert1,
			makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
			&gatewayapi_v1alpha3.BackendTLSPolicy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
					TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
								Kind: "Service",
								Name: "tlssvc",
							},
						},
					},
					Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
						CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
							Kind: "ConfigMap",
							Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
						}},
						Hostname: "127.0.0.1",
					},
				},
			},
		},
		wantBackendTLSPolicyConditions: []*status.BackendTLSPolicyStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "basic"},
			PolicyAncestorStatuses: []*gatewayapi_v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: gatewayapi.GatewayParentRef("projectcontour", "contour"),
					Conditions: []meta_v1.Condition{
						{
							Type:    string(gatewayapi_v1alpha2.PolicyConditionAccepted),
							Status:  meta_v1.ConditionFalse,
							Reason:  string(gatewayapi_v1alpha2.PolicyReasonInvalid),
							Message: "BackendTLSPolicy.Spec.Validation.Hostname \"127.0.0.1\" is invalid. Hostname must be a valid RFC 1123 fully qualified domain name. Wildcard domains and numeric IP addresses are not allowed",
						},
					},
				},
			},
		}},
	})
}

func gatewayAcceptedCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.GatewayConditionAccepted),
		Status:  contour_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.GatewayReasonAccepted),
		Message: "Gateway is accepted",
	}
}

func routeResolvedRefsCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.RouteConditionResolvedRefs),
		Status:  contour_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.RouteReasonResolvedRefs),
		Message: "References resolved",
	}
}

func routeAcceptedHTTPRouteCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.RouteConditionAccepted),
		Status:  contour_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.RouteReasonAccepted),
		Message: "Accepted HTTPRoute",
	}
}

func routePartialMatchConflict(reason gatewayapi_v1.RouteConditionReason, message string) meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.RouteConditionPartiallyInvalid),
		Status:  contour_v1.ConditionTrue,
		Reason:  string(reason),
		Message: message,
	}
}

func routeAcceptedFalse(reason gatewayapi_v1.RouteConditionReason, message string) meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.RouteConditionAccepted),
		Status:  contour_v1.ConditionFalse,
		Reason:  string(reason),
		Message: message,
	}
}

func routeAcceptedGRPCRouteCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.RouteConditionAccepted),
		Status:  contour_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.RouteReasonAccepted),
		Message: "Accepted GRPCRoute",
	}
}

func routeAcceptedTCPRouteCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.RouteConditionAccepted),
		Status:  contour_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.RouteReasonAccepted),
		Message: "Accepted TCPRoute",
	}
}

func listenerProgrammedCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.ListenerConditionProgrammed),
		Status:  meta_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.ListenerReasonProgrammed),
		Message: "Valid listener",
	}
}

func listenerAcceptedCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.ListenerConditionAccepted),
		Status:  meta_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.ListenerReasonAccepted),
		Message: "Listener accepted",
	}
}

func listenerResolvedRefsCondition() meta_v1.Condition {
	return meta_v1.Condition{
		Type:    string(gatewayapi_v1.ListenerConditionResolvedRefs),
		Status:  meta_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1.ListenerReasonResolvedRefs),
		Message: "Listener references resolved",
	}
}

func listenerValidConditions() []meta_v1.Condition {
	return []meta_v1.Condition{
		listenerProgrammedCondition(),
		listenerAcceptedCondition(),
		listenerResolvedRefsCondition(),
	}
}
