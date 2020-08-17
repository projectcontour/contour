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
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/timeout"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTimeoutsNotSpecified(t *testing.T) {
	// the contour.EventHandler.ListenerConfig has no timeout values specified
	rh, c, done := setup(t)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(listenerType, contour.ENVOY_HTTP_LISTENER).Equals(&v2.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&v2.Listener{
				Name:          contour.ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManagerBuilder().
					RouteConfigName(contour.ENVOY_HTTP_LISTENER).
					MetricsPrefix(contour.ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy.FileAccessLogEnvoy(contour.DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					Get(),
				),
			}),
	})
}

func TestNonZeroTimeoutsSpecified(t *testing.T) {
	withTimeouts := func(conf *contour.ListenerConfig) {
		conf.ConnectionIdleTimeout = timeout.DurationSetting(7 * time.Second)
		conf.StreamIdleTimeout = timeout.DurationSetting(70 * time.Second)
		conf.MaxConnectionDuration = timeout.DurationSetting(700 * time.Second)
		conf.ConnectionShutdownGracePeriod = timeout.DurationSetting(7000 * time.Second)
	}

	rh, c, done := setup(t, withTimeouts)
	defer done()

	s1 := fixture.NewService("backend").
		WithPorts(v1.ServicePort{Name: "http", Port: 80})
	rh.OnAdd(s1)

	hp1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: s1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: matchconditions(prefixMatchCondition("/")),
				Services: []projcontour.Service{{
					Name: s1.Name,
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(hp1)

	c.Request(listenerType, contour.ENVOY_HTTP_LISTENER).Equals(&v2.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&v2.Listener{
				Name:          contour.ENVOY_HTTP_LISTENER,
				Address:       envoy.SocketAddress("0.0.0.0", 8080),
				SocketOptions: envoy.TCPKeepaliveSocketOptions(),
				FilterChains: envoy.FilterChains(envoy.HTTPConnectionManagerBuilder().
					RouteConfigName(contour.ENVOY_HTTP_LISTENER).
					MetricsPrefix(contour.ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy.FileAccessLogEnvoy(contour.DEFAULT_HTTP_ACCESS_LOG)).
					DefaultFilters().
					ConnectionIdleTimeout(timeout.DurationSetting(7 * time.Second)).
					StreamIdleTimeout(timeout.DurationSetting(70 * time.Second)).
					MaxConnectionDuration(timeout.DurationSetting(700 * time.Second)).
					ConnectionShutdownGracePeriod(timeout.DurationSetting(7000 * time.Second)).
					Get(),
				),
			}),
	})
}
