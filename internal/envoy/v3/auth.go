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
	core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_extensions_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
)

// UpstreamTLSContext creates an envoy_v3_tls.UpstreamTlsContext. By default
// UpstreamTLSContext returns a HTTP/1.1 TLS enabled context. A list of
// additional ALPN protocols can be provided.
func UpstreamTLSContext(peerValidationContext *dag.PeerValidationContext, sni string, clientSecret *dag.Secret, alpnProtocols ...string) *tls_v3.UpstreamTlsContext {
	var clientSecretConfigs []*tls_v3.SdsSecretConfig
	if clientSecret != nil {
		clientSecretConfigs = []*tls_v3.SdsSecretConfig{{
			Name:      envoy.Secretname(clientSecret),
			SdsConfig: ConfigSource("contour"),
		}}
	}

	context := &tls_v3.UpstreamTlsContext{
		CommonTlsContext: &tls_v3.CommonTlsContext{
			AlpnProtocols:                  alpnProtocols,
			TlsCertificateSdsSecretConfigs: clientSecretConfigs,
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

func validationContext(ca []byte, subjectName string) *tls_v3.CommonTlsContext_ValidationContext {
	vc := &tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &tls_v3.CertificateValidationContext{
			TrustedCa: &core_v3.DataSource{
				// TODO(dfc) update this for SDS
				Specifier: &core_v3.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
		},
	}

	if len(subjectName) > 0 {
		vc.ValidationContext.MatchSubjectAltNames = []*matcher_v3.StringMatcher{{
			MatchPattern: &matcher_v3.StringMatcher_Exact{
				Exact: subjectName,
			}},
		}
	}

	return vc
}

// DownstreamTLSContext creates a new DownstreamTlsContext.
func DownstreamTLSContext(serverSecret *dag.Secret, tlsMinProtoVersion tls_v3.TlsParameters_TlsProtocol, cipherSuites []string, peerValidationContext *dag.PeerValidationContext, alpnProtos ...string) *tls_v3.DownstreamTlsContext {
	context := &tls_v3.DownstreamTlsContext{
		CommonTlsContext: &tls_v3.CommonTlsContext{
			TlsParams: &tls_v3.TlsParameters{
				TlsMinimumProtocolVersion: tlsMinProtoVersion,
				TlsMaximumProtocolVersion: tls_v3.TlsParameters_TLSv1_3,
				CipherSuites:              cipherSuites,
			},
			TlsCertificateSdsSecretConfigs: []*tls_v3.SdsSecretConfig{{
				Name:      envoy.Secretname(serverSecret),
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

func http2ProtocolOptions() map[string]*any.Any {
	return map[string]*any.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
			&envoy_extensions_upstream_http_v3.HttpProtocolOptions{
				UpstreamProtocolOptions: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
					ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
						ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
					},
				},
			}),
	}
}
