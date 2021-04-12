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
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// Framework provides a collection of helpful functions for
// writing end-to-end (E2E) tests for Contour.
type Framework struct {
	Client client.Client

	t             *testing.T
	httpUrlBase   string
	httpsUrlBase  string
	retryInterval time.Duration
	retryTimeout  time.Duration
}

func NewFramework(t *testing.T) *Framework {
	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	contourv1.AddToScheme(scheme)
	gatewayv1alpha1.AddToScheme(scheme)
	certmanagerv1.AddToScheme(scheme)

	crClient, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	require.NoError(t, err)

	return &Framework{
		Client: crClient,

		t:             t,
		httpUrlBase:   "http://127.0.0.1:9080",
		httpsUrlBase:  "https://127.0.0.1:9443",
		retryInterval: time.Second,
		retryTimeout:  30 * time.Second,
	}
}

// CreateHTTPProxyAndWaitFor creates the provided HTTPProxy in the Kubernetes API
// and then waits for the specified condition to be true.
func (f *Framework) CreateHTTPProxyAndWaitFor(proxy *contourv1.HTTPProxy, condition func(*contourv1.HTTPProxy) bool) (*contourv1.HTTPProxy, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), proxy))

	ticker := time.NewTicker(f.retryInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(f.retryTimeout)
	defer timeout.Stop()

	res := &contourv1.HTTPProxy{}
	for {
		select {
		case <-ticker.C:
			err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(proxy), res)
			if err == nil && condition(res) {
				return res, true
			}
		case <-timeout.C:
			// return the last response for logging/debugging purposes
			return res, false
		}
	}
}

// HTTPProxyValid returns true if the proxy has a .status.currentStatus
// of "valid".
func HTTPProxyValid(proxy *contourv1.HTTPProxy) bool {
	return proxy != nil && proxy.Status.CurrentStatus == "valid"
}

// CreateEchoWorkload creates the ingress-conformance-echo fixture, specifically
// the deployment and service, in the given namespace and with the given name, or
// fails the test if it encounters an error. Namespace is defaulted to "default"
// and name is defaulted to "ingress-conformance-echo" if not provided.
func (f *Framework) CreateEchoWorkload(ns, name string) {
	valOrDefault := func(val, defaultVal string) string {
		if val != "" {
			return val
		}
		return defaultVal
	}

	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "conformance-echo",
							Image: "k8s.gcr.io/ingressconformance/echoserver:v0.0.1",
							Env: []corev1.EnvVar{
								{
									Name:  "INGRESS_NAME",
									Value: name,
								},
								{
									Name:  "SERVICE_NAME",
									Value: name,
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-api",
									ContainerPort: 3000,
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(3000),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(f.t, f.Client.Create(context.TODO(), deployment))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http-api"),
				},
			},
			Selector: map[string]string{"app.kubernetes.io/name": name},
		},
	}
	require.NoError(f.t, f.Client.Create(context.TODO(), service))
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

// HTTPRequestUntil repeatedly makes HTTP requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (f *Framework) HTTPRequestUntil(condition func(*http.Response) bool, url, host string, opts ...func(*http.Request)) (*http.Response, bool) {
	makeRequest := func() (*http.Response, error) {
		req, err := http.NewRequest("GET", f.httpUrlBase+url, nil)
		require.NoError(f.t, err, "error creating HTTP request")

		req.Host = host
		for _, opt := range opts {
			opt(req)
		}

		return http.DefaultClient.Do(req)
	}

	return f.requestUntil(makeRequest, condition)
}

// HTTPSRequestUntil repeatedly makes HTTPS requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (f *Framework) HTTPSRequestUntil(condition func(*http.Response) bool, url, host string, opts ...func(*http.Request)) (*http.Response, bool) {
	makeRequest := func() (*http.Response, error) {
		req, err := http.NewRequest("GET", f.httpsUrlBase+url, nil)
		require.NoError(f.t, err, "error creating HTTP request")

		req.Host = host
		for _, opt := range opts {
			opt(req)
		}

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}

		client := &http.Client{
			Transport: transport,
		}

		return client.Do(req)
	}

	return f.requestUntil(makeRequest, condition)
}

func (f *Framework) requestUntil(makeRequest func() (*http.Response, error), condition func(*http.Response) bool) (*http.Response, bool) {
	// make an immediate request and return if it succeeds
	if res, err := makeRequest(); err == nil && condition(res) {
		return res, true
	}

	// otherwise, enter a retry loop
	ticker := time.NewTicker(f.retryInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(f.retryTimeout)
	defer timeout.Stop()

	var res *http.Response
	var err error
	for {
		select {
		case <-ticker.C:
			res, err = makeRequest()
			if err == nil && condition(res) {
				return res, true
			}
		case <-timeout.C:
			// return the last response for logging/debugging purposes
			return res, false
		}
	}
}

// IsOK returns true if the response has a 200
// status code, or false otherwise.
func IsOK(res *http.Response) bool {
	return HasStatusCode(200)(res)
}

// HasStatusCode returns a function that returns true
// if the response has the specified status code, or
// false otherwise.
func HasStatusCode(code int) func(*http.Response) bool {
	return func(res *http.Response) bool {
		return res != nil && res.StatusCode == code
	}
}

// GetEchoResponseBody decodes an HTTP response body that is
// expected to have come from ingress-conformance-echo into an
// EchoResponseBody, or fails the test if it encounters an error.
func (f *Framework) GetEchoResponseBody(body io.Reader) EchoResponseBody {
	var echoBody EchoResponseBody
	require.NoError(f.t, json.NewDecoder(body).Decode(&echoBody))

	return echoBody
}

type EchoResponseBody struct {
	Path      string              `json:"path"`
	Host      string              `json:"host"`
	Headers   map[string][]string `json:"headers"`
	Namespace string              `json:"namespace"`
	Ingress   string              `json:"ingress"`
	Service   string              `json:"service"`
	Pod       string              `json:"pod"`
}

func (erb *EchoResponseBody) GetHeader(name string) string {
	return strings.Join(erb.Headers[name], ",")
}
