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

	envoy_config_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_overload_v3 "github.com/envoyproxy/go-control-plane/envoy/config/overload/v3"
	envoy_access_logger_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_regex_engines_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/regex_engines/v3"
	envoy_fixed_heap_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/resource_monitors/fixed_heap/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

// WriteBootstrap writes bootstrap configuration to files.
func (e *EnvoyGen) WriteBootstrap(c *envoy.BootstrapConfig) error {
	// Create Envoy bootstrap config and associated resource files.
	steps, err := e.bootstrap(c)
	if err != nil {
		return err
	}

	if c.ResourcesDir != "" {
		// Setting permissions to 0777 explicitly.
		// Refer Issue: https://github.com/projectcontour/contour/issues/3264
		// The secrets in this directory are "pointers" to actual secrets
		// mounted from Kubernetes secrets; that means the actual secrets aren't 0777
		if err := os.MkdirAll(path.Join(c.ResourcesDir, "sds"), 0o777); err != nil {
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
func (e *EnvoyGen) bootstrap(c *envoy.BootstrapConfig) ([]bootstrapf, error) {
	var steps []bootstrapf

	if c.GrpcClientCert == "" && c.GrpcClientKey == "" && c.GrpcCABundle == "" {
		steps = append(steps,
			func(*envoy.BootstrapConfig) (string, proto.Message) {
				return c.Path, e.bootstrapConfig(c)
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
				b := e.bootstrapConfig(c)
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
			b := e.bootstrapConfig(c)
			b.StaticResources.Clusters[0].TransportSocket = UpstreamTLSTransportSocket(
				upstreamSdsTLSContext(sdsTLSCertificatePath, sdsValidationContextPath))
			return c.Path, b
		},
	)

	return steps, nil
}

func (e *EnvoyGen) bootstrapConfig(c *envoy.BootstrapConfig) *envoy_config_bootstrap_v3.Bootstrap {
	bootstrap := &envoy_config_bootstrap_v3.Bootstrap{
		LayeredRuntime: &envoy_config_bootstrap_v3.LayeredRuntime{
			Layers: []*envoy_config_bootstrap_v3.RuntimeLayer{
				{
					Name: "dynamic",
					LayerSpecifier: &envoy_config_bootstrap_v3.RuntimeLayer_RtdsLayer_{
						RtdsLayer: &envoy_config_bootstrap_v3.RuntimeLayer_RtdsLayer{
							Name:       DynamicRuntimeLayerName,
							RtdsConfig: e.GetConfigSource(),
						},
					},
				},
				// Admin layer needs to be included here to maintain ability to
				// modify runtime settings via admin console. We have it as the
				// last layer so changes made via admin console override any
				// settings from previous layers.
				// See https://www.envoyproxy.io/docs/envoy/latest/configuration/operations/runtime#admin-console
				{
					Name: "admin",
					LayerSpecifier: &envoy_config_bootstrap_v3.RuntimeLayer_AdminLayer_{
						AdminLayer: &envoy_config_bootstrap_v3.RuntimeLayer_AdminLayer{},
					},
				},
			},
		},
		DynamicResources: &envoy_config_bootstrap_v3.Bootstrap_DynamicResources{
			LdsConfig: e.GetConfigSource(),
			CdsConfig: e.GetConfigSource(),
		},
		StaticResources: &envoy_config_bootstrap_v3.Bootstrap_StaticResources{
			Clusters: []*envoy_config_cluster_v3.Cluster{{
				DnsLookupFamily:      parseDNSLookupFamily(c.DNSLookupFamily),
				Name:                 "contour",
				AltStatName:          strings.Join([]string{c.Namespace, "contour", strconv.Itoa(c.GetXdsGRPCPort())}, "_"),
				ConnectTimeout:       durationpb.New(5 * time.Second),
				ClusterDiscoveryType: clusterDiscoveryTypeForAddress(c.GetXdsAddress(), envoy_config_cluster_v3.Cluster_STRICT_DNS),
				LbPolicy:             envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "contour",
					Endpoints: Endpoints(
						SocketAddress(c.GetXdsAddress(), c.GetXdsGRPCPort()),
					),
				},
				UpstreamConnectionOptions: &envoy_config_cluster_v3.UpstreamConnectionOptions{
					TcpKeepalive: &envoy_config_core_v3.TcpKeepalive{
						KeepaliveProbes:   wrapperspb.UInt32(3),
						KeepaliveTime:     wrapperspb.UInt32(30),
						KeepaliveInterval: wrapperspb.UInt32(5),
					},
				},
				TypedExtensionProtocolOptions: protocolOptions(HTTPVersion2, timeout.DefaultSetting(), nil),
				CircuitBreakers: &envoy_config_cluster_v3.CircuitBreakers{
					Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
						Priority:           envoy_config_core_v3.RoutingPriority_HIGH,
						MaxConnections:     wrapperspb.UInt32(100000),
						MaxPendingRequests: wrapperspb.UInt32(100000),
						MaxRequests:        wrapperspb.UInt32(60000000),
						MaxRetries:         wrapperspb.UInt32(50),
						TrackRemaining:     true,
					}, {
						Priority:           envoy_config_core_v3.RoutingPriority_DEFAULT,
						MaxConnections:     wrapperspb.UInt32(100000),
						MaxPendingRequests: wrapperspb.UInt32(100000),
						MaxRequests:        wrapperspb.UInt32(60000000),
						MaxRetries:         wrapperspb.UInt32(50),
						TrackRemaining:     true,
					}},
				},
			}, {
				Name:                 "envoy-admin",
				AltStatName:          strings.Join([]string{c.Namespace, "envoy-admin", strconv.Itoa(c.GetAdminPort())}, "_"),
				ConnectTimeout:       durationpb.New(250 * time.Millisecond),
				ClusterDiscoveryType: clusterDiscoveryTypeForAddress(c.GetAdminAddress(), envoy_config_cluster_v3.Cluster_STATIC),
				LbPolicy:             envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "envoy-admin",
					Endpoints: Endpoints(
						unixSocketAddress(c.GetAdminAddress()),
					),
				},
			}},
		},
		DefaultRegexEngine: &envoy_config_core_v3.TypedExtensionConfig{
			Name:        "envoy.regex_engines.google_re2",
			TypedConfig: protobuf.MustMarshalAny(&envoy_regex_engines_v3.GoogleRE2{}),
		},
		Admin: &envoy_config_bootstrap_v3.Admin{
			AccessLog: adminAccessLog(c.GetAdminAccessLogPath()),
			Address:   unixSocketAddress(c.GetAdminAddress()),
		},
	}
	if c.MaximumHeapSizeBytes > 0 {
		bootstrap.OverloadManager = &envoy_config_overload_v3.OverloadManager{
			RefreshInterval: durationpb.New(250 * time.Millisecond),
			ResourceMonitors: []*envoy_config_overload_v3.ResourceMonitor{
				{
					Name: "envoy.resource_monitors.fixed_heap",
					ConfigType: &envoy_config_overload_v3.ResourceMonitor_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(
							&envoy_fixed_heap_v3.FixedHeapConfig{
								MaxHeapSizeBytes: c.MaximumHeapSizeBytes,
							}),
					},
				},
			},
			Actions: []*envoy_config_overload_v3.OverloadAction{
				{
					Name: "envoy.overload_actions.shrink_heap",
					Triggers: []*envoy_config_overload_v3.Trigger{
						{
							Name: "envoy.resource_monitors.fixed_heap",
							TriggerOneof: &envoy_config_overload_v3.Trigger_Threshold{
								Threshold: &envoy_config_overload_v3.ThresholdTrigger{
									Value: 0.95,
								},
							},
						},
					},
				},
				{
					Name: "envoy.overload_actions.stop_accepting_requests",
					Triggers: []*envoy_config_overload_v3.Trigger{
						{
							Name: "envoy.resource_monitors.fixed_heap",
							TriggerOneof: &envoy_config_overload_v3.Trigger_Threshold{
								Threshold: &envoy_config_overload_v3.ThresholdTrigger{
									Value: 0.98,
								},
							},
						},
					},
				},
			},
		}
	}
	return bootstrap
}

