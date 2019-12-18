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

package featuretests

import (
	"testing"

	"k8s.io/client-go/tools/cache"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
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
		ObjectMeta: meta("default/kuard"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	vhost := &projcontour.HTTPProxy{
		ObjectMeta: meta("default/kuard"),
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "kuard.projectcontour.io",
			},
			Routes: []projcontour.Route{{
				Conditions: conditions(prefixCondition("/api")),
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				PathRewrite: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{
							Replacement: "/api/v1",
						},
					},
				},
			}},
		},
	}

	rh.OnAdd(vhost)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					envoy.Route(
						routePrefix("/api/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1/"),
					),
					envoy.Route(
						routePrefix("/api"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	)

	// Update the vhost to make the replacement ambiguous. This should remove the generated config.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewrite.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Replacement: "/api/v1"},
					{Replacement: "/api/v2"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Equals(projcontour.Status{
		CurrentStatus: k8s.StatusInvalid,
		Description:   "ambiguous prefix replacement",
	})

	// The replacement isn't ambiguous any more because only one of the prefixes matches.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewrite.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/foo", Replacement: "/api/v1"},
					{Prefix: "/api", Replacement: "/api/v2"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					envoy.Route(
						routePrefix("/api/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2/"),
					),
					envoy.Route(
						routePrefix("/api"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	)

	// But having duplicate prefixes in the replacements makes
	// it ambigious again.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewrite.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/foo", Replacement: "/api/v1"},
					{Prefix: "/foo", Replacement: "/api/v2"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http"),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Equals(projcontour.Status{
		CurrentStatus: k8s.StatusInvalid,
		Description:   "duplicate replacement prefix '/foo'",
	})

	// The "/api" prefix should have precedence over the empty prefix.
	vhost = update(rh, vhost,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].PathRewrite.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/api", Replacement: "/api/full"},
					{Prefix: "", Replacement: "/api/empty"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("kuard.projectcontour.io",
					envoy.Route(
						routePrefix("/api/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/full/"),
					),
					envoy.Route(
						routePrefix("/api"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/full"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
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
					envoy.Route(
						routePrefix("/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/empty"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	)
}

func multiInclude(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: meta("default/kuard"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	vhost1 := &projcontour.HTTPProxy{
		ObjectMeta: meta("default/host1"),
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host1.projectcontour.io",
			},
			Includes: []projcontour.Include{{
				Name:       "app",
				Namespace:  "default",
				Conditions: conditions(prefixCondition("/v1")),
			}},
		},
	}

	vhost2 := &projcontour.HTTPProxy{
		ObjectMeta: meta("default/host2"),
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host2.projectcontour.io",
			},
			Includes: []projcontour.Include{{
				Name:       "app",
				Namespace:  "default",
				Conditions: conditions(prefixCondition("/v2")),
			}},
		},
	}

	app := &projcontour.HTTPProxy{
		ObjectMeta: meta("default/app"),
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				PathRewrite: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Prefix: "/v2", Replacement: "/api/v2"},
						{Prefix: "/v1", Replacement: "/api/v1"},
					},
				},
			}},
		},
	}

	rh.OnAdd(vhost1)
	rh.OnAdd(vhost2)
	rh.OnAdd(app)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					envoy.Route(routePrefix("/v1/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1/"),
					),
					envoy.Route(routePrefix("/v1"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1"),
					),
				),
				envoy.VirtualHost("host2.projectcontour.io",
					envoy.Route(routePrefix("/v2/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2/"),
					),
					envoy.Route(routePrefix("/v2"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v2"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	)

	// Remove one of the replacements, and one cluster loses the rewrite.
	update(rh, app,
		func(app *projcontour.HTTPProxy) {
			app.Spec.Routes[0].PathRewrite.ReplacePrefix =
				[]projcontour.ReplacePrefix{
					{Prefix: "/v1", Replacement: "/api/v1"},
				}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					envoy.Route(routePrefix("/v1/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1/"),
					),
					envoy.Route(routePrefix("/v1"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/api/v1"),
					),
				),
				envoy.VirtualHost("host2.projectcontour.io",
					envoy.Route(routePrefix("/v2"),
						routeCluster("default/kuard/8080/da39a3ee5e"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	)
}

func replaceWithSlash(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: meta("default/kuard"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	vhost1 := &projcontour.HTTPProxy{
		ObjectMeta: meta("default/host1"),
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host1.projectcontour.io",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				Conditions: conditions(prefixCondition("/foo")),
				PathRewrite: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Replacement: "/"},
					},
				},
			}},
		},
	}

	vhost2 := &projcontour.HTTPProxy{
		ObjectMeta: meta("default/host2"),
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "host2.projectcontour.io",
			},
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				Conditions: conditions(prefixCondition("/bar/")),
				PathRewrite: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Replacement: "/"},
					},
				},
			}},
		},
	}

	rh.OnAdd(vhost1)
	rh.OnAdd(vhost2)

	// Make sure that when we rewrite prefix routing conditions
	// '/foo' and '/foo/' to '/', we don't omit the '/' or emit
	// too many '/'s.
	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					envoy.Route(routePrefix("/foo/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					),
					envoy.Route(routePrefix("/foo"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					),
				),
				envoy.VirtualHost("host2.projectcontour.io",
					envoy.Route(routePrefix("/bar/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					),
					envoy.Route(routePrefix("/bar"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	)

	// Not swap the routing and replacement prefixes. Because the routing
	// prefix is '/', the replacement should just end up being prepended
	// to whatever the client URL is. No special handling of trailing '/'.
	update(rh, vhost2,
		func(vhost *projcontour.HTTPProxy) {
			vhost.Spec.Routes[0].Conditions = conditions(prefixCondition("/"))
			vhost.Spec.Routes[0].PathRewrite = &projcontour.PathRewritePolicy{
				ReplacePrefix: []projcontour.ReplacePrefix{
					{Replacement: "/bar"},
				},
			}
		})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("host1.projectcontour.io",
					envoy.Route(routePrefix("/foo/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					),
					envoy.Route(routePrefix("/foo"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/"),
					),
				),
				envoy.VirtualHost("host2.projectcontour.io",
					envoy.Route(routePrefix("/"),
						withPrefixRewrite(routeCluster("default/kuard/8080/da39a3ee5e"), "/bar"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	}).Status(vhost1).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
	).Status(vhost2).Like(
		projcontour.Status{CurrentStatus: k8s.StatusValid},
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
		ObjectMeta: meta("artifactory/service"),
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&projcontour.HTTPProxy{
		ObjectMeta: meta("artifactory/routes"),
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Services: []projcontour.Service{{
					Name: "service",
					Port: 8080,
				}},
				PathRewrite: &projcontour.PathRewritePolicy{
					ReplacePrefix: []projcontour.ReplacePrefix{
						{Prefix: "/v2/container-sandbox", Replacement: "/artifactory/api/docker/container-sandbox/v2"},
						{Prefix: "/v2/container-release", Replacement: "/artifactory/api/docker/container-release/v2"},
						{Prefix: "/v2/container-external", Replacement: "/artifactory/api/docker/container-external/v2"},
						{Prefix: "/v2/container-public", Replacement: "/artifactory/api/docker/container-public/v2"},
					},
				},
			}},
		},
	})

	rh.OnAdd(&projcontour.HTTPProxy{
		ObjectMeta: meta("artifactory/artifactory"),
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "artifactory.projectcontour.io",
			},
			Includes: []projcontour.Include{
				{Name: "routes", Conditions: conditions(prefixCondition("/v2/container-sandbox"))},
				{Name: "routes", Conditions: conditions(prefixCondition("/v2/container-release"))},
				{Name: "routes", Conditions: conditions(prefixCondition("/v2/container-external"))},
				{Name: "routes", Conditions: conditions(prefixCondition("/v2/container-public"))},
			},
		},
	})

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost("artifactory.projectcontour.io",

					envoy.Route(routePrefix("/v2/container-sandbox/"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-sandbox/v2/"),
					),
					envoy.Route(routePrefix("/v2/container-sandbox"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-sandbox/v2"),
					),
					envoy.Route(routePrefix("/v2/container-release/"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-release/v2/"),
					),
					envoy.Route(routePrefix("/v2/container-release"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-release/v2"),
					),
					envoy.Route(routePrefix("/v2/container-public/"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-public/v2/"),
					),
					envoy.Route(routePrefix("/v2/container-public"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-public/v2"),
					),
					envoy.Route(routePrefix("/v2/container-external/"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-external/v2/"),
					),
					envoy.Route(routePrefix("/v2/container-external"),
						withPrefixRewrite(routeCluster("artifactory/service/8080/da39a3ee5e"),
							"/artifactory/api/docker/container-external/v2"),
					),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
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
