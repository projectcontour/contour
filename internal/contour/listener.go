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
	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/golang/protobuf/ptypes/struct" // package name is structpb
)

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	listenerCache
	Cond
}

func defaultListener() *v2.Listener {
	const (
		router     = "envoy.router"
		httpFilter = "envoy.http_connection_manager"
		accessLog  = "envoy.file_access_log"
	)

	sv := func(s string) *structpb.Value {
		return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: s}}
	}
	bv := func(b bool) *structpb.Value {
		return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: b}}
	}
	st := func(m map[string]*structpb.Value) *structpb.Value {
		return &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: m}}}
	}
	lv := func(v ...*structpb.Value) *structpb.Value {
		return &structpb.Value{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: v}}}
	}
	l := &v2.Listener{
		Name: "ingress_http", // TODO(dfc) should come from the name of the service port
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
						"codec_type":  sv("http1"),        // let's not go crazy now
						"stat_prefix": sv("ingress_http"), // TODO(dfc) should this come from pod.Name?
						"rds": st(map[string]*structpb.Value{
							"route_config_name": sv("ingress_http"), // TODO(dfc) needed for grpc?
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
	return l
}
