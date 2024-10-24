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

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/fixture"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
)

func TestDefaultCompression(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	httpListener := defaultHTTPListener()
	httpListener.FilterChains = envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
		RouteConfigName(xdscache_v3.ENVOY_HTTP_LISTENER).
		MetricsPrefix(xdscache_v3.ENVOY_HTTP_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy(xdscache_v3.DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
		DefaultFilters().
		Get(),
	)

	c.Request(listenerType, xdscache_v3.ENVOY_HTTP_LISTENER).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, httpListener),
	})
}

func TestDisableCompression(t *testing.T) {
	withDisableCompression := func(conf *xdscache_v3.ListenerConfig) {
		conf.Compression = &contour_v1alpha1.EnvoyCompression{
			Algorithm: contour_v1alpha1.DisabledCompression,
		}
	}

	rh, c, done := setup(t, withDisableCompression)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	httpListener := defaultHTTPListener()
	httpListener.FilterChains = envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
		Compression(&contour_v1alpha1.EnvoyCompression{
			Algorithm: contour_v1alpha1.DisabledCompression,
		}).
		RouteConfigName(xdscache_v3.ENVOY_HTTP_LISTENER).
		MetricsPrefix(xdscache_v3.ENVOY_HTTP_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy(xdscache_v3.DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
		DefaultFilters().
		Get(),
	)

	c.Request(listenerType, xdscache_v3.ENVOY_HTTP_LISTENER).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, httpListener),
	})
}

func TestBrotliCompression(t *testing.T) {
	withBrotliCompression := func(conf *xdscache_v3.ListenerConfig) {
		conf.Compression = &contour_v1alpha1.EnvoyCompression{
			Algorithm: contour_v1alpha1.BrotliCompression,
		}
	}

	rh, c, done := setup(t, withBrotliCompression)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	httpListener := defaultHTTPListener()
	httpListener.FilterChains = envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
		Compression(&contour_v1alpha1.EnvoyCompression{
			Algorithm: contour_v1alpha1.BrotliCompression,
		}).
		RouteConfigName(xdscache_v3.ENVOY_HTTP_LISTENER).
		MetricsPrefix(xdscache_v3.ENVOY_HTTP_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy(xdscache_v3.DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
		DefaultFilters().
		Get(),
	)

	c.Request(listenerType, xdscache_v3.ENVOY_HTTP_LISTENER).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, httpListener),
	})
}

func TestZstdCompression(t *testing.T) {
	withZstdCompression := func(conf *xdscache_v3.ListenerConfig) {
		conf.Compression = &contour_v1alpha1.EnvoyCompression{
			Algorithm: contour_v1alpha1.ZstdCompression,
		}
	}

	rh, c, done := setup(t, withZstdCompression)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	hp1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	httpListener := defaultHTTPListener()
	httpListener.FilterChains = envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
		Compression(&contour_v1alpha1.EnvoyCompression{
			Algorithm: contour_v1alpha1.ZstdCompression,
		}).
		RouteConfigName(xdscache_v3.ENVOY_HTTP_LISTENER).
		MetricsPrefix(xdscache_v3.ENVOY_HTTP_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy(xdscache_v3.DEFAULT_HTTP_ACCESS_LOG, "", nil, contour_v1alpha1.LogLevelInfo)).
		DefaultFilters().
		Get(),
	)

	c.Request(listenerType, xdscache_v3.ENVOY_HTTP_LISTENER).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, httpListener),
	})
}
