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

package v3

import (
	"testing"

	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func TestSDSVisibility(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(s1)

	// assert that the secret is _not_ visible as it is
	// not referenced by any ingress/httpproxy
	c.Request(secretType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   nil,
		TypeUrl:     secretType,
		Nonce:       "0",
	})

	// i1 is a tls ingress
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "backend",
									Port: networking_v1.ServiceBackendPort{Number: 80},
								},
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// i1 has a default route to backend:80, but there is no matching service.
	c.Request(secretType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources:   nil,
		TypeUrl:     secretType,
		Nonce:       "1",
	})
}

func TestSDSShouldNotIncrementVersionNumberForUnrelatedSecret(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	svc1 := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 80})

	s1 := featuretests.TLSSecret(t, "secret", &featuretests.ServerCertificate)
	rh.OnAdd(s1)

	// i1 is a tls ingress
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *featuretests.IngressBackend(svc1),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(svc1)

	res := c.Request(secretType)
	res.Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, secret(s1)),
	})
	vers := res.VersionInfo

	// verify that requesting the same resource without change
	// does not bump the current version_info.
	res = c.Request(secretType)
	res.Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, secret(s1)),
	})
	assert.Equal(t, vers, res.VersionInfo)

	// s2 is not referenced by any active ingress object.
	s2 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "unrelated",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: s1.Data,
	}
	rh.OnAdd(s2)

	res = c.Request(secretType)
	res.Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, secret(s1)),
	})
	assert.Equal(t, vers, res.VersionInfo)

	// Verify that deleting an unreferenced secret does not
	// bump the current version_info.
	rh.OnDelete(s2)
	res = c.Request(secretType)
	res.Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t, secret(s1)),
	})
	assert.Equal(t, vers, res.VersionInfo)

	// Verify that deleting a referenced secret does
	// bump the current version_info.
	rh.OnDelete(s1)
	res = c.Request(secretType)
	res.Equals(&envoy_service_discovery_v3.DiscoveryResponse{})
	assert.NotEqual(t, vers, res.VersionInfo)
}

// issue 1169, an invalid certificate should not be
// presented by SDS even if referenced by an ingress object.
func TestSDSshouldNotPublishInvalidSecret(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	// s1 is NOT a tls secret
	s1 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
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
	i1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "invalid",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "kuard.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: networking_v1.IngressBackend{
								Service: &networking_v1.IngressServiceBackend{
									Name: "backend",
									Port: networking_v1.ServiceBackendPort{Number: 80},
								},
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	// SDS should be empty
	c.Request(secretType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: "1",
		Resources:   nil,
		TypeUrl:     secretType,
		Nonce:       "1",
	})
}

func secret(sec *core_v1.Secret) *envoy_transport_socket_tls_v3.Secret {
	return envoy_v3.Secret(&dag.Secret{
		Object: sec,
	})
}
