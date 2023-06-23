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

package v3

import (
	"testing"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpstreamTLSTransportSocket(t *testing.T) {
	cg := NewConfigGenerator()
	context := cg.UpstreamTLSContext(nil, "", nil, "h2")
	want := &envoy_core_v3.TransportSocket{
		Name: "envoy.transport_sockets.tls",
		ConfigType: &envoy_core_v3.TransportSocket_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(context),
		},
	}
	got := UpstreamTLSTransportSocket(context)
	protobuf.ExpectEqual(t, want, got)
}

func TestDownstreamTLSTransportSocket(t *testing.T) {
	serverSecret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-cert",
				Namespace: "default",
			},
			Data: map[string][]byte{
				v1.TLSCertKey:       []byte("cert"),
				v1.TLSPrivateKeyKey: []byte("key"),
			},
		},
	}
	cg := NewConfigGenerator()
	context := cg.DownstreamTLSContext(serverSecret, envoy_tls_v3.TlsParameters_TLSv1_2, nil, nil, "client-subject-name", "h2", "http/1.1")
	want := &envoy_core_v3.TransportSocket{
		Name: "envoy.transport_sockets.tls",
		ConfigType: &envoy_core_v3.TransportSocket_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(context),
		},
	}
	got := DownstreamTLSTransportSocket(context)
	protobuf.ExpectEqual(t, want, got)
}
