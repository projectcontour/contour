package protobuf

import (
	"log"
	"testing"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_http_connection_manager_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
)

func TestConvertListener(t *testing.T) {
	hcm := envoy_api_v2_listener.Filter{
		Name: wellknown.HTTPConnectionManager,
		ConfigType: &envoy_api_v2_listener.Filter_TypedConfig{
			TypedConfig: MustMarshalAny(&envoy_http_connection_manager_v2.HttpConnectionManager{}),
		},
	}

	l2 := &envoy_api_v2.Listener{
		Name: "http_listener",
		Address: &envoy_api_v2_core.Address{
			Address: &envoy_api_v2_core.Address_SocketAddress{
				SocketAddress: &envoy_api_v2_core.SocketAddress{
					Protocol: envoy_api_v2_core.SocketAddress_TCP,
					Address:  "127.0.0.1",
					PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
						PortValue: uint32(80),
					},
				},
			},
		},
		ListenerFilters: []*envoy_api_v2_listener.ListenerFilter{
			&envoy_api_v2_listener.ListenerFilter{
				Name: wellknown.ProxyProtocol,
			},
		},
		FilterChains: []*envoy_api_v2_listener.FilterChain{{
			Filters: []*envoy_api_v2_listener.Filter{&hcm},
		}},
		SocketOptions: []*envoy_api_v2_core.SocketOption{
			// Enable TCP keep-alive.
			{
				Description: "Enable TCP keep-alive",
				Level:       1,
				Name:        9,
				Value:       &envoy_api_v2_core.SocketOption_IntValue{IntValue: 1},
				State:       envoy_api_v2_core.SocketOption_STATE_LISTENING,
			},
			// The time (in seconds) the connection needs to remain idle
			// before TCP starts sending keepalive probes.
			{
				Description: "TCP keep-alive initial idle time",
				Level:       6,
				Name:        4,
				Value:       &envoy_api_v2_core.SocketOption_IntValue{IntValue: 45},
				State:       envoy_api_v2_core.SocketOption_STATE_LISTENING,
			},
			// The time (in seconds) between individual keepalive probes.
			{
				Description: "TCP keep-alive time between probes",
				Level:       6,
				Name:        5,
				Value:       &envoy_api_v2_core.SocketOption_IntValue{IntValue: 5},
				State:       envoy_api_v2_core.SocketOption_STATE_LISTENING,
			},
			// The maximum number of TCP keep-alive probes to send before
			// giving up and killing the connection if no response is
			// obtained from the other end.
			{
				Description: "TCP keep-alive probe count",
				Level:       6,
				Name:        6,
				Value:       &envoy_api_v2_core.SocketOption_IntValue{IntValue: 9},
				State:       envoy_api_v2_core.SocketOption_STATE_LISTENING,
			},
		},
	}

	l3 := envoy_config_listener_v3.Listener{}

	assert.NoError(t, ConvertTo(l2, &l3))

	log.Printf((&jsonpb.Marshaler{Indent: "    "}).MarshalToString(&l3))
}
