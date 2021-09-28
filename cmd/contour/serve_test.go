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

package main

import (
	"testing"

	"github.com/projectcontour/contour/pkg/config"

	"k8s.io/utils/pointer"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestGetDAGBuilder(t *testing.T) {
	commonAssertions := func(t *testing.T, builder *dag.Builder) {
		t.Helper()

		// note that these first two assertions will not hold when a gateway
		// is configured, but we don't currently have test cases that cover
		// that so it's OK to keep them in the "common" assertions for now.
		assert.Len(t, builder.Processors, 4)
		assert.IsType(t, &dag.ListenerProcessor{}, builder.Processors[len(builder.Processors)-1])
	}

	t.Run("all default options", func(t *testing.T) {
		serve := &Serve{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily})
		commonAssertions(t, &got)
		assert.Empty(t, got.Source.ConfiguredSecretRefs)
	})

	t.Run("client cert specified", func(t *testing.T) {
		clientCert := &types.NamespacedName{Namespace: "client-ns", Name: "client-name"}

		serve := &Serve{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, clientCert: clientCert})
		commonAssertions(t, &got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{clientCert})
	})

	t.Run("fallback cert specified", func(t *testing.T) {
		fallbackCert := &types.NamespacedName{Namespace: "fallback-ns", Name: "fallback-name"}

		serve := &Serve{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, fallbackCert: fallbackCert})
		commonAssertions(t, &got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{fallbackCert})
	})

	t.Run("client and fallback certs specified", func(t *testing.T) {
		clientCert := &types.NamespacedName{Namespace: "client-ns", Name: "client-name"}
		fallbackCert := &types.NamespacedName{Namespace: "fallback-ns", Name: "fallback-name"}

		serve := &Serve{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, clientCert: clientCert, fallbackCert: fallbackCert})
		commonAssertions(t, &got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{clientCert, fallbackCert})
	})

	t.Run("request and response headers policy specified", func(t *testing.T) {

		requestHP := &contour_api_v1alpha1.HeadersPolicy{
			Set: map[string]string{
				"req-set-key-1": "req-set-val-1",
				"req-set-key-2": "req-set-val-2",
			},
			Remove: []string{"req-remove-key-1", "req-remove-key-2"},
		}

		responseHP := &contour_api_v1alpha1.HeadersPolicy{
			Set: map[string]string{
				"res-set-key-1": "res-set-val-1",
				"res-set-key-2": "res-set-val-2",
			},
			Remove: []string{"res-remove-key-1", "res-remove-key-2"},
		}

		serve := &Serve{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, requestHP: requestHP, responseHP: responseHP})
		commonAssertions(t, &got)

		httpProxyProcessor := mustGetHTTPProxyProcessor(t, &got)
		assert.EqualValues(t, requestHP.Set, httpProxyProcessor.RequestHeadersPolicy.Set)
		assert.ElementsMatch(t, requestHP.Remove, httpProxyProcessor.RequestHeadersPolicy.Remove)
		assert.EqualValues(t, responseHP.Set, httpProxyProcessor.ResponseHeadersPolicy.Set)
		assert.ElementsMatch(t, responseHP.Remove, httpProxyProcessor.ResponseHeadersPolicy.Remove)
	})

	// TODO(3453): test additional properties of the DAG builder (processor fields, cache fields, Gateway tests (requires a client fake))
}

func mustGetHTTPProxyProcessor(t *testing.T, builder *dag.Builder) *dag.HTTPProxyProcessor {
	t.Helper()
	for i := range builder.Processors {
		found, ok := builder.Processors[i].(*dag.HTTPProxyProcessor)
		if ok {
			return found
		}
	}

	require.FailNow(t, "HTTPProxyProcessor not found in list of DAG builder's processors")
	return nil
}

