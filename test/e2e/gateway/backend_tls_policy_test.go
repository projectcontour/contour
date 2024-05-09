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

package gateway

import (
	"context"
	"encoding/json"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermeta_v1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/test/e2e"
)

func testBackendTLSPolicy(namespace string, gateway types.NamespacedName) {
	Specify("Creates a BackendTLSPolicy configures an HTTPRoute to use TLS to a backend service", func() {
		protocolVersion := "TLSv1.3"
		t := f.T()

		// Top level issuer.
		selfSignedIssuer := &certmanagerv1.Issuer{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "selfsigned",
			},
			Spec: certmanagerv1.IssuerSpec{
				IssuerConfig: certmanagerv1.IssuerConfig{
					SelfSigned: &certmanagerv1.SelfSignedIssuer{},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), selfSignedIssuer))

		// CA to sign backend certs with.
		caCertificate := &certmanagerv1.Certificate{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "ca-cert",
			},
			Spec: certmanagerv1.CertificateSpec{
				IsCA: true,
				Usages: []certmanagerv1.KeyUsage{
					certmanagerv1.UsageSigning,
					certmanagerv1.UsageCertSign,
				},
				CommonName: "ca-cert",
				SecretName: "ca-cert",
				IssuerRef: certmanagermeta_v1.ObjectReference{
					Name: "selfsigned",
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), caCertificate))

		// Issuer based on CA to generate new certs with.
		basedOnCAIssuer := &certmanagerv1.Issuer{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "ca-issuer",
			},
			Spec: certmanagerv1.IssuerSpec{
				IssuerConfig: certmanagerv1.IssuerConfig{
					CA: &certmanagerv1.CAIssuer{
						SecretName: "ca-cert",
					},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), basedOnCAIssuer))

		// Backend server cert signed by CA.
		backendServerCert := &certmanagerv1.Certificate{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "backend-server-cert",
			},
			Spec: certmanagerv1.CertificateSpec{
				Usages: []certmanagerv1.KeyUsage{
					certmanagerv1.UsageServerAuth,
				},
				CommonName: "echo-secure",
				DNSNames:   []string{"echo-secure"},
				SecretName: "backend-server-cert",
				IssuerRef: certmanagermeta_v1.ObjectReference{
					Name: "ca-issuer",
				},
			},
		}

		require.NoError(f.T(), f.Client.Create(context.TODO(), backendServerCert))
		f.Fixtures.EchoSecure.Deploy(namespace, "echo-secure", func(_ *apps_v1.Deployment, service *core_v1.Service) {
			delete(service.Annotations, "projectcontour.io/upstream-protocol.tls")
		})

		route := &gatewayapi_v1.HTTPRoute{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-route-1",
			},
			Spec: gatewayapi_v1.HTTPRouteSpec{
				Hostnames: []gatewayapi_v1.Hostname{"backend-tls-policy.projectcontour.io"},
				CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
					ParentRefs: []gatewayapi_v1.ParentReference{
						gatewayapi.GatewayParentRef(gateway.Namespace, gateway.Name),
					},
				},
				Rules: []gatewayapi_v1.HTTPRouteRule{
					{
						Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						BackendRefs: gatewayapi.HTTPBackendRef("echo-secure", 443, 1),
					},
				},
			},
		}
		require.True(f.T(), f.CreateHTTPRouteAndWaitFor(route, e2e.HTTPRouteAccepted))

		backendTLSPolicy := &gatewayapi_v1alpha3.BackendTLSPolicy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "echo-secure-backend-tls-policy",
				Namespace: namespace,
			},
			Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
				TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "echo-secure",
						},
					},
				},
				Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
					CACertificateRefs: []gatewayapi_v1.LocalObjectReference{
						{
							Group: "",
							Kind:  "Secret",
							Name:  "backend-server-cert",
						},
					},
					Hostname: "echo-secure",
				},
			},
		}

		require.True(f.T(), f.CreateBackendTLSPolicyAndWaitFor(backendTLSPolicy, e2e.BackendTLSPolicyAccepted))

		type responseTLSDetails struct {
			TLS struct {
				Version string
			}
		}

		// Ensure http (insecure) request routes to echo-secure.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      "backend-tls-policy.projectcontour.io",
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(t, res)
		assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		assert.Equal(t, "echo-secure", f.GetEchoResponseBody(res.Body).Service)

		// Get cert presented to backend app.
		tlsInfo := new(responseTLSDetails)
		require.NoError(f.T(), json.Unmarshal(res.Body, tlsInfo))
		assert.Equal(f.T(), tlsInfo.TLS.Version, protocolVersion)
	})
}
