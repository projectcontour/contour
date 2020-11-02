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

package xds

import (
	"fmt"
	"strings"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_auth_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_core_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_config_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_filter_http_ext_authz_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	envoy_config_filter_http_lua_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/lua/v2"
	envoy_config_filter_network_http_connection_manager_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_config_filter_network_tcp_proxy_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_access_loggers_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_extensions_filters_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_extensions_filters_http_lua_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_extensions_filters_network_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_extensions_transport_sockets_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/protobuf"

	"github.com/golang/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterLoadAssignmentName generates the name used for an EDS
// ClusterLoadAssignment, given a fully qualified Service name and
// port. This name is a contract between the producer of a cluster
// (i.e. the EDS service) and the consumer of a cluster (most likely
// a HTTP Route Action).
func ClusterLoadAssignmentName(service types.NamespacedName, portName string) string {
	name := []string{
		service.Namespace,
		service.Name,
		portName,
	}

	// If the port is empty, omit it.
	if portName == "" {
		return strings.Join(name[:2], "/")
	}

	return strings.Join(name, "/")
}

// TypeMapping maps xDS type URLs from v2 to v3.
var TypeMapping map[string]string

func init() {
	TypeMapping = make(map[string]string)

	entry := func(from proto.Message, to proto.Message) {
		TypeMapping[protobuf.AnyMessageTypeOf(from)] = protobuf.AnyMessageTypeOf(to)
	}

	// Fundamental xDS resource types.
	entry(&envoy_api_v2.Listener{}, &envoy_config_listener_v3.Listener{})
	entry(&envoy_api_v2.Cluster{}, &envoy_config_cluster_v3.Cluster{})
	entry(&envoy_api_v2.RouteConfiguration{}, &envoy_config_route_v3.RouteConfiguration{})
	entry(&envoy_api_v2.ClusterLoadAssignment{}, &envoy_config_endpoint_v3.ClusterLoadAssignment{})
	entry(&envoy_api_auth_v2.Secret{}, &envoy_extensions_transport_sockets_tls_v3.Secret{})

	// Other embedded resources used by Contour.
	entry(&envoy_config_accesslog_v2.FileAccessLog{},
		&envoy_extensions_access_loggers_file_v3.FileAccessLog{})

	entry(&envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute{},
		&envoy_extensions_filters_http_ext_authz_v3.ExtAuthzPerRoute{})

	entry(&envoy_config_filter_http_ext_authz_v2.ExtAuthz{},
		&envoy_extensions_filters_http_ext_authz_v3.ExtAuthz{})

	entry(&envoy_config_filter_http_lua_v2.Lua{},
		&envoy_extensions_filters_http_lua_v3.Lua{})

	entry(&envoy_api_auth_v2.UpstreamTlsContext{},
		&envoy_extensions_transport_sockets_tls_v3.UpstreamTlsContext{})

	entry(&envoy_api_auth_v2.DownstreamTlsContext{},
		&envoy_extensions_transport_sockets_tls_v3.DownstreamTlsContext{})

	entry(&envoy_config_filter_network_http_connection_manager_v2.HttpConnectionManager{},
		&envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{})

	entry(&envoy_config_filter_network_tcp_proxy_v2.TcpProxy{},
		&envoy_extensions_filters_network_tcp_proxy_v3.TcpProxy{})
}

func rewriteAnyMessage(a *any.Any) {
	if a != nil {
		anyval := ptypes.DynamicAny{}
		if err := ptypes.UnmarshalAny(a, &anyval); err != nil {
			panic(fmt.Sprintf("failed to unmarshal %T: %s", a, err.Error()))
		}

		newMsg := protobuf.MustMarshalAny(Rewrite(anyval.Message))
		if replacement, ok := TypeMapping[newMsg.TypeUrl]; ok {
			newMsg.TypeUrl = replacement
		}

		a.TypeUrl = newMsg.TypeUrl
		a.Value = newMsg.Value
	}
}

func rewriteConfigSource(s *envoy_api_core_v2.ConfigSource) {
	if s != nil {
		s.ResourceApiVersion = envoy_api_core_v2.ApiVersion_V3
		s.GetApiConfigSource().TransportApiVersion = envoy_api_core_v2.ApiVersion_V3
	}
}

// Rewrite changes the given xDS message to use the v3 xDS API.
//
// Since the v2 and v3 APIs are wire-compatible, we just rewrite
// the type names for type URLs in any.Any messages. This allows Envoy
// to do the actual conversion, and Envoy takes care of migrating
// deprecated fields.
func Rewrite(in proto.Message) proto.Message {
	switch msg := in.(type) {
	case *envoy_api_v2.ClusterLoadAssignment:
		return msg
	case *envoy_api_auth_v2.Secret:
		return msg

	case *envoy_api_v2.Cluster:
		if e := msg.GetEdsClusterConfig(); e != nil {
			rewriteConfigSource(e.GetEdsConfig())
		}

		if t := msg.GetTransportSocket(); t != nil {
			rewriteAnyMessage(t.GetTypedConfig())
		}

		return msg

	case *envoy_api_v2.RouteConfiguration:
		for _, v := range msg.GetVirtualHosts() {
			for _, r := range v.GetRoutes() {
				for _, conf := range r.GetTypedPerFilterConfig() {
					rewriteAnyMessage(conf)
				}
			}
		}

		return msg

	case *envoy_api_v2.Listener:
		for _, filter := range msg.ListenerFilters {
			rewriteAnyMessage(filter.GetTypedConfig())
		}

		for _, chain := range msg.FilterChains {
			for _, filter := range chain.Filters {
				rewriteAnyMessage(filter.GetTypedConfig())
			}

			if t := chain.GetTransportSocket(); t != nil {
				rewriteAnyMessage(t.GetTypedConfig())
			}
		}

		for _, a := range msg.AccessLog {
			rewriteAnyMessage(a.GetTypedConfig())
		}

		return msg

	case *envoy_config_filter_network_http_connection_manager_v2.HttpConnectionManager:
		if r := msg.GetRds(); r != nil {
			rewriteConfigSource(r.GetConfigSource())
		}

		for _, f := range msg.HttpFilters {
			rewriteAnyMessage(f.GetTypedConfig())
		}

		for _, l := range msg.AccessLog {
			rewriteAnyMessage(l.GetTypedConfig())
		}

		return msg

	case *envoy_api_auth_v2.DownstreamTlsContext:
		for _, s := range msg.GetCommonTlsContext().TlsCertificateSdsSecretConfigs {
			rewriteConfigSource(s.GetSdsConfig())
		}

		return msg

	case *envoy_api_auth_v2.UpstreamTlsContext:
		for _, s := range msg.GetCommonTlsContext().TlsCertificateSdsSecretConfigs {
			rewriteConfigSource(s.GetSdsConfig())
		}

		return msg

	default:
		// Any messages that don't have any embedded version information
		// that needs conversion can just be returned unchanged.
		return msg
	}
}
