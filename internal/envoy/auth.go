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

package envoy

import (
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
)

var (
	// This is the list of default ciphers used by contour 1.9.1. A handful are
	// commented out, as they're arguably less secure. They're also unnecessary
	// - most of the clients that might need to use the commented ciphers are
	// unable to connect without TLS 1.0, which contour never enables.
	//
	// This list is ignored if the client and server negotiate TLS 1.3.
	//
	// The commented ciphers are left in place to simplify updating this list for future
	// versions of envoy.
	ciphers = []string{
		"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
		"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
		"ECDHE-ECDSA-AES128-SHA",
		"ECDHE-RSA-AES128-SHA",
		//"AES128-GCM-SHA256",
		//"AES128-SHA",
		"ECDHE-ECDSA-AES256-GCM-SHA384",
		"ECDHE-RSA-AES256-GCM-SHA384",
		"ECDHE-ECDSA-AES256-SHA",
		"ECDHE-RSA-AES256-SHA",
		//"AES256-GCM-SHA384",
		//"AES256-SHA",
	}
)

// UpstreamTLSContext creates an envoy_api_v2_auth.UpstreamTlsContext. By default
// UpstreamTLSContext returns a HTTP/1.1 TLS enabled context. A list of
// additional ALPN protocols can be provided.
func UpstreamTLSContext(peerValidationContext *dag.PeerValidationContext, sni string, alpnProtocols ...string) *envoy_api_v2_auth.UpstreamTlsContext {
	context := &envoy_api_v2_auth.UpstreamTlsContext{
		CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
			AlpnProtocols: alpnProtocols,
		},
		Sni: sni,
	}

	if peerValidationContext.GetCACertificate() != nil && len(peerValidationContext.GetSubjectName()) > 0 {
		// We have to explicitly assign the value from validationContext
		// to context.CommonTlsContext.ValidationContextType because the
		// latter is an interface. Returning nil from validationContext
		// directly into this field boxes the nil into the unexported
		// type of this grpc OneOf field which causes proto marshaling
		// to explode later on.
		vc := validationContext(peerValidationContext.GetCACertificate(), peerValidationContext.GetSubjectName())
		if vc != nil {
			context.CommonTlsContext.ValidationContextType = vc
		}
	}

	return context
}

func validationContext(ca []byte, subjectName string) *envoy_api_v2_auth.CommonTlsContext_ValidationContext {
	vc := &envoy_api_v2_auth.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_api_v2_auth.CertificateValidationContext{
			TrustedCa: &envoy_api_v2_core.DataSource{
				// TODO(dfc) update this for SDS
				Specifier: &envoy_api_v2_core.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
		},
	}

	if len(subjectName) > 0 {
		vc.ValidationContext.MatchSubjectAltNames = []*matcher.StringMatcher{{
			MatchPattern: &matcher.StringMatcher_Exact{
				Exact: subjectName,
			}},
		}
	}

	return vc
}

// DownstreamTLSContext creates a new DownstreamTlsContext.
func DownstreamTLSContext(serverSecret *dag.Secret, tlsMinProtoVersion envoy_api_v2_auth.TlsParameters_TlsProtocol, peerValidationContext *dag.PeerValidationContext, alpnProtos ...string) *envoy_api_v2_auth.DownstreamTlsContext {
	context := &envoy_api_v2_auth.DownstreamTlsContext{
		CommonTlsContext: &envoy_api_v2_auth.CommonTlsContext{
			TlsParams: &envoy_api_v2_auth.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: envoy_api_v2_auth.TlsParameters_TLSv1_3,
				CipherSuites:              ciphers,
			},
			TlsCertificateSdsSecretConfigs: []*envoy_api_v2_auth.SdsSecretConfig{{
				Name:      Secretname(serverSecret),
				SdsConfig: ConfigSource("contour"),
			}},
			AlpnProtocols: alpnProtos,
		},
	}

	if peerValidationContext.GetCACertificate() != nil {
		vc := validationContext(peerValidationContext.GetCACertificate(), "")
		if vc != nil {
			context.CommonTlsContext.ValidationContextType = vc
			context.RequireClientCertificate = protobuf.Bool(true)
		}
	}

	return context
}
