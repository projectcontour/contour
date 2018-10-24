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
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/gogo/protobuf/types"
)

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() listener.ListenerFilter {
	return listener.ListenerFilter{
		Name:   "envoy.listener.tls_inspector",
		Config: new(types.Struct),
	}
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route and access log.
func HTTPConnectionManager(routename, accessLogPath string) listener.Filter {
	return listener.Filter{
		Name: "envoy.http_connection_manager",
		Config: &types.Struct{
			Fields: map[string]*types.Value{
				"stat_prefix": sv(routename),
				"rds": st(map[string]*types.Value{
					"route_config_name": sv(routename),
					"config_source": st(map[string]*types.Value{
						"api_config_source": st(map[string]*types.Value{
							"api_type": sv("GRPC"),
							"grpc_services": lv(
								st(map[string]*types.Value{
									"envoy_grpc": st(map[string]*types.Value{
										"cluster_name": sv("contour"),
									}),
								}),
							),
						}),
					}),
				}),
				"http_filters": lv(
					st(map[string]*types.Value{
						"name": sv("envoy.gzip"),
					}),
					st(map[string]*types.Value{
						"name": sv("envoy.grpc_web"),
					}),
					st(map[string]*types.Value{
						"name": sv("envoy.router"),
					}),
				),
				"http_protocol_options": st(map[string]*types.Value{
					"accept_http_10": {Kind: &types.Value_BoolValue{BoolValue: true}},
				}),
				"access_log":         accesslog(accessLogPath),
				"use_remote_address": {Kind: &types.Value_BoolValue{BoolValue: true}}, // TODO(jbeda) should this ever be false?
			},
		},
	}
}

// SocketAddress creates a new TCP core.Address.
func SocketAddress(address string, port int) core.Address {
	return core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  address,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

// DownstreamTLSContext creates a new DownstreamTlsContext.
func DownstreamTLSContext(cert, key []byte, tlsMinProtoVersion auth.TlsParameters_TlsProtocol, alpnProtos ...string) *auth.DownstreamTlsContext {
	return &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: auth.TlsParameters_TLSv1_3,
			},
			TlsCertificates: []*auth.TlsCertificate{{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: cert,
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: key,
					},
				},
			}},
			AlpnProtocols: alpnProtos,
		},
	}
}

func accesslog(path string) *types.Value {
	return lv(
		st(map[string]*types.Value{
			"name": sv("envoy.file_access_log"),
			"config": st(map[string]*types.Value{
				"path": sv(path),
			}),
		}),
	)
}

func sv(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func st(m map[string]*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: m}}}
}

func lv(v ...*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: v}}}
}
