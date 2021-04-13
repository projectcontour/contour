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

package v3

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	envoy_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
)

// WriteBootstrap writes bootstrap configuration to files.
func WriteBootstrap(c *envoy.BootstrapConfig) error {
	// Create Envoy bootstrap config and associated resource files.
	steps, err := bootstrap(c)
	if err != nil {
		return err
	}

	if c.ResourcesDir != "" {
		// Setting permissions to 0777 explicitly.
		// Refer Issue: https://github.com/projectcontour/contour/issues/3264
		// The secrets in this directory are "pointers" to actual secrets
		// mounted from Kubernetes secrets; that means the actual secrets aren't 0777
		if err := os.MkdirAll(path.Join(c.ResourcesDir, "sds"), 0777); err != nil {
			return err
		}
	}

	// Write all configuration files out to filesystem.
	for _, step := range steps {
		if err := envoy.WriteConfig(step(c)); err != nil {
			return err
		}
	}

	return nil
}

type bootstrapf func(*envoy.BootstrapConfig) (string, proto.Message)

// bootstrap creates a new v3 bootstrap configuration and associated resource files.
func bootstrap(c *envoy.BootstrapConfig) ([]bootstrapf, error) {
	var steps []bootstrapf

	if c.GrpcClientCert == "" && c.GrpcClientKey == "" && c.GrpcCABundle == "" {
		steps = append(steps,
			func(*envoy.BootstrapConfig) (string, proto.Message) {
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
			func(*envoy.BootstrapConfig) (string, proto.Message) {
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
	sdsTLSCertificatePath := path.Join(c.ResourcesDir, envoy.SDSResourcesSubdirectory, envoy.SDSTLSCertificateFile)
	sdsValidationContextPath := path.Join(c.ResourcesDir, envoy.SDSResourcesSubdirectory, envoy.SDSValidationContextFile)

	steps = append(steps,
		func(*envoy.BootstrapConfig) (string, proto.Message) {
			return sdsTLSCertificatePath, tlsCertificateSdsSecretConfig(c)
		},
		func(*envoy.BootstrapConfig) (string, proto.Message) {
			return sdsValidationContextPath, validationContextSdsSecretConfig(c)
		},
		func(*envoy.BootstrapConfig) (string, proto.Message) {
			b := bootstrapConfig(c)
			b.StaticResources.Clusters[0].TransportSocket = UpstreamTLSTransportSocket(
				upstreamSdsTLSContext(sdsTLSCertificatePath, sdsValidationContextPath))
			return c.Path, b
		},
	)

	return steps, nil
}

func bootstrapConfig(c *envoy.BootstrapConfig) *envoy_bootstrap_v3.Bootstrap {
	return &envoy_bootstrap_v3.Bootstrap{
		DynamicResources: &envoy_bootstrap_v3.Bootstrap_DynamicResources{
			LdsConfig: ConfigSource("contour"),
			CdsConfig: ConfigSource("contour"),
		},
		StaticResources: &envoy_bootstrap_v3.Bootstrap_StaticResources{
			Clusters: []*envoy_cluster_v3.Cluster{{
				DnsLookupFamily:      parseDNSLookupFamily(c.DNSLookupFamily),
				Name:                 "contour",
				AltStatName:          strings.Join([]string{c.Namespace, "contour", strconv.Itoa(c.GetXdsGRPCPort())}, "_"),
				ConnectTimeout:       protobuf.Duration(5 * time.Second),
				ClusterDiscoveryType: ClusterDiscoveryTypeForAddress(c.GetXdsAddress(), envoy_cluster_v3.Cluster_STRICT_DNS),
				LbPolicy:             envoy_cluster_v3.Cluster_ROUND_ROBIN,
				LoadAssignment: &envoy_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "contour",
					Endpoints: Endpoints(
						SocketAddress(c.GetXdsAddress(), c.GetXdsGRPCPort()),
					),
				},
				UpstreamConnectionOptions: &envoy_cluster_v3.UpstreamConnectionOptions{
					TcpKeepalive: &envoy_core_v3.TcpKeepalive{
						KeepaliveProbes:   protobuf.UInt32(3),
						KeepaliveTime:     protobuf.UInt32(30),
						KeepaliveInterval: protobuf.UInt32(5),
					},
				},
				TypedExtensionProtocolOptions: http2ProtocolOptions(),
				CircuitBreakers: &envoy_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_cluster_v3.CircuitBreakers_Thresholds{{
						Priority:           envoy_core_v3.RoutingPriority_HIGH,
						MaxConnections:     protobuf.UInt32(100000),
						MaxPendingRequests: protobuf.UInt32(100000),
						MaxRequests:        protobuf.UInt32(60000000),
						MaxRetries:         protobuf.UInt32(50),
					}, {
						Priority:           envoy_core_v3.RoutingPriority_DEFAULT,
						MaxConnections:     protobuf.UInt32(100000),
						MaxPendingRequests: protobuf.UInt32(100000),
						MaxRequests:        protobuf.UInt32(60000000),
						MaxRetries:         protobuf.UInt32(50),
					}},
				},
			}, {
				Name:                 "service-stats",
				AltStatName:          strings.Join([]string{c.Namespace, "service-stats", strconv.Itoa(c.GetAdminPort())}, "_"),
				ConnectTimeout:       protobuf.Duration(250 * time.Millisecond),
				ClusterDiscoveryType: ClusterDiscoveryTypeForAddress(c.GetAdminAddress(), envoy_cluster_v3.Cluster_LOGICAL_DNS),
				LbPolicy:             envoy_cluster_v3.Cluster_ROUND_ROBIN,
				LoadAssignment: &envoy_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "service-stats",
					Endpoints: Endpoints(
						SocketAddress(c.GetAdminAddress(), c.GetAdminPort()),
					),
				},
			}},
		},
		Admin: &envoy_bootstrap_v3.Admin{
			AccessLogPath: c.GetAdminAccessLogPath(),
			Address:       SocketAddress(c.GetAdminAddress(), c.GetAdminPort()),
		},
	}
}

func upstreamFileTLSContext(c *envoy.BootstrapConfig) *envoy_tls_v3.UpstreamTlsContext {
	context := &envoy_tls_v3.UpstreamTlsContext{
		CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
			TlsCertificates: []*envoy_tls_v3.TlsCertificate{{
				CertificateChain: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_Filename{
						Filename: c.GrpcClientCert,
					},
				},
				PrivateKey: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_Filename{
						Filename: c.GrpcClientKey,
					},
				},
			}},
			ValidationContextType: &envoy_tls_v3.CommonTlsContext_ValidationContext{
				ValidationContext: &envoy_tls_v3.CertificateValidationContext{
					TrustedCa: &envoy_core_v3.DataSource{
						Specifier: &envoy_core_v3.DataSource_Filename{
							Filename: c.GrpcCABundle,
						},
					},
					// TODO(youngnick): Does there need to be a flag wired down to here?
					MatchSubjectAltNames: []*envoy_matcher_v3.StringMatcher{{
						MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
							Exact: "contour",
						}},
					},
				},
			},
		},
	}
	return context
}