func adminAccessLog(logPath string) []*envoy_config_accesslog_v3.AccessLog {
	return []*envoy_config_accesslog_v3.AccessLog{
		{
			Name: "envoy.access_loggers.file",
			ConfigType: &envoy_config_accesslog_v3.AccessLog_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_access_logger_file_v3.FileAccessLog{
					Path: logPath,
				}),
			},
		},
	}
}

func upstreamFileTLSContext(c *envoy.BootstrapConfig) *envoy_transport_socket_tls_v3.UpstreamTlsContext {
	context := &envoy_transport_socket_tls_v3.UpstreamTlsContext{
		CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
			TlsParams: &envoy_transport_socket_tls_v3.TlsParameters{
				TlsMaximumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
			},
			TlsCertificates: []*envoy_transport_socket_tls_v3.TlsCertificate{{
				CertificateChain: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_Filename{
						Filename: c.GrpcClientCert,
					},
				},
				PrivateKey: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_Filename{
						Filename: c.GrpcClientKey,
					},
				},
			}},
			ValidationContextType: &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
				ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
					TrustedCa: &envoy_config_core_v3.DataSource{
						Specifier: &envoy_config_core_v3.DataSource_Filename{
							Filename: c.GrpcCABundle,
						},
					},
					// TODO(youngnick): Does there need to be a flag wired down to here?
					MatchTypedSubjectAltNames: []*envoy_transport_socket_tls_v3.SubjectAltNameMatcher{
						{
							SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
							Matcher: &envoy_matcher_v3.StringMatcher{
								MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
									Exact: "contour",
								},
							},
						},
					},
				},
			},
		},
	}
	return context
}

