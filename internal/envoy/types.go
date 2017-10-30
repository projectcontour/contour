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

package envoy

// JSON types for CDS API response derived from
// https://github.com/envoyproxy/envoy/blob/master/source/common/json/config_schemas.cc

// TODO(dfc) find a json schema parser that can generate this file automatically.

// Config is the top level Envoy configuration.
type Config struct {
	Listeners                  []*Listener `json:"listeners"`
	*LDS                       `json:"lds,omitempty"`
	*Admin                     `json:"admin,omitempty"`
	ClusterManager             `json:"cluster_manager"`
	FlagsPath                  string `json:"flags_path,omitempty"`
	StatsdUDPIPAddress         string `json:"statsd_udp_ip_address,omitempty"`
	StatsdTCPClusterName       string `json:"statsd_tcp_cluster_name,omitempty"`
	StatsdFlushIntervalMs      string `json:"stats_flush_interval_ms,omitempty"`
	WatchdogMissTimeoutMs      string `json:"watchdog_miss_timeout_ms,omitempty"`
	WatchdogMegamissTimeoutMs  string `json:"watchdog_megamiss_timeout_ms,omitempty"`
	WatchdogKillTimeoutMs      string `json:"watchdog_kill_timeout_ms,omitempty"`
	WatchdogMultikillTimeoutMs string `json:"watchdog_multikill_timeout_ms,omitempty"`
	*Tracing                   `json:"tracing,omitempty"`
	*RateLimitService          `json:"rate_limit_service,omitempty"`
	*Runtime                   `json:"runtime,omitempty"`
}

type Listener struct {
	Name                          string      `json:"name,omitempty"`
	Address                       string      `json:"address"`
	Filters                       []Filter    `json:"filters"`
	SSLContext                    *SSLContext `json:"ssl_context,omitempty"`
	BindToPort                    string      `json:"bind_to_port,omitempty"`                      // bool, defaults to "true"
	UseProxyProto                 string      `json:"use_proxy_proto,omitempty"`                   // bool, defaults to "false"
	UseOriginalDest               string      `json:"use_original_dst,omitempty"`                  // bool, defaults to "false"
	PerConnectionBufferLimitBytes string      `json:"per_connection_buffer_limit_bytes,omitempty"` // integer, defaults to 1MiB
}

type Filter interface {
	// TODO(dfc) enforce MarshalJSON
}

type HttpConnectionManager struct {
	Type   string                      `json:"type"` // must be "read"
	Name   string                      `json:"name"` // must be "http_connection_manager"
	Config HttpConnectionManagerConfig `json:"config"`
}

type HttpConnectionManagerConfig struct {
	CodecType    string `json:"codec_type"` // one of http1, http2, auto
	StatPrefix   string `json:"stat_prefix"`
	*RDS         `json:"rds,omitempty"`
	*RouteConfig `json:"route_config,omitempty"`
	Filters      []Filter `json:"filters"`
	AddUserAgent bool     `json:"add_user_agent,omitempty"`
	*Tracing     `json:"tracing,omitempty"`
	// "http1_settings": "{...}",
	// "http2_settings": "{...}",
	ServerName         string      `json:"server_name,omitempty"` // defaults to "envoy"
	IdleTimeoutSeconds int         `json:"idle_timeout_s,omitempty"`
	DrainTimeoutMs     int         `json:"drain_timeout_ms,omitempty"`
	AccessLog          []AccessLog `json:"access_log,omitempty"`
	UseRemoteAddress   bool        `json:"use_remote_address,omitempty"`
	ForwardClientCert  string      `json:"forward_client_cert,omitempty"`
	// "set_current_client_cert": "...",
	GenerateRequestID bool `json:"generate_request_id,omitempty"`
}

type AccessLog struct {
	Path string `json:"path"`
	// "format": "...",
	// "filter": "{...}",
}

type Router struct {
	Type   string       `json:"type"` // must be "decoder"
	Name   string       `json:"name"` // must be "router"
	Config RouterConfig `json:"config"`
}

type RouterConfig struct {
	DynamicStats bool `json:"dynamic_stats,omitempty"`
}

type ClusterManager struct {
	Clusters          []*Cluster `json:"clusters"`
	*SDS              `json:"sds,omitempty"`
	LocalClusterName  string `json:"local_cluster_namei,omitempty"`
	*OutlierDetection `json:"outlier_detection,omitempty"`
	*CDS              `json:"cds,omitempty"`
}

type SDS struct {
	Cluster        `json:"cluster"`
	RefreshDelayMs int `json:"refresh_delay_ms"`
}

type CDS struct {
	Cluster        `json:"cluster"`
	RefreshDelayMs int `json:"refresh_delay_ms"`
}

