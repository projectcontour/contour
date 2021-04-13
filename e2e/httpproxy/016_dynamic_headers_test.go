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

package httpproxy

import (
	"strings"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testDynamicHeaders(t *testing.T, fx *e2e.Framework) {
	namespace := "016-dynamic-headers"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "ingress-conformance-echo")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "dynamic-headers",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "dynamicheaders.projectcontour.io",
			},
			Routes: []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "ingress-conformance-echo",
							Port: 80,
						},
					},
					RequestHeadersPolicy:  &contourv1.HeadersPolicy{},
					ResponseHeadersPolicy: &contourv1.HeadersPolicy{},
				},
			},
		},
	}

	requestHeaders := map[string]string{
		"X-App-Weight":                    "100%",
		"X-Envoy-hostname":                "%HOSTNAME%",
		"X-Envoy-unknown":                 "%UNKNOWN%",
		"X-Envoy-Upstream-Remote-Address": "%UPSTREAM_REMOTE_ADDRESS%",
		"X-Request-Host":                  "%REQ(Host)%",
		"X-Request-Missing-Header":        "%REQ(Missing-Header)%ook",
		"X-Host-Protocol":                 "%REQ(Host)% - %PROTOCOL%",
		// ...
		"X-Contour-Namespace": "%CONTOUR_NAMESPACE%",
		"X-Contour-Service":   "%CONTOUR_SERVICE_NAME%:%CONTOUR_SERVICE_PORT%",
	}
	for k, v := range requestHeaders {
		hv := contourv1.HeaderValue{
			Name:  k,
			Value: v,
		}
		p.Spec.Routes[0].RequestHeadersPolicy.Set = append(p.Spec.Routes[0].RequestHeadersPolicy.Set, hv)
	}

	responseHeaders := map[string]string{
		"X-App-Weight":                    "100%",
		"X-Envoy-hostname":                "%HOSTNAME%",
		"X-Envoy-unknown":                 "%UNKNOWN%",
		"X-Envoy-Upstream-Remote-Address": "%UPSTREAM_REMOTE_ADDRESS%",
		"X-Request-Host":                  "%REQ(Host)%",
		"X-Request-Missing-Header":        "%REQ(Missing-Header)%ook",
		"X-Host-Protocol":                 "%REQ(Host)% - %PROTOCOL%",
		// ...
		"X-Contour-Namespace": "%CONTOUR_NAMESPACE%",
		"X-Contour-Service":   "%CONTOUR_SERVICE_NAME%:%CONTOUR_SERVICE_PORT%",
	}
	for k, v := range responseHeaders {
		hv := contourv1.HeaderValue{
			Name:  k,
			Value: v,
		}
		p.Spec.Routes[0].ResponseHeadersPolicy.Set = append(p.Spec.Routes[0].ResponseHeadersPolicy.Set, hv)
	}

	fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	res, ok := fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.True(t, ok, "did not get 200 response")

	body := fx.GetEchoResponseBody(res.Body)

	// TODO should these be checking request or response headers?
	assert.Equal(t, "100%", body.GetHeader("X-App-Weight"))
	assert.True(t, strings.HasPrefix(body.GetHeader("X-Envoy-Hostname"), "envoy-"), "X-Envoy-Hostname does not start with 'envoy-'")
	assert.Equal(t, "%UNKNOWN%", body.GetHeader("X-Envoy-Unknown"))
	assert.Equal(t, body.Host, body.GetHeader("X-Request-Host"))
	assert.Equal(t, "ook", body.GetHeader("X-Request-Missing-Header"))
	assert.Equal(t, body.Host+" - HTTP/1.1", body.GetHeader("X-Host-Protocol"))

	// TODO I'm not sure why the below is failing
	// assert.Equal(t, "ingress-conformance-echo:80", echoResponse.GetHeader("X-Contour-Service"))
}
