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
	"github.com/projectcontour/contour/internal/status"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
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
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{
						FallbackCertificate: tc.fallbackCertificate,
					},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&ListenerProcessor{},
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

	run(t, "duplicate path conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictingIncludeConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidConflictingIncludeConditions.Name,
				Namespace: proxyInvalidConflictingIncludeConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Orphaned(),
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(),
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

	run(t, "duplicate header conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidConflictHeaderConditions.Name,
				Namespace: proxyInvalidConflictHeaderConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().Orphaned(),
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().Orphaned(),
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
			{Name: proxyInvalidDuplicateHeaderAndPathConditions.Name,
				Namespace: proxyInvalidDuplicateHeaderAndPathConditions.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeIncludeError, "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Orphaned(),
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(),
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

	run(t, "requestHeadersPolicy, Host header invalid on Service", testcase{
		objs: []interface{}{invalidRequestHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: invalidRequestHeadersPolicyService.Name, Namespace: invalidRequestHeadersPolicyService.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeServiceError, "RequestHeadersPolicyInvalid", `rewriting "Host" header is not supported on request headers`),
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
						Secure: pointer.Bool(true),
					},
					{
						Name:     "a-cookie",
						SameSite: pointer.String("Lax"),
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
								Secure: pointer.Bool(true),
							},
							{
								Name:     "a-cookie",
								SameSite: pointer.String("Lax"),
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
			{Name: proxyAuthFallback.Name, Namespace: proxyAuthFallback.Namespace}: fixture.NewValidCondition().
				WithError(contour_api_v1.ConditionTypeTLSError, "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client authorization are incompatible"),
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
}

func validGatewayStatusUpdate(listenerName string, kind gatewayapi_v1beta1.Kind, attachedRoutes int) []*status.GatewayStatusUpdate {
	return []*status.GatewayStatusUpdate{
		{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionTrue,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonReady),
					Message: status.MessageValidGateway,
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				listenerName: {
					Name:           gatewayapi_v1beta1.SectionName(listenerName),
					AttachedRoutes: int32(attachedRoutes),
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{
							Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
							Kind:  kind,
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionTrue,
							Reason:  "Ready",
							Message: "Valid listener",
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
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&ListenerProcessor{},
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
									From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
					Labels: map[string]string{
						"app": "contour",
					},
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
					Conditions: []metav1.Condition{{
						Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
						Message: "Accepted HTTPRoute",
					}},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
										Kind:      gatewayapi.KindPtr("Service"),
										Namespace: gatewayapi.NamespacePtr(kuardService.Namespace),
										Name:      gatewayapi_v1beta1.ObjectName(kuardService.Name),
										Port:      gatewayapi.PortNumPtr(8080),
									},
									Weight: pointer.Int32(1),
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
					{
						Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
						Message: "Accepted HTTPRoute",
					},
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
							{
								Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
								Status:  contour_api_v1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
								Message: "Accepted HTTPRoute",
							},
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
							{
								Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
								Status:  contour_api_v1.ConditionTrue,
								Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
								Message: "Accepted HTTPRoute",
							},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("doesnt-start-with-slash"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchExact),
								Value: pointer.StringPtr("doesnt-start-with-slash"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("/foo///bar"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchExact),
								Value: pointer.StringPtr("//foo/bar"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr("UNKNOWN"), // <---- unknown type to break the test
								Value: pointer.StringPtr("/"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
						{
							Type:    string(status.ConditionNotImplemented),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(status.ReasonPathMatchType),
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
					Labels: map[string]string{
						"app": "contour",
					},
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
						{
							Type:    string(status.ConditionNotImplemented),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(status.ReasonPathMatchType),
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("/"),
							},
							Headers: []gatewayapi_v1beta1.HTTPHeaderMatch{
								{
									Type:  gatewayapi.HeaderMatchTypePtr(gatewayapi_v1beta1.HeaderMatchRegularExpression), // <---- RegularExpression type not yet supported
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
						{
							Type:    string(status.ConditionNotImplemented),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(status.ReasonHeaderMatchType),
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("/"),
							},
							QueryParams: []gatewayapi_v1beta1.HTTPQueryParamMatch{
								{
									Type:  gatewayapi.QueryParamMatchTypePtr(gatewayapi_v1beta1.QueryParamMatchRegularExpression), // <---- RegularExpression type not yet supported
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
						{
							Type:    string(status.ConditionNotImplemented),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(status.ReasonQueryParamMatchType),
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
					Labels: map[string]string{
						"app": "contour",
					},
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
										Kind: gatewayapi.KindPtr("Service"),
										Port: gatewayapi.PortNumPtr(8080),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("/"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("invalid-one", 8080, 1),
					}, {
						Matches: []gatewayapi_v1beta1.HTTPRouteMatch{{
							Path: &gatewayapi_v1beta1.HTTPPathMatch{
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("/blog"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
										Kind: gatewayapi.KindPtr("Service"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
										Kind:      gatewayapi.KindPtr("Service"),
										Namespace: gatewayapi.NamespacePtr("some-other-namespace"),
										Name:      "service",
										Port:      gatewayapi.PortNumPtr(8080),
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
							Message: "Spec.Rules.BackendRef.Namespace must match the route's namespace or be covered by a ReferencePolicy/ReferenceGrant",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.GatewayClassReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
			&gatewayapi_v1alpha2.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
					From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
						Group:     gatewayapi_v1alpha2.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1alpha2.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("tls", "TLSRoute", 0),
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferencePolicy/ReferenceGrant",
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
			&gatewayapi_v1alpha2.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
					From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
						Group:     gatewayapi_v1alpha2.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1alpha2.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
						Name: gatewayapi.ObjectNamePtr("secret"),
					}},
				},
			},
		},
		wantGatewayStatusUpdate: validGatewayStatusUpdate("tls", "TLSRoute", 0),
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
			&gatewayapi_v1alpha2.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "wrong-namespace",
				},
				Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
					From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
						Group:     gatewayapi_v1alpha2.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1alpha2.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferencePolicy/ReferenceGrant",
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
			&gatewayapi_v1alpha2.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
					From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
						Group:     gatewayapi_v1alpha2.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1alpha2.Namespace("wrong-namespace"),
					}},
					To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferencePolicy/ReferenceGrant",
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
			&gatewayapi_v1alpha2.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
					From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
						Group:     gatewayapi_v1alpha2.GroupName,
						Kind:      "WrongKind",
						Namespace: gatewayapi_v1alpha2.Namespace("projectontour"),
					}},
					To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferencePolicy/ReferenceGrant",
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
			&gatewayapi_v1alpha2.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
					From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
						Group:     gatewayapi_v1alpha2.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1alpha2.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
						Kind: "WrongKind",
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferencePolicy/ReferenceGrant",
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
					Name:     "tls",
					Port:     443,
					Protocol: gatewayapi_v1beta1.TLSProtocolType,
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							gatewayapi.CertificateRef("secret", "tls-cert-namespace"),
						},
					},
					AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
			&gatewayapi_v1alpha2.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert-reference-policy",
					Namespace: "tls-cert-namespace",
				},
				Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
					From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
						Group:     gatewayapi_v1alpha2.GroupName,
						Kind:      "Gateway",
						Namespace: gatewayapi_v1alpha2.Namespace("projectcontour"),
					}},
					To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
						Name: gatewayapi.ObjectNamePtr("wrong-name"),
					}},
				},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidCertificateRef),
							Message: "Spec.VirtualHost.TLS.CertificateRefs \"secret\" namespace must match the Gateway's namespace or be covered by a ReferencePolicy/ReferenceGrant",
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
					Labels: map[string]string{
						"app": "contour",
					},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: gatewayapi.ListenerHostname("*.projectcontour.io"),
					},
					{
						Name:     "listener-2",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: gatewayapi.ListenerHostname("specific.hostname.io"),
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
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
					},
				},
			},
		}},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{
			{
				FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
				Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
					gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
					gatewayapi_v1beta1.GatewayConditionReady: {
						Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.GatewayReasonReady),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1beta1.SectionName("listener-1"),
						AttachedRoutes: int32(1),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
								Kind:  "HTTPRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    "Ready",
								Status:  metav1.ConditionTrue,
								Reason:  "Ready",
								Message: "Valid listener",
							},
						},
					},
					"listener-2": {
						Name:           gatewayapi_v1beta1.SectionName("listener-2"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
								Kind:  "HTTPRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    "Ready",
								Status:  metav1.ConditionTrue,
								Reason:  "Ready",
								Message: "Valid listener",
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
								From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: gatewayapi.ListenerHostname("*.projectcontour.io"),
					},
					{
						Name:     "listener-2",
						Port:     80,
						Protocol: gatewayapi_v1beta1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
							Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
								From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
							},
						},
						Hostname: gatewayapi.ListenerHostname("specific.hostname.io"),
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
					gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
					gatewayapi_v1beta1.GatewayConditionReady: {
						Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
						Status:  contour_api_v1.ConditionTrue,
						Reason:  string(gatewayapi_v1beta1.GatewayReasonReady),
						Message: status.MessageValidGateway,
					},
				},
				ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
					"listener-1": {
						Name:           gatewayapi_v1beta1.SectionName("listener-1"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
								Kind:  "HTTPRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    "Ready",
								Status:  metav1.ConditionTrue,
								Reason:  "Ready",
								Message: "Valid listener",
							},
						},
					},
					"listener-2": {
						Name:           gatewayapi_v1beta1.SectionName("listener-2"),
						AttachedRoutes: int32(0),
						SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
							{
								Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
								Kind:  "HTTPRoute",
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:    "Ready",
								Status:  metav1.ConditionTrue,
								Reason:  "Ready",
								Message: "Valid listener",
							},
						},
					},
				},
			},
		},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
						{
							Type:    string(status.ConditionNotImplemented),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(status.ReasonNotImplemented),
							Message: "HTTPRoute.Spec.Rules.Filters: Only one mirror filter is supported.",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
									Group: gatewayapi.GroupPtr(""),
									Kind:  gatewayapi.KindPtr("Service"),
									Port:  gatewayapi.PortNumPtr(8080),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
									Group: gatewayapi.GroupPtr(""),
									Kind:  gatewayapi.KindPtr("Service"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("/"),
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
								Type:  gatewayapi.PathMatchTypePtr(gatewayapi_v1beta1.PathMatchPathPrefix),
								Value: pointer.StringPtr("/blog"),
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
									Group:     gatewayapi.GroupPtr(""),
									Kind:      gatewayapi.KindPtr("Service"),
									Namespace: gatewayapi.NamespacePtr("some-other-namespace"),
									Name:      gatewayapi_v1beta1.ObjectName("kuard2"),
									Port:      gatewayapi.PortNumPtr(8080),
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
							Message: "Spec.Rules.Filters.RequestMirror.BackendRef.Namespace must match the route's namespace or be covered by a ReferencePolicy/ReferenceGrant",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
							Type:    string(status.ConditionNotImplemented),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(status.ReasonHTTPRouteFilterType),
							Message: "HTTPRoute.Spec.Rules.BackendRef.Filters: Only RequestHeaderModifier type is supported.",
						},
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
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
					Labels: map[string]string{
						"app": "contour",
					},
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
							RequestHeaderModifier: &gatewayapi_v1beta1.HTTPRequestHeaderFilter{
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
						},
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
					Labels: map[string]string{
						"app": "contour",
					},
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
									RequestHeaderModifier: &gatewayapi_v1beta1.HTTPRequestHeaderFilter{
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
						{
							Type:    string(gatewayapi_v1beta1.RouteConditionAccepted),
							Status:  contour_api_v1.ConditionTrue,
							Reason:  string(gatewayapi_v1beta1.RouteReasonAccepted),
							Message: "Accepted HTTPRoute",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
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
							Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName),
							Kind:  "HTTPRoute",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionTrue,
							Reason:  "Ready",
							Message: "Valid listener",
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
							{Group: gatewayapi.GroupPtr("invalid-group"), Kind: "HTTPRoute"},
						},
						Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
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
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
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
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionResolvedRefs),
							Status:  metav1.ConditionFalse,
							Reason:  string(gatewayapi_v1beta1.ListenerReasonInvalidRouteKinds),
							Message: "Kind \"FooRoute\" is not supported, kind must be \"HTTPRoute\" or \"TLSRoute\"",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
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
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							{
								Group: gatewayapi.GroupPtr("invalid-group"),
								Kind:  gatewayapi.KindPtr("NotASecret"),
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
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
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
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
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
	run(t, "TLS certificate ref to a non-secret on a TLS listener results in a listener condition", testcase{
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
						CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
							{
								Group: gatewayapi.GroupPtr("invalid-group"),
								Kind:  gatewayapi.KindPtr("NotASecret"),
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
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
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

	run(t, "nonexistent TLS certificate ref on a TLS listener results in a listener condition", testcase{
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
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
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
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
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Invalid listener, see other listener conditions for details",
						},
						{
							Type:    string(gatewayapi_v1beta1.ListenerConditionDetached),
							Status:  metav1.ConditionTrue,
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"https": {
					Name: "https",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS is required when protocol is \"HTTPS\".",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS is required when protocol is \"TLS\".",
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
							From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
						Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModePassthrough),
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
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"tls": {
					Name: "tls",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "TLSRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.TLS.CertificateRefs cannot be defined when TLS Mode is \"Passthrough\".",
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
								From:     gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSelector),
								Selector: nil,
							},
						},
					}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.AllowedRoutes.Namespaces.Selector is required when Listener.AllowedRoutes.Namespaces.From is set to \"Selector\".",
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
								From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSelector),
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
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Error parsing Listener.AllowedRoutes.Namespaces.Selector: values: Invalid value: []string{\"error\"}: values set must be empty for exists and does not exist.",
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
								From:     gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromSelector),
								Selector: &metav1.LabelSelector{},
							},
						},
					}},
			},
		},
		wantGatewayStatusUpdate: []*status.GatewayStatusUpdate{{
			FullName: types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			Conditions: map[gatewayapi_v1beta1.GatewayConditionType]metav1.Condition{
				gatewayapi_v1beta1.GatewayConditionScheduled: gatewayScheduledCondition(),
				gatewayapi_v1beta1.GatewayConditionReady: {
					Type:    string(gatewayapi_v1beta1.GatewayConditionReady),
					Status:  contour_api_v1.ConditionFalse,
					Reason:  string(gatewayapi_v1beta1.GatewayReasonListenersNotValid),
					Message: "Listeners are not valid",
				},
			},
			ListenerStatus: map[string]*gatewayapi_v1beta1.ListenerStatus{
				"http": {
					Name: "http",
					SupportedKinds: []gatewayapi_v1beta1.RouteGroupKind{
						{Group: gatewayapi.GroupPtr(gatewayapi_v1alpha2.GroupName), Kind: "HTTPRoute"},
					},
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Listener.AllowedRoutes.Namespaces.Selector must specify at least one MatchLabel or MatchExpression.",
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
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&ListenerProcessor{},
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

	gateways := []*gatewayapi_v1beta1.Gateway{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1beta1.GatewaySpec{
			Listeners: []gatewayapi_v1beta1.Listener{{
				Name:     "tls-default-terminate",
				Port:     443,
				Protocol: gatewayapi_v1beta1.TLSProtocolType,
				TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
					// Mode is not defined and should default to "Terminate".
					CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
						gatewayapi.CertificateRef(fixture.SecretProjectContourCert.Name, fixture.SecretProjectContourCert.Namespace),
					},
				},
				AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
					Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
						From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
					},
				},
			}},
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1beta1.GatewaySpec{
			Listeners: []gatewayapi_v1beta1.Listener{{
				Name:     "tls-terminate",
				Port:     443,
				Protocol: gatewayapi_v1beta1.TLSProtocolType,
				TLS: &gatewayapi_v1beta1.GatewayTLSConfig{
					Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModeTerminate),
					CertificateRefs: []gatewayapi_v1beta1.SecretObjectReference{
						gatewayapi.CertificateRef(fixture.SecretProjectContourCert.Name, fixture.SecretProjectContourCert.Namespace),
					},
				},
				AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
					Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
						From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
					},
				},
			}},
		},
	}, {
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
					Mode: gatewayapi.TLSModeTypePtr(gatewayapi_v1beta1.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1beta1.AllowedRoutes{
					Namespaces: &gatewayapi_v1beta1.RouteNamespaces{
						From: gatewayapi.FromNamespacesPtr(gatewayapi_v1beta1.NamespacesFromAll),
					},
				},
			}},
		},
	}}

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

	// Both a "mode: terminate" & a "mode: passthrough" should return the same
	// valid status. This loops through all three types (Not Defined, Terminate, Passthrough)
	// and validates the proper status is set. Note when not defined, the default is "terminate".
	for _, gw := range gateways {

		run(t, "TLSRoute: spec.rules.backendRef.name not specified", testcase{
			gateway: gw,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
						Labels: map[string]string{
							"app": "contour",
						},
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1alpha2.ParentReference{
								gatewayapi.GatewayParentRefV1Alpha2("projectcontour", "contour"),
							},
						},
						Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1alpha2.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1alpha2.BackendObjectReference{
										Kind: gatewayapi.KindPtrV1Alpha2("Service"),
										Port: gatewayapi.PortNumPtrV1Alpha2(8080),
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
						Labels: map[string]string{
							"app": "contour",
						},
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1alpha2.ParentReference{
								gatewayapi.GatewayParentRefV1Alpha2("projectcontour", "contour"),
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
						Labels: map[string]string{
							"app": "contour",
						},
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1alpha2.ParentReference{
								gatewayapi.GatewayParentRefV1Alpha2("projectcontour", "contour"),
							},
						},
						Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1alpha2.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1alpha2.BackendObjectReference{
										Kind: gatewayapi.KindPtrV1Alpha2("Service"),
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
						Labels: map[string]string{
							"app": "contour",
						},
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1alpha2.ParentReference{
								gatewayapi.GatewayParentRefV1Alpha2("projectcontour", "contour"),
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
						Labels: map[string]string{
							"app": "contour",
						},
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1alpha2.ParentReference{
								gatewayapi.GatewayParentRefV1Alpha2("projectcontour", "contour"),
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
						Labels: map[string]string{
							"app": "contour",
						},
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1alpha2.ParentReference{
								gatewayapi.GatewayParentRefV1Alpha2("projectcontour", "contour"),
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
						Labels: map[string]string{
							"app": "contour",
						},
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1alpha2.ParentReference{
								gatewayapi.GatewayParentRefV1Alpha2("projectcontour", "contour"),
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
								gatewayapi.GatewayParentRefV1Alpha2(gw.Namespace, gw.Name),
							},
						},
						Hostnames: []gatewayapi_v1alpha2.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef(kuardService.Name, 8080, pointer.Int32(0)),
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
	}
}

func gatewayScheduledCondition() metav1.Condition {
	return metav1.Condition{
		Type:    string(gatewayapi_v1beta1.GatewayConditionScheduled),
		Status:  contour_api_v1.ConditionTrue,
		Reason:  string(gatewayapi_v1beta1.GatewayReasonScheduled),
		Message: "Gateway is scheduled",
	}
}
