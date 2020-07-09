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

// Package envoy contains APIs for translating between Contour
// objects and Envoy configuration APIs and types.
package envoy

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	clusterv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/protobuf"
)

// sdsResourcesSubdirectory stores the subdirectory name where SDS path resources are stored to.
const sdsResourcesSubdirectory = "sds"

// sdsTLSCertificateFile stores the path to the SDS resource with Envoy's
// client certificate and key for XDS gRPC connection.
const sdsTLSCertificateFile = "xds-tls-certicate.json"

// sdsValidationContextFile stores the path to the SDS resource with
// CA certificates for Envoy to use for the XDS gRPC connection.
const sdsValidationContextFile = "xds-validation-context.json"

// WriteBootstrap writes bootstrap configuration to files.
func WriteBootstrap(c *BootstrapConfig) error {
	// Create Envoy bootstrap config and associated resource files.
	steps, err := bootstrap(c)
	if err != nil {
		return err
	}

	if c.ResourcesDir != "" {
		if err := os.MkdirAll(path.Join(c.ResourcesDir, "sds"), 0750); err != nil {
			return err
		}
	}

	// Write all configuration files out to filesystem.
	for _, step := range steps {
		if err := writeConfig(step(c)); err != nil {
			return err
		}
	}

	return nil
}

type bootstrapf func(*BootstrapConfig) (string, proto.Message)

// bootstrap creates a new v2 bootstrap configuration and associated resource files.
func bootstrap(c *BootstrapConfig) ([]bootstrapf, error) {
	steps := []bootstrapf{}

	if c.GrpcClientCert == "" && c.GrpcClientKey == "" && c.GrpcCABundle == "" {
		steps = append(steps,
			func(*BootstrapConfig) (string, proto.Message) {
				return c.Path, bootstrapConfig(c)
			})

		return steps, nil
	}

	for _, f := range []string{c.GrpcClientCert, c.GrpcClientKey, c.GrpcCABundle} {
		// If any of of the TLS options is not empty, they all must be not empty.
		if f == "" {
			return nil, fmt.Errorf(
				"you must supply all TLS parameters - %q, %q, %q, or none of them",
				"--envoy-cafile", "--envoy-cert-file", "--envoy-key-file")
		}

		if !c.SkipFilePathCheck {
			// If the TLS secrets aren't set up properly,
			// some files may not be present. In this case,
			// envoy will reject the bootstrap configuration,
			// but there is no way to detect and fix that. If
			// we check and fail here, that is visible in the
			// Pod lifecycle and therefore fixable.
			fi, err := os.Stat(f)
			if err != nil {
				return nil, err
			}
			if fi.Size() == 0 {
				return nil, fmt.Errorf("%q is empty", f)
			}
		}
	}

	if c.ResourcesDir == "" {
		// For backwards compatibility, the old behavior
		// is to use direct certificate and key file paths in
		// bootstrap config. Envoy does not support rotation
		// of xDS certificate files in this case.

		steps = append(steps,
			func(*BootstrapConfig) (string, proto.Message) {
				b := bootstrapConfig(c)
				b.StaticResources.Clusters[0].TransportSocket = UpstreamTLSTransportSocket(
					upstreamFileTLSContext(c))
				return c.Path, b
			})

		return steps, nil
	}

	// xDS certificate rotation is supported by Envoy by using SDS path based resource files.
	// These files are JSON representation of the SDS protobuf messages that normally get sent over the xDS connection,
	// but for xDS connection itself, bootstrapping is done by storing the SDS resources in a local filesystem.
	// Envoy will monitor and reload the resource files and the certificate and key files referred from the SDS resources.
	//
	// Two files are written to ResourcesDir:
	// - SDS resource for xDS client certificate and key for authenticating Envoy towards Contour.
	// - SDS resource for trusted CA certificate for validating Contour server certificate.
	sdsTLSCertificatePath := path.Join(c.ResourcesDir, sdsResourcesSubdirectory, sdsTLSCertificateFile)
	sdsValidationContextPath := path.Join(c.ResourcesDir, sdsResourcesSubdirectory, sdsValidationContextFile)

	steps = append(steps,
		func(*BootstrapConfig) (string, proto.Message) {
			return sdsTLSCertificatePath, tlsCertificateSdsSecretConfig(c)
		},
		func(*BootstrapConfig) (string, proto.Message) {
			return sdsValidationContextPath, validationContextSdsSecretConfig(c)
		},
		func(*BootstrapConfig) (string, proto.Message) {
			b := bootstrapConfig(c)
			b.StaticResources.Clusters[0].TransportSocket = UpstreamTLSTransportSocket(
				upstreamSdsTLSContext(sdsTLSCertificatePath, sdsValidationContextPath))
			return c.Path, b
		},
	)

	return steps, nil
}

