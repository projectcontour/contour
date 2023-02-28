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

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/ref"
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
		assert.IsType(t, &dag.ListenerProcessor{}, builder.Processors[0])
	}

	t.Run("all default options", func(t *testing.T) {
		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily})
		commonAssertions(t, got)
		assert.Empty(t, got.Source.ConfiguredSecretRefs)
	})

	t.Run("client cert specified", func(t *testing.T) {
		clientCert := &types.NamespacedName{Namespace: "client-ns", Name: "client-name"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, clientCert: clientCert})
		commonAssertions(t, got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{clientCert})
	})

	t.Run("fallback cert specified", func(t *testing.T) {
		fallbackCert := &types.NamespacedName{Namespace: "fallback-ns", Name: "fallback-name"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, fallbackCert: fallbackCert})
		commonAssertions(t, got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{fallbackCert})
	})

	t.Run("client and fallback certs specified", func(t *testing.T) {
		clientCert := &types.NamespacedName{Namespace: "client-ns", Name: "client-name"}
		fallbackCert := &types.NamespacedName{Namespace: "fallback-ns", Name: "fallback-name"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, clientCert: clientCert, fallbackCert: fallbackCert})
		commonAssertions(t, got)
		assert.ElementsMatch(t, got.Source.ConfiguredSecretRefs, []*types.NamespacedName{clientCert, fallbackCert})
	})

	t.Run("request and response headers policy specified", func(t *testing.T) {

		policy := &contour_api_v1alpha1.PolicyConfig{
			RequestHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"req-set-key-1": "req-set-val-1",
					"req-set-key-2": "req-set-val-2",
				},
				Remove: []string{"req-remove-key-1", "req-remove-key-2"},
			},
			ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"res-set-key-1": "res-set-val-1",
					"res-set-key-2": "res-set-val-2",
				},
				Remove: []string{"res-remove-key-1", "res-remove-key-2"},
			},
			ApplyToIngress: ref.To(false),
		}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{rootNamespaces: []string{}, dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily, headersPolicy: policy})
		commonAssertions(t, got)

		httpProxyProcessor := mustGetHTTPProxyProcessor(t, got)
		assert.EqualValues(t, policy.RequestHeadersPolicy.Set, httpProxyProcessor.RequestHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.RequestHeadersPolicy.Remove, httpProxyProcessor.RequestHeadersPolicy.Remove)
		assert.EqualValues(t, policy.ResponseHeadersPolicy.Set, httpProxyProcessor.ResponseHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.ResponseHeadersPolicy.Remove, httpProxyProcessor.ResponseHeadersPolicy.Remove)

		ingressProcessor := mustGetIngressProcessor(t, got)
		assert.EqualValues(t, map[string]string(nil), ingressProcessor.RequestHeadersPolicy.Set)
		assert.ElementsMatch(t, map[string]string(nil), ingressProcessor.RequestHeadersPolicy.Remove)
		assert.EqualValues(t, map[string]string(nil), ingressProcessor.ResponseHeadersPolicy.Set)
		assert.ElementsMatch(t, map[string]string(nil), ingressProcessor.ResponseHeadersPolicy.Remove)
	})

	t.Run("request and response headers policy specified for ingress", func(t *testing.T) {

		policy := &contour_api_v1alpha1.PolicyConfig{
			RequestHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"req-set-key-1": "req-set-val-1",
					"req-set-key-2": "req-set-val-2",
				},
				Remove: []string{"req-remove-key-1", "req-remove-key-2"},
			},
			ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
				Set: map[string]string{
					"res-set-key-1": "res-set-val-1",
					"res-set-key-2": "res-set-val-2",
				},
				Remove: []string{"res-remove-key-1", "res-remove-key-2"},
			},
			ApplyToIngress: ref.To(true),
		}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{
			rootNamespaces:  []string{},
			dnsLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily,
			headersPolicy:   policy,
		})
		commonAssertions(t, got)

		ingressProcessor := mustGetIngressProcessor(t, got)
		assert.EqualValues(t, policy.RequestHeadersPolicy.Set, ingressProcessor.RequestHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.RequestHeadersPolicy.Remove, ingressProcessor.RequestHeadersPolicy.Remove)
		assert.EqualValues(t, policy.ResponseHeadersPolicy.Set, ingressProcessor.ResponseHeadersPolicy.Set)
		assert.ElementsMatch(t, policy.ResponseHeadersPolicy.Remove, ingressProcessor.ResponseHeadersPolicy.Remove)
	})

	t.Run("single ingress class specified", func(t *testing.T) {
		ingressClassNames := []string{"aclass"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{
			rootNamespaces:    []string{},
			dnsLookupFamily:   contour_api_v1alpha1.AutoClusterDNSFamily,
			ingressClassNames: ingressClassNames,
		})
		commonAssertions(t, got)
		assert.EqualValues(t, ingressClassNames, got.Source.IngressClassNames)
	})

	t.Run("multiple comma-separated ingress classes specified", func(t *testing.T) {
		ingressClassNames := []string{"aclass", "bclass", "cclass"}

		serve := &Server{
			log: logrus.StandardLogger(),
		}
		got := serve.getDAGBuilder(dagBuilderConfig{
			rootNamespaces:    []string{},
			dnsLookupFamily:   contour_api_v1alpha1.AutoClusterDNSFamily,
			ingressClassNames: ingressClassNames,
		})
		commonAssertions(t, got)
		assert.EqualValues(t, ingressClassNames, got.Source.IngressClassNames)
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
