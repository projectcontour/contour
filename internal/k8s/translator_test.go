// Copyright © 2020 VMware
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

	"github.com/projectcontour/contour/internal/assert"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestConvertConvertExtensionsIngressToNetworkingIngress(t *testing.T) {
	type testcase struct {
		obj       interface{}
		want      interface{}
		wantError error
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			got, err := translateExtensionsIngressToNetworkingIngress(tc.obj)
			assert.Equal(t, tc.wantError, err)
			assert.Equal(t, tc.want, got)
		})
	}

	run(t, "extensionsv1beta1.Ingress -> v1beta1.Ingress", testcase{
		obj: &extensionsv1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example",
				Namespace: "default",
			},
			Spec: extensionsv1beta1.IngressSpec{
				Backend: &extensionsv1beta1.IngressBackend{
					ServiceName: "kuard",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		want: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "kuard",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		wantError: nil,
	})

}
