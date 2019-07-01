// Copyright © 2019 Heptio
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
	"log"
	"strconv"
	"strings"
	"time"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	clusterv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
)

// Bootstrap creates a new v2 Bootstrap configuration.
func Bootstrap(c *BootstrapConfig) *bootstrap.Bootstrap {
	b := &bootstrap.Bootstrap{
		DynamicResources: &bootstrap.Bootstrap_DynamicResources{
			LdsConfig: ConfigSource("contour"),
			CdsConfig: ConfigSource("contour"),
		},
		StaticResources: &bootstrap.Bootstrap_StaticResources{
			Clusters: []api.Cluster{{
				Name:                 "contour",
				AltStatName:          strings.Join([]string{c.Namespace, "contour", strconv.Itoa(intOrDefault(c.XDSGRPCPort, 8001))}, "_"),
				ConnectTimeout:       5 * time.Second,
				ClusterDiscoveryType: ClusterDiscoveryType(api.Cluster_STRICT_DNS),
				LbPolicy:             api.Cluster_ROUND_ROBIN,
				LoadAssignment: &api.ClusterLoadAssignment{
					ClusterName: "contour",
					Endpoints: []endpoint.LocalityLbEndpoints{{
						LbEndpoints: []endpoint.LbEndpoint{
							LBEndpoint(stringOrDefault(c.XDSAddress, "127.0.0.1"), intOrDefault(c.XDSGRPCPort, 8001)),
						},
					}},
				},
				Http2ProtocolOptions: new(core.Http2ProtocolOptions), // enables http2
				CircuitBreakers: &clusterv2.CircuitBreakers{
					Thresholds: []*clusterv2.CircuitBreakers_Thresholds{{
						Priority:           core.RoutingPriority_HIGH,
						MaxConnections:     u32(100000),
						MaxPendingRequests: u32(100000),
						MaxRequests:        u32(60000000),
						MaxRetries:         u32(50),
					}, {
						Priority:           core.RoutingPriority_DEFAULT,
						MaxConnections:     u32(100000),
						MaxPendingRequests: u32(100000),
						MaxRequests:        u32(60000000),
						MaxRetries:         u32(50),
					}},
				},
			}, {
				Name:                 "service-stats",
				AltStatName:          strings.Join([]string{c.Namespace, "service-stats", strconv.Itoa(intOrDefault(c.AdminPort, 9001))}, "_"),
				ConnectTimeout:       250 * time.Millisecond,
				ClusterDiscoveryType: ClusterDiscoveryType(api.Cluster_LOGICAL_DNS),
				LbPolicy:             api.Cluster_ROUND_ROBIN,
				LoadAssignment: &api.ClusterLoadAssignment{
					ClusterName: "service-stats",
					Endpoints: []endpoint.LocalityLbEndpoints{{
						LbEndpoints: []endpoint.LbEndpoint{
							LBEndpoint(stringOrDefault(c.AdminAddress, "127.0.0.1"), intOrDefault(c.AdminPort, 9001)),
						},
					}},
				},
			}},
		},
		Admin: &bootstrap.Admin{
			AccessLogPath: stringOrDefault(c.AdminAccessLogPath, "/dev/null"),
			Address:       SocketAddress(stringOrDefault(c.AdminAddress, "127.0.0.1"), intOrDefault(c.AdminPort, 9001)),
		},
	}

	if c.GrpcClientCert != "" || c.GrpcClientKey != "" || c.GrpcCABundle != "" {
		// If one of the two TLS options is not empty, they all must be not empty
		if !(c.GrpcClientCert != "" && c.GrpcClientKey != "" && c.GrpcCABundle != "") {
			log.Fatal("You must supply all three TLS parameters - --envoy-cafile, --envoy-cert-file, --envoy-key-file, or none of them.")
		}
		b.StaticResources.Clusters[0].TlsContext = upstreamFileTLSContext(c.GrpcCABundle, c.GrpcClientCert, c.GrpcClientKey)
	}

	return b
}

func upstreamFileTLSContext(cafile, certfile, keyfile string) *auth.UpstreamTlsContext {
	if certfile == "" {
		// Nothig to do
		return nil
	}

	if certfile == "" {
		// Nothing to do
		return nil
	}

	if cafile == "" {
		// You currently must supply a CA file, not just use others.
		return nil
	}
	context := &auth.UpstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsCertificates: []*auth.TlsCertificate{
				{
					CertificateChain: &core.DataSource{
						Specifier: &core.DataSource_Filename{
							Filename: certfile,
						},
					},
					PrivateKey: &core.DataSource{
						Specifier: &core.DataSource_Filename{
							Filename: keyfile,
						},
					},
				},
			},
			ValidationContextType: &auth.CommonTlsContext_ValidationContext{
				ValidationContext: &auth.CertificateValidationContext{
					TrustedCa: &core.DataSource{
						Specifier: &core.DataSource_Filename{
							Filename: cafile,
						},
					},
					// TODO(youngnick): Does there need to be a flag wired down to here?
					VerifySubjectAltName: []string{"contour"},
				},
			},
		},
	}

	return context
}

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func intOrDefault(i, def int) int {
	if i == 0 {
		return def
	}
	return i
}

// BootstrapConfig holds configuration values for a v2.Bootstrap.
type BootstrapConfig struct {
	// AdminAccessLogPath is the path to write the access log for the administration server.
	// Defaults to /dev/null.
	AdminAccessLogPath string

	// AdminAddress is the TCP address that the administration server will listen on.
	// Defaults to 127.0.0.1.
	AdminAddress string

	// AdminPort is the port that the administration server will listen on.
	// Defaults to 9001.
	AdminPort int

	// XDSAddress is the TCP address of the gRPC XDS management server.
	// Defaults to 127.0.0.1.
	XDSAddress string

	// XDSGRPCPort is the management server port that provides the v2 gRPC API.
	// Defaults to 8001.
	XDSGRPCPort int

	// Namespace is the namespace where Contour is running
	Namespace string

	//GrpcCABundle is the filename that contains a CA certificate chain that can
	//verify the client cert.
	GrpcCABundle string

	// GrpcClientCert is the filename that contains a client certificate. May contain a full bundle if you
	// don't want to pass a CA Bundle.
	GrpcClientCert string

	// GrpcClientKey is the filename that contains a client key for secure gRPC with TLS.
	GrpcClientKey string
}
