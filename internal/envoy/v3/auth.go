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
	envoy_api_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_v3_tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// UpstreamTLSContext creates an envoy_v3_tls.UpstreamTlsContext. By default
// UpstreamTLSContext returns a HTTP/1.1 TLS enabled context. A list of
// additional ALPN protocols can be provided.
func UpstreamTLSContext(peerValidationContext *dag.PeerValidationContext, sni string, clientSecret *dag.Secret, upstreamTLS *dag.UpstreamTLS, alpnProtocols ...string) *envoy_v3_tls.UpstreamTlsContext {
	var clientSecretConfigs []*envoy_v3_tls.SdsSecretConfig
	if clientSecret != nil {
		clientSecretConfigs = []*envoy_v3_tls.SdsSecretConfig{{
			Name:      envoy.Secretname(clientSecret),
			SdsConfig: ConfigSource("contour"),
		}}
	}

	context := &envoy_v3_tls.UpstreamTlsContext{
		CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
			AlpnProtocols:                  alpnProtocols,
			TlsCertificateSdsSecretConfigs: clientSecretConfigs,
		},
		Sni: sni,
	}

	if upstreamTLS != nil {
		context.CommonTlsContext.TlsParams = &envoy_v3_tls.TlsParameters{
			TlsMinimumProtocolVersion: ParseTLSVersion(upstreamTLS.MinimumProtocolVersion),
			TlsMaximumProtocolVersion: ParseTLSVersion(upstreamTLS.MaximumProtocolVersion),
			CipherSuites:              upstreamTLS.CipherSuites,
		}
	}

	if peerValidationContext.GetCACertificate() != nil && len(peerValidationContext.GetSubjectNames()) > 0 {
		// We have to explicitly assign the value from validationContext
		// to context.CommonTlsContext.ValidationContextType because the
		// latter is an interface. Returning nil from validationContext
		// directly into this field boxes the nil into the unexported
		// type of this grpc OneOf field which causes proto marshaling
		// to explode later on.
		vc := validationContext(peerValidationContext.GetCACertificate(), peerValidationContext.GetSubjectNames(), false, nil, false)
		if vc != nil {
			// TODO: update this for SDS (CommonTlsContext_ValidationContextSdsSecretConfig) instead of inlining it.
			context.CommonTlsContext.ValidationContextType = vc
		}
	}

	return context
}

// TODO: update this for SDS (CommonTlsContext_ValidationContextSdsSecretConfig) instead of inlining it.
func validationContext(ca []byte, subjectNames []string, skipVerifyPeerCert bool, crl []byte, onlyVerifyLeafCertCrl bool) *envoy_v3_tls.CommonTlsContext_ValidationContext {
	vc := &envoy_v3_tls.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_v3_tls.CertificateValidationContext{
			TrustChainVerification: envoy_v3_tls.CertificateValidationContext_VERIFY_TRUST_CHAIN,
		},
	}

	if skipVerifyPeerCert {
		vc.ValidationContext.TrustChainVerification = envoy_v3_tls.CertificateValidationContext_ACCEPT_UNTRUSTED
	}

	if len(ca) > 0 {
		vc.ValidationContext.TrustedCa = &envoy_api_v3_core.DataSource{
			Specifier: &envoy_api_v3_core.DataSource_InlineBytes{
				InlineBytes: ca,
			},
		}
	}

	for _, san := range subjectNames {
		vc.ValidationContext.MatchTypedSubjectAltNames = append(
			vc.ValidationContext.MatchTypedSubjectAltNames,
			&envoy_v3_tls.SubjectAltNameMatcher{
				SanType: envoy_v3_tls.SubjectAltNameMatcher_DNS,
				Matcher: &matcher.StringMatcher{
					MatchPattern: &matcher.StringMatcher_Exact{
						Exact: san,
					},
				},
			},
		)
	}

	if len(crl) > 0 {
		vc.ValidationContext.Crl = &envoy_api_v3_core.DataSource{
			Specifier: &envoy_api_v3_core.DataSource_InlineBytes{
				InlineBytes: crl,
			},
		}
		vc.ValidationContext.OnlyVerifyLeafCertCrl = onlyVerifyLeafCertCrl
	}

	return vc
}

// DownstreamTLSContext creates a new DownstreamTlsContext.
func DownstreamTLSContext(serverSecret *dag.Secret, tlsMinProtoVersion, tlsMaxProtoVersion envoy_v3_tls.TlsParameters_TlsProtocol, cipherSuites []string, peerValidationContext *dag.PeerValidationContext, alpnProtos ...string) *envoy_v3_tls.DownstreamTlsContext {
	context := &envoy_v3_tls.DownstreamTlsContext{
		CommonTlsContext: &envoy_v3_tls.CommonTlsContext{
			TlsParams: &envoy_v3_tls.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: tlsMaxProtoVersion,
				CipherSuites:              cipherSuites,
			},
			TlsCertificateSdsSecretConfigs: []*envoy_v3_tls.SdsSecretConfig{{
				Name:      envoy.Secretname(serverSecret),
				SdsConfig: ConfigSource("contour"),
			}},
			AlpnProtocols: alpnProtos,
		},
	}
	if peerValidationContext != nil {
		vc := validationContext(peerValidationContext.GetCACertificate(), []string{}, peerValidationContext.SkipClientCertValidation,
			peerValidationContext.GetCRL(), peerValidationContext.OnlyVerifyLeafCertCrl)
		if vc != nil {
			context.CommonTlsContext.ValidationContextType = vc
			context.RequireClientCertificate = wrapperspb.Bool(!peerValidationContext.OptionalClientCertificate)
		}
	}

	return context
}