func TestConvertServeContext(t *testing.T) {

	defaultContext := newServeContext()
	defaultContext.ServerConfig = ServerConfig{
		xdsAddr:     "127.0.0.1",
		xdsPort:     8001,
		caFile:      "/certs/ca.crt",
		contourCert: "/certs/cert.crt",
		contourKey:  "/certs/cert.key",
	}

	defaultContext.ingressClassName = "coolclass"
	defaultContext.Config.IngressStatusAddress = "1.2.3.4"
	defaultContext.Config.GatewayConfig = &config.GatewayParameters{
		ControllerName: "projectcontour.io/projectcontour/contour",
	}
	defaultContext.Config.TLS.ClientCertificate = config.NamespacedName{
		Name:      "cert",
		Namespace: "secretplace",
	}

	cases := map[string]struct {
		serveContext  *serveContext
		contourConfig contour_api_v1alpha1.ContourConfigurationSpec
	}{
		"default ServeContext": {
			serveContext: defaultContext,
			contourConfig: contour_api_v1alpha1.ContourConfigurationSpec{
				XDSServer: contour_api_v1alpha1.XDSServerConfig{
					Type:    contour_api_v1alpha1.ContourServerType,
					Address: "127.0.0.1",
					Port:    8001,
					TLS: &contour_api_v1alpha1.TLS{
						CAFile:   "/certs/ca.crt",
						CertFile: "/certs/cert.crt",
						KeyFile:  "/certs/cert.key",
						Insecure: false,
					},
				},
				Ingress: &contour_api_v1alpha1.IngressConfig{
					ClassName:     pointer.StringPtr("coolclass"),
					StatusAddress: pointer.StringPtr("1.2.3.4"),
				},
				Debug: contour_api_v1alpha1.DebugConfig{
					Address:                 "127.0.0.1",
					Port:                    6060,
					DebugLogLevel:           contour_api_v1alpha1.InfoLog,
					KubernetesDebugLogLevel: 0,
				},
				Health: contour_api_v1alpha1.HealthConfig{
					Address: "0.0.0.0",
					Port:    8000,
				},
				Envoy: contour_api_v1alpha1.EnvoyConfig{
					Service: contour_api_v1alpha1.NamespacedName{
						Name:      "envoy",
						Namespace: "projectcontour",
					},
					HTTPListener: contour_api_v1alpha1.EnvoyListener{
						Address:   "0.0.0.0",
						Port:      8080,
						AccessLog: "/dev/stdout",
					},
					HTTPSListener: contour_api_v1alpha1.EnvoyListener{
						Address:   "0.0.0.0",
						Port:      8443,
						AccessLog: "/dev/stdout",
					},
					Metrics: contour_api_v1alpha1.MetricsConfig{
						Address: "0.0.0.0",
						Port:    8002,
					},
					ClientCertificate: &contour_api_v1alpha1.NamespacedName{
						Name:      "cert",
						Namespace: "secretplace",
					},
					Logging: contour_api_v1alpha1.EnvoyLogging{
						AccessLogFormat:       contour_api_v1alpha1.EnvoyAccessLog,
						AccessLogFormatString: nil,
						AccessLogFields: contour_api_v1alpha1.AccessLogFields([]string{
							"@timestamp",
							"authority",
							"bytes_received",
							"bytes_sent",
							"downstream_local_address",
							"downstream_remote_address",
							"duration",
							"method",
							"path",
							"protocol",
							"request_id",
							"requested_server_name",
							"response_code",
							"response_flags",
							"uber_trace_id",
							"upstream_cluster",
							"upstream_host",
							"upstream_local_address",
							"upstream_service_time",
							"user_agent",
							"x_forwarded_for",
						}),
					},
					DefaultHTTPVersions: nil,
					Timeouts: &contour_api_v1alpha1.TimeoutParameters{
						ConnectionIdleTimeout: pointer.StringPtr("60s"),
					},
					Cluster: contour_api_v1alpha1.ClusterParameters{
						DNSLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily,
					},
					Network: contour_api_v1alpha1.NetworkParameters{
						EnvoyAdminPort: 9001,
					},
				},
				Gateway: &contour_api_v1alpha1.GatewayConfig{
					ControllerName: "projectcontour.io/projectcontour/contour",
				},
				HTTPProxy: contour_api_v1alpha1.HTTPProxyConfig{
					DisablePermitInsecure: false,
					FallbackCertificate:   nil,
				},
				LeaderElection: contour_api_v1alpha1.LeaderElectionConfig{
					LeaseDuration: "15s",
					RenewDeadline: "10s",
					RetryPeriod:   "2s",
					Configmap: contour_api_v1alpha1.NamespacedName{
						Name:      "leader-elect",
						Namespace: "projectcontour",
					},
					DisableLeaderElection: false,
				},
				EnableExternalNameService: false,
				RateLimitService:          nil,
				Policy: &contour_api_v1alpha1.PolicyConfig{
					RequestHeadersPolicy:  &contour_api_v1alpha1.HeadersPolicy{},
					ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{},
				},
				Metrics: contour_api_v1alpha1.MetricsConfig{
					Address: "0.0.0.0",
					Port:    8000,
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			converted := tc.serveContext.convertToContourConfigurationSpec()
			assert.Equal(t, tc.contourConfig, converted)
		})
	}
}

func mustGetIngressProcessor(t *testing.T, builder *dag.Builder) *dag.IngressProcessor {
	t.Helper()
	for i := range builder.Processors {
		found, ok := builder.Processors[i].(*dag.IngressProcessor)
		if ok {
			return found
		}
	}

	require.FailNow(t, "IngressProcessor not found in list of DAG builder's processors")
	return nil
}
