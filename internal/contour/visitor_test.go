// Copyright © 2018 Heptio
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

package contour

import (
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/google/go-cmp/cmp"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/envoy"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestVisitClusters(t *testing.T) {
	tests := map[string]struct {
		root dag.Visitable
		want map[string]*v2.Cluster
	}{
		"TCPService forward": {
			root: &dag.SecureVirtualHost{
				VirtualHost: dag.VirtualHost{
					Port: 443,
					Host: "www.example.com",
					TCPProxy: &dag.TCPProxy{
						Services: []*dag.TCPService{{
							Name:      "example",
							Namespace: "default",
							ServicePort: &v1.ServicePort{
								Protocol:   "TCP",
								Port:       443,
								TargetPort: intstr.FromInt(8443),
							},
						}},
					},
				},
				Secret: new(dag.Secret),
			},
			want: clustermap(
				&v2.Cluster{
					Name:        "default/example/443/da39a3ee5e",
					AltStatName: "default_example_443",
					Type:        v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   envoy.ConfigSource("contour"),
						ServiceName: "default/example",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
					CommonLbConfig: envoy.ClusterCommonLBConfig(),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitClusters(tc.root)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestVisitListeners(t *testing.T) {
	p1 := &dag.TCPProxy{
		Services: []*dag.TCPService{{
			Name:      "example",
			Namespace: "default",
			ServicePort: &v1.ServicePort{
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(8443),
			},
		}},
	}

	tests := map[string]struct {
		root dag.Visitable
		want map[string]*v2.Listener
	}{
		"TCPService forward": {
			root: &dag.SecureVirtualHost{
				VirtualHost: dag.VirtualHost{
					Port:     443,
					Host:     "tcpproxy.example.com",
					TCPProxy: p1,
				},
				Secret: &dag.Secret{
					Object: &v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: secretdata("certificate", "key"),
					},
				},
				MinProtoVersion: auth.TlsParameters_TLSv1_1,
			},
			want: listenermap(
				&v2.Listener{
					Name:    ENVOY_HTTPS_LISTENER,
					Address: envoy.SocketAddress("0.0.0.0", 8443),
					FilterChains: []listener.FilterChain{{
						FilterChainMatch: &listener.FilterChainMatch{
							ServerNames: []string{"tcpproxy.example.com"},
						},
						TlsContext: tlscontext(auth.TlsParameters_TLSv1_1),
						Filters:    filters(envoy.TCPProxy(ENVOY_HTTPS_LISTENER, p1, DEFAULT_HTTPS_ACCESS_LOG)),
					}},
					ListenerFilters: []listener.ListenerFilter{
						envoy.TLSInspector(),
					},
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitListeners(tc.root, new(ListenerVisitorConfig))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
