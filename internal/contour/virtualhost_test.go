// Copyright © 2018 Heptio
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
package contour

import (
	"testing"

	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSslRedirect(t *testing.T) {
	tests := map[string]struct {
		i     *v1beta1.Ingress
		valid bool
	}{
		"redirect to https": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubenetes.io/ingress.ssl-redirect": "true",
					},
				},
			},
			valid: true,
		},
		"don't redirect to https": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubenetes.io/ingress.ssl-redirect": "false",
					},
				},
			},
			valid: false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := sslRedirect(tc.i)
			want := tc.valid
			if got != want {
				t.Fatalf("sslRedirect: got: %v, want: %v", got, want)
			}
		})
	}
}
