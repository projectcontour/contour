// Copyright © 2017 Heptio
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
	"fmt"
	"path/filepath"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/gogo/protobuf/types"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
)

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	listenerCache
	Cond
}

// recomputeTLSListerner recomputes the SSL listener for port 8443
// using the list of ingresses and secrets provided. If the list of
// TLS enabled listeners is zero, the listener is removed.
func (lc *ListenerCache) recomputeTLSListener(ingresses map[metadata]*v1beta1.Ingress, secrets map[metadata]*v1.Secret) {
	l := &v2.Listener{
		Name: ENVOY_HTTPS_LISTENER, // TODO(dfc) should come from the name of the service port
		Address: &v2.Address{
			Address: &v2.Address_SocketAddress{
				SocketAddress: &v2.SocketAddress{
					Protocol: v2.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &v2.SocketAddress_PortValue{
						PortValue: 8443,
					},
				},
			},
		},
	}

	for _, i := range ingresses {
		if len(i.Spec.TLS) == 0 {
			// this ingress does not use TLS, skip it
			fmt.Printf("ingress %s/%s does not use tls\n", i.Namespace, i.Name)
			continue
		}
		for _, tls := range i.Spec.TLS {
			_, ok := secrets[metadata{name: tls.SecretName, namespace: i.Namespace}]
			if !ok {
				fmt.Printf("ingress %s/%s: secret %s/%s not found in cache\n", i.Namespace, i.Name, i.Namespace, tls.SecretName)
				continue
			}
			fmt.Printf("ingress %s/%s: secret %s/%s found in cache\n", i.Namespace, i.Name, i.Namespace, tls.SecretName)

			const base = "/config/ssl"

			fc := &v2.FilterChain{
				FilterChainMatch: &v2.FilterChainMatch{
					SniDomains: tls.Hosts,
				},
				TlsContext: &v2.DownstreamTlsContext{
					CommonTlsContext: &v2.CommonTlsContext{
						TlsCertificates: []*v2.TlsCertificate{{
							CertificateChain: &v2.DataSource{
								&v2.DataSource_Filename{
									Filename: filepath.Join(base, i.Namespace, tls.SecretName, v1.TLSCertKey),
								},
							},
							PrivateKey: &v2.DataSource{
								&v2.DataSource_Filename{
									Filename: filepath.Join(base, i.Namespace, tls.SecretName, v1.TLSPrivateKeyKey),
								},
							},
						}},
					},
				},
				Filters: []*v2.Filter{{
					Name: httpFilter,
					Config: &types.Struct{
						Fields: map[string]*types.Value{
							"codec_type":  sv("auto"),
							"stat_prefix": sv("ingress_https"),
							"rds": st(map[string]*types.Value{
								"route_config_name": sv("ingress_http"), // TODO(dfc) issue 103
								"config_source": st(map[string]*types.Value{
									"api_config_source": st(map[string]*types.Value{
										"api_type": sv("grpc"),
										"cluster_name": lv(
											sv("xds_cluster"),
										),
									}),
								}),
							}),
							"http_filters": lv(
								st(map[string]*types.Value{
									"name": sv(router),
								}),
							),
							"access_log": st(map[string]*types.Value{
								"name": sv(accessLog),
								"config": st(map[string]*types.Value{
									"path": sv("/dev/stdout"),
								}),
							}),
						},
					},
				}},
			}
			l.FilterChains = append(l.FilterChains, fc)
		}
	}

	defer lc.Notify()
	switch len(l.FilterChains) {
	case 0:
		// no tls ingresses registered, remove the listener
		lc.Remove(ENVOY_HTTPS_LISTENER)
	default:
		// at least one tls ingress registered, refresh listener
		lc.Add(l)
	}
}

const (
	ENVOY_HTTP_LISTENER  = "ingress_http"
	ENVOY_HTTPS_LISTENER = "ingress_https"

	router     = "envoy.router"
	httpFilter = "envoy.http_connection_manager"
	accessLog  = "envoy.file_access_log"
)

func defaultListener() *v2.Listener {
	return &v2.Listener{
		Name: ENVOY_HTTP_LISTENER, // TODO(dfc) should come from the name of the service port
		Address: &v2.Address{
			Address: &v2.Address_SocketAddress{
				SocketAddress: &v2.SocketAddress{
					Protocol: v2.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &v2.SocketAddress_PortValue{
						PortValue: 8080,
					},
				},
			},
		},
		FilterChains: []*v2.FilterChain{{
			Filters: []*v2.Filter{{
				Name: httpFilter,
				Config: &types.Struct{
					Fields: map[string]*types.Value{
						"codec_type":  sv("http1"),        // let's not go crazy now
						"stat_prefix": sv("ingress_http"), // TODO(dfc) should this come from pod.Name?
						"rds": st(map[string]*types.Value{
							"route_config_name": sv("ingress_http"), // TODO(dfc) needed for grpc?
							"config_source": st(map[string]*types.Value{
								"api_config_source": st(map[string]*types.Value{
									"api_type": sv("grpc"),
									"cluster_name": lv(
										sv("xds_cluster"),
									),
								}),
							}),
						}),
						"http_filters": lv(
							st(map[string]*types.Value{
								"name": sv(router),
							}),
						),
						"access_log": st(map[string]*types.Value{
							"name": sv(accessLog),
							"config": st(map[string]*types.Value{
								"path": sv("/dev/stdout"),
							}),
						}),
						"use_remote_address": bv(true), // TODO(jbeda) should this ever be false?
					},
				},
			}},
		}},
	}
}

func sv(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func bv(b bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}}
}

func st(m map[string]*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: m}}}
}
func lv(v ...*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: v}}}
}
