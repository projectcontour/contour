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

package e2e

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StartLocalHTTPService starts a local HTTP server on the test host and registers
// it in the cluster under the given namespace and name as a headless Service and
// EndpointSlice.
//
// CONTOUR_E2E_LOCAL_HOST must be set to a host IP reachable from within the
// cluster. The test is skipped if the variable is absent.
//
// Returns a handler setter that is safe for concurrent use from the test
// goroutine.
func StartLocalHTTPService(t ginkgo.GinkgoTInterface, c client.Client, ns, name string) func(http.HandlerFunc) {
	hostIP := os.Getenv("CONTOUR_E2E_LOCAL_HOST")
	if hostIP == "" {
		ginkgo.Skip("CONTOUR_E2E_LOCAL_HOST must be set to a host IP reachable from within the cluster")
	}

	var handler atomic.Pointer[http.HandlerFunc]

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("LocalHTTPService %s/%s received request: Method=%s, Path=%s, Headers=%v",
			ns, name, r.Method, r.URL.Path, r.Header)
		(*handler.Load())(w, r)
	}))

	listener, err := net.Listen("tcp", net.JoinHostPort(hostIP, "0"))
	require.NoError(t, err)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	port := listener.Addr().(*net.TCPAddr).Port

	require.NoError(t, c.Create(context.TODO(), &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{Name: name, Namespace: ns},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{{Name: "http", Protocol: core_v1.ProtocolTCP, Port: 80}},
		},
	}))
	require.NoError(t, c.Create(context.TODO(), &discovery_v1.EndpointSlice{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels: map[string]string{discovery_v1.LabelServiceName: name},
		},
		AddressType: discovery_v1.AddressTypeIPv4,
		Endpoints: []discovery_v1.Endpoint{
			{Addresses: []string{hostIP}, Conditions: discovery_v1.EndpointConditions{Ready: ptr.To(true)}},
		},
		Ports: []discovery_v1.EndpointPort{
			{Name: ptr.To("http"), Port: ptr.To(int32(port)), Protocol: ptr.To(core_v1.ProtocolTCP)}, //nolint:gosec
		},
	}))

	return func(h http.HandlerFunc) { handler.Store(&h) }
}
