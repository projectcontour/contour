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

package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/projectcontour/contour/internal/assert"
)

func TestResourceKindExists(t *testing.T) {
	type testcase struct {
		gvr             schema.GroupVersionResource
		apiResourceList map[schema.GroupVersionResource]struct{}
		want            bool
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			clients := &Clients{
				apiResources: &APIResources{
					serverResources: tc.apiResourceList,
				},
			}
			got := clients.ResourceExists(tc.gvr)
			assert.Equal(t, tc.want, got)
		})
	}

	valid := schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1beta1",
		Resource: "ingress",
	}

	invalid := schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1beta1",
		Resource: "ingressclass",
	}

	run(t, "ingress exist", testcase{
		gvr:  valid,
		want: true,
		apiResourceList: map[schema.GroupVersionResource]struct{}{
			valid: struct{}{},
		},
	})

	run(t, "ingressclass does not exist", testcase{
		gvr:  invalid,
		want: false,
		apiResourceList: map[schema.GroupVersionResource]struct{}{
			valid: struct{}{},
		},
	})

}
