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
		want                map[types.NamespacedName]Status
	}

	tests := make(map[string]testcase)

	// Common test fixtures (used across more than one test)

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

	// Tests using common fixtures
	tests["root proxy does not specify FQDN"] = testcase{
		objs: []interface{}{proxyNoFQDN},
		want: map[types.NamespacedName]Status{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: {Object: proxyNoFQDN, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
		},
	}

	// Simple Valid HTTPProxy
	proxyValidHomeService := &projcontour.HTTPProxy{
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

	tests["valid proxy"] = testcase{
		objs: []interface{}{proxyValidHomeService, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyValidHomeService.Name, Namespace: proxyValidHomeService.Namespace}: {Object: proxyValidHomeService, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
		},
	}

	// Multiple Includes, one invalid
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

	tests["proxy has multiple includes, one is invalid"] = testcase{
		objs: []interface{}{proxyMultiIncludeOneInvalid, proxyChildValidFoo2, proxyChildInvalidBadPort, fixture.ServiceRootsFoo2, fixture.ServiceRootsFoo3InvalidPort},
		want: map[types.NamespacedName]Status{
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}:                 {Object: proxyChildValidFoo2, Status: "valid", Description: "valid HTTPProxy"},
			{Name: proxyChildInvalidBadPort.Name, Namespace: proxyChildInvalidBadPort.Namespace}:       {Object: proxyChildInvalidBadPort, Status: "invalid", Description: `service "foo3": port must be in the range 1-65535`},
			{Name: proxyMultiIncludeOneInvalid.Name, Namespace: proxyMultiIncludeOneInvalid.Namespace}: {Object: proxyMultiIncludeOneInvalid, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
		},
	}

	tests["multi-parent children is not orphaned when one of the parents is invalid"] = testcase{
		objs: []interface{}{proxyNoFQDN, proxyChildValidFoo2, proxyIncludeValidChild, fixture.ServiceRootsKuard, fixture.ServiceRootsFoo2},
		want: map[types.NamespacedName]Status{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}:                       {Object: proxyNoFQDN, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}:       {Object: proxyChildValidFoo2, Status: "valid", Description: "valid HTTPProxy"},
			{Name: proxyIncludeValidChild.Name, Namespace: proxyIncludeValidChild.Namespace}: {Object: proxyIncludeValidChild, Status: "valid", Description: "valid HTTPProxy", Vhost: "example.com"},
		},
	}

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

	proxyTCPSharedService := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: fixture.ServiceRootsNginx.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
				TLS: &projcontour.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: fixture.ServiceRootsNginx.Name,
					Port: 80,
				}},
			},
		},
	}

	// issue 1399
	tests["service shared across ingress and httpproxy tcpproxy"] = testcase{
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
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
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

	// issue 1347
	tests["tcpproxy with tls delegation failure"] = testcase{
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
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
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

	// issue 1348
	tests["routes with tls delegation failure"] = testcase{
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
	}

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

	proxyPassthroughProxyNonSecure := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: serviceTLSPassthrough.Namespace,
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
					Name: serviceTLSPassthrough.Name,
					Port: 80, // proxy non secure traffic to port 80
				}},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: serviceTLSPassthrough.Name,
					Port: 443, // ssl passthrough to secure port
				}},
			},
		},
	}

	// issue 910
	tests["non tls routes can be combined with tcp proxy"] = testcase{
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
	}

	proxyMultipleIncludersSite1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site1",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "site1.com",
			},
			Includes: []projcontour.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultipleIncludersSite2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site2",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "site2.com",
			},
			Includes: []projcontour.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultiIncludeChild := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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

	tests["two root httpproxies delegated to the same object should not conflict on hostname"] = testcase{
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
	}

	// proxyInvalidNegativePortHomeService is invalid because it contains a service with negative port
	proxyInvalidNegativePortHomeService := &projcontour.HTTPProxy{
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

	tests["proxy invalid port in service"] = testcase{
		objs: []interface{}{proxyInvalidNegativePortHomeService},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidNegativePortHomeService.Name, Namespace: proxyInvalidNegativePortHomeService.Namespace}: {Object: proxyInvalidNegativePortHomeService, Status: "invalid", Description: `service "home": port must be in the range 1-65535`, Vhost: "example.com"},
		},
	}

	// proxyInvalidOutsideRootNamespace is invalid because it lives outside the roots namespace
	proxyInvalidOutsideRootNamespace := &projcontour.HTTPProxy{
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

	tests["root proxy outside of roots namespace"] = testcase{
		objs: []interface{}{proxyInvalidOutsideRootNamespace},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidOutsideRootNamespace.Name, Namespace: proxyInvalidOutsideRootNamespace.Namespace}: {Object: proxyInvalidOutsideRootNamespace, Status: "invalid", Description: "root HTTPProxy cannot be defined in this namespace"},
		},
	}

	// proxyInvalidIncludeCycle is invalid because it delegates to itself, producing a cycle
	proxyInvalidIncludeCycle := &projcontour.HTTPProxy{
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

	tests["proxy self-edge produces a cycle"] = testcase{
		objs: []interface{}{proxyInvalidIncludeCycle, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidIncludeCycle.Name, Namespace: proxyInvalidIncludeCycle.Namespace}: {
				Object:      proxyInvalidIncludeCycle,
				Status:      "invalid",
				Description: "root httpproxy cannot delegate to another root httpproxy",
				Vhost:       "example.com",
			},
		},
	}

	// proxyIncludesProxyWithIncludeCycle delegates to proxy8, which is invalid because proxy8 delegates back to proxy8
	proxyIncludesProxyWithIncludeCycle := &projcontour.HTTPProxy{
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

	proxyIncludedChildInvalidIncludeCycle := &projcontour.HTTPProxy{
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

	tests["proxy child delegates to parent, producing a cycle"] = testcase{
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
	}

	tests["proxy orphaned route"] = testcase{
		objs: []interface{}{proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]Status{
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: {Object: proxyIncludedChildInvalidIncludeCycle, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy"},
		},
	}

	proxyIncludedChildValid := &projcontour.HTTPProxy{
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

	// proxyNotRootIncludeRootProxy delegates to proxyWildCardFQDN but it is invalid because it is missing fqdn
	proxyNotRootIncludeRootProxy := &projcontour.HTTPProxy{
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

	tests["proxy invalid parent orphans children"] = testcase{
		objs: []interface{}{proxyNotRootIncludeRootProxy, proxyIncludedChildValid},
		want: map[types.NamespacedName]Status{
			{Name: proxyNotRootIncludeRootProxy.Name, Namespace: proxyNotRootIncludeRootProxy.Namespace}: {Object: proxyNotRootIncludeRootProxy, Status: "invalid", Description: "Spec.VirtualHost.Fqdn must be specified"},
			{Name: proxyIncludedChildValid.Name, Namespace: proxyIncludedChildValid.Namespace}:           {Object: proxyIncludedChildValid, Status: "orphaned", Description: "this HTTPProxy is not part of a delegation chain from a root HTTPProxy"},
		},
	}

	// proxyWildCardFQDN is invalid because it contains a wildcarded fqdn
	proxyWildCardFQDN := &projcontour.HTTPProxy{
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

	tests["proxy invalid FQDN contains wildcard"] = testcase{
		objs: []interface{}{proxyWildCardFQDN},
		want: map[types.NamespacedName]Status{
			{Name: proxyWildCardFQDN.Name, Namespace: proxyWildCardFQDN.Namespace}: {Object: proxyWildCardFQDN, Status: "invalid", Description: `Spec.VirtualHost.Fqdn "example.*.com" cannot use wildcards`, Vhost: "example.*.com"},
		},
	}

	// proxyInvalidServiceInvalid is invalid because it references an invalid service
	proxyInvalidServiceInvalid := &projcontour.HTTPProxy{
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

	tests["proxy missing service shows invalid status"] = testcase{
		objs: []interface{}{proxyInvalidServiceInvalid},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidServiceInvalid.Name, Namespace: proxyInvalidServiceInvalid.Namespace}: {
				Object:      proxyInvalidServiceInvalid,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: service "roots/invalid" not found`,
				Vhost:       proxyInvalidServiceInvalid.Spec.VirtualHost.Fqdn,
			},
		},
	}

	// proxyInvalidServicePortInvalid is invalid because it references an invalid port on a service
	proxyInvalidServicePortInvalid := &projcontour.HTTPProxy{
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

	tests["proxy with service missing port shows invalid status"] = testcase{
		objs: []interface{}{proxyInvalidServicePortInvalid, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidServicePortInvalid.Name, Namespace: proxyInvalidServicePortInvalid.Namespace}: {
				Object:      proxyInvalidServicePortInvalid,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: port "9999" on service "roots/home" not matched`,
				Vhost:       proxyInvalidServicePortInvalid.Spec.VirtualHost.Fqdn,
			},
		},
	}

	proxyValidExampleCom := &projcontour.HTTPProxy{
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
	proxyValidReuseExampleCom := &projcontour.HTTPProxy{
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

	tests["insert conflicting proxies due to fqdn reuse"] = testcase{
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
	}

	proxyRootIncludesRoot := &projcontour.HTTPProxy{
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

	proxyRootIncludedByRoot := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
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
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	tests["root proxy including another root"] = testcase{
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
	}

	proxyIncludesRootDifferentFQDN := &projcontour.HTTPProxy{
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

	proxyRootIncludedByRootDiffFQDN := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	tests["root proxy including another root w/ different hostname"] = testcase{
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
	}

	proxyValidIncludeBlogMarketing := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxyRootValidIncludesBlogMarketing := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name:      proxyValidIncludeBlogMarketing.Name,
				Namespace: proxyValidIncludeBlogMarketing.Namespace,
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
		},
	}

	tests["proxy includes another"] = testcase{
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
	}

	proxyValidWithMirror := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
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

	tests["proxy with mirror"] = testcase{
		objs: []interface{}{proxyValidWithMirror, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyValidWithMirror.Name, Namespace: proxyValidWithMirror.Namespace}: {
				Object:      proxyValidWithMirror,
				Status:      "valid",
				Description: "valid HTTPProxy",
				Vhost:       "example.com",
			},
		},
	}

	proxyInvalidTwoMirrors := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
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

	tests["proxy with two mirrors"] = testcase{
		objs: []interface{}{proxyInvalidTwoMirrors, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidTwoMirrors.Name, Namespace: proxyInvalidTwoMirrors.Namespace}: {
				Object:      proxyInvalidTwoMirrors,
				Status:      "invalid",
				Description: "only one service per route may be nominated as mirror",
				Vhost:       "example.com",
			},
		},
	}

	// proxy28 is a proxy with duplicated route condition headers
	proxyInvalidDuplicateMatchConditionHeaders := &projcontour.HTTPProxy{
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

	tests["duplicate route condition headers"] = testcase{
		objs: []interface{}{proxyInvalidDuplicateMatchConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidDuplicateMatchConditionHeaders.Name, Namespace: proxyInvalidDuplicateMatchConditionHeaders.Namespace}: {
				Object: proxyInvalidDuplicateMatchConditionHeaders,
				Status: "invalid", Description: "cannot specify duplicate header 'exact match' conditions in the same route",
				Vhost: "example.com",
			},
		},
	}

	// proxy29 is a proxy with duplicated invalid include condition headers
	proxyInvalidDuplicateIncludeCondtionHeaders := &projcontour.HTTPProxy{
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
	proxyValidDelegatedRoots := &projcontour.HTTPProxy{
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

	tests["duplicate include condition headers"] = testcase{
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
	}

	// proxy31 is a proxy with duplicated valid route condition headers
	proxyInvalidRouteConditionHeaders := &projcontour.HTTPProxy{
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

	tests["duplicate valid route condition headers"] = testcase{
		objs: []interface{}{proxyInvalidRouteConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidRouteConditionHeaders.Name, Namespace: proxyInvalidRouteConditionHeaders.Namespace}: {
				Object: proxyInvalidRouteConditionHeaders,
				Status: "valid", Description: "valid HTTPProxy",
				Vhost: "example.com",
			},
		},
	}

	proxyInvalidMultiplePrefixes := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	tests["proxy with two prefix conditions on route"] = testcase{
		objs: []interface{}{proxyInvalidMultiplePrefixes, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidMultiplePrefixes.Name, Namespace: proxyInvalidMultiplePrefixes.Namespace}: {
				Object:      proxyInvalidMultiplePrefixes,
				Status:      "invalid",
				Description: "route: more than one prefix is not allowed in a condition block",
				Vhost:       "example.com",
			},
		},
	}

	proxyInvalidTwoPrefixesWithInclude := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidChildTeamA := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "teama",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	tests["proxy with two prefix conditions orphans include"] = testcase{
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
	}

	proxyInvalidPrefixNoSlash := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	tests["proxy with prefix conditions on route that does not start with slash"] = testcase{
		objs: []interface{}{proxyInvalidPrefixNoSlash, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidPrefixNoSlash.Name, Namespace: proxyInvalidPrefixNoSlash.Namespace}: {
				Object:      proxyInvalidPrefixNoSlash,
				Status:      "invalid",
				Description: "route: prefix conditions must start with /, api was supplied",
				Vhost:       "example.com",
			},
		},
	}

	proxyInvalidIncludePrefixNoSlash := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	tests["proxy with include prefix that does not start with slash"] = testcase{
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
	}

	// invalid because tcpproxy both includes another httpproxy
	// and has a list of services.
	proxyInvalidTCPProxyIncludeAndService := &projcontour.HTTPProxy{
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
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	tests["tcpproxy cannot specify services and include"] = testcase{
		objs: []interface{}{proxyInvalidTCPProxyIncludeAndService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidTCPProxyIncludeAndService.Name, Namespace: proxyInvalidTCPProxyIncludeAndService.Namespace}: {
				Object:      proxyInvalidTCPProxyIncludeAndService,
				Status:      "invalid",
				Description: "tcpproxy: cannot specify services and include in the same httpproxy",
				Vhost:       "passthrough.example.com",
			},
		},
	}

	// Invalid because tcpproxy neither includes another httpproxy
	// nor has a list of services.
	proxyTCPNoServiceOrInclusion := &projcontour.HTTPProxy{
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

	tests["tcpproxy empty"] = testcase{
		objs: []interface{}{proxyTCPNoServiceOrInclusion, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPNoServiceOrInclusion.Name, Namespace: proxyTCPNoServiceOrInclusion.Namespace}: {
				Object:      proxyTCPNoServiceOrInclusion,
				Status:      "invalid",
				Description: "tcpproxy: either services or inclusion must be specified",
				Vhost:       "passthrough.example.com",
			},
		},
	}

	// proxy38 is invalid when combined with proxy39 as the latter
	// is a root httpproxy.
	proxyTCPIncludesFoo := &projcontour.HTTPProxy{
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
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	tests["tcpproxy w/ missing include"] = testcase{
		objs: []interface{}{proxyTCPIncludesFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: {
				Object:      proxyTCPIncludesFoo,
				Status:      "invalid",
				Description: "tcpproxy: include roots/foo not found",
				Vhost:       "passthrough.example.com",
			},
		},
	}

	proxyValidTCPRoot := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	tests["tcpproxy includes another root"] = testcase{
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
	}

	proxyTCPValidChildFoo := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	tests["tcpproxy includes valid child"] = testcase{
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
	}

	// proxy41 is a proxy with conflicting include conditions
	proxyInvalidConflictingIncludeConditions := &projcontour.HTTPProxy{
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

	// proxy41a is a child of proxy41
	proxyValidBlogTeamA := &projcontour.HTTPProxy{
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
					Name: fixture.ServiceTeamAKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxyValidproxy41a is a child of proxy41
	proxyValidBlogTeamB := &projcontour.HTTPProxy{
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
					Name: fixture.ServiceTeamBKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	tests["duplicate path conditions on an include"] = testcase{
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
	}

	// proxy42 is a proxy with conflicting include header conditions
	proxyInvalidConflictHeaderConditions := &projcontour.HTTPProxy{
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

	tests["duplicate header conditions on an include"] = testcase{
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
	}

	// proxy43 is a proxy with conflicting include header conditions
	proxyInvalidDuplicateHeaderAndPathConditions := &projcontour.HTTPProxy{
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

	tests["duplicate header+path conditions on an include"] = testcase{
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
	}

	// proxy44's include is missing
	proxyInvalidMissingInclude := &projcontour.HTTPProxy{
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

	tests["httpproxy w/ missing include"] = testcase{
		objs: []interface{}{proxyInvalidMissingInclude, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidMissingInclude.Name, Namespace: proxyInvalidMissingInclude.Namespace}: {
				Object:      proxyInvalidMissingInclude,
				Status:      "invalid",
				Description: "include roots/child not found",
				Vhost:       "example.com",
			},
		},
	}

	proxyTCPInvalidMissingService := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tcp-proxy-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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

	tests["httpproxy w/ tcpproxy w/ missing service"] = testcase{
		objs: []interface{}{proxyTCPInvalidMissingService},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPInvalidMissingService.Name, Namespace: proxyTCPInvalidMissingService.Namespace}: {
				Object:      proxyTCPInvalidMissingService,
				Status:      "invalid",
				Description: `Spec.TCPProxy unresolved service reference: service "roots/not-found" not found`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	}

	proxyTCPInvalidPortNotMatched := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-proxy-service-missing-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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
					Name: fixture.ServiceRootsKuard.Name,
					Port: 9999,
				}},
			},
		},
	}

	tests["httpproxy w/ tcpproxy w/ service missing port"] = testcase{
		objs: []interface{}{proxyTCPInvalidPortNotMatched, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPInvalidPortNotMatched.Name, Namespace: proxyTCPInvalidPortNotMatched.Namespace}: {
				Object:      proxyTCPInvalidPortNotMatched,
				Status:      "invalid",
				Description: `Spec.TCPProxy unresolved service reference: port "9999" on service "roots/kuard" not matched`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	}

	proxyTCPInvalidMissingTLS := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tls",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
			},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	tests["httpproxy w/ tcpproxy missing tls"] = testcase{
		objs: []interface{}{proxyTCPInvalidMissingTLS},
		want: map[types.NamespacedName]Status{
			{Name: proxyTCPInvalidMissingTLS.Name, Namespace: proxyTCPInvalidMissingTLS.Namespace}: {
				Object:      proxyTCPInvalidMissingTLS,
				Status:      "invalid",
				Description: "Spec.TCPProxy requires that either Spec.TLS.Passthrough or Spec.TLS.SecretName be set",
				Vhost:       "tcpproxy.example.com",
			},
		},
	}

	proxyInvalidMissingServiceWithTCPProxy := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{Name: "missing", Port: 9000},
				},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	tests["httpproxy w/ tcpproxy missing service"] = testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyInvalidMissingServiceWithTCPProxy},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidMissingServiceWithTCPProxy.Name, Namespace: proxyInvalidMissingServiceWithTCPProxy.Namespace}: {
				Object:      proxyInvalidMissingServiceWithTCPProxy,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: service "roots/missing" not found`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	}

	proxyRoutePortNotMatchedWithTCP := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{Name: fixture.ServiceRootsKuard.Name, Port: 9999},
				},
			}},
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	tests["tcpproxy route unmatched service port"] = testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyRoutePortNotMatchedWithTCP},
		want: map[types.NamespacedName]Status{
			{Name: proxyRoutePortNotMatchedWithTCP.Name, Namespace: proxyRoutePortNotMatchedWithTCP.Namespace}: {
				Object:      proxyRoutePortNotMatchedWithTCP,
				Status:      "invalid",
				Description: `Spec.Routes unresolved service reference: port "9999" on service "roots/kuard" not matched`,
				Vhost:       "tcpproxy.example.com",
			},
		},
	}

	proxyTCPValidIncludeChild := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				Include: &projcontour.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidIncludesChild := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{
				IncludesDeprecated: &projcontour.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidChild := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			TCPProxy: &projcontour.TCPProxy{
				Services: []projcontour.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	tests["valid HTTPProxy.TCPProxy - plural"] = testcase{
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
	}

	tests["valid HTTPProxy.TCPProxy"] = testcase{
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
	}

	// issue 2309, each route must have at least one service
	proxyInvalidNoServices := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
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

	// issue 2309, each route must have at least one service
	tests["invalid HTTPProxy due to empty route.service"] = testcase{
		objs: []interface{}{proxyInvalidNoServices, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: proxyInvalidNoServices.Name, Namespace: proxyInvalidNoServices.Namespace}: {
				Object:      proxyInvalidNoServices,
				Status:      "invalid",
				Description: "route.services must have at least one entry",
				Vhost:       "missing-service.example.com",
			},
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

	tests["invalid fallback certificate passed to contour"] = testcase{
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
	}

	tests["fallback certificate requested but cert not configured in contour"] = testcase{
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

	tests["fallback certificate requested and clientValidation also configured"] = testcase{
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
	}

	// a proxy with TLS configured with passthrough and
	// client validation is invalid
	tlsPassthroughAndValidation := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
					ClientValidation: &projcontour.DownstreamValidation{
						CACertificate: "aCAcert",
					},
				},
			},
			TCPProxy: &projcontour.TCPProxy{},
		},
	}

	tests["passthrough and client auth are incompatible tlsPassthroughAndValidation"] = testcase{
		objs: []interface{}{fixture.SecretRootsCert, tlsPassthroughAndValidation},
		want: map[types.NamespacedName]Status{
			{Name: tlsPassthroughAndValidation.Name, Namespace: tlsPassthroughAndValidation.Namespace}: {
				Object:      tlsPassthroughAndValidation,
				Status:      "invalid",
				Description: "Spec.VirtualHost.TLS passthrough cannot be combined with tls.clientValidation",
				Vhost:       tlsPassthroughAndValidation.Spec.VirtualHost.Fqdn,
			},
		},
	}

	// a proxy with TLS configured with *both* passthrough and
	// a secret name is invalid.
	tlsPassthroughAndSecretName := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					Passthrough: true,
					SecretName:  fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &projcontour.TCPProxy{},
		},
	}

	tests["tcpproxy with TLS passthrough and secret name both specified"] = testcase{
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
	}

	// a proxy with TLS configured with *neither* passthrough nor
	// a secret name is invalid.
	tlsNoPassthroughOrSecretName := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &projcontour.TLS{
					Passthrough: false,
					SecretName:  "",
				},
			},
			TCPProxy: &projcontour.TCPProxy{},
		},
	}

	tests["httpproxy w/ tcpproxy with neither TLS passthrough nor secret name specified"] = testcase{
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
	}

	// a proxy without any routes, includes, or a tcp proxy
	// is invalid.
	emptyProxy := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty",
			Namespace: "roots",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
		},
	}

	tests["proxy with no routes, includes, or tcpproxy is invalid"] = testcase{
		objs: []interface{}{emptyProxy},
		want: map[types.NamespacedName]Status{
			{Name: emptyProxy.Name, Namespace: emptyProxy.Namespace}: {
				Object:      emptyProxy,
				Status:      "invalid",
				Description: "HTTPProxy.Spec must have at least one Route, Include, or a TCPProxy",
				Vhost:       emptyProxy.Spec.VirtualHost.Fqdn,
			},
		},
	}

	invalidRequestHeadersPolicyService := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						RequestHeadersPolicy: &projcontour.HeadersPolicy{
							Set: []projcontour.HeaderValue{{
								Name:  "Host",
								Value: "external.com",
							}},
						},
					},
				},
			}},
		},
	}

	tests["requestHeadersPolicy, Host header invalid on Service"] = testcase{
		objs: []interface{}{invalidRequestHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: invalidRequestHeadersPolicyService.Name, Namespace: invalidRequestHeadersPolicyService.Namespace}: {
				Object:      invalidRequestHeadersPolicyService,
				Status:      "invalid",
				Description: "rewriting \"Host\" header is not supported on a service",
				Vhost:       invalidRequestHeadersPolicyService.Spec.VirtualHost.Fqdn,
			},
		},
	}

	invalidResponseHeadersPolicyService := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						ResponseHeadersPolicy: &projcontour.HeadersPolicy{
							Set: []projcontour.HeaderValue{{
								Name:  "Host",
								Value: "external.com",
							}},
						},
					},
				},
			}},
		},
	}

	tests["responseHeadersPolicy, Host header invalid on Service"] = testcase{
		objs: []interface{}{invalidResponseHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: invalidResponseHeadersPolicyService.Name, Namespace: invalidResponseHeadersPolicyService.Namespace}: {
				Object:      invalidResponseHeadersPolicyService,
				Status:      "invalid",
				Description: "rewriting \"Host\" header is not supported on response headers",
				Vhost:       invalidResponseHeadersPolicyService.Spec.VirtualHost.Fqdn,
			},
		},
	}

	invalidResponseHeadersPolicyRoute := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPRoute",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
					},
				},
				ResponseHeadersPolicy: &projcontour.HeadersPolicy{
					Set: []projcontour.HeaderValue{{
						Name:  "Host",
						Value: "external.com",
					}},
				},
			}},
		},
	}

	tests["responseHeadersPolicy, Host header invalid on Route"] = testcase{
		objs: []interface{}{invalidResponseHeadersPolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]Status{
			{Name: invalidResponseHeadersPolicyRoute.Name, Namespace: invalidResponseHeadersPolicyRoute.Namespace}: {
				Object:      invalidResponseHeadersPolicyRoute,
				Status:      "invalid",
				Description: "rewriting \"Host\" header is not supported on response headers",
				Vhost:       invalidResponseHeadersPolicyRoute.Spec.VirtualHost.Fqdn,
			},
		},
	}

	proxyAuthFallback := fixture.NewProxy("roots/fallback-incompat").
		WithSpec(projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "invalid.com",
				TLS: &projcontour.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
				Authorization: &projcontour.AuthorizationServer{
					ExtensionServiceRef: projcontour.ExtensionServiceReference{
						Namespace: "auth",
						Name:      "extension",
					},
				},
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{Name: "app-server", Port: 80}},
			}},
		})

	tests["incompat"] = testcase{
		objs: []interface{}{fixture.SecretRootsCert, proxyAuthFallback},
		want: map[types.NamespacedName]Status{
			{Name: proxyAuthFallback.Name, Namespace: proxyAuthFallback.Namespace}: {
				Object:      proxyAuthFallback,
				Status:      "invalid",
				Description: "Spec.Virtualhost.TLS fallback & client authorization are incompatible",
				Vhost:       proxyAuthFallback.Spec.VirtualHost.Fqdn,
			},
		},
	}

	invalidResponseTimeout := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{
				{
					Services: []projcontour.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &projcontour.TimeoutPolicy{
						Response: "invalid-val",
					},
				},
			},
		},
	}

	tests["proxy with invalid response timeout value is invalid"] = testcase{
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
	}

	invalidIdleTimeout := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{
				{
					Services: []projcontour.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &projcontour.TimeoutPolicy{
						Idle: "invalid-val",
					},
				},
			},
		},
	}

	tests["proxy with invalid idle timeout value is invalid"] = testcase{
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
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
}
