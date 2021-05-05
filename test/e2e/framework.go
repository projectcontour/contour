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

// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	// needed if tests are run against GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// Framework provides a collection of helpful functions for
// writing end-to-end (E2E) tests for Contour.
type Framework struct {
	// Client is a controller-runtime Kubernetes client.
	Client client.Client

	// RetryInterval is how often to retry polling operations.
	RetryInterval time.Duration

	// RetryTimeout is how long to continue trying polling
	// operations before giving up.
	RetryTimeout time.Duration

	// Fixtures provides helpers for working with test fixtures,
	// i.e. sample workloads that can be used as proxy targets.
	Fixtures *Fixtures

	// HTTP provides helpers for making HTTP/HTTPS requests.
	HTTP *HTTP

	t *testing.T
}

func NewFramework(t *testing.T) *Framework {
	scheme := runtime.NewScheme()
	require.NoError(t, kubescheme.AddToScheme(scheme))
	require.NoError(t, contourv1.AddToScheme(scheme))
	require.NoError(t, gatewayv1alpha1.AddToScheme(scheme))
	require.NoError(t, certmanagerv1.AddToScheme(scheme))

	crClient, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	require.NoError(t, err)

	httpURLBase := os.Getenv("CONTOUR_E2E_HTTP_URL_BASE")
	if httpURLBase == "" {
		httpURLBase = "http://127.0.0.1:9080"
	}

	httpsURLBase := os.Getenv("CONTOUR_E2E_HTTPS_URL_BASE")
	if httpsURLBase == "" {
		httpsURLBase = "https://127.0.0.1:9443"
	}

	return &Framework{
		Client:        crClient,
		RetryInterval: time.Second,
		RetryTimeout:  60 * time.Second,
		Fixtures: &Fixtures{
			Echo: &Echo{
				client: crClient,
				t:      t,
			},
		},
		HTTP: &HTTP{
			HTTPURLBase:   httpURLBase,
			HTTPSURLBase:  httpsURLBase,
			RetryInterval: time.Second,
			RetryTimeout:  60 * time.Second,
			t:             t,
		},
		t: t,
	}
}

// RunParallel runs the provided set of subtests in parallel and blocks
// until they're all done running.
func (f *Framework) RunParallel(name string, subtests map[string]func(t *testing.T, f *Framework)) {
	f.t.Run(name, func(t *testing.T) {
		for name, tc := range subtests {
			tc := tc
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				tc(t, f)
			})
		}
	})
}

// CreateHTTPProxyAndWaitFor creates the provided HTTPProxy in the Kubernetes API
// and then waits for the specified condition to be true.
func (f *Framework) CreateHTTPProxyAndWaitFor(proxy *contourv1.HTTPProxy, condition func(*contourv1.HTTPProxy) bool) (*contourv1.HTTPProxy, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), proxy))

	res := &contourv1.HTTPProxy{}

	if err := wait.PollImmediate(f.RetryInterval, f.RetryTimeout, func() (bool, error) {
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(proxy), res); err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}

		return condition(res), nil
	}); err != nil {
		// return the last response for logging/debugging purposes
		return res, false
	}

	return res, true
}

// CreateHTTPRouteAndWaitFor creates the provided HTTPRoute in the Kubernetes API
// and then waits for the specified condition to be true.
func (f *Framework) CreateHTTPRouteAndWaitFor(route *gatewayv1alpha1.HTTPRoute, condition func(*gatewayv1alpha1.HTTPRoute) bool) (*gatewayv1alpha1.HTTPRoute, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), route))

	res := &gatewayv1alpha1.HTTPRoute{}

	if err := wait.PollImmediate(f.RetryInterval, f.RetryTimeout, func() (bool, error) {
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(route), res); err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}

		return condition(res), nil
	}); err != nil {
		// return the last response for logging/debugging purposes
		return res, false
	}

	return res, true
}

// CreateNamespace creates a namespace with the given name in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) CreateNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"contour-e2e-ns": "true"},
		},
	}
	require.NoError(f.t, f.Client.Create(context.TODO(), ns))
}

// DeleteNamespace deletes the namespace with the given name in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) DeleteNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	require.NoError(f.t, f.Client.Delete(context.TODO(), ns))
}

// CreateSelfSignedCert creates a self-signed Issuer if it doesn't already exist
// and uses it to create a self-signed Certificate. It returns a cleanup function.
func (f *Framework) CreateSelfSignedCert(ns, name, secretName, dnsName string) func() {
	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "selfsigned",
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}

	if err := f.Client.Create(context.TODO(), issuer); err != nil && !errors.IsAlreadyExists(err) {
		require.FailNow(f.t, "error creating Issuer: %v", err)
	}

	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: certmanagerv1.CertificateSpec{
			DNSNames:   []string{dnsName},
			SecretName: secretName,
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: "selfsigned",
			},
		},
	}
	require.NoError(f.t, f.Client.Create(context.TODO(), cert))

	return func() {
		require.NoError(f.t, f.Client.Delete(context.TODO(), cert))
		require.NoError(f.t, f.Client.Delete(context.TODO(), issuer))
	}
}

// GetEchoResponseBody decodes an HTTP response body that is
// expected to have come from ingress-conformance-echo into an
// EchoResponseBody, or fails the test if it encounters an error.
func (f *Framework) GetEchoResponseBody(body []byte) EchoResponseBody {
	var echoBody EchoResponseBody

	require.NoError(f.t, json.Unmarshal(body, &echoBody))

	return echoBody
}

type EchoResponseBody struct {
	Path      string      `json:"path"`
	Host      string      `json:"host"`
	Headers   http.Header `json:"headers"`
	Namespace string      `json:"namespace"`
	Ingress   string      `json:"ingress"`
	Service   string      `json:"service"`
	Pod       string      `json:"pod"`
}

func (erb *EchoResponseBody) GetHeader(name string) string {
	return strings.Join(erb.Headers[name], ",")
}
