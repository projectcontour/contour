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
	"context"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl_cache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
)

// fakeManager implements manager.Manager interface for testing.
// Only GetAPIReader() is implemented; other methods panic if called.
type fakeManager struct {
	apiReader client.Reader
}

var _ manager.Manager = &fakeManager{}

func (f *fakeManager) GetAPIReader() client.Reader                                  { return f.apiReader }
func (f *fakeManager) Add(manager.Runnable) error                                   { panic("not implemented") }
func (f *fakeManager) Elected() <-chan struct{}                                     { panic("not implemented") }
func (f *fakeManager) AddMetricsServerExtraHandler(string, http.Handler) error      { panic("not implemented") }
func (f *fakeManager) AddHealthzCheck(string, healthz.Checker) error                { panic("not implemented") }
func (f *fakeManager) AddReadyzCheck(string, healthz.Checker) error                 { panic("not implemented") }
func (f *fakeManager) Start(context.Context) error                                  { panic("not implemented") }
func (f *fakeManager) GetWebhookServer() webhook.Server                             { panic("not implemented") }
func (f *fakeManager) GetLogger() logr.Logger                                       { panic("not implemented") }
func (f *fakeManager) GetControllerOptions() config.Controller                      { panic("not implemented") }
func (f *fakeManager) GetHTTPClient() *http.Client                                  { panic("not implemented") }
func (f *fakeManager) GetConfig() *rest.Config                                      { panic("not implemented") }
func (f *fakeManager) GetCache() ctrl_cache.Cache                                   { panic("not implemented") }
func (f *fakeManager) GetScheme() *runtime.Scheme                                   { panic("not implemented") }
func (f *fakeManager) GetClient() client.Client                                     { panic("not implemented") }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer                         { panic("not implemented") }
func (f *fakeManager) GetEventRecorderFor(string) record.EventRecorder              { panic("not implemented") }
func (f *fakeManager) GetRESTMapper() meta.RESTMapper                               { panic("not implemented") }

