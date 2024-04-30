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

package incluster

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testSimpleSmoke(namespace string) {
	Specify("simple smoke test", func() {
		// Make multiple instances to ensure many events/updates are
		// processed correctly.
		// This test may become flaky and should be investigated if there
		// are changes that cause differences between the leader and
		// non-leader contour instances.
		for i := range 20 {
			f.Fixtures.Echo.Deploy(namespace, fmt.Sprintf("echo-%d", i))

			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      fmt.Sprintf("smoke-test-%d", i),
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: fmt.Sprintf("smoke-test-%d.projectcontour.io", i),
					},
					Routes: []contour_v1.Route{
						{
							Services: []contour_v1.Service{
								{
									Name: fmt.Sprintf("echo-%d", i),
									Port: 80,
								},
							},
						},
					},
				},
			}
			require.True(f.T(), f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(f.T(), res, "request never succeeded")
			require.Truef(f.T(), ok, "expected 200 response code, got %d for echo-%d", res.StatusCode, i)
		}
	})
}
