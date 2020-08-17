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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSDSVisibility(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	// add secret
	rh.OnAdd(s1)

	// assert that the secret is _not_ visible as it is
	// not referenced by any ingress/httpproxy
	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   nil,
		TypeUrl:     secretType,
		Nonce:       "0",
	})

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "backend",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// i1 has a default route to backend:80, but there is no matching service.
	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources:   nil,
		TypeUrl:     secretType,
		Nonce:       "1",
	})
}

// TODO(youngnick)#2782: This test has been a noop for a long time - because it compares
// the values of VersionInfo and Nonce, which are ignored in assert.Equal.
func TestSDSShouldNotIncrementVersionNumberForUnrelatedSecret(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	// add secret
	rh.OnAdd(s1)

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "backend",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(fixture.NewService("backend").
		WithPorts(v1.ServicePort{Name: "http", Port: 80}))
	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources:   resources(t, secret(s1)),
		TypeUrl:     secretType,
		Nonce:       "2",
	})

	// verify that requesting the same resource without change
	// does not bump the current version_info.

	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources:   resources(t, secret(s1)),
		TypeUrl:     secretType,
		Nonce:       "2",
	})

	// s2 is not referenced by any active ingress object.
	s2 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrelated",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: s1.Data,
	}
	rh.OnAdd(s2)

	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources:   resources(t, secret(s1)),
		TypeUrl:     secretType,
		Nonce:       "2",
	})
}

// issue 1169, an invalid certificate should not be
// presented by SDS even if referenced by an ingress object.
func TestSDSshouldNotPublishInvalidSecret(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// s1 is NOT a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: "default",
		},
		Type: "kubernetes.io/dockerconfigjson",
		Data: map[string][]byte{
			".dockerconfigjson": []byte("ewogICAgImF1dGhzIjogewogICAgICAgICJodHRwczovL2luZGV4LmRvY2tlci5pby92MS8iOiB7CiAgICAgICAgICAgICJhdXRoIjogImMzUi4uLnpFMiIKICAgICAgICB9CiAgICB9Cn0K"),
		},
	}
	// add secret
	rh.OnAdd(s1)

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "invalid",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "backend",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// SDS should be empty
	c.Request(secretType).Equals(&v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources:   nil,
		TypeUrl:     secretType,
		Nonce:       "1",
	})
}

func secret(sec *v1.Secret) *envoy_api_v2_auth.Secret {
	return envoy.Secret(&dag.Secret{
		Object: sec,
	})
}