func TestGetDAGBuilder(t *testing.T) {
	commonAssertions := func(t *testing.T, builder *dag.Builder) {
		t.Helper()

		// note that these first two assertions will not hold when a gateway
		// is configured, but we don't currently have test cases that cover
		// that so it's OK to keep them in the "common" assertions for now.
		assert.Len(t, builder.Processors, 4)
		assert.IsType(t, &dag.ListenerProcessor{}, builder.Processors[0])

		ingressProcessor := mustGetIngressProcessor(t, builder)
		assert.True(t, ingressProcessor.SetSourceMetadataOnRoutes)

		httpProxyProcessor := mustGetHTTPProxyProcessor(t, builder)
		assert.True(t, httpProxyProcessor.SetSourceMetadataOnRoutes)
	}

	t.Run("all default options", func(t *testing.T) {
		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_v1alpha1.AutoClusterDNSFamily})
		commonAssertions(t, got)
		assert.Empty(t, got.Source.ConfiguredSecretRefs)
	})

	t.Run("client cert specified", func(t *testing.T) {
		clientCert := &types.NamespacedName{Namespace: "client-ns", Name: "client-name"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_v1alpha1.AutoClusterDNSFamily, clientCert: clientCert})
		commonAssertions(t, got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{clientCert})
	})

	t.Run("fallback cert specified", func(t *testing.T) {
		fallbackCert := &types.NamespacedName{Namespace: "fallback-ns", Name: "fallback-name"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_v1alpha1.AutoClusterDNSFamily, fallbackCert: fallbackCert})
		commonAssertions(t, got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{fallbackCert})
	})

	t.Run("client and fallback certs specified", func(t *testing.T) {
		clientCert := &types.NamespacedName{Namespace: "client-ns", Name: "client-name"}
		fallbackCert := &types.NamespacedName{Namespace: "fallback-ns", Name: "fallback-name"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_v1alpha1.AutoClusterDNSFamily, clientCert: clientCert, fallbackCert: fallbackCert})
		commonAssertions(t, got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{clientCert, fallbackCert})
	})

	t.Run("request and response headers policy specified", func(t *testing.T) {
		policy := &contour_v1alpha1.PolicyConfig{
			RequestHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"req-set-key-1": "req-set-val-1",
					"req-set-key-2": "req-set-val-2",
				},
				Remove: []string{"req-remove-key-1", "req-remove-key-2"},
			},
			ResponseHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"res-set-key-1": "res-set-val-1",
					"res-set-key-2": "res-set-val-2",
				},
				Remove: []string{"res-remove-key-1", "res-remove-key-2"},
			},
			ApplyToIngress: ptr.To(false),
		}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_v1alpha1.AutoClusterDNSFamily, headersPolicy: policy})
		commonAssertions(t, got)

		httpProxyProcessor := mustGetHTTPProxyProcessor(t, got)
		assert.Equal(t, policy.RequestHeadersPolicy.Set, httpProxyProcessor.RequestHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.RequestHeadersPolicy.Remove, httpProxyProcessor.RequestHeadersPolicy.Remove)
		assert.Equal(t, policy.ResponseHeadersPolicy.Set, httpProxyProcessor.ResponseHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.ResponseHeadersPolicy.Remove, httpProxyProcessor.ResponseHeadersPolicy.Remove)

		ingressProcessor := mustGetIngressProcessor(t, got)
		assert.Equal(t, map[string]string(nil), ingressProcessor.RequestHeadersPolicy.Set)
		assert.ElementsMatch(t, map[string]string(nil), ingressProcessor.RequestHeadersPolicy.Remove)
		assert.Equal(t, map[string]string(nil), ingressProcessor.ResponseHeadersPolicy.Set)
		assert.ElementsMatch(t, map[string]string(nil), ingressProcessor.ResponseHeadersPolicy.Remove)
	})

	t.Run("GlobalCircuitBreakerDefaults specified for all processors", func(t *testing.T) {
		g := contour_v1alpha1.CircuitBreakers{
			MaxConnections: 100,
		}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{
			gatewayRef:                   &types.NamespacedName{Namespace: "projectcontour", Name: "contour"},
			rootNamespaces:               []string{},
			dnsLookupFamily:              contour_v1alpha1.AutoClusterDNSFamily,
			globalCircuitBreakerDefaults: &g,
		})

		iProcessor := mustGetIngressProcessor(t, got)
		assert.Equal(t, iProcessor.GlobalCircuitBreakerDefaults, &g)

		hProcessor := mustGetHTTPProxyProcessor(t, got)
		assert.Equal(t, hProcessor.GlobalCircuitBreakerDefaults, &g)

		gProcessor := mustGetGatewayAPIProcessor(t, got)
		assert.Equal(t, gProcessor.GlobalCircuitBreakerDefaults, &g)
	})

	t.Run("request and response headers policy specified for ingress", func(t *testing.T) {
		policy := &contour_v1alpha1.PolicyConfig{
			RequestHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"req-set-key-1": "req-set-val-1",
					"req-set-key-2": "req-set-val-2",
				},
				Remove: []string{"req-remove-key-1", "req-remove-key-2"},
			},
			ResponseHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"res-set-key-1": "res-set-val-1",
					"res-set-key-2": "res-set-val-2",
				},
				Remove: []string{"res-remove-key-1", "res-remove-key-2"},
			},
			ApplyToIngress: ptr.To(true),
		}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{
			rootNamespaces:  []string{},
			dnsLookupFamily: contour_v1alpha1.AutoClusterDNSFamily,
			headersPolicy:   policy,
		})
		commonAssertions(t, got)

		ingressProcessor := mustGetIngressProcessor(t, got)
		assert.Equal(t, policy.RequestHeadersPolicy.Set, ingressProcessor.RequestHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.RequestHeadersPolicy.Remove, ingressProcessor.RequestHeadersPolicy.Remove)
		assert.Equal(t, policy.ResponseHeadersPolicy.Set, ingressProcessor.ResponseHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.ResponseHeadersPolicy.Remove, ingressProcessor.ResponseHeadersPolicy.Remove)
	})

	t.Run("single ingress class specified", func(t *testing.T) {
		ingressClassNames := []string{"aclass"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{
			rootNamespaces:    []string{},
			dnsLookupFamily:   contour_v1alpha1.AutoClusterDNSFamily,
			ingressClassNames: ingressClassNames,
		})
		commonAssertions(t, got)
		assert.Equal(t, ingressClassNames, got.Source.IngressClassNames)
	})

	t.Run("multiple comma-separated ingress classes specified", func(t *testing.T) {
		ingressClassNames := []string{"aclass", "bclass", "cclass"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{
			rootNamespaces:    []string{},
			dnsLookupFamily:   contour_v1alpha1.AutoClusterDNSFamily,
			ingressClassNames: ingressClassNames,
		})
		commonAssertions(t, got)
		assert.Equal(t, ingressClassNames, got.Source.IngressClassNames)
	})

	// TODO(3453): test additional properties of the DAG builder (processor fields, cache fields, Gateway tests (requires a client fake))
}

