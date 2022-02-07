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

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
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
	// Multiple classes: Annotation set, no spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, "something,somethingelse"))
	// Multiple classes: No annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("aclass"),
		},
	}, "something,somethingelse"))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("aclass"),
		},
	}, "somethingelse,something"))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: pointer.StringPtr("something"),
		},
	}, "something,somethingelse"))
}

func TestMatchesHTTPProxy(t *testing.T) {
	// No annotation, no spec field set, class not configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{}, ""))
	// Annotation set to default, no spec field set, class not configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
	}, ""))
	// No annotation set, spec field set to default, class not configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "contour",
		},
	}, ""))
	// Annotation set, no spec field set, class not configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, ""))
	// No annotation set, spec field set, class not configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, ""))
	// No annotation, no spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{}, "something"))
	// Annotation set, no spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
	}, "something"))
	// No annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, "something"))
	// Annotation set, no spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, "something"))
	// No annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, "something"))
	// Annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, "something"))
	// Annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, "something"))
	// Multiple classes: Annotation set, no spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
	}, "something,somethingelse"))
	// Multiple classes: No annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, "athing,something,somethingelse"))
	// Multiple classes: Annotation set, no spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, "something,somethingelse"))
	// Multiple classes: No annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, "somethingelse,something"))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, "somethingelse,something"))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, "something,somethingelse"))
}