func upstreamSdsTLSContext(certificateSdsFile, validationSdsFile string) *envoy_tls_v3.UpstreamTlsContext {
	context := &envoy_tls_v3.UpstreamTlsContext{
		CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
			TlsCertificateSdsSecretConfigs: []*envoy_tls_v3.SdsSecretConfig{{
				Name: "contour_xds_tls_certificate",
				SdsConfig: &envoy_core_v3.ConfigSource{
					ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
					ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_Path{
						Path: certificateSdsFile,
					},
				},
			}},
			ValidationContextType: &envoy_tls_v3.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: &envoy_tls_v3.SdsSecretConfig{
					Name: "contour_xds_tls_validation_context",
					SdsConfig: &envoy_core_v3.ConfigSource{
						ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
						ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_Path{
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
func tlsCertificateSdsSecretConfig(c *envoy.BootstrapConfig) *envoy_service_discovery_v3.DiscoveryResponse {
	secret := &envoy_tls_v3.Secret{
		Name: "contour_xds_tls_certificate",
		Type: &envoy_tls_v3.Secret_TlsCertificate{
			TlsCertificate: &envoy_tls_v3.TlsCertificate{
				CertificateChain: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_Filename{
						Filename: c.GrpcClientCert,
					},
				},
				PrivateKey: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_Filename{
						Filename: c.GrpcClientKey,
					},
				},
			},
		},
	}

	return &envoy_service_discovery_v3.DiscoveryResponse{
		Resources: []*any.Any{protobuf.MustMarshalAny(secret)},
	}
}

// validationContextSdsSecretConfig creates DiscoveryResponse with file based SDS resource
// including path to CA certificate bundle
func validationContextSdsSecretConfig(c *envoy.BootstrapConfig) *envoy_service_discovery_v3.DiscoveryResponse {
	secret := &envoy_tls_v3.Secret{
		Name: "contour_xds_tls_validation_context",
		Type: &envoy_tls_v3.Secret_ValidationContext{
			ValidationContext: &envoy_tls_v3.CertificateValidationContext{
				TrustedCa: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_Filename{
						Filename: c.GrpcCABundle,
					},
				},
				MatchSubjectAltNames: []*envoy_matcher_v3.StringMatcher{{
					MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
						Exact: "contour",
					}},
				},
			},
		},
	}

	return &envoy_service_discovery_v3.DiscoveryResponse{
		Resources: []*any.Any{protobuf.MustMarshalAny(secret)},
	}
}
