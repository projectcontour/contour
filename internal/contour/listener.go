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
	"github.com/golang/protobuf/ptypes/struct"
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
					Config: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"codec_type":  sv("auto"),
							"stat_prefix": sv("ingress_https"),
							"rds": st(map[string]*structpb.Value{
								"route_config_name": sv("ingress_http"), // TODO(dfc) issue 103
								"config_source": st(map[string]*structpb.Value{
									"api_config_source": st(map[string]*structpb.Value{
										"api_type": sv("grpc"),
										"cluster_name": lv(
											sv("xds_cluster"),
										),
									}),
								}),
							}),
							"http_filters": lv(
								st(map[string]*structpb.Value{
									"name": sv(router),
								}),
							),
							"access_log": st(map[string]*structpb.Value{
								"name": sv(accessLog),
								"config": st(map[string]*structpb.Value{
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
				Config: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"codec_type":  sv("auto"),
						"stat_prefix": sv("ingress_http"),
						"rds": st(map[string]*structpb.Value{
							"route_config_name": sv("ingress_http"), // TODO(dfc) issue 103
							"config_source": st(map[string]*structpb.Value{
								"api_config_source": st(map[string]*structpb.Value{
									"api_type": sv("grpc"),
									"cluster_name": lv(
										sv("xds_cluster"),
									),
								}),
							}),
						}),
						"http_filters": lv(
							st(map[string]*structpb.Value{
								"name": sv(router),
							}),
						),
						"access_log": st(map[string]*structpb.Value{
							"name": sv(accessLog),
							"config": st(map[string]*structpb.Value{
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

func sv(s string) *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: s}}
}

func bv(b bool) *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: b}}
}

func st(m map[string]*structpb.Value) *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: m}}}
}
func lv(v ...*structpb.Value) *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: v}}}
}
