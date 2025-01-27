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
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
)

// UpstreamTLSContext creates an envoy_transport_socket_tls_v3.UpstreamTlsContext. By default
// UpstreamTLSContext returns a HTTP/1.1 TLS enabled context. A list of
// additional ALPN protocols can be provided.
func (e *EnvoyGen) UpstreamTLSContext(peerValidationContext *dag.PeerValidationContext, sni string, clientSecret *dag.Secret, upstreamTLS *dag.UpstreamTLS, alpnProtocols ...string) *envoy_transport_socket_tls_v3.UpstreamTlsContext {
	var clientSecretConfigs []*envoy_transport_socket_tls_v3.SdsSecretConfig
	if clientSecret != nil {
		clientSecretConfigs = []*envoy_transport_socket_tls_v3.SdsSecretConfig{{
			Name:      envoy.Secretname(clientSecret),
			SdsConfig: e.GetConfigSource(),
		}}
	}

	context := &envoy_transport_socket_tls_v3.UpstreamTlsContext{
		CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
			AlpnProtocols:                  alpnProtocols,
			TlsCertificateSdsSecretConfigs: clientSecretConfigs,
		},
		Sni: sni,
	}

	if upstreamTLS != nil {
		context.CommonTlsContext.TlsParams = &envoy_transport_socket_tls_v3.TlsParameters{
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
func validationContext(ca []byte, subjectNames []string, skipVerifyPeerCert bool, crl []byte, onlyVerifyLeafCertCrl bool) *envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext {
	vc := &envoy_transport_socket_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_transport_socket_tls_v3.CertificateValidationContext{
			TrustChainVerification: envoy_transport_socket_tls_v3.CertificateValidationContext_VERIFY_TRUST_CHAIN,
		},
	}

	if skipVerifyPeerCert {
		vc.ValidationContext.TrustChainVerification = envoy_transport_socket_tls_v3.CertificateValidationContext_ACCEPT_UNTRUSTED
	}

	if len(ca) > 0 {
		vc.ValidationContext.TrustedCa = &envoy_config_core_v3.DataSource{
			Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
				InlineBytes: ca,
			},
		}
	}

	for _, san := range subjectNames {
		vc.ValidationContext.MatchTypedSubjectAltNames = append(
			vc.ValidationContext.MatchTypedSubjectAltNames,
			&envoy_transport_socket_tls_v3.SubjectAltNameMatcher{
				SanType: envoy_transport_socket_tls_v3.SubjectAltNameMatcher_DNS,
				Matcher: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
						Exact: san,
					},
				},
			},
		)
	}

	if len(crl) > 0 {
		vc.ValidationContext.Crl = &envoy_config_core_v3.DataSource{
			Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
				InlineBytes: crl,
			},
		}
		vc.ValidationContext.OnlyVerifyLeafCertCrl = onlyVerifyLeafCertCrl
	}

	return vc
}

// DownstreamTLSContext creates a new DownstreamTlsContext.
func (e *EnvoyGen) DownstreamTLSContext(serverSecret *dag.Secret, tlsMinProtoVersion, tlsMaxProtoVersion envoy_transport_socket_tls_v3.TlsParameters_TlsProtocol, cipherSuites []string, peerValidationContext *dag.PeerValidationContext, alpnProtos ...string) *envoy_transport_socket_tls_v3.DownstreamTlsContext {
	context := &envoy_transport_socket_tls_v3.DownstreamTlsContext{
		CommonTlsContext: &envoy_transport_socket_tls_v3.CommonTlsContext{
			TlsParams: &envoy_transport_socket_tls_v3.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: tlsMaxProtoVersion,
				CipherSuites:              cipherSuites,
			},
			TlsCertificateSdsSecretConfigs: []*envoy_transport_socket_tls_v3.SdsSecretConfig{{
				Name:      envoy.Secretname(serverSecret),
				SdsConfig: e.GetConfigSource(),
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
