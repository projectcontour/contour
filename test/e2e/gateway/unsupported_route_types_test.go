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

package gateway

import (
	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/internal/gatewayapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func testTCPRouteIsRejected(namespace string) {
	Specify("TCPRoutes are rejected", func() {
		route := &gatewayapi_v1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "tcproute-1",
			},
			Spec: gatewayapi_v1alpha2.TCPRouteSpec{
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentRef{
						gatewayapi.GatewayParentRef("", "http"),
					},
				},
				Rules: []gatewayapi_v1alpha2.TCPRouteRule{
					{
						BackendRefs: []gatewayapi_v1alpha2.BackendRef{
							{
								BackendObjectReference: gatewayapi.ServiceBackendObjectRef("foo", 80),
							},
						},
					},
				},
			},
		}
		f.CreateTCPPRouteAndWaitFor(route, tcpRouteRejected)
	})
}

func testUDPRouteIsRejected(namespace string) {
	Specify("UDPRoutes are rejected", func() {
		route := &gatewayapi_v1alpha2.UDPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "udproute-1",
			},
			Spec: gatewayapi_v1alpha2.UDPRouteSpec{
				CommonRouteSpec: gatewayapi_v1alpha2.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1alpha2.ParentRef{
						gatewayapi.GatewayParentRef("", "http"),
					},
				},
				Rules: []gatewayapi_v1alpha2.UDPRouteRule{
					{
						BackendRefs: []gatewayapi_v1alpha2.BackendRef{
							{
								BackendObjectReference: gatewayapi.ServiceBackendObjectRef("foo", 80),
							},
						},
					},
				},
			},
		}
		f.CreateUDPPRouteAndWaitFor(route, udpRouteRejected)
	})
}
