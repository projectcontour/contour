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

package incluster

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func testHeaderMatchIncludesMemoryUsage(namespace string) {
	Specify("many includes with header match conditions do not cause a spike in memory usage", func() {
		f.Fixtures.Echo.Deploy(namespace, "echo")

		root := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      "root",
				Namespace: namespace,
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "root-header-include-memory-usage.com",
				},
			},
		}

		const (
			numChildren      = 100
			numHeaderMatches = 5
		)

		for i := range numChildren {
			include := contour_v1.Include{
				Name: fmt.Sprintf("child-%d", i),
			}
			for h := 0; h < numHeaderMatches; h++ {
				include.Conditions = append(include.Conditions, contour_v1.MatchCondition{
					Header: &contour_v1.HeaderMatchCondition{
						Name:  fmt.Sprintf("X-Foo-Child-%d-Header-%d", i, h),
						Exact: "foo-XXXXXXXXXXXXXXXXXXXXXX",
					},
				})
			}
			root.Spec.Includes = append(root.Spec.Includes, include)

			child := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      fmt.Sprintf("child-%d", i),
					Namespace: namespace,
				},
				Spec: contour_v1.HTTPProxySpec{
					Routes: []contour_v1.Route{
						{
							Services: []contour_v1.Service{
								{
									Name: "echo",
									Port: 80,
								},
							},
						},
					},
				},
			}
			require.NoError(f.T(), f.CreateHTTPProxy(child))
		}

		// Wait for root to be valid.
		require.True(f.T(), f.CreateHTTPProxyAndWaitFor(root, e2e.HTTPProxyValid))

		// Ensure there are no container restarts.
		require.Never(f.T(), func() bool {
			pods := new(core_v1.PodList)
			podListOptions := &client.ListOptions{
				LabelSelector: labels.SelectorFromSet(f.Deployment.ContourDeployment.Spec.Selector.MatchLabels),
				Namespace:     f.Deployment.ContourDeployment.Namespace,
			}
			if err := f.Client.List(context.TODO(), pods, podListOptions); err != nil {
				return true
			}
			anyPodRestarts := false
			for _, pod := range pods.Items {
				anyPodRestarts = anyPodRestarts || pod.Status.ContainerStatuses[0].RestartCount > 0
			}
			return anyPodRestarts
		}, time.Second*10, time.Millisecond*100)
	})
}
