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

package e2e

import (
	"bytes"
	"context"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/envoyproxy/go-control-plane/api/filter/accesslog"
	"github.com/envoyproxy/go-control-plane/api/filter/network"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	cgrpc "github.com/heptio/contour/internal/grpc"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNonTLSListener(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// assert that without any ingress objects registered
	// there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []*types.Any{},
		TypeUrl:     cgrpc.ListenerType,
		Nonce:       "0",
	}, fetchLDS(t, cc))

	// i1 is a simple ingress, no hostname, no tls.
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}

	// add it and assert that we now have a ingress_http listener
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, listener("ingress_http", "0.0.0.0", 8080,
				filterchain(false, httpfilter("ingress_http")),
			)),
		},
		TypeUrl: cgrpc.ListenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc))
}

func TestTLSListener(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	}

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}

	// add secret
	rh.OnAdd(s1)

	// assert that there are no active listeners
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []*types.Any{},
		TypeUrl:     cgrpc.ListenerType,
		Nonce:       "0",
	}, fetchLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, listener("ingress_http", "0.0.0.0", 8080,
				filterchain(false, httpfilter("ingress_http")),
			)),
			any(t, listener("ingress_https", "0.0.0.0", 8443,
				filterchaintls(
					[]string{"kuard.example.com"},
					"certificate", "key",
					false, httpfilter("ingress_https")),
			)),
		},
		TypeUrl: cgrpc.ListenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc))

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, listener("ingress_http", "0.0.0.0", 8080,
				filterchain(false, httpfilter("ingress_http")),
			)),
		},
		TypeUrl: cgrpc.ListenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc))
}

func fetchLDS(t *testing.T, cc *grpc.ClientConn) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewListenerDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	resp, err := rds.FetchListeners(ctx, new(v2.DiscoveryRequest))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func backend(name string, port intstr.IntOrString) *v1beta1.IngressBackend {
	return &v1beta1.IngressBackend{
		ServiceName: name,
		ServicePort: port,
	}
}

func listener(name, address string, port uint32, filterchains ...*v2.FilterChain) *v2.Listener {
	return &v2.Listener{
		Name:         name,
		Address:      socketaddress(address, port),
		FilterChains: filterchains,
	}
}

func socketaddress(address string, port uint32) *v2.Address {
	return &v2.Address{
		Address: &v2.Address_SocketAddress{
			SocketAddress: &v2.SocketAddress{
				Protocol: v2.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &v2.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func filterchain(useproxy bool, filters ...*v2.Filter) *v2.FilterChain {
	fc := v2.FilterChain{
		Filters: filters,
	}
	if useproxy {
		fc.UseProxyProto = &types.BoolValue{Value: true}
	}
	return &fc
}

func filterchaintls(domains []string, cert, key string, useproxy bool, filters ...*v2.Filter) *v2.FilterChain {
	fc := filterchain(useproxy, filters...)
	fc.FilterChainMatch = &v2.FilterChainMatch{
		SniDomains: domains,
	}
	fc.TlsContext = &v2.DownstreamTlsContext{
		CommonTlsContext: &v2.CommonTlsContext{
			TlsParams: &v2.TlsParameters{
				TlsMinimumProtocolVersion: v2.TlsParameters_TLSv1_1,
			},
			TlsCertificates: []*v2.TlsCertificate{{
				CertificateChain: &v2.DataSource{
					Specifier: &v2.DataSource_InlineBytes{
						InlineBytes: []byte(cert),
					},
				},
				PrivateKey: &v2.DataSource{
					Specifier: &v2.DataSource_InlineBytes{
						InlineBytes: []byte(key),
					},
				},
			}},
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
	}
	return fc
}

func httpfilter(routename string) *v2.Filter {
	return &v2.Filter{
		Name: "envoy.http_connection_manager",
		Config: messageToStruct(&network.HttpConnectionManager{
			StatPrefix: routename,
			RouteSpecifier: &network.HttpConnectionManager_Rds{
				Rds: &network.Rds{
					ConfigSource: v2.ConfigSource{
						ConfigSourceSpecifier: &v2.ConfigSource_ApiConfigSource{
							ApiConfigSource: &v2.ApiConfigSource{
								ApiType:      v2.ApiConfigSource_GRPC,
								ClusterNames: []string{"xds_cluster"},
								GrpcServices: []*v2.GrpcService{{
									TargetSpecifier: &v2.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &v2.GrpcService_EnvoyGrpc{
											ClusterName: "xds_cluster",
										},
									},
								}},
							},
						},
					},
					RouteConfigName: routename,
				},
			},
			AccessLog: []*accesslog.AccessLog{{
				Name: "envoy.file_access_log",
				Config: messageToStruct(&accesslog.FileAccessLog{
					Path: "/dev/stdout",
				}),
			}},
			UseRemoteAddress: &types.BoolValue{Value: true},
			HttpFilters: []*network.HttpFilter{{
				Name: "envoy.router",
			}},
		}),
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
