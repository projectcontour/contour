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

package featuretests

import (
	"testing"

	"github.com/projectcontour/contour/internal/fixture"
	"k8s.io/client-go/tools/cache"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Update helper to modify a proxy and call rh.OnUpdate. Returns the modified object.
func update(rh cache.ResourceEventHandler, old *projcontour.HTTPProxy, modify func(*projcontour.HTTPProxy)) *projcontour.HTTPProxy {
	updated := old.DeepCopy()

	modify(updated)

	rh.OnUpdate(old, updated)
	return updated
}

func basic(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	vhost := fixture.NewProxy("kuard").WithSpec(
		projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.projectcontour.io",
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/api")),
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				PathRewritePolicy: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{
							Replacement: "/api/v1",
						},
					},
				},
			}},
		})

	rh.OnAdd(vhost)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/api/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/api"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)

	// Update the vhost to make the replacement ambiguous. This should remove the generated config.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewritePolicy.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Replacement: "/api/v1"},
					{Replacement: "/api/v2"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Equals(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusInvalid,
		Description:   "ambiguous prefix replacement",
	})

	// The replacement isn't ambiguous any more because only one of the prefixes matches.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewritePolicy.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/foo", Replacement: "/api/v1"},
					{Prefix: "/api", Replacement: "/api/v2"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/api/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/api"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)

	// But having duplicate prefixes in the replacements makes
	// it ambigious again.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewritePolicy.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/foo", Replacement: "/api/v1"},
					{Prefix: "/foo", Replacement: "/api/v2"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Equals(projcontour.HTTPProxyStatus{
		CurrentStatus: k8s.StatusInvalid,
		Description:   "duplicate replacement prefix '/foo'",
	})

	// The "/api" prefix should have precedence over the empty prefix.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewritePolicy.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/api", Replacement: "/api/full"},
					{Prefix: "", Replacement: "/api/empty"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/api/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/full/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/api"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/full"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)

	// If we remove the prefix match condition, the implicit '/' prefix
	// will be used. So we expect that the default replacement prefix
	// will be used.
	update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].Conditions = nil
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/empty"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)
}

func multiInclude(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	vhost1 := fixture.NewProxy("host1").WithSpec(
		projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host1.projectcontour.io",
			},
			Includes: []projcontour.Include{{
				Name:       "app",
				Namespace:  "default",
				Conditions: matchconditions(prefixMatchCondition("/v1")),
			}},
		})

	vhost2 := fixture.NewProxy("host2").WithSpec(
		projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host2.projectcontour.io",
			},
			Includes: []projcontour.Include{{
				Name:       "app",
				Namespace:  "default",
				Conditions: matchconditions(prefixMatchCondition("/v2")),
			}},
		})

	app := fixture.NewProxy("app").WithSpec(
		projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				PathRewritePolicy: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Prefix: "/v2", Replacement: "/api/v2"},
						{Prefix: "/v1", Replacement: "/api/v1"},
					},
				},
			}},
		})

	rh.OnAdd(vhost1)
	rh.OnAdd(vhost2)
	rh.OnAdd(app)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/v1/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/v1"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1"),
					},
				),
				envoy.VirtualHost("host2.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/v2/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/v2"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)

	// Remove one of the replacements, and one cluster loses the rewrite.
	update(rh, app,
		func(app *projcontour.HTTPProxy) {
			app.Spec.Routes[0].PathRewritePolicy.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/v1", Replacement: "/api/v1"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/v1/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/v1"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1"),
					},
				),
				envoy.VirtualHost("host2.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/v2"),
						Action: routeCluster("default/kuard/8080/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)
}

