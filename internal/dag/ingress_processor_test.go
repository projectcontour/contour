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

package dag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestHttpPaths(t *testing.T) {
	tests := map[string]struct {
		rule networking_v1.IngressRule
		want []networking_v1.HTTPIngressPath
	}{
		"zero value": {
			rule: networking_v1.IngressRule{},
			want: nil,
		},
		"empty paths": {
			rule: networking_v1.IngressRule{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{},
				},
			},
			want: nil,
		},
		"several paths": {
			rule: networking_v1.IngressRule{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}, {
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			},
			want: []networking_v1.HTTPIngressPath{{
				Backend: *backendv1("kuard", intstr.FromString("http")),
			}, {
				Path:    "/kuarder",
				Backend: *backendv1("kuarder", intstr.FromInt(8080)),
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := httppaths(tc.rule)
			assert.Equal(t, tc.want, got)
		})
	}
}