func mustGetGatewayAPIProcessor(t *testing.T, builder *dag.Builder) *dag.GatewayAPIProcessor {
	t.Helper()
	for i := range builder.Processors {
		found, ok := builder.Processors[i].(*dag.GatewayAPIProcessor)
		if ok {
			return found
		}
	}

	require.FailNow(t, "GatewayAPIProcessor not found in list of DAG builder's processors")
	return nil
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

func TestSetupTracingService(t *testing.T) {
	// Helper to create a fake client with an ExtensionService
	newFakeClient := func(objects ...runtime.Object) client.Reader {
		scheme := runtime.NewScheme()
		require.NoError(t, contour_v1alpha1.AddToScheme(scheme))
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(objects...).
			Build()
	}

	// Helper to create a Server with a fake manager
	newServerWithClient := func(c client.Reader) *Server {
		return &Server{
			log: logrus.StandardLogger(),
			mgr: &fakeManager{apiReader: c},
		}
	}

	// Create a basic ExtensionService for tests
	extensionService := &contour_v1alpha1.ExtensionService{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "otel-collector",
			Namespace: "tracing",
		},
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: "otel-collector", Port: 4317},
			},
		},
	}

	tests := map[string]struct {
		tracingConfig *contour_v1alpha1.TracingConfig
		objects       []runtime.Object
		want          *xdscache_v3.TracingConfig
		wantErr       string
	}{
		"nil tracing config returns nil": {
			tracingConfig: nil,
			objects:       nil,
			want:          nil,
			wantErr:       "",
		},
		"extension service not found returns error": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "nonexistent",
					Namespace: "tracing",
				},
			},
			objects: nil,
			wantErr: "error getting extension service",
		},
		"basic config with defaults": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 256,
				CustomTags: []*xdscache_v3.CustomTag{
					{TagName: "podName", EnvironmentName: "HOSTNAME"},
					{TagName: "podNamespace", EnvironmentName: "CONTOUR_NAMESPACE"},
				},
			},
		},
		"includePodDetail false omits pod tags": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				IncludePodDetail: ptr.To(false),
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 256,
				CustomTags:       nil,
			},
		},
		"custom service name": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				ServiceName:      ptr.To("my-service"),
				IncludePodDetail: ptr.To(false),
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "my-service",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 256,
				CustomTags:       nil,
			},
		},
		"custom sampling rates": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				OverallSampling:  ptr.To("50.5"),
				ClientSampling:   ptr.To("75.0"),
				RandomSampling:   ptr.To("25.0"),
				IncludePodDetail: ptr.To(false),
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  50.5,
				ClientSampling:   75.0,
				RandomSampling:   25.0,
				MaxPathTagLength: 256,
				CustomTags:       nil,
			},
		},
		"invalid sampling rate defaults to 100": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				OverallSampling:  ptr.To("invalid"),
				ClientSampling:   ptr.To("not-a-number"),
				RandomSampling:   ptr.To(""),
				IncludePodDetail: ptr.To(false),
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 256,
				CustomTags:       nil,
			},
		},
		"zero sampling rate defaults to 100": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				OverallSampling:  ptr.To("0"),
				ClientSampling:   ptr.To("0.0"),
				RandomSampling:   ptr.To("0"),
				IncludePodDetail: ptr.To(false),
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 256,
				CustomTags:       nil,
			},
		},
		"custom max path tag length": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				MaxPathTagLength: ptr.To(uint32(512)),
				IncludePodDetail: ptr.To(false),
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 512,
				CustomTags:       nil,
			},
		},
		"custom tags with literal and request header": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				IncludePodDetail: ptr.To(false),
				CustomTags: []*contour_v1alpha1.CustomTag{
					{
						TagName: "env",
						Literal: "production",
					},
					{
						TagName:           "method",
						RequestHeaderName: ":method",
					},
				},
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 256,
				CustomTags: []*xdscache_v3.CustomTag{
					{TagName: "env", Literal: "production"},
					{TagName: "method", RequestHeaderName: ":method"},
				},
			},
		},
		"custom tags combined with pod detail tags": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				IncludePodDetail: ptr.To(true),
				CustomTags: []*contour_v1alpha1.CustomTag{
					{
						TagName: "env",
						Literal: "staging",
					},
				},
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "contour",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  100.0,
				ClientSampling:   100.0,
				RandomSampling:   100.0,
				MaxPathTagLength: 256,
				CustomTags: []*xdscache_v3.CustomTag{
					{TagName: "podName", EnvironmentName: "HOSTNAME"},
					{TagName: "podNamespace", EnvironmentName: "CONTOUR_NAMESPACE"},
					{TagName: "env", Literal: "staging"},
				},
			},
		},
		"full configuration": {
			tracingConfig: &contour_v1alpha1.TracingConfig{
				ExtensionService: &contour_v1alpha1.NamespacedName{
					Name:      "otel-collector",
					Namespace: "tracing",
				},
				IncludePodDetail: ptr.To(true),
				ServiceName:      ptr.To("my-app"),
				OverallSampling:  ptr.To("10.0"),
				ClientSampling:   ptr.To("20.0"),
				RandomSampling:   ptr.To("30.0"),
				MaxPathTagLength: ptr.To(uint32(128)),
				CustomTags: []*contour_v1alpha1.CustomTag{
					{TagName: "version", Literal: "v1.0.0"},
				},
			},
			objects: []runtime.Object{extensionService},
			want: &xdscache_v3.TracingConfig{
				ServiceName: "my-app",
				ExtensionServiceConfig: xdscache_v3.ExtensionServiceConfig{
					ExtensionService: types.NamespacedName{
						Name:      "otel-collector",
						Namespace: "tracing",
					},
				},
				OverallSampling:  10.0,
				ClientSampling:   20.0,
				RandomSampling:   30.0,
				MaxPathTagLength: 128,
				CustomTags: []*xdscache_v3.CustomTag{
					{TagName: "podName", EnvironmentName: "HOSTNAME"},
					{TagName: "podNamespace", EnvironmentName: "CONTOUR_NAMESPACE"},
					{TagName: "version", Literal: "v1.0.0"},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fakeClient := newFakeClient(tc.objects...)
			server := newServerWithClient(fakeClient)

			got, err := server.setupTracingService(tc.tracingConfig)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
