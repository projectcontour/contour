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

package listener

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

const (
	ENVOY_HTTP_LISTENER            = "ingress_http"
	ENVOY_HTTPS_LISTENER           = "ingress_https"
	DEFAULT_HTTP_ACCESS_LOG        = "/dev/stdout"
	DEFAULT_HTTP_LISTENER_ADDRESS  = "0.0.0.0"
	DEFAULT_HTTP_LISTENER_PORT     = 8080
	DEFAULT_HTTPS_ACCESS_LOG       = "/dev/stdout"
	DEFAULT_HTTPS_LISTENER_ADDRESS = DEFAULT_HTTP_LISTENER_ADDRESS
	DEFAULT_HTTPS_LISTENER_PORT    = 8443

	router     = "envoy.router"
	grpcWeb    = "envoy.grpc_web"
	httpFilter = "envoy.http_connection_manager"
	accessLog  = "envoy.file_access_log"
)

type Visitor struct {
	*dag.DAG
}

func (v *Visitor) Visit() map[string]*v2.Listener {
	m := make(map[string]*v2.Listener)
	http := 0
	v.DAG.Visit(func(v dag.Vertex) {
		switch v := v.(type) {
		case *dag.VirtualHost:
			if v.Port == 80 {
				http++
			}
		}
	})
	if http > 0 {
		m[ENVOY_HTTP_LISTENER] = &v2.Listener{
			Name:    ENVOY_HTTP_LISTENER,
			Address: socketaddress(DEFAULT_HTTP_LISTENER_ADDRESS, DEFAULT_HTTP_LISTENER_PORT),
			FilterChains: []listener.FilterChain{
				filterchain(false, httpfilter(ENVOY_HTTP_LISTENER, DEFAULT_HTTPS_ACCESS_LOG)),
			},
		}
	}
	return m
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

func httpfilter(routename, accessLogPath string) listener.Filter {
	return listener.Filter{
		Name: httpFilter,
		Config: &types.Struct{
			Fields: map[string]*types.Value{
				"stat_prefix": sv(routename),
				"rds": st(map[string]*types.Value{
					"route_config_name": sv(routename),
					"config_source": st(map[string]*types.Value{
						"api_config_source": st(map[string]*types.Value{
							"api_type": sv("GRPC"),
							"cluster_names": lv(
								sv("contour"),
							),
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
						"name": sv(grpcWeb),
					}),
					st(map[string]*types.Value{
						"name": sv(router),
					}),
				),
				"use_remote_address": bv(true), // TODO(jbeda) should this ever be false?
				"access_log":         accesslog(accessLogPath),
			},
		},
	}
}

func accesslog(path string) *types.Value {
	return lv(
		st(map[string]*types.Value{
			"name": sv(accessLog),
			"config": st(map[string]*types.Value{
				"path": sv(path),
			}),
		}),
	)
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
