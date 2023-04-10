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

//go:build e2e

package httpproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"net/http"
	"strings"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	. "github.com/onsi/ginkgo/v2"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/projectcontour/yages/yages"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testGRPCServicePlaintext(namespace string) {
	Specify("requests to a gRPC service configured with plaintext work as expected", func() {
		t := f.T()

		f.Fixtures.GRPC.Deploy(namespace, "grpc-echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", "echo", "grpc-echo-plaintext.projectcontour.io")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "grpc-echo-plaintext",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "grpc-echo-plaintext.projectcontour.io",
					TLS: &contourv1.TLS{
						SecretName: "echo",
					},
				},
				Routes: []contourv1.Route{
					{
						// So we can make TLS and non-TLs requests.
						PermitInsecure: true,
						Services: []contourv1.Service{
							{
								Name:     "grpc-echo",
								Port:     9000,
								Protocol: ref.To("h2c"),
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/yages.Echo/Ping",
							},
						},
					},
				},
			},
		}
		_, ok := f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)
		require.True(t, ok)

		insecureAddr := strings.TrimPrefix(f.HTTP.HTTPURLBase, "http://")
		secureAddr := strings.TrimPrefix(f.HTTP.HTTPSURLBase, "https://")

		for addr, transportCreds := range map[string]credentials.TransportCredentials{
			insecureAddr: insecure.NewCredentials(),
			secureAddr: credentials.NewTLS(&tls.Config{
				//nolint:gosec
				InsecureSkipVerify: true,
			}),
		} {
			dialCtx, dialCancel := context.WithTimeout(context.Background(), time.Second*30)
			defer dialCancel()
			retryOpts := []grpc_retry.CallOption{
				// Retry if Envoy returns unavailable, the upstream
				// may not be healthy yet.
				// Also retry if we get the unimplemented status, see:
				// https://github.com/projectcontour/contour/issues/4707
				// Also retry unauthenticated to accommodate eventual consistency
				// after a global ExtAuth test.
				grpc_retry.WithCodes(codes.Unavailable, codes.Unimplemented, codes.Unauthenticated),
				grpc_retry.WithBackoff(grpc_retry.BackoffExponential(time.Millisecond * 10)),
				grpc_retry.WithMax(20),
			}
			conn, err := grpc.DialContext(dialCtx, addr,
				grpc.WithBlock(),
				grpc.WithAuthority(p.Spec.VirtualHost.Fqdn),
				grpc.WithTransportCredentials(transportCreds),
				grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(retryOpts...)),
			)
			require.NoError(t, err)
			defer conn.Close()

			// Give significant leeway for retries to complete with exponential backoff.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
			defer cancel()
			client := yages.NewEchoClient(conn)
			resp, err := client.Ping(ctx, &yages.Empty{})

			require.NoErrorf(t, err, "gRPC error code %d", status.Code(err))
			require.Equal(t, "pong", resp.Text)
		}
	})
}

func testGRPCWeb(namespace string) {
	Specify("grpc-Web HTTP requests to a gRPC service work as expected", func() {
		t := f.T()

		f.Fixtures.GRPC.Deploy(namespace, "grpc-echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo", "echo", "grpc-web.projectcontour.io")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "grpc-web",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "grpc-web.projectcontour.io",
					TLS: &contourv1.TLS{
						SecretName: "echo",
					},
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name:     "grpc-echo",
								Port:     9000,
								Protocol: ref.To("h2c"),
							},
						},
					},
				},
			},
		}
		_, ok := f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)
		require.True(t, ok)

		// One byte marker that this is a data frame, and 4 bytes
		// for the length (we can use 0 since the yages.Empty message
		// is actually empty and has no fields).
		// See: https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md
		bodyData := []byte{0x0, 0x0, 0x0, 0x0, 0x0}

		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host: p.Spec.VirtualHost.Fqdn,
			Path: "/yages.Echo/Ping",
			Body: bytes.NewReader(bodyData),
			RequestOpts: []func(*http.Request){
				func(req *http.Request) {
					req.Method = http.MethodPost
				},
				e2e.OptSetHeaders(map[string]string{
					"Content-Type": "application/grpc-web+proto",
					"Accept":       "application/grpc-web+proto",
					"X-Grpc-Web":   "1",
				}),
			},
			Condition: func(res *e2e.HTTPResponse) bool {
				if e2e.HasStatusCode(http.StatusOK)(res) {
					resp := parseGRPCWebResponse(res.Body)
					return resp.content != nil && resp.content.Text == "pong" &&
						resp.trailers["grpc-status"] == "0"
				}
				return false
			},
		})
		require.NotNil(t, res, "request never succeeded")
		require.Truef(t, ok, "expected 200 response code, gRPC status OK, and response data %s, got HTTP status %d and raw body: %q", "pong", res.StatusCode, string(res.Body))
	})
}

type grpcWebResponse struct {
	content  *yages.Content
	trailers map[string]string
}

func parseGRPCWebResponse(body []byte) grpcWebResponse {
	defer GinkgoRecover()
	t := f.T()

	response := grpcWebResponse{
		trailers: make(map[string]string),
	}

	// Should have at least the data frame marker and 4 bytes for size.
	require.Greater(t, len(body), 5)
	currentPos := 0
	// Data frame marker.
	require.Equal(t, uint8(0x0), body[currentPos])
	currentPos++

	// Get data frame length.
	dataLen := 0
	for _, b := range body[currentPos : currentPos+4] {
		dataLen = dataLen<<8 + int(b)
	}
	currentPos += 4

	if dataLen > 0 {
		require.Greater(t, len(body), currentPos+dataLen)

		content := new(yages.Content)
		require.NoError(t, proto.Unmarshal(body[currentPos:currentPos+dataLen], content))
		response.content = content
		currentPos += dataLen
	}

	// Should have at least the trailers frame marker and 4 bytes for size.
	require.GreaterOrEqual(t, len(body), currentPos+5)
	// Trailers frame marker.
	require.Equal(t, uint8(0x80), body[currentPos])
	currentPos++

	// Get trailers frame length.
	trailersLen := 0
	for _, b := range body[currentPos : currentPos+4] {
		trailersLen = trailersLen<<8 + int(b)
	}
	currentPos += 4

	if trailersLen > 0 {
		require.Equal(t, len(body), currentPos+trailersLen)

		trailersKV := strings.Split(strings.TrimSpace(string(body[currentPos:])), "\r\n")
		for _, kv := range trailersKV {
			k, v, found := strings.Cut(kv, ":")
			require.True(t, found)
			response.trailers[k] = v
		}
	}

	return response
}