type LDS struct {
	Cluster        string `json:"cluster"`
	RefreshDelayMs string `json:"refresh_delay_ms"`
}

type RDS struct {
	Cluster         string `json:"cluster"`
	RouteConfigName string `json:"route_config_name"`
	RefreshDelayMs  int    `json:"refresh_delay_ms"`
}

type Cluster struct {
	Name                          string `json:"name"`
	Type                          string `json:"type"`
	ConnectTimeoutMs              int    `json:"connect_timeout_ms"`
	PerConnectionBufferLimitBytes int    `json:"per_connection_buffer_limit_bytes,omitempty"`
	LBType                        string `json:"lb_type"`
	Hosts                         []Host `json:"hosts,omitempty"`
	ServiceName                   string `json:"service_name,omitempty"`
	// *HealthCheck                  `json:"health_check,omitempty"`
	MaxRequestsPerConnection int `json:"max_requests_per_connection,omitempty"`
	// "circuit_breakers": "{...}",
	// "ssl_context": "{...}",
	// "features": "...",
	// "http2_settings": "{...}",
	// "cleanup_interval_ms": "...",
	// "dns_refresh_rate_ms": "...",
	// "dns_lookup_family": "...",
	// "dns_resolvers": [],
	// "outlier_detection": "{...}"
}

type Host struct {
	URL string `json:"url"`
}

type SDSHost struct {
	IPAddress string `json:"ip_address"`
	Port      int    `json:"port"`
	Tags      struct {
		AZ                  string `json:"az,omitempty"`
		Canary              bool   `json:"canary,omitempty"`
		LoadBalancingWeight int    `json:"load_balancing_weight,omitempty"`
	} `json:"tags"`
}

type SSLContext struct {
	// "alpn_protocols": "...",
	// "cert_chain_file": "...",
	// "private_key_file": "...",
	// "ca_cert_file": "...",
	// "verify_certificate_hash": "...",
	// "verify_subject_alt_name": [],
	// "cipher_suites": "...",
	// "ecdh_curves": "...",
	// "sni": "..."
}

type Admin struct {
	AccessLogPath string `json:"access_log_path"`
	ProfilePath   string `json:"profile_path,omitempty"`
	Address       string `json:"address"`
}

type OutlierDetection struct {
	EventLogPath string `json:"event_log_path,omitempty"`
}

type Tracing struct {
	// "http": {
	//   "driver": "{...}"
	// }
}

type RateLimitService struct {
	// "type": "grpc_service",
	// "config": {
	//   "cluster_name": "..."
	// }
}

type RouteConfig struct {
	ValidateClusters bool           `json:"validate_clusters,omitempty"`
	VirtualHosts     []*VirtualHost `json:"virtual_hosts"`
	// "internal_only_headers": [],
	// "response_headers_to_add": [],
	// "response_headers_to_remove": [],
	// "request_headers_to_add": []
}

type VirtualHost struct {
	Name       string   `json:"name"`
	Domains    []string `json:"domains"`
	Routes     []Route  `json:"routes"`
	RequireSSL string   `json:"require_ssl,omitempty"`
	// "virtual_clusters": [],
	// "rate_limits": [],
	// "request_headers_to_add": []
}

func (v *VirtualHost) AddDomain(d string) { v.Domains = append(v.Domains, d) }
func (v *VirtualHost) AddRoute(r Route)   { v.Routes = append(v.Routes, r) }

type Route struct {
	Prefix        string `json:"prefix,omitempty"`
	Path          string `json:"path,omitempty"`
	Regex         string `json:"regex,omitempty"`
	Cluster       string `json:"cluster,omitempty"`
	ClusterHeader string `json:"cluster_header,omitempty"`
	// "weighted_clusters" : "{...}",
	HostRedirect    string `json:"host_redirect,omitempty"`
	PathRedirect    string `json:"path_redirect,omitempty"`
	PrefixRewrite   string `json:"prefix_rewrite,omitempty"`
	HostRewrite     string `json:"host_rewrite,omitempty"`
	AutoHostRewrite bool   `json:"auto_host_rewrite,omitempty"`
	// "case_sensitive": "...",
	UseWebsocket bool `json:"use_websocket,omitempty"`
	TimeoutMS    int  `json:"timeout_ms,omitempty"`
	// "runtime": "{...}",
	// "retry_policy": "{...}",
	// "shadow": "{...}",
	Priority string `json:"priority,omitempty"`
	// "headers": [],
	// "rate_limits": [],
	// "include_vh_rate_limits" : "...",
	// "hash_policy": "{...}",
	// "request_headers_to_add" : [],
	// "opaque_config": [],
	// "cors": "{...}",
	// "decorator" : "{...}"
}

type Runtime struct {
	// "symlink_root": "...",
	// "subdirectory": "...",
	// "override_subdirectory": "..."
}
