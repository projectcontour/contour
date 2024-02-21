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

package ingress

import (
	"context"
	"testing"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
)

var f = e2e.NewFramework(false)

func TestIngress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingress tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	// Delete resources individually instead of deleting the entire contour
	// namespace as a performance optimization, because deleting non-empty
	// namespaces can take up to a couple minutes to complete.
	require.NoError(f.T(), f.Deployment.DeleteResourcesForLocalContour())
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("Ingress", func() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfiguration  *contour_v1alpha1.ContourConfiguration
		contourConfigFile     string
		additionalContourArgs []string
	)

	BeforeEach(func() {
		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = e2e.DefaultContourConfigFileParams()

		contourConfiguration = e2e.DefaultContourConfiguration()

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	// JustBeforeEach is called after each of the nested BeforeEach are
	// called, so it is a final setup step before running a test.
	// A nested BeforeEach may have modified Contour config, so we wait
	// until here to start Contour.
	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, contourConfiguration, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyUpdated())
	})

	AfterEach(func() {
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.NamespacedTest("ingress-tls-wildcard-host", testTLSWildcardHost)

	f.NamespacedTest("backend-tls", func(namespace string) {
		Context("with backend tls", func() {
			BeforeEach(func() {
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
						IssuerRef: certmanagermetav1.ObjectReference{
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

				// Backend client cert, can use for upstream validation as well.
				backendClientCert := &certmanagerv1.Certificate{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      "backend-client-cert",
					},
					Spec: certmanagerv1.CertificateSpec{
						Usages: []certmanagerv1.KeyUsage{
							certmanagerv1.UsageClientAuth,
						},
						CommonName: "client",
						SecretName: "backend-client-cert",
						IssuerRef: certmanagermetav1.ObjectReference{
							Name: "ca-issuer",
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), backendClientCert))

				contourConfig.TLS = config.TLSParameters{
					ClientCertificate: config.NamespacedName{
						Namespace: namespace,
						Name:      "backend-client-cert",
					},
				}
				contourConfiguration.Spec.Envoy.ClientCertificate = &contour_v1alpha1.NamespacedName{
					Namespace: namespace,
					Name:      "backend-client-cert",
				}
			})

			testBackendTLS(namespace)
		})
	})

	f.NamespacedTest("ingress-multiple-ingress-classes-contour", func(namespace string) {
		Context("multiple ingress classes with contour ingress ClassName set", func() {
			BeforeEach(func() {
				additionalContourArgs = []string{
					"--ingress-class-name=contour,team1",
				}
				contourConfiguration.Spec.Ingress = &contour_v1alpha1.IngressConfig{
					ClassNames: []string{"contour", "team1"},
				}
			})
			testIngressClass(namespace, "contour")
		})
	})

	f.NamespacedTest("ingress-multiple-ingress-classes-team1", func(namespace string) {
		Context("multiple ingress classes with team1 ingress ClassName set", func() {
			BeforeEach(func() {
				additionalContourArgs = []string{
					"--ingress-class-name=contour,team1",
				}
				contourConfiguration.Spec.Ingress = &contour_v1alpha1.IngressConfig{
					ClassNames: []string{"contour", "team1"},
				}
			})
			testIngressClass(namespace, "team1")
		})
	})

	f.NamespacedTest("long-path-match", testLongPathMatch)

	Context("with global headers policy defined", func() {
		BeforeEach(func() {
			contourConfig.Policy.RequestHeadersPolicy.Set = map[string]string{
				"X-Contour-GlobalRequestHeader": "foo",
			}
			contourConfig.Policy.ResponseHeadersPolicy.Set = map[string]string{
				"X-Contour-GlobalResponseHeader": "bar",
			}

			contourConfiguration.Spec.Policy = &contour_v1alpha1.PolicyConfig{
				RequestHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
					Set: map[string]string{
						"X-Contour-GlobalRequestHeader": "foo",
					},
				},
				ResponseHeadersPolicy: &contour_v1alpha1.HeadersPolicy{
					Set: map[string]string{
						"X-Contour-GlobalResponseHeader": "bar",
					},
				},
			}
		})

		Context("when ApplyToIngress is false", func() {
			BeforeEach(func() {
				contourConfig.Policy.ApplyToIngress = false
				contourConfiguration.Spec.Policy.ApplyToIngress = ptr.To(false)
			})

			f.NamespacedTest("global-headers-policy-apply-to-ingress-false", testGlobalHeadersPolicy(false))
		})

		Context("when ApplyToIngress is true", func() {
			BeforeEach(func() {
				contourConfig.Policy.ApplyToIngress = true
				contourConfiguration.Spec.Policy.ApplyToIngress = ptr.To(true)
			})

			f.NamespacedTest("global-headers-policy-apply-to-ingress-true", testGlobalHeadersPolicy(true))
		})
	})
})
