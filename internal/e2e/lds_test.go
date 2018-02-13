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
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_config_v2_accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoy_config_v2_http_conn_mgr "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
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
		Resources:   []types.Any{},
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
		Resources: []types.Any{
			any(t, listener("ingress_http", "0.0.0.0", 8080,
				filterchain(false, httpfilter("ingress_http")),
			)),
		},
		TypeUrl: cgrpc.ListenerType,
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
		TypeUrl:     cgrpc.ListenerType,
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
		Resources:   []types.Any{},
		TypeUrl:     cgrpc.ListenerType,
		Nonce:       "0",
	}, fetchLDS(t, cc))

	// add ingress and assert the existence of ingress_http and ingres_https
	rh.OnAdd(i1)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
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
		Resources:   []types.Any{},
		TypeUrl:     cgrpc.ListenerType,
		Nonce:       "0",
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

func listener(name, address string, port uint32, filterchains ...envoy_api_v2_listener.FilterChain) *v2.Listener {
	return &v2.Listener{
		Name:         name,
		Address:      socketaddress(address, port),
		FilterChains: filterchains,
	}
}

func socketaddress(address string, port uint32) envoy_api_v2_core.Address {
	return envoy_api_v2_core.Address{
		Address: &envoy_api_v2_core.Address_SocketAddress{
			SocketAddress: &envoy_api_v2_core.SocketAddress{
				Protocol: envoy_api_v2_core.TCP,
				Address:  address,
				PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func filterchain(useproxy bool, filters ...envoy_api_v2_listener.Filter) envoy_api_v2_listener.FilterChain {
	fc := envoy_api_v2_listener.FilterChain{
		Filters: filters,
	}
	if useproxy {
		fc.UseProxyProto = &types.BoolValue{Value: true}
	}
	return fc
}

func filterchaintls(domains []string, cert, key string, useproxy bool, filters ...envoy_api_v2_listener.Filter) envoy_api_v2_listener.FilterChain {
	fc := filterchain(useproxy, filters...)
	fc.FilterChainMatch = &envoy_api_v2_listener.FilterChainMatch{
		SniDomains: domains,
	}
	fc.TlsContext = &envoy_api_v2_auth.DownstreamTlsContext{
		CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
			TlsParams: &envoy_api_v2_auth.TlsParameters{
				TlsMinimumProtocolVersion: envoy_api_v2_auth.TlsParameters_TLSv1_1,
			},
			TlsCertificates: []*envoy_api_v2_auth.TlsCertificate{{
				CertificateChain: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
						InlineBytes: []byte(cert),
					},
				},
				PrivateKey: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
						InlineBytes: []byte(key),
					},
				},
			}},
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
	}
	return fc
}

func httpfilter(routename string) envoy_api_v2_listener.Filter {
	return envoy_api_v2_listener.Filter{
		Name: "envoy.http_connection_manager",
		Config: messageToStruct(&envoy_config_v2_http_conn_mgr.HttpConnectionManager{
			StatPrefix: routename,
			RouteSpecifier: &envoy_config_v2_http_conn_mgr.HttpConnectionManager_Rds{
				Rds: &envoy_config_v2_http_conn_mgr.Rds{
					ConfigSource: envoy_api_v2_core.ConfigSource{
						ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_ApiConfigSource{
							ApiConfigSource: &envoy_api_v2_core.ApiConfigSource{
								ApiType:      envoy_api_v2_core.ApiConfigSource_GRPC,
								ClusterNames: []string{"contour"},
								GrpcServices: []*envoy_api_v2_core.GrpcService{{
									TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
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
			AccessLog: []*envoy_config_v2_accesslog.AccessLog{{
				Name: "envoy.file_access_log",
				Config: messageToStruct(&envoy_config_v2_accesslog.FileAccessLog{
					Path: "/dev/stdout",
				}),
			}},
			UseRemoteAddress: &types.BoolValue{Value: true},
			HttpFilters: []*envoy_config_v2_http_conn_mgr.HttpFilter{{
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
