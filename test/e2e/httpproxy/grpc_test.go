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
// +build e2e

package httpproxy

import (
	"context"
	"strings"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/mhausenblas/yages/yages"
	. "github.com/onsi/ginkgo/v2"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func testGRPCServicePlaintext(namespace string) {
	Specify("requests to a gRPC service configured with plaintext work as expected", func() {
		t := f.T()

		f.Fixtures.GRPC.Deploy(namespace, "grpc-echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "grpc-echo-plaintext",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "grpc-echo-plaintext.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name:     "grpc-echo",
								Port:     9000,
								Protocol: pointer.String("h2c"),
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

		grpcAddr := strings.TrimPrefix(f.HTTP.HTTPURLBase, "http://")

		dialCtx, dialCancel := context.WithTimeout(context.Background(), time.Second*30)
		defer dialCancel()
		retryOpts := []grpc_retry.CallOption{
			// Retry if Envoy returns unavailable, the upstream
			// may not be healthy yet.
			grpc_retry.WithCodes(codes.Unavailable),
			grpc_retry.WithMax(20),
		}
		conn, err := grpc.DialContext(dialCtx, grpcAddr,
			grpc.WithBlock(),
			grpc.WithAuthority(p.Spec.VirtualHost.Fqdn),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(retryOpts...)),
		)
		require.NoError(t, err)
		defer conn.Close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		client := yages.NewEchoClient(conn)
		resp, err := client.Ping(ctx, &yages.Empty{},
			grpc.WaitForReady(true),
		)
		require.NoErrorf(t, err, "gRPC error code %d", status.Code(err))
		require.Equal(t, "pong", resp.Text)
	})
}