func replaceWithSlash(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: fixture.ObjectMeta("default/kuard"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	vhost1 := fixture.NewProxy("host1").WithSpec(
		projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host1.projectcontour.io",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				Conditions: matchconditions(prefixMatchCondition("/foo")),
				PathRewritePolicy: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Replacement: "/"},
					},
				},
			}},
		})

	vhost2 := fixture.NewProxy("host2").WithSpec(
		projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host2.projectcontour.io",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				Conditions: matchconditions(prefixMatchCondition("/bar/")),
				PathRewritePolicy: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Replacement: "/"},
					},
				},
			}},
		})

	rh.OnAdd(vhost1)
	rh.OnAdd(vhost2)

	// Make sure that when we rewrite prefix routing conditions
	// '/foo' and '/foo/' to '/', we don't omit the '/' or emit
	// too many '/'s.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/foo/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/foo"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					},
				),
				envoy.VirtualHost("host2.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/bar/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/bar"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)

	// Not swap the routing and replacement prefixes. Because the routing
	// prefix is '/', the replacement should just end up being prepended
	// to whatever the client URL is. No special handling of trailing '/'.
	update(rh, vhost2,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].Conditions = matchconditions(prefixMatchCondition("/"))
			vhost.Spec.Routes[0].PathRewritePolicy = &projcontour.PathRewritePolicy{
				ReplacePrefix: []projcontour.ReplacePrefix{
					{Replacement: "/bar"},
				},
			}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/foo/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					},
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/foo"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					},
				),
				envoy.VirtualHost("host2.projectcontour.io",
					&envoy_api_v2_route.Route{
						Match:  routePrefix("/"),
						Action: withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/bar"),
					},
				),
			),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.HTTPProxyStatus{CurrentStatus: k8s.StatusValid},
	)
}

// artifactoryDocker simulates an Artifactory Docker registry service.
// Artifactory is hosting multiple Docker repositories and we need to
// rewrite the external path used by the docker client to the Artifactory
// API path. We take advantage of multiple inclusion to generate the
// different prefix paths, and then use a single replacement block on
// the route to the service.
func artifactoryDocker(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: fixture.ObjectMeta("artifactory/service"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(fixture.NewProxy("artifactory/routes").WithSpec(
		projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "service",
					Port: 8080,
				}},
				PathRewritePolicy: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Prefix: "/v2/container-sandbox", Replacement: "/artifactory/api/docker/container-sandbox/v2"},
						{Prefix: "/v2/container-release", Replacement: "/artifactory/api/docker/container-release/v2"},
						{Prefix: "/v2/container-external", Replacement: "/artifactory/api/docker/container-external/v2"},
						{Prefix: "/v2/container-public", Replacement: "/artifactory/api/docker/container-public/v2"},
					},
				},
			}},
		}),
	)

	rh.OnAdd(fixture.NewProxy("artifactory/artifactory").WithSpec(
		projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "artifactory.projectcontour.io",
			},
			Includes: []projcontour.Include{
				{Name: "routes", Conditions: matchconditions(prefixMatchCondition("/v2/container-sandbox"))},
				{Name: "routes", Conditions: matchconditions(prefixMatchCondition("/v2/container-release"))},
				{Name: "routes", Conditions: matchconditions(prefixMatchCondition("/v2/container-external"))},
				{Name: "routes", Conditions: matchconditions(prefixMatchCondition("/v2/container-public"))},
			},
		}),
	)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("artifactory.projectcontour.io",

					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-sandbox/"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-sandbox/v2/"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-sandbox"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-sandbox/v2"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-release/"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-release/v2/"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-release"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-release/v2"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-public/"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-public/v2/"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-public"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-public/v2"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-external/"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-external/v2/"),
					},
					&envoy_api_v2_route.Route{
						Match: routePrefix("/v2/container-external"),
						Action: withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-external/v2"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func TestHTTPProxyPathPrefix(t *testing.T) {
	subtests := []struct {
		Name string
		Func func(*testing.T)
	}{
		{Name: "Basic", Func: basic},
		{Name: "MultiInclude", Func: multiInclude},
		{Name: "ReplaceWithSlash", Func: replaceWithSlash},
		{Name: "ArtifactoryDocker", Func: artifactoryDocker},
	}

	for _, s := range subtests {
		t.Run(s.Name, s.Func)
	}
}
