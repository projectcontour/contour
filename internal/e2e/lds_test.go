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

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoy_config_v2_http_conn_mgr "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
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
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
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
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc))

	// i2 is the same as i1 but has the kubernetes.io/ingress.allow-http: "false" annotation
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}

	// update i1 to i2 and verify that ingress_http has gone.
	rh.OnUpdate(i1, i2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, fetchLDS(t, cc))

	// i3 is similar to i2, but uses the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	// to force 80 -> 443 upgrade
	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}

	// update i2 to i3 and check that ingress_http has returned
	rh.OnUpdate(i2, i3)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
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
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, fetchLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: socketaddress("0.0.0.0", 8443),
				FilterChains: []listener.FilterChain{
					filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc))

	// i2 is the same as i1 but has the kubernetes.io/ingress.allow-http: "false" annotation
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}

	// update i1 to i2 and verify that ingress_http has gone.
	rh.OnUpdate(i1, i2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: socketaddress("0.0.0.0", 8443),
				FilterChains: []listener.FilterChain{
					filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc))

	// delete secret and assert that ingress_https is removed
	rh.OnDelete(s1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     listenerType,
		Nonce:       "0",
	}, fetchLDS(t, cc))
}

func TestLDSFilter(t *testing.T) {
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

	// add ingress and fetch ingress_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, &v2.Listener{
				Name:    "ingress_https",
				Address: socketaddress("0.0.0.0", 8443),
				FilterChains: []listener.FilterChain{
					filterchaintls([]string{"kuard.example.com"}, "certificate", "key", false, httpfilter("ingress_https")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc, "ingress_https"))

	// fetch ingress_http
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{

			any(t, &v2.Listener{
				Name:    "ingress_http",
				Address: socketaddress("0.0.0.0", 8080),
				FilterChains: []listener.FilterChain{
					filterchain(false, httpfilter("ingress_http")),
				},
			}),
		},
		TypeUrl: listenerType,
		Nonce:   "0",
	}, fetchLDS(t, cc, "ingress_http"))

	// fetch something non existent.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     listenerType, Nonce: "0",
	}, fetchLDS(t, cc, "HTTP"))
}

func fetchLDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewListenerDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	resp, err := rds.FetchListeners(ctx, &v2.DiscoveryRequest{
		TypeUrl:       listenerType,
		ResourceNames: rn,
	})
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

func socketaddress(address string, port uint32) core.Address {
	return core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  address,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func filterchain(useproxy bool, filters ...listener.Filter) listener.FilterChain {
	fc := listener.FilterChain{
		Filters: filters,
	}
	if useproxy {
		fc.UseProxyProto = &types.BoolValue{Value: true}
	}
	return fc
}

func filterchaintls(domains []string, cert, key string, useproxy bool, filters ...listener.Filter) listener.FilterChain {
	fc := filterchain(useproxy, filters...)
	fc.FilterChainMatch = &listener.FilterChainMatch{
		SniDomains: domains,
	}
	fc.TlsContext = &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: auth.TlsParameters_TLSv1_1,
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
	return fc
}

func httpfilter(routename string) listener.Filter {
	return listener.Filter{
		Name: "envoy.http_connection_manager",
		Config: messageToStruct(&envoy_config_v2_http_conn_mgr.HttpConnectionManager{
			StatPrefix: routename,
			RouteSpecifier: &envoy_config_v2_http_conn_mgr.HttpConnectionManager_Rds{
				Rds: &envoy_config_v2_http_conn_mgr.Rds{
					ConfigSource: core.ConfigSource{
						ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
							ApiConfigSource: &core.ApiConfigSource{
								ApiType:      core.ApiConfigSource_GRPC,
								ClusterNames: []string{"contour"},
								GrpcServices: []*core.GrpcService{{
									TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
											ClusterName: "contour",
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
			HttpFilters: []*envoy_config_v2_http_conn_mgr.HttpFilter{
				{Name: "envoy.grpc_web"},
				{Name: "envoy.router"},
			},
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
