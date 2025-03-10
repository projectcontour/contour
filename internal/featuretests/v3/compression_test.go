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

func TestCompression(t *testing.T) {
	tests := map[string]struct {
		algorithm contour_v1alpha1.CompressionAlgorithm
		want      contour_v1alpha1.CompressionAlgorithm
	}{
		"default":  {algorithm: "", want: contour_v1alpha1.GzipCompression},
		"disabled": {algorithm: contour_v1alpha1.DisabledCompression, want: contour_v1alpha1.DisabledCompression},
		"brotli":   {algorithm: contour_v1alpha1.BrotliCompression, want: contour_v1alpha1.BrotliCompression},
		"zstd":     {algorithm: contour_v1alpha1.ZstdCompression, want: contour_v1alpha1.ZstdCompression},
		"gzip":     {algorithm: contour_v1alpha1.GzipCompression, want: contour_v1alpha1.GzipCompression},
	}

	s1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rh, c, done := setup(t, func(conf *xdscache_v3.ListenerConfig) {
				if tc.algorithm != "" {
					conf.Compression = &contour_v1alpha1.EnvoyCompression{
						Algorithm: tc.algorithm,
					}
				}
			})
			defer done()

			rh.OnAdd(s1)
			rh.OnAdd(hp1)
			httpListener := defaultHTTPListener()
			envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
				XDSClusterName: envoy_v3.DefaultXDSClusterName,
			})
			httpListener.FilterChains = envoy_v3.FilterChains(envoyGen.HTTPConnectionManagerBuilder().
				Compression(&contour_v1alpha1.EnvoyCompression{
					Algorithm: tc.want,
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
		})
	}
}
