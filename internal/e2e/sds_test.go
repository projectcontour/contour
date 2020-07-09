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

package e2e

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSDSVisibility(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	c := &Contour{
		T:          t,
		ClientConn: cc,
	}

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
		Resources:   resources(t),
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
							Backend: *backend("backend", intstr.FromInt(80)),
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
		Resources:   resources(t),
		TypeUrl:     secretType,
		Nonce:       "1",
	})
}

func TestSDSShouldNotIncrementVersionNumberForUnrelatedSecret(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	c := &Contour{
		T:          t,
		ClientConn: cc,
	}

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
							Backend: *backend("backend", intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     80,
			}},
		},
	})

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
	rh, cc, done := setup(t)
	defer done()

	c := &Contour{
		T:          t,
		ClientConn: cc,
	}

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
							Backend: *backend("backend", intstr.FromInt(80)),
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
		Resources:   resources(t),
		TypeUrl:     secretType,
		Nonce:       "1",
	})
}

func secret(sec *v1.Secret) *envoy_api_v2_auth.Secret {
	return envoy.Secret(&dag.Secret{
		Object: sec,
	})
}

const (
	// generated by https://www.selfsignedcertificate.com
	CERTIFICATE = `-----BEGIN CERTIFICATE-----
MIIDHTCCAgWgAwIBAgIJAOv27DGlF3qdMA0GCSqGSIb3DQEBBQUAMCUxIzAhBgNV
BAMMGmJvcmluZy13b3puaWFrLmV4YW1wbGUuY29tMB4XDTE5MTIwNTAxMzQzM1oX
DTI5MTIwMjAxMzQzM1owJTEjMCEGA1UEAwwaYm9yaW5nLXdvem5pYWsuZXhhbXBs
ZS5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDbgwFwfbikZxPb
NYidPuNJoexq5W9fJrB/3jqsWox8pfess0bw/EL/VcEUqlrcuo40Md0MxApPuoPj
eZCOZYhrA2XgcVTMnq61vusnuvmeG/qcrd5apSOoopSo2pmmI1rsJ1AVpheA+eR6
uoWVILK8uYtPmcOQAoCU/E6iZYDLZ0AEiU16kz/cGfWx9lBukd+LQ+ZRQnLDiEI/
4hRmrZrEdJoDglzIgJVI+c8OfwbLq5eRMY2fYnxqm/1BJhqjDBc4Q8ufYgfOwobu
JdVoSgiFy7wyH0GxMk4LRR6yJXLs1yjaihLERbjzlStvFVl4yidpE6Bi0amKW8HT
Qxgk7iRRAgMBAAGjUDBOMB0GA1UdDgQWBBTLcIMeWLFiL2waFL6FPomNZR7gFDAf
BgNVHSMEGDAWgBTLcIMeWLFiL2waFL6FPomNZR7gFDAMBgNVHRMEBTADAQH/MA0G
CSqGSIb3DQEBBQUAA4IBAQBQLWokaWuFeSWLpxxaBX6aatgKAKNUSqDWNzM9zVMH
xJVDywWJT3pwq7JUXujVS/c9mzCPJEsn7OQPihQECRq09l/nBK0kn9I1X6X1SMtD
OJbpEWfQQxgstdgeC6pxrZRanF5a7EWO0pFSfjuM1ABjsdExaG3C8+wgEqOjHFDS
NaW826GOFf/uMOnavpG6QePECAtJVpLAZPw6Rah6cAZrYUUezM/Tg+8JUhYUS20F
STZG5knGQIe6kksWGkJUhMu8xLdH2HKtUVAkDu7jITy2WZbg0O/Pxe30b4qyt29Y
813p8G+7188EFDBGNihYYVJ+GJ/d/WPoptSHJOfShtbk
-----END CERTIFICATE-----`

	RSA_PRIVATE_KEY = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA24MBcH24pGcT2zWInT7jSaHsauVvXyawf946rFqMfKX3rLNG
8PxC/1XBFKpa3LqONDHdDMQKT7qD43mQjmWIawNl4HFUzJ6utb7rJ7r5nhv6nK3e
WqUjqKKUqNqZpiNa7CdQFaYXgPnkerqFlSCyvLmLT5nDkAKAlPxOomWAy2dABIlN
epM/3Bn1sfZQbpHfi0PmUUJyw4hCP+IUZq2axHSaA4JcyICVSPnPDn8Gy6uXkTGN
n2J8apv9QSYaowwXOEPLn2IHzsKG7iXVaEoIhcu8Mh9BsTJOC0UesiVy7Nco2ooS
xEW485UrbxVZeMonaROgYtGpilvB00MYJO4kUQIDAQABAoIBAF5L671gNIZjRVNg
rtwl3MuPxJizEOHGJAH5/Ch4CWuufDPzG6GALGO1eekfuUKi3V2sofHO8UMIs4lv
elrBYRXfcs80wCHadODcL/Z0SrDSAhl2U1OLJ0NU/BmBNon5HCDgTnXOUMB2GOFj
6OiEEGQkLKU4P5tIh+X4cOswQWCeoVjW0JVgni20hi3LJNTxSNYeU5VFvPKtoBLl
8nFqF3ky+bqYfS6H6qM/mO+XL0NQ2wjMteyUeDXcVGfsf7Ir21SUw3zGaeBJl55B
6BrUgfxVOKuxkw2bwxmu8HX+CxlMMMzaRt+5URFbfOaMgXzjpikrxdeFAAGeu0m4
bidUR5UCgYEA8lRGqYfowoOCrV8Ksn8nM0Z9PlnmKM5d9mQ875sm/SYLO43h+s0D
R4VWmLzaGyi0m0036lxIthDfbbGWSjmNrgQ0YIS7ilmBPMUKKYzXgDoiI76aJBTz
UMpWutb+VYimPPorLKcxNb3BjR3QHx7vCRS2gV5izV0djtMkKc53OXsCgYEA5+Uz
A7cmO8gHyxlW6SA3+wMH6VKP5ABTkDmKfRF3NCv4UHNn4TtlNuS1D3ZMNXWgCtz6
qJ/bRTAqseBIX15pzR/MvyNmHRUN3A2Ba6vB2pJux+ZyQjxn3Z+gisjX+eN3LvTU
YpcJNi0HSuV57n4AAk5YPO5iMEFw95vfBn3MMaMCgYEAnFwyqAsQ7gmLVTDBJ0GS
Wqx9/bBmKShXSreM9hIHi0pz7v5ytLB6EDkCElWw6dtPBfJCRQ88v3WNpSr0TXpr
Z8BAx5J9rBxqnnqJPxwopQ1dn/DJZsS55wRYCADXZPtiQHAvUYWj5AhHjjWRZ7M/
C3348OqlF9ugSdsFN5CIL2cCgYEAqt5lop03XOFdbLe1JH4LAbgQAkpFoDjlWeYs
N0/BR/4GMDF5H6sGP1ZyW3xNVy7eyGJfiBSSGv8M1phue2c0CmMeGNDakx9KYRTK
gi3C32z6l+0jz852sgTG5Lxs98I1tbHNNQAZV4QCVZuVJrhNBWX4+pykWO4/cRO3
WC8lYIUCgYBmmN4z0MR2YWoRvN3lYey3bRGAvsSU6ouiFo40UZdZaRXc1sA3oc+5
6Di3f8eOIhM5IekOBoaTBf90V8seB6Nw+/jzAViG1HDI7k0ZOoApDuFS6NYk1/bU
dk98FvYdyAjjgNsxXCyx7vIgYU3OgVNgvFsFubX/Uk66fcfCpPBMLg==
-----END RSA PRIVATE KEY-----`
)

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
	}
}