func bootstrapConfig(c *BootstrapConfig) *envoy_api_bootstrap.Bootstrap {
	return &envoy_api_bootstrap.Bootstrap{
		DynamicResources: &envoy_api_bootstrap.Bootstrap_DynamicResources{
			LdsConfig: ConfigSource("contour"),
			CdsConfig: ConfigSource("contour"),
		},
		StaticResources: &envoy_api_bootstrap.Bootstrap_StaticResources{
			Clusters: []*api.Cluster{{
				Name:                 "contour",
				AltStatName:          strings.Join([]string{c.Namespace, "contour", strconv.Itoa(c.xdsGRPCPort())}, "_"),
				ConnectTimeout:       protobuf.Duration(5 * time.Second),
				ClusterDiscoveryType: ClusterDiscoveryType(api.Cluster_STRICT_DNS),
				LbPolicy:             api.Cluster_ROUND_ROBIN,
				LoadAssignment: &api.ClusterLoadAssignment{
					ClusterName: "contour",
					Endpoints: Endpoints(
						SocketAddress(c.xdsAddress(), c.xdsGRPCPort()),
					),
				},
				UpstreamConnectionOptions: &api.UpstreamConnectionOptions{
					TcpKeepalive: &envoy_api_v2_core.TcpKeepalive{
						KeepaliveProbes:   protobuf.UInt32(3),
						KeepaliveTime:     protobuf.UInt32(30),
						KeepaliveInterval: protobuf.UInt32(5),
					},
				},
				Http2ProtocolOptions: new(envoy_api_v2_core.Http2ProtocolOptions), // enables http2
				CircuitBreakers: &clusterv2.CircuitBreakers{
					Thresholds: []*clusterv2.CircuitBreakers_Thresholds{{
						Priority:           envoy_api_v2_core.RoutingPriority_HIGH,
						MaxConnections:     protobuf.UInt32(100000),
						MaxPendingRequests: protobuf.UInt32(100000),
						MaxRequests:        protobuf.UInt32(60000000),
						MaxRetries:         protobuf.UInt32(50),
					}, {
						Priority:           envoy_api_v2_core.RoutingPriority_DEFAULT,
						MaxConnections:     protobuf.UInt32(100000),
						MaxPendingRequests: protobuf.UInt32(100000),
						MaxRequests:        protobuf.UInt32(60000000),
						MaxRetries:         protobuf.UInt32(50),
					}},
				},
			}, {
				Name:                 "service-stats",
				AltStatName:          strings.Join([]string{c.Namespace, "service-stats", strconv.Itoa(c.adminPort())}, "_"),
				ConnectTimeout:       protobuf.Duration(250 * time.Millisecond),
				ClusterDiscoveryType: ClusterDiscoveryType(api.Cluster_LOGICAL_DNS),
				LbPolicy:             api.Cluster_ROUND_ROBIN,
				LoadAssignment: &api.ClusterLoadAssignment{
					ClusterName: "service-stats",
					Endpoints: Endpoints(
						SocketAddress(c.adminAddress(), c.adminPort()),
					),
				},
			}},
		},
		Admin: &envoy_api_bootstrap.Admin{
			AccessLogPath: c.adminAccessLogPath(),
			Address:       SocketAddress(c.adminAddress(), c.adminPort()),
		},
	}
}

func upstreamFileTLSContext(c *BootstrapConfig) *envoy_api_v2_auth.UpstreamTlsContext {
	context := &envoy_api_v2_auth.UpstreamTlsContext{
		CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
			TlsCertificates: []*envoy_api_v2_auth.TlsCertificate{{
				CertificateChain: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_Filename{
						Filename: c.GrpcClientCert,
					},
				},
				PrivateKey: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_Filename{
						Filename: c.GrpcClientKey,
					},
				},
			}},
			ValidationContextType: &envoy_api_v2_auth.CommonTlsContext_ValidationContext{
				ValidationContext: &envoy_api_v2_auth.CertificateValidationContext{
					TrustedCa: &envoy_api_v2_core.DataSource{
						Specifier: &envoy_api_v2_core.DataSource_Filename{
							Filename: c.GrpcCABundle,
						},
					},
					// TODO(youngnick): Does there need to be a flag wired down to here?
					MatchSubjectAltNames: []*matcher.StringMatcher{{
						MatchPattern: &matcher.StringMatcher_Exact{
							Exact: "contour",
						}},
					},
				},
			},
		},
	}
	return context
}

