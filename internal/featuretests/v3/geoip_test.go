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

package v3

import (
	"testing"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_filter_http_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	envoy_filter_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
)

// TestGeoFilterPolicy verifies that a TLS vhost with a GeoIP allow/deny policy
// produces an RBAC per-route filter config (on the HTTPS route) that matches on
// the geolocation headers populated by the Envoy GeoIP filter.
func TestGeoFilterPolicy(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	sec1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(sec1)

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
	rh.OnAdd(s1)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "geofilter",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "geo.test.com",
				TLS: &contour_v1.TLS{
					SecretName: "secret",
				},
				GeoDenyFilterPolicy: []contour_v1.GeoFilterPolicy{
					{Dimension: contour_v1.GeoDimensionAnon, Value: "true"},
					{Dimension: contour_v1.GeoDimensionAnonVpn, Value: "true"},
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(routeType, "https/geo.test.com").Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: routeType,
		Resources: resources(t,
			envoy_v3.RouteConfiguration("https/geo.test.com", virtualHostWithFilters(envoy_v3.VirtualHost(hp1.Spec.VirtualHost.Fqdn,
				&envoy_config_route_v3.Route{
					Match:  routePrefix("/"),
					Action: routeCluster("default/backend/80/da39a3ee5e"),
				},
			), withFilterConfig(envoy_v3.RBACFilterName, &envoy_filter_http_rbac_v3.RBACPerRoute{Rbac: &envoy_filter_http_rbac_v3.RBAC{
				Rules: &envoy_config_rbac_v3.RBAC{
					Action: envoy_config_rbac_v3.RBAC_DENY,
					Policies: map[string]*envoy_config_rbac_v3.Policy{
						"filter-rules": {
							Permissions: []*envoy_config_rbac_v3.Permission{
								{Rule: &envoy_config_rbac_v3.Permission_Any{Any: true}},
							},
							Principals: []*envoy_config_rbac_v3.Principal{
								geoHeaderPrincipal(envoy_v3.GeoIPAnonHeader, "true"),
								geoHeaderPrincipal(envoy_v3.GeoIPAnonVpnHeader, "true"),
							},
						},
					},
				},
			}}),
			)),
		),
	})
}

// geoHeaderPrincipal builds an RBAC principal matching a geolocation request
// header exactly, mirroring envoy_v3.geoHeaderName + rbacFilterConfig.
func geoHeaderPrincipal(name, value string) *envoy_config_rbac_v3.Principal {
	return &envoy_config_rbac_v3.Principal{
		Identifier: &envoy_config_rbac_v3.Principal_Header{
			Header: &envoy_config_route_v3.HeaderMatcher{
				Name: name,
				HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_StringMatch{
					StringMatch: &envoy_matcher_v3.StringMatcher{
						MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{Exact: value},
					},
				},
			},
		},
	}
}

// containsFilter reports whether names contains want.
func containsFilter(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// httpsHCMFilterNames returns the HTTP filter names from the secure (HTTPS)
// listener's filter chain matching fqdn (the per-SNI HCM for that vhost).
func httpsHCMFilterNames(t *testing.T, resp *envoy_service_discovery_v3.DiscoveryResponse, fqdn string) []string {
	t.Helper()
	for _, r := range resp.Resources {
		l := &envoy_config_listener_v3.Listener{}
		require.NoError(t, proto.Unmarshal(r.Value, l))
		if l.Name != xdscache_v3.ENVOY_HTTPS_LISTENER {
			continue
		}
		for _, fc := range l.FilterChains {
			if fc.FilterChainMatch == nil {
				continue
			}
			for _, n := range fc.FilterChainMatch.GetServerNames() {
				if n != fqdn {
					continue
				}
				require.NotEmpty(t, fc.Filters, "filter chain for %q has no network filters", fqdn)
				hcm := &envoy_filter_network_http_connection_manager_v3.HttpConnectionManager{}
				require.NoError(t, proto.Unmarshal(fc.Filters[0].GetTypedConfig().Value, hcm))
				names := make([]string, 0, len(hcm.HttpFilters))
				for _, f := range hcm.HttpFilters {
					names = append(names, f.Name)
				}
				return names
			}
		}
	}
	t.Fatalf("secure filter chain for %q not found in LDS response", fqdn)
	return nil
}

// TestGeoIPFilterGating verifies the GeoIP filter is added to a secure vhost's
// HCM only when the global geoIP config is set and the vhost uses geo rules.
// With no global config, the filter is absent (geo RBAC is still produced).
func TestGeoIPFilterGating(t *testing.T) {
	geoProxy := func() *contour_v1.HTTPProxy {
		return &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "geofilter", Namespace: "default",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "geo.test.com",
					TLS:  &contour_v1.TLS{SecretName: "secret"},
					GeoDenyFilterPolicy: []contour_v1.GeoFilterPolicy{
						{Dimension: contour_v1.GeoDimensionAnon, Value: "true"},
					},
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{Name: "backend", Port: 80}},
				}},
			},
		}
	}

	// With the global geoIP config set, the secure vhost's HCM includes the
	// GeoIP filter.
	t.Run("global geoIP config set adds the filter", func(t *testing.T) {
		rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
			conf.GeoIPConfig = &dag.GeoIPConfig{AnonDbPath: "/anon.mmdb"}
		})
		defer done()

		rh.OnAdd(featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate))
		s1 := fixture.NewService("backend").
			WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
		rh.OnAdd(s1)
		rh.OnAdd(geoProxy())

		names := httpsHCMFilterNames(t, c.Request(listenerType, xdscache_v3.ENVOY_HTTPS_LISTENER).DiscoveryResponse, "geo.test.com")
		require.True(t, containsFilter(names, envoy_v3.GeoIPFilterName),
			"expected GeoIP filter %q in secure HCM, got %v", envoy_v3.GeoIPFilterName, names)
	})

	// Without global geoIP config, the GeoIP filter is absent.
	t.Run("no global geoIP config omits the filter", func(t *testing.T) {
		rh, c, done := setup(t)
		defer done()

		rh.OnAdd(featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate))
		s1 := fixture.NewService("backend").
			WithPorts(core_v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)})
		rh.OnAdd(s1)
		rh.OnAdd(geoProxy())

		names := httpsHCMFilterNames(t, c.Request(listenerType, xdscache_v3.ENVOY_HTTPS_LISTENER).DiscoveryResponse, "geo.test.com")
		require.False(t, containsFilter(names, envoy_v3.GeoIPFilterName),
			"did not expect GeoIP filter %q in secure HCM, got %v", envoy_v3.GeoIPFilterName, names)
	})
}
