// Copyright © 2018 Heptio
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

package envoy

import (
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/google/go-cmp/cmp"
)

func TestSocketAddress(t *testing.T) {
	const (
		addr = "foo.example.com"
		port = 8123
	)

	got := SocketAddress(addr, port)
	want := core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  addr,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestDownstreamTLSContext(t *testing.T) {
	const (
		cert = "foo"
		key  = "secret"
	)
	got := DownstreamTLSContext([]byte(cert), []byte(key), auth.TlsParameters_TLSv1_1, "h2", "http/1.1")
	want := &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: auth.TlsParameters_TLSv1_1,
				TlsMaximumProtocolVersion: auth.TlsParameters_TLSv1_3,
			},
			TlsCertificates: []*auth.TlsCertificate{{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(cert),
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(key),
					},
				},
			}},
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}
