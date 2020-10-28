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
	"log"
	"strings"

	envoy_api_auth_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_config_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_filter_http_ext_authz_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	envoy_config_filter_http_lua_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/lua/v2"
	envoy_config_filter_network_http_connection_manager_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_access_loggers_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_extensions_filters_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_extensions_filters_http_lua_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
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

// Fixup takes a xDS v3 resource and upgrades any embedded resources
// to xDS v3. This only takes care of resource types that Contour will
// generate.
func Fixup(in proto.Message) proto.Message {
	switch msg := in.(type) {
	case *envoy_config_listener_v3.Listener:
		for i := range msg.ListenerFilters {
			msg.ListenerFilters[i] = Fixup(msg.ListenerFilters[i]).(*envoy_config_listener_v3.ListenerFilter)
		}

		for _, chain := range msg.FilterChains {
			for i := range chain.Filters {
				chain.Filters[i].ConfigType = &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: Fixup(chain.Filters[i].GetTypedConfig()).(*any.Any),
				}
			}

			if t := chain.GetTransportSocket(); t != nil {
				chain.TransportSocket.ConfigType = &envoy_config_core_v3.TransportSocket_TypedConfig{
					TypedConfig: Fixup(t.GetTypedConfig()).(*any.Any),
				}
			}
		}

		return in

	case *any.Any:
		anyval := ptypes.DynamicAny{}
		if err := ptypes.UnmarshalAny(msg, &anyval); err != nil {
			panic(fmt.Sprintf("failed to unmarshal %T: %s", msg, err.Error()))
		}

		return protobuf.MustMarshalAny(Fixup(anyval.Message))

	case *envoy_config_listener_v3.Filter:
		if c := msg.GetTypedConfig(); c != nil {
			msg.ConfigType = &envoy_config_listener_v3.Filter_TypedConfig{
				TypedConfig: Fixup(c).(*any.Any),
			}
		}

		return msg

	case *envoy_config_listener_v3.ListenerFilter:
		if c := msg.GetTypedConfig(); c != nil {
			msg.ConfigType = &envoy_config_listener_v3.ListenerFilter_TypedConfig{
				TypedConfig: Fixup(c).(*any.Any),
			}
		}

		return msg

	case *envoy_config_cluster_v3.Cluster:
		if e := msg.GetEdsClusterConfig(); e != nil {
			e.GetEdsConfig().ResourceApiVersion = envoy_config_core_v3.ApiVersion_V3
			e.GetEdsConfig().GetApiConfigSource().TransportApiVersion = envoy_config_core_v3.ApiVersion_V3
		}

		if t := msg.GetTransportSocket(); t != nil {
			t.ConfigType = &envoy_config_core_v3.TransportSocket_TypedConfig{
				TypedConfig: Fixup(t.GetTypedConfig()).(*any.Any),
			}
		}

		return msg

	case *envoy_config_route_v3.RouteConfiguration:
		for _, v := range msg.GetVirtualHosts() {
			for _, r := range v.GetRoutes() {
				for name, conf := range r.GetTypedPerFilterConfig() {
					r.TypedPerFilterConfig[name] = Fixup(conf).(*any.Any)
				}
			}
		}

		return msg

	case *envoy_extensions_transport_sockets_tls_v3.Secret:
		return msg

	case *envoy_config_endpoint_v3.ClusterLoadAssignment:
		return msg

	case *envoy_config_filter_network_http_connection_manager_v2.HttpConnectionManager:
		v3 := envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{}
		protobuf.MustConvertTo(msg, &v3)

		if r := v3.GetRds(); r != nil {
			r.GetConfigSource().ResourceApiVersion = envoy_config_core_v3.ApiVersion_V3
			r.GetConfigSource().GetApiConfigSource().TransportApiVersion = envoy_config_core_v3.ApiVersion_V3
		}

		for _, f := range v3.HttpFilters {
			if c := f.GetTypedConfig(); c != nil {
				f.ConfigType = &envoy_extensions_filters_network_http_connection_manager_v3.HttpFilter_TypedConfig{
					TypedConfig: Fixup(c).(*any.Any),
				}
			}
		}

		return &v3

	case *envoy_api_auth_v2.DownstreamTlsContext:
		v3 := envoy_extensions_transport_sockets_tls_v3.DownstreamTlsContext{}
		protobuf.MustConvertTo(msg, &v3)

		for _, s := range v3.GetCommonTlsContext().TlsCertificateSdsSecretConfigs {
			s.GetSdsConfig().ResourceApiVersion = envoy_config_core_v3.ApiVersion_V3
			s.GetSdsConfig().GetApiConfigSource().TransportApiVersion = envoy_config_core_v3.ApiVersion_V3
		}

		return &v3

	case *envoy_api_auth_v2.UpstreamTlsContext:
		v3 := envoy_extensions_transport_sockets_tls_v3.UpstreamTlsContext{}
		protobuf.MustConvertTo(msg, &v3)

		for _, s := range v3.GetCommonTlsContext().TlsCertificateSdsSecretConfigs {
			s.GetSdsConfig().ResourceApiVersion = envoy_config_core_v3.ApiVersion_V3
			s.GetSdsConfig().GetApiConfigSource().TransportApiVersion = envoy_config_core_v3.ApiVersion_V3
		}

		return &v3

	case *envoy_config_filter_http_lua_v2.Lua:
		v3 := envoy_extensions_filters_http_lua_v3.Lua{}
		protobuf.MustConvertTo(msg, &v3)

		return &v3

	case *envoy_config_filter_http_ext_authz_v2.ExtAuthz:
		v3 := envoy_extensions_filters_http_ext_authz_v3.ExtAuthz{}
		protobuf.MustConvertTo(msg, &v3)

		return &v3

	case *envoy_config_filter_http_ext_authz_v2.ExtAuthzPerRoute:
		v3 := envoy_extensions_filters_http_ext_authz_v3.ExtAuthzPerRoute{}
		protobuf.MustConvertTo(msg, &v3)

		return &v3

	case *envoy_config_accesslog_v2.FileAccessLog:
		v3 := envoy_extensions_access_loggers_file_v3.FileAccessLog{}
		protobuf.MustConvertTo(msg, &v3)

		return &v3

	default:
		log.Printf("missing conversion for %T, good luck", msg)
		return msg
	}
}
