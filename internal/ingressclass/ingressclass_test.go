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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func TestMatchesIngress(t *testing.T) {
	// No annotation, no spec field set, class not configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{}, nil))
	// Annotation set to default, no spec field set, class not configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
	}, nil))
	// No annotation set, spec field set to default, class not configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("contour"),
		},
	}, nil))
	// Annotation set, no spec field set, class not configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, nil))
	// No annotation set, spec field set, class not configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("aclass"),
		},
	}, nil))
	// No annotation, no spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{}, []string{"something"}))
	// Annotation set, no spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
	}, []string{"something"}))
	// No annotation set, spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("something"),
		},
	}, []string{"something"}))
	// Annotation set, no spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, []string{"something"}))
	// No annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("aclass"),
		},
	}, []string{"something"}))
	// Annotation set, spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("aclass"),
		},
	}, []string{"something"}))
	// Annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("something"),
		},
	}, []string{"something"}))
	// Multiple classes: Annotation set, no spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, []string{"something", "somethingelse"}))
	// Multiple classes: No annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("aclass"),
		},
	}, []string{"something", "somethingelse"}))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.True(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("aclass"),
		},
	}, []string{"somethingelse", "something"}))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.False(t, MatchesIngress(&networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: ptr.To("something"),
		},
	}, []string{"something", "somethingelse"}))
}

func TestMatchesHTTPProxy(t *testing.T) {
	// No annotation, no spec field set, class not configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{}, nil))
	// Annotation set to default, no spec field set, class not configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
	}, nil))
	// No annotation set, spec field set to default, class not configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "contour",
		},
	}, nil))
	// Annotation set, no spec field set, class not configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, nil))
	// No annotation set, spec field set, class not configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, nil))
	// No annotation, no spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{}, []string{"something"}))
	// Annotation set, no spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
	}, []string{"something"}))
	// No annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, []string{"something"}))
	// Annotation set, no spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, []string{"something"}))
	// No annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, []string{"something"}))
	// Annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, []string{"something"}))
	// Annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, []string{"something"}))
	// Multiple classes: Annotation set, no spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
	}, []string{"something", "somethingelse"}))
	// Multiple classes: No annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, []string{"athing", "something", "somethingelse"}))
	// Multiple classes: Annotation set, no spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
	}, []string{"something", "somethingelse"}))
	// Multiple classes: No annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, []string{"somethingelse", "something"}))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.True(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "something",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "aclass",
		},
	}, []string{"somethingelse", "something"}))
	// Multiple classes: Annotation set, spec field set, class configured
	assert.False(t, MatchesHTTPProxy(&contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "foo",
			},
		},
		Spec: contour_v1.HTTPProxySpec{
			IngressClassName: "something",
		},
	}, []string{"something", "somethingelse"}))
}
