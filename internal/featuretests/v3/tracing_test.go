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

	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/internal/timeout"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTracing(t *testing.T) {
	tracingConfig := &xdscache_v3.TracingConfig{
		ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
			ExtensionService: k8s.NamespacedNameFrom("projectcontour/otel-collector"),
			Timeout:          timeout.DefaultSetting(),
		},
		ServiceName:      "contour",
		OverallSampling:  100,
		MaxPathTagLength: 256,
		CustomTags: []*xdscache_v3.CustomTag{
			{
				TagName: "literal",
				Literal: "this is literal",
			},
			{
				TagName:         "environment",
				EnvironmentName: "HOSTNAME",
			},
			{
				TagName:           "header",
				RequestHeaderName: "X-Custom-Header",
			},
		},
	}
	withTrace := func(conf *xdscache_v3.ListenerConfig) {
		conf.TracingConfig = tracingConfig
	}
	rh, c, done := setup(t, withTrace)
	defer done()

	rh.OnAdd(fixture.NewService("projectcontour/otel-collector").
		WithPorts(corev1.ServicePort{Port: 4317}))

	rh.OnAdd(featuretests.Endpoints("projectcontour", "otel-collector", corev1.EndpointSubset{
		Addresses: featuretests.Addresses("10.244.41.241"),
		Ports:     featuretests.Ports(featuretests.Port("", 4317)),
	}))

	rh.OnAdd(&v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("projectcontour/otel-collector"),
		Spec: v1alpha1.ExtensionServiceSpec{
			Services: []v1alpha1.ExtensionServiceTarget{
				{Name: "otel-collector", Port: 4317},
			},
			Protocol: ref.To("h2c"),
			TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
				Response: defaultResponseTimeout.String(),
			},
		},
	})

	rh.OnAdd(fixture.NewService("projectcontour/app-server").
		WithPorts(corev1.ServicePort{Port: 80}))

	rh.OnAdd(featuretests.Endpoints("projectcontour", "app-server", corev1.EndpointSubset{
		Addresses: featuretests.Addresses("10.244.184.102"),
		Ports:     featuretests.Ports(featuretests.Port("", 80)),
	}))

	p := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "projectcontour",
			Name:      "app-server",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
						{
							Name: "app-server",
							Port: 80,
						},
					},
				},
			},
		},
	}
	rh.OnAdd(p)

	httpListener := defaultHTTPListener()
	httpListener.FilterChains = envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
		RouteConfigName(xdscache_v3.ENVOY_HTTP_LISTENER).
		MetricsPrefix(xdscache_v3.ENVOY_HTTP_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy(xdscache_v3.DEFAULT_HTTP_ACCESS_LOG, "", nil, v1alpha1.LogLevelInfo)).
		DefaultFilters().
		Tracing(envoy_v3.TracingConfig(&envoy_v3.EnvoyTracingConfig{
			ExtensionService: tracingConfig.ExtensionService,
			ServiceName:      tracingConfig.ServiceName,
			Timeout:          tracingConfig.Timeout,
			OverallSampling:  tracingConfig.OverallSampling,
			MaxPathTagLength: tracingConfig.MaxPathTagLength,
			CustomTags: []*envoy_v3.CustomTag{
				{
					TagName: "literal",
					Literal: "this is literal",
				},
				{
					TagName:         "environment",
					EnvironmentName: "HOSTNAME",
				},
				{
					TagName:           "header",
					RequestHeaderName: "X-Custom-Header",
				},
			},
		})).
		Get(),
	)

	c.Request(listenerType, xdscache_v3.ENVOY_HTTP_LISTENER).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl:   listenerType,
		Resources: resources(t, httpListener),
	})

	c.Request(clusterType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/projectcontour/otel-collector", "extension/projectcontour/otel-collector", "extension_projectcontour_otel-collector")),
			),
			DefaultCluster(
				cluster("projectcontour/app-server/80/da39a3ee5e", "projectcontour/app-server", "projectcontour_app-server_80"),
			),
		),
	})
}
