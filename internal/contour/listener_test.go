// Copyright © 2017 Heptio
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

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestListenerCacheRecomputeListener(t *testing.T) {
	lc := new(ListenerCache)
	assertCacheEmpty(t, lc)

	i := map[metadata]*v1beta1.Ingress{
		metadata{name: "example", namespace: "default"}: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
	}
	lc.recomputeListener(i)
	assertCacheNotEmpty(t, lc)
}

func TestListenerCacheRecomputeTLSListener(t *testing.T) {
	lc := new(ListenerCache)
	assertCacheEmpty(t, lc)

	i := map[metadata]*v1beta1.Ingress{
		metadata{name: "example", namespace: "default"}: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend("backend", intstr.FromInt(80)),
			},
		},
	}
	s := make(map[metadata]*v1.Secret)
	lc.recomputeTLSListener(i, s)
	assertCacheEmpty(t, lc) // expect cache to be empty, this is not a tls enabled ingress

	i[metadata{name: "example", namespace: "default"}] = &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"whatever.example.com"},
				SecretName: "secret",
			}},
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	lc.recomputeTLSListener(i, s)
	assertCacheEmpty(t, lc) // expect cache to be empty, this ingress is tls enabled, but missing secret

	s[metadata{name: "secret", namespace: "default"}] = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
	}
	lc.recomputeTLSListener(i, s)
	assertCacheNotEmpty(t, lc) // we've got the secret and the ingress, we should have at least one listener
}

func assertCacheEmpty(t *testing.T, lc *ListenerCache) {
	if len(lc.values) > 0 {
		t.Fatalf("len(lc.values): expected 0, got %d", len(lc.values))
	}
}

func assertCacheNotEmpty(t *testing.T, lc *ListenerCache) {
	if len(lc.values) == 0 {
		t.Fatalf("len(lc.values): expected > 0, got %d", len(lc.values))
	}
}
