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

package contour

import (
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/timeout"
)

// ListenerConfig holds configuration parameters for visitListeners.
type ListenerConfig struct {
	// Envoy's HTTP (non TLS) listener address.
	// If not set, defaults to DEFAULT_HTTP_LISTENER_ADDRESS.
	HTTPAddress string

	// Envoy's HTTP (non TLS) listener port.
	// If not set, defaults to DEFAULT_HTTP_LISTENER_PORT.
	HTTPPort int

	// Envoy's HTTP (non TLS) access log path.
	// If not set, defaults to DEFAULT_HTTP_ACCESS_LOG.
	HTTPAccessLog string

	// Envoy's HTTPS (TLS) listener address.
	// If not set, defaults to DEFAULT_HTTPS_LISTENER_ADDRESS.
	HTTPSAddress string

	// Envoy's HTTPS (TLS) listener port.
	// If not set, defaults to DEFAULT_HTTPS_LISTENER_PORT.
	HTTPSPort int

	// Envoy's HTTPS (TLS) access log path.
	// If not set, defaults to DEFAULT_HTTPS_ACCESS_LOG.
	HTTPSAccessLog string

	// UseProxyProto configures all listeners to expect a PROXY
	// V1 or V2 preamble.
	// If not set, defaults to false.
	UseProxyProto bool

	// MinimumTLSVersion defines the minimum TLS protocol version the proxy should accept.
	MinimumTLSVersion envoy_api_v2_auth.TlsParameters_TlsProtocol

	// DefaultHTTPVersions defines the default set of HTTP
	// versions the proxy should accept. If not specified, all
	// supported versions are accepted. This is applied to both
	// HTTP and HTTPS listeners but has practical effect only for
	// HTTPS, because we don't support h2c.
	DefaultHTTPVersions []envoy.HTTPVersionType

	// AccessLogType defines if Envoy logs should be output as Envoy's default or JSON.
	// Valid values: 'envoy', 'json'
	// If not set, defaults to 'envoy'
	AccessLogType string

	// AccessLogFields sets the fields that should be shown in JSON logs.
	// Valid entries are the keys from internal/envoy/accesslog.go:jsonheaders
	// Defaults to a particular set of fields.
	AccessLogFields []string

	// RequestTimeout configures the request_timeout for all Connection Managers.
	RequestTimeout timeout.Setting

	// ConnectionIdleTimeout configures the common_http_protocol_options.idle_timeout for all
	// Connection Managers.
	ConnectionIdleTimeout timeout.Setting

	// StreamIdleTimeout configures the stream_idle_timeout for all Connection Managers.
	StreamIdleTimeout timeout.Setting

	// MaxConnectionDuration configures the common_http_protocol_options.max_connection_duration for all
	// Connection Managers.
	MaxConnectionDuration timeout.Setting

	// ConnectionShutdownGracePeriod configures the drain_timeout for all Connection Managers.
	ConnectionShutdownGracePeriod timeout.Setting

	// StaticListeners holds a map of Contour configured static listeners.
	StaticListeners map[string]*v2.Listener
}

// httpAddress returns the port for the HTTP (non TLS)
// listener or DEFAULT_HTTP_LISTENER_ADDRESS if not configured.
func (lvc *ListenerConfig) httpAddress() string {
	if lvc.HTTPAddress != "" {
		return lvc.HTTPAddress
	}
	return DEFAULT_HTTP_LISTENER_ADDRESS
}

// httpPort returns the port for the HTTP (non TLS)
// listener or DEFAULT_HTTP_LISTENER_PORT if not configured.
func (lvc *ListenerConfig) httpPort() int {
	if lvc.HTTPPort != 0 {
		return lvc.HTTPPort
	}
	return DEFAULT_HTTP_LISTENER_PORT
}

// httpAccessLog returns the access log for the HTTP (non TLS)
// listener or DEFAULT_HTTP_ACCESS_LOG if not configured.
func (lvc *ListenerConfig) httpAccessLog() string {
	if lvc.HTTPAccessLog != "" {
		return lvc.HTTPAccessLog
	}
	return DEFAULT_HTTP_ACCESS_LOG
}

// httpsAddress returns the port for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_LISTENER_ADDRESS if not configured.
func (lvc *ListenerConfig) httpsAddress() string {
	if lvc.HTTPSAddress != "" {
		return lvc.HTTPSAddress
	}
	return DEFAULT_HTTPS_LISTENER_ADDRESS
}

// httpsPort returns the port for the HTTPS (TLS) listener
// or DEFAULT_HTTPS_LISTENER_PORT if not configured.
func (lvc *ListenerConfig) httpsPort() int {
	if lvc.HTTPSPort != 0 {
		return lvc.HTTPSPort
	}
	return DEFAULT_HTTPS_LISTENER_PORT
}

// httpsAccessLog returns the access log for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_ACCESS_LOG if not configured.
func (lvc *ListenerConfig) httpsAccessLog() string {
	if lvc.HTTPSAccessLog != "" {
		return lvc.HTTPSAccessLog
	}
	return DEFAULT_HTTPS_ACCESS_LOG
}

// accesslogType returns the access log type that should be configured
// across all listener types or DEFAULT_ACCESS_LOG_TYPE if not configured.
func (lvc *ListenerConfig) accesslogType() string {
	if lvc.AccessLogType != "" {
		return lvc.AccessLogType
	}
	return DEFAULT_ACCESS_LOG_TYPE
}

// accesslogFields returns the access log fields that should be configured
// for Envoy, or a default set if not configured.
func (lvc *ListenerConfig) accesslogFields() []string {
	if lvc.AccessLogFields != nil {
		return lvc.AccessLogFields
	}
	return envoy.DefaultFields
}

func (lvc *ListenerConfig) newInsecureAccessLog() []*envoy_api_v2_accesslog.AccessLog {
	switch lvc.accesslogType() {
	case "json":
		return envoy.FileAccessLogJSON(lvc.httpAccessLog(), lvc.accesslogFields())
	default:
		return envoy.FileAccessLogEnvoy(lvc.httpAccessLog())
	}
}

func (lvc *ListenerConfig) newSecureAccessLog() []*envoy_api_v2_accesslog.AccessLog {
	switch lvc.accesslogType() {
	case "json":
		return envoy.FileAccessLogJSON(lvc.httpsAccessLog(), lvc.accesslogFields())
	default:
		return envoy.FileAccessLogEnvoy(lvc.httpsAccessLog())
	}
}

// minTLSVersion returns the requested minimum TLS protocol
// version or envoy_api_v2_auth.TlsParameters_TLSv1_1 if not configured.
func (lvc *ListenerConfig) minTLSVersion() envoy_api_v2_auth.TlsParameters_TlsProtocol {
	if lvc.MinimumTLSVersion > envoy_api_v2_auth.TlsParameters_TLSv1_1 {
		return lvc.MinimumTLSVersion
	}
	return envoy_api_v2_auth.TlsParameters_TLSv1_1
}

// staticListeners returns an array of []xds.Resources representing the
// Contour configured static listeners.
func (lvc *ListenerConfig) StaticListenersProto() []xds.Resource {
	return translateListeners(nil, lvc)
}