func upstreamSdsTLSContext(certificateSdsFile, validationSdsFile string) *envoy_api_v2_auth.UpstreamTlsContext {
	context := &envoy_api_v2_auth.UpstreamTlsContext{
		CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
			TlsCertificateSdsSecretConfigs: []*envoy_api_v2_auth.SdsSecretConfig{{
				SdsConfig: &envoy_api_v2_core.ConfigSource{
					ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_Path{
						Path: certificateSdsFile,
					},
				},
			}},
			ValidationContextType: &envoy_api_v2_auth.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: &envoy_api_v2_auth.SdsSecretConfig{
					SdsConfig: &envoy_api_v2_core.ConfigSource{
						ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_Path{
							Path: validationSdsFile,
						},
					},
				},
			},
		},
	}
	return context
}

// tlsCertificateSdsSecretConfig creates DiscoveryResponse with file based SDS resource
// including paths to TLS certificates and key
func tlsCertificateSdsSecretConfig(c *BootstrapConfig) *api.DiscoveryResponse {
	secret := &envoy_api_v2_auth.Secret{
		Type: &envoy_api_v2_auth.Secret_TlsCertificate{
			TlsCertificate: &envoy_api_v2_auth.TlsCertificate{
				CertificateChain: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_Filename{
						Filename: c.GrpcClientCert,
					},
				},
				PrivateKey: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_Filename{
						Filename: c.GrpcClientKey,
					},
				},
			},
		},
	}

	return &api.DiscoveryResponse{
		Resources: []*any.Any{protobuf.MustMarshalAny(secret)},
	}
}

// validationContextSdsSecretConfig creates DiscoveryResponse with file based SDS resource
// including path to CA certificate bundle
func validationContextSdsSecretConfig(c *BootstrapConfig) *api.DiscoveryResponse {
	secret := &envoy_api_v2_auth.Secret{
		Type: &envoy_api_v2_auth.Secret_ValidationContext{
			ValidationContext: &envoy_api_v2_auth.CertificateValidationContext{
				TrustedCa: &envoy_api_v2_core.DataSource{
					Specifier: &envoy_api_v2_core.DataSource_Filename{
						Filename: c.GrpcCABundle,
					},
				},
				MatchSubjectAltNames: []*matcher.StringMatcher{{
					MatchPattern: &matcher.StringMatcher_Exact{
						Exact: "contour",
					}},
				},
			},
		},
	}

	return &api.DiscoveryResponse{
		Resources: []*any.Any{protobuf.MustMarshalAny(secret)},
	}
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

	// Path is the filename for the bootstrap configuration file to be created.
	Path string

	// ResourcesDir is the directory where out of line Envoy resources can be placed.
	ResourcesDir string

	// SkipFilePathCheck specifies whether to skip checking whether files
	// referenced in the configuration actually exist. This option is for
	// testing only.
	SkipFilePathCheck bool
}

func (c *BootstrapConfig) xdsAddress() string   { return stringOrDefault(c.XDSAddress, "127.0.0.1") }
func (c *BootstrapConfig) xdsGRPCPort() int     { return intOrDefault(c.XDSGRPCPort, 8001) }
func (c *BootstrapConfig) adminAddress() string { return stringOrDefault(c.AdminAddress, "127.0.0.1") }
func (c *BootstrapConfig) adminPort() int       { return intOrDefault(c.AdminPort, 9001) }
func (c *BootstrapConfig) adminAccessLogPath() string {
	return stringOrDefault(c.AdminAccessLogPath, "/dev/null")
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

func writeConfig(filename string, config proto.Message) (err error) {
	var out *os.File

	if filename == "-" {
		out = os.Stdout
	} else {
		out, err = os.Create(filename)
		if err != nil {
			return
		}
		defer func() {
			err = out.Close()
		}()
	}

	m := &jsonpb.Marshaler{OrigName: true}
	return m.Marshal(out, config)
}
