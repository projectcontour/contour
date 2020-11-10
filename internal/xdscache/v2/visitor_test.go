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

package v2

import (
	"testing"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v2 "github.com/projectcontour/contour/internal/envoy/v2"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestVisitClusters(t *testing.T) {
	tests := map[string]struct {
		root dag.Vertex
		want map[string]*envoy_api_v2.Cluster
	}{
		"TCPService forward": {
			root: &dag.Listener{
				Port: 443,
				VirtualHosts: virtualhosts(
					&dag.SecureVirtualHost{
						VirtualHost: dag.VirtualHost{
							Name: "www.example.com",
						},
						TCPProxy: &dag.TCPProxy{
							Clusters: []*dag.Cluster{{
								Upstream: &dag.Service{
									Weighted: dag.WeightedService{
										Weight:           1,
										ServiceName:      "example",
										ServiceNamespace: "default",
										ServicePort: v1.ServicePort{
											Protocol:   "TCP",
											Port:       443,
											TargetPort: intstr.FromInt(8443),
										},
									},
								},
							}},
						},
						Secret: new(dag.Secret),
					},
				),
			},
			want: clustermap(
				&envoy_api_v2.Cluster{
					Name:                 "default/example/443/da39a3ee5e",
					AltStatName:          "default_example_443",
					ClusterDiscoveryType: envoy_v2.ClusterDiscoveryType(envoy_api_v2.Cluster_EDS),
					EdsClusterConfig: &envoy_api_v2.Cluster_EdsClusterConfig{
						EdsConfig:   envoy_v2.ConfigSource("contour"),
						ServiceName: "default/example",
					},
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitClusters(tc.root)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestVisitListeners(t *testing.T) {
	p1 := &dag.TCPProxy{
		Clusters: []*dag.Cluster{{
			Upstream: &dag.Service{
				Weighted: dag.WeightedService{
					Weight:           1,
					ServiceName:      "example",
					ServiceNamespace: "default",
					ServicePort: v1.ServicePort{
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				},
			},
		}},
	}

	tests := map[string]struct {
		root dag.Vertex
		want map[string]*envoy_api_v2.Listener
	}{
		"TCPService forward": {
			root: &dag.Listener{
				Port: 443,
				VirtualHosts: virtualhosts(
					&dag.SecureVirtualHost{
						VirtualHost: dag.VirtualHost{
							Name: "tcpproxy.example.com",
						},
						TCPProxy: p1,
						Secret: &dag.Secret{
							Object: &v1.Secret{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "secret",
									Namespace: "default",
								},
								Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
							},
						},
						MinTLSVersion: envoy_api_v2_auth.TlsParameters_TLSv1_2,
					},
				),
			},
			want: listenermap(
				&envoy_api_v2.Listener{
					Name:    ENVOY_HTTPS_LISTENER,
					Address: envoy_v2.SocketAddress("0.0.0.0", 8443),
					FilterChains: []*envoy_api_v2_listener.FilterChain{{
						FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
							ServerNames: []string{"tcpproxy.example.com"},
						},
						TransportSocket: transportSocket("secret", envoy_api_v2_auth.TlsParameters_TLSv1_2),
						Filters:         envoy_v2.Filters(envoy_v2.TCPProxy(ENVOY_HTTPS_LISTENER, p1, envoy_v2.FileAccessLogEnvoy(DEFAULT_HTTPS_ACCESS_LOG))),
					}},
					ListenerFilters: envoy_v2.ListenerFilters(
						envoy_v2.TLSInspector(),
					),
					SocketOptions: envoy_v2.TCPKeepaliveSocketOptions(),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitListeners(tc.root, new(ListenerConfig))
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestVisitSecrets(t *testing.T) {
	tests := map[string]struct {
		root dag.Vertex
		want map[string]*envoy_api_v2_auth.Secret
	}{
		"TCPService forward": {
			root: &dag.Listener{
				Port: 443,
				VirtualHosts: virtualhosts(
					&dag.SecureVirtualHost{
						VirtualHost: dag.VirtualHost{
							Name: "www.example.com",
						},
						TCPProxy: &dag.TCPProxy{
							Clusters: []*dag.Cluster{{
								Upstream: &dag.Service{
									Weighted: dag.WeightedService{
										Weight:           1,
										ServiceName:      "example",
										ServiceNamespace: "default",
										ServicePort: v1.ServicePort{
											Protocol:   "TCP",
											Port:       443,
											TargetPort: intstr.FromInt(8443),
										},
									},
								},
							}},
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
					},
				),
			},
			want: secretmap(&envoy_api_v2_auth.Secret{
				Name: "default/secret/735ad571c1",
				Type: &envoy_api_v2_auth.Secret_TlsCertificate{
					TlsCertificate: &envoy_api_v2_auth.TlsCertificate{
						PrivateKey: &envoy_api_v2_core.DataSource{
							Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
								InlineBytes: []byte("key"),
							},
						},
						CertificateChain: &envoy_api_v2_core.DataSource{
							Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
								InlineBytes: []byte("certificate"),
							},
						},
					},
				},
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitSecrets(tc.root)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func virtualhosts(vx ...dag.Vertex) []dag.Vertex {
	return vx
}