func upstreamSdsTLSContext(certificateSdsFile, validationSdsFile string) *envoy_transport_socket_tls_v3.UpstreamTlsContext {
	return &envoy_transport_socket_tls_v3.UpstreamTlsContext{
		CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
			TlsParams: &envoy_transport_socket_tls_v3.TlsParameters{
				TlsMaximumProtocolVersion: envoy_transport_socket_tls_v3.TlsParameters_TLSv1_3,
			},
			TlsCertificateSdsSecretConfigs: []*envoy_transport_socket_tls_v3.SdsSecretConfig{{
				Name: "contour_xds_tls_certificate",
				SdsConfig: &envoy_config_core_v3.ConfigSource{
					ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
					ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_PathConfigSource{
						PathConfigSource: &envoy_config_core_v3.PathConfigSource{
							Path: certificateSdsFile,
						},
					},
				},
			}},
			ValidationContextType: &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: &envoy_transport_socket_tls_v3.SdsSecretConfig{
					Name: "contour_xds_tls_validation_context",
					SdsConfig: &envoy_config_core_v3.ConfigSource{
						ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
						ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_PathConfigSource{
							PathConfigSource: &envoy_config_core_v3.PathConfigSource{
								Path: validationSdsFile,
							},
						},
					},
				},
			},
		},
	}
}

// tlsCertificateSdsSecretConfig creates DiscoveryResponse with file based SDS resource
// including paths to TLS certificates and key
func tlsCertificateSdsSecretConfig(c *envoy.BootstrapConfig) *envoy_service_discovery_v3.DiscoveryResponse {
	secret := &envoy_transport_socket_tls_v3.Secret{
		Name: "contour_xds_tls_certificate",
		Type: &envoy_transport_socket_tls_v3.Secret_TlsCertificate{
			TlsCertificate: &envoy_transport_socket_tls_v3.TlsCertificate{
				CertificateChain: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_Filename{
						Filename: c.GrpcClientCert,
					},
				},
				PrivateKey: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_Filename{
						Filename: c.GrpcClientKey,
					},
				},
			},
		},
	}

	return &envoy_service_discovery_v3.DiscoveryResponse{
		Resources: []*anypb.Any{protobuf.MustMarshalAny(secret)},
	}
}

// validationContextSdsSecretConfig creates DiscoveryResponse with file based SDS resource
// including path to CA certificate bundle
func validationContextSdsSecretConfig(c *envoy.BootstrapConfig) *envoy_service_discovery_v3.DiscoveryResponse {
	secret := &envoy_transport_socket_tls_v3.Secret{
		Name: "contour_xds_tls_validation_context",
		Type: &envoy_transport_socket_tls_v3.Secret_ValidationContext{
			ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
				TrustedCa: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_Filename{
						Filename: c.GrpcCABundle,
					},
				},
				MatchTypedSubjectAltNames: []*envoy_transport_socket_tls_v3.SubjectAltNameMatcher{
					{
						SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
						Matcher: &envoy_matcher_v3.StringMatcher{
							MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
								Exact: "contour",
							},
						},
					},
				},
			},
		},
	}

	return &envoy_service_discovery_v3.DiscoveryResponse{
		Resources: []*anypb.Any{protobuf.MustMarshalAny(secret)},
	}
}
