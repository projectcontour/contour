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
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/projectcontour/contour/test/e2e"
)

func testLongPathMatch(namespace string) {
	Specify("long path matches should be properly programmed", func() {
		f.Fixtures.Echo.Deploy(namespace, "echo")

		// Just on the edge, should be RE2 program size 101 before regex optimizations.
		longPrefixMatch := "/" + strings.Repeat("a", 82)
		reallyLongPrefixMatch := "/" + strings.Repeat("b", 500)
		longRegexMatch := "/" + strings.Repeat("c", 200) + ".*"

		i := &networking_v1.Ingress{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "long-patch-match",
			},
			Spec: networking_v1.IngressSpec{
				Rules: []networking_v1.IngressRule{
					{
						Host: "long-patch-match.ingress.projectcontour.io",
						IngressRuleValue: networking_v1.IngressRuleValue{
							HTTP: &networking_v1.HTTPIngressRuleValue{
								Paths: []networking_v1.HTTPIngressPath{
									{
										PathType: ptr.To(networking_v1.PathTypePrefix),
										Path:     longPrefixMatch,
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "echo",
												Port: networking_v1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										PathType: ptr.To(networking_v1.PathTypePrefix),
										Path:     reallyLongPrefixMatch,
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "echo",
												Port: networking_v1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										PathType: ptr.To(networking_v1.PathTypeImplementationSpecific),
										Path:     longRegexMatch,
										Backend: networking_v1.IngressBackend{
											Service: &networking_v1.IngressServiceBackend{
												Name: "echo",
												Port: networking_v1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), i))

		cases := []string{
			longPrefixMatch,
			reallyLongPrefixMatch,
			// Cut off end .* when making request.
			longRegexMatch[:len(longRegexMatch)-2],
		}
		for _, path := range cases {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      i.Spec.Rules[0].Host,
				Path:      path,
				Condition: e2e.HasStatusCode(http.StatusOK),
			})
			require.NotNil(f.T(), res, "request never succeeded")
			require.Truef(f.T(), ok, "expected %d response code, got %d", http.StatusOK, res.StatusCode)
		}
	})
}
