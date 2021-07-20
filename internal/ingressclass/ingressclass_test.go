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

package ingressclass

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestMatchesIngress(t *testing.T) {
	// No annotation, no spec field set, class not configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{}, ""))
	// Annotation set to default, no spec field set, class not configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
	}, ""))
	// No annotation set, spec field set to default, class not configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("contour"),
		},
	}, ""))
	// Annotation set, no spec field set, class not configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, ""))
	// No annotation set, spec field set, class not configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("aclass"),
		},
	}, ""))
	// No annotation, no spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{}, "something"))
	// Annotation set, no spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
	}, "something"))
	// No annotation set, spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("something"),
		},
	}, "something"))
	// Annotation set, no spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, "something"))
	// No annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("aclass"),
		},
	}, "something"))
	// Annotation set, spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("aclass"),
		},
	}, "something"))
	// Annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("something"),
		},
	}, "something"))
}

func TestMatchesAnnotation(t *testing.T) {
	// This is a matrix test, we are testing the annotation parser
	// across various annotations, with two options:
	// ingress class is empty
	// ingress class is not empty.
	tests := map[string]struct {
		fixture metav1.Object
		// these are results for empty and "contour" ingress class
		// respectively.
		want []bool
	}{
		"ingress nginx kubernetes.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: []bool{false, false},
		},
		"ingress nginx projectcontour.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": "nginx",
					},
				},
			},
			want: []bool{false, false},
		},
		"ingress contour kubernetes.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": DefaultClassName,
					},
				},
			},
			want: []bool{true, true},
		},
		"ingress contour projectcontour.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": DefaultClassName,
					},
				},
			},
			want: []bool{true, true},
		},
		"no annotation": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "noannotation",
					Namespace: "default",
				},
			},
			want: []bool{true, false},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cases := []string{"", DefaultClassName}
			for i := 0; i < len(cases); i++ {
				got := MatchesAnnotation(tc.fixture, cases[i])
				if tc.want[i] != got {
					t.Errorf("matching %v against ingress class %q: expected %v, got %v", tc.fixture, cases[i], tc.want[i], got)
				}
			}

		})
	}
}
