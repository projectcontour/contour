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
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
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

// UpstreamTLSContext creates an auth.UpstreamTlsContext. By default
// UpstreamTLSContext returns a HTTP/1.1 TLS enabled context. A list of
// additional ALPN protocols can be provided.
func UpstreamTLSContext(ca []byte, subjectName string, alpnProtocols ...string) *auth.UpstreamTlsContext {
	context := &auth.UpstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			AlpnProtocols: alpnProtocols,
		},
	}

	// we have to do explicitly assign the value from validationContext
	// to context.CommonTlsContext.ValidationContextType because the latter
	// is an interface, returning nil from validationContext directly into
	// this field boxes the nil into the unexported type of this grpc OneOf field
	// which causes proto marshaling to explode later on. Not happy Jan.
	vc := validationContext(ca, subjectName)
	if vc != nil {
		context.CommonTlsContext.ValidationContextType = vc
	}

	return context
}

func validationContext(ca []byte, subjectName string) *auth.CommonTlsContext_ValidationContext {
	if len(ca) < 1 {
		// no ca provided, nothing to do
		return nil
	}

	if len(subjectName) < 1 {
		// no subject name provided, nothing to do
		return nil
	}

	return &auth.CommonTlsContext_ValidationContext{
		ValidationContext: &auth.CertificateValidationContext{
			TrustedCa: &core.DataSource{
				// TODO(dfc) update this for SDS
				Specifier: &core.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
			VerifySubjectAltName: []string{subjectName},
		},
	}
}

// DownstreamTLSContext creates a new DownstreamTlsContext.
func DownstreamTLSContext(secretName string, tlsMinProtoVersion auth.TlsParameters_TlsProtocol, alpnProtos ...string) *auth.DownstreamTlsContext {
	return &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: auth.TlsParameters_TLSv1_3,
				CipherSuites:              ciphers,
			},
			TlsCertificateSdsSecretConfigs: []*auth.SdsSecretConfig{{
				Name:      secretName,
				SdsConfig: ConfigSource("contour"),
			}},
			AlpnProtocols: alpnProtos,
		},
	}
}
