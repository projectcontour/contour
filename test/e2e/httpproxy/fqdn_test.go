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
// +build e2e

package httpproxy

import (
	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testWildcardSubdomainFQDN(namespace string) {
	Specify("invalid wildcard subdomain fqdn", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "wildcard-subdomain",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "*.projectcontour.io",
				},
				Routes: []contourv1.Route{{
					Services: []contourv1.Service{{
						Name: "ingress-conformance-echo",
						Port: 80,
					}},
				}},
			},
		}

		// Creation should fail the kubebuilder CRD validations.
		err := f.CreateHTTPProxy(p)
		require.NotNil(t, err, "Expected invalid subdomain wildcard to be rejected.")
	})
}

func testWildcardFQDN(namespace string) {
	Specify("invalid wildcard fqdn", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "wildcard-subdomain",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "*",
				},
				Routes: []contourv1.Route{{
					Services: []contourv1.Service{{
						Name: "ingress-conformance-echo",
						Port: 80,
					}},
				}},
			},
		}

		// Creation should fail the kubebuilder CRD validations.
		err := f.CreateHTTPProxy(p)
		require.NotNil(t, err, "Expected invalid wildcard to be rejected.")
	})
}
