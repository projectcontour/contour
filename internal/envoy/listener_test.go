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

package envoy

import (
	"bytes"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoy_accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoy_config_v2_tcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/google/go-cmp/cmp"
	"github.com/heptio/contour/internal/dag"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSocketAddress(t *testing.T) {
	const (
		addr = "foo.example.com"
		port = 8123
	)

	got := SocketAddress(addr, port)
	want := core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  addr,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestDownstreamTLSContext(t *testing.T) {
	const (
		cert = "foo"
		key  = "secret"
	)
	got := DownstreamTLSContext([]byte(cert), []byte(key), auth.TlsParameters_TLSv1_1, "h2", "http/1.1")
	want := &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: auth.TlsParameters_TLSv1_1,
				TlsMaximumProtocolVersion: auth.TlsParameters_TLSv1_3,
			},
			TlsCertificates: []*auth.TlsCertificate{{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(cert),
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(key),
					},
				},
			}},
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestTCPProxy(t *testing.T) {
	const (
		statPrefix    = "ingress_https"
		accessLogPath = "/dev/stdout"
	)

	s1 := &dag.TCPService{
		Name:      "example",
		Namespace: "default",
		ServicePort: &v1.ServicePort{
			Protocol:   "TCP",
			Port:       443,
			TargetPort: intstr.FromInt(8443),
		},
	}
	s2 := &dag.TCPService{
		Name:      "example2",
		Namespace: "default",
		ServicePort: &v1.ServicePort{
			Protocol:   "TCP",
			Port:       443,
			TargetPort: intstr.FromInt(8443),
		},
		Weight: 20,
	}

	tests := map[string]struct {
		proxy *dag.TCPProxy
		want  listener.Filter
	}{
		"single cluster": {
			proxy: &dag.TCPProxy{
				Services: []*dag.TCPService{
					s1,
				},
			},
			want: listener.Filter{
				Name: util.TCPProxy,
				Config: messageToStruct(&envoy_config_v2_tcpproxy.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_Cluster{
						Cluster: Clustername(s1),
					},
					AccessLog: []*envoy_accesslog.AccessLog{{
						Name:   util.FileAccessLog,
						Config: messageToStruct(fileAccessLog(accessLogPath)),
					}},
				}),
			},
		},
		"multiple cluster": {
			proxy: &dag.TCPProxy{
				Services: []*dag.TCPService{
					s2, s1, // assert that these are sorted
				},
			},
			want: listener.Filter{
				Name: util.TCPProxy,
				Config: messageToStruct(&envoy_config_v2_tcpproxy.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &envoy_config_v2_tcpproxy.TcpProxy_WeightedClusters{
						WeightedClusters: &envoy_config_v2_tcpproxy.TcpProxy_WeightedCluster{
							Clusters: []*envoy_config_v2_tcpproxy.TcpProxy_WeightedCluster_ClusterWeight{{
								Name:   Clustername(s1),
								Weight: 1,
							}, {
								Name:   Clustername(s2),
								Weight: 20,
							}},
						},
					},
					AccessLog: []*envoy_accesslog.AccessLog{{
						Name:   util.FileAccessLog,
						Config: messageToStruct(fileAccessLog(accessLogPath)),
					}},
				}),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := TCPProxy(statPrefix, tc.proxy, accessLogPath)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

// messageToStruct encodes a protobuf Message into a Struct.
// Hilariously, it uses JSON as the intermediary.
// author:glen@turbinelabs.io
func messageToStruct(msg proto.Message) *types.Struct {
	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, msg); err != nil {
		panic(err)
	}

	pbs := &types.Struct{}
	if err := jsonpb.Unmarshal(buf, pbs); err != nil {
		panic(err)
	}

	return pbs
}

func fileAccessLog(path string) *accesslog_v2.FileAccessLog {
	return &accesslog_v2.FileAccessLog{
		Path: path,
	}
}
