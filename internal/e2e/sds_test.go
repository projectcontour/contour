// Copyright © 2019 VMware
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
	// not referenced by any ingress/ingressroute
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

// sample data from https://8gwifi.org/PemParserFunctions.jsp

const (
	CERTIFICATE = `-----BEGIN CERTIFICATE-----
MIIFtTCCA52gAwIBAgIJAO0cq2lJPZZJMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIEwpTb21lLVN0YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTQwMzEyMTc0NzU5WhcNMTkwMzEyMTc0NzU5WjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECBMKU29tZS1TdGF0ZTEhMB8GA1UEChMYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIIC
CgKCAgEAsgzs6vN2sveHVraXV0zdoVyhWUHWNQ0xnhHTPhjt5ggHmSvrUxvUpXfK
WCP9gZo59Q7dx0ydjqBsdooXComVP4kGDjulvOHWgvcVmwTsL0bAMqmsCyyJKM6J
Wqi8E+CPTOpMBWdapUxvwaSmop8geiTtnX0aV4zGXwsz2mwdogbounQjMB/Ew7vv
8XtqwXSpnR7kM5HPfM7wb9F8MjlRuna6Nt2V7i0oUr+EEt6fIYEVZFiHTSUzDLaz
2eClJeCNdvyqaeGCCqs+LunMq3kZjO9ahtS2+1qZxfBzac/0KXRYnLa0kGQHZbw0
ecgdZC9YpqqMeTeSnJPPX4/TQt54qVLQXM3+h8xvwt3lItcJPZR0v+0yQe5QEwPL
4c5UF81jfGrYfEzmGth6KRImRMdFLF9+F7ozAgGqCLQt3eV2YMXIBYfZS9L/lO/Q
3m4MGARZXUE3jlkcfFlcbnA0uwMBSjdNUsw4zHjVwk6aG5CwYFYVHG9n5v4qCxKV
ENRinzgGRnwkNyADecvbcQ30/UOuhU5YBnfFSYrrhq/fyCbpneuxk2EouL3pk/GA
7mGzqhjPYzaaNGVZ8n+Yys0kxuP9XDOUEDkjXpa/SzeZEk9FXMlLc7Wydj/7ES4r
6SYCs4KMr+p7CjFg/a7IdepLQ3txrZecrBxoG5mBDYgCJCfLBu0CAwEAAaOBpzCB
pDAdBgNVHQ4EFgQUWQI/JOoU+RrUPUED63dMfd2JMFkwdQYDVR0jBG4wbIAUWQI/
JOoU+RrUPUED63dMfd2JMFmhSaRHMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIEwpT
b21lLVN0YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGSCCQDt
HKtpST2WSTAMBgNVHRMEBTADAQH/MA0GCSqGSIb3DQEBBQUAA4ICAQBwGbAmiLHE
jubdLeMygwrV8VjlOVxV41wvRi6y1B5Bdvh71HPoOZdvuiZOogzxB22Tzon6Uv5q
8uuAy37rHLlQTOqLwdLJOu/ijMirAkh13gQWt4oZGUDikyiI4PMNo/hr6XoZWUfU
fwmcAzoEMln8HyISluTau1mtQIDgvGprU472GqC4AC7bYeED+ChCevc7Ytjl4zte
/tw8u3nqrkESYBIA2yEgyFAr1pRwJPM/T1U6Ehalp1ZTeQcAXEa7IC6ht2NlN1FC
fk2KQmrk4Z3jaSVv8GxshA354W+UEpti0o6Fv+2ozkAaQ1/xjiNwBTHtgJ1/AG1j
bDYcCFfmYmND0RFjvVu7ma+UNdKQ+t1o7ip4tHQUTEFvdqoaCLN09PcTVgvm71Lr
s8IOldiMgiCjQK3e0jwXx78tXs/msMzVI+9AR9aNzo0Y42C97ctlGu3+v07Zp+x4
6w1rg3eklJM02davNWK2EUSetn9EWsIJXU34Bj7mnI/2DFo292GVNw1kT5Bf4IvA
T74gsJLB6wacN4Ue6zPtIvrK93DABAfRUmrAWmH8+7MJolSC/rabJF3E2CeBTYqZ
R5M5azDV1CIhIeOTiPA/mq5fL1UrgVbB+IATIsUAQfuWivDyoeu96LB/QswyHAWG
8k2fPbA2QVWJpcnryesCy3qtzwbHSYbshQ==
-----END CERTIFICATE-----`

	RSA_PRIVATE_KEY = `-----BEGIN RSA PRIVATE KEY-----
MIIJKQIBAAKCAgEAsgzs6vN2sveHVraXV0zdoVyhWUHWNQ0xnhHTPhjt5ggHmSvr
UxvUpXfKWCP9gZo59Q7dx0ydjqBsdooXComVP4kGDjulvOHWgvcVmwTsL0bAMqms
CyyJKM6JWqi8E+CPTOpMBWdapUxvwaSmop8geiTtnX0aV4zGXwsz2mwdogbounQj
MB/Ew7vv8XtqwXSpnR7kM5HPfM7wb9F8MjlRuna6Nt2V7i0oUr+EEt6fIYEVZFiH
TSUzDLaz2eClJeCNdvyqaeGCCqs+LunMq3kZjO9ahtS2+1qZxfBzac/0KXRYnLa0
kGQHZbw0ecgdZC9YpqqMeTeSnJPPX4/TQt54qVLQXM3+h8xvwt3lItcJPZR0v+0y
Qe5QEwPL4c5UF81jfGrYfEzmGth6KRImRMdFLF9+F7ozAgGqCLQt3eV2YMXIBYfZ
S9L/lO/Q3m4MGARZXUE3jlkcfFlcbnA0uwMBSjdNUsw4zHjVwk6aG5CwYFYVHG9n
5v4qCxKVENRinzgGRnwkNyADecvbcQ30/UOuhU5YBnfFSYrrhq/fyCbpneuxk2Eo
uL3pk/GA7mGzqhjPYzaaNGVZ8n+Yys0kxuP9XDOUEDkjXpa/SzeZEk9FXMlLc7Wy
dj/7ES4r6SYCs4KMr+p7CjFg/a7IdepLQ3txrZecrBxoG5mBDYgCJCfLBu0CAwEA
AQKCAgA1Vrvu0sq/aHnp1z9VTtiiS26mn5t9PxubH/npg2xZWhR0pXyU5CR7AXzj
lLyQA9TS/gYge2pD3PlBNbMbXAYTB4iB4QqQoBM0HrMhQoNC0m4nfz7kBg585Aqv
1xao2b/0KchmYgT8uf5Mw3eMBiGjlcZ9RIoMqkaPGHsLNxJVhL5ZhQs5knrOrFGA
RRnBJKLfR+7TKB5BZHkQ9m+/V/6M3p6AazdMJ8kJqQf24yxGzDXNXtwBl2BIsb8F
SVAQHcojWCPxHjZn3c7+HNpMkDXAS8AR3k2G1Sh17MeWbk7V0F3vbKiBDQZOSuhp
hzKO3cQwAa2dbrGEKJ+aICsIwD7i8sbvw3E7sWsEhJHrXuG51alrD2NpB1QiCVgv
a3ikF5SPbqtX4htlRYmzYwZM8jtB79yStORWKou0+v5SsCliT7xqU1exrygsVGdz
lWnYu8R/YIQoWEn6rC3CwhcwwHBBKeDjjaMMD7SYIIiC11vjANnKCobVcaPrpENS
Dycct8acc8SkP5XLTwcqSv66D/O2EU/+mnwJCpBqXa8SC7Bnku9WyncJBfuDFQQl
JFrV5uhxtzhfYRCE7UcsTRX82yrA0BIsV+SWnAQEh4zIvuEwSmPcF0mY518+/kpk
HSxGNrBwb1ja4+vsXHkUuNXOWG6BLiZ70yDOXZeZwYkCIgQSHQKCAQEA3P0ADDW+
ZuDBBMMPscnwTakFnIaS7od2W6eJhKdnu10afW0rhbug1Y7w7gzLF3CtESWo/tWb
fl9ndsXAEtLpSZgFOFuMA+H9iQOsTMz6tx4zXhXA2jGt98fahYsWjdyFq7UhEijr
mQCr13FMc9KEfh/lEeSfBERdRnhCBpGAqYAXfdp/l19EIMWTofxa51q64LDjQ55u
nVTz2G8nr7HVp+rBKk2gnLFyweSBXkrLGxaLTCaxJEeFrBga2jv5WJGcXX4LXncu
1egUqsqmlzOepL6Q/W9QId9iWltcVTDW3wRuO9MkDURkqAP24RLFNXcOoAbI8ePR
R6PaINsQbYk+UwKCAQEAzkJsfYzD4rnyRYkwq0N9vQuwZQ7UKhtkvPnQWEcawTz2
+fCYg6HEmM475mAstYaL3H4v1mGz4Fq9UTxIWcAiSPJdIJAHq5/i8Y4mruLzc14y
wPZRjTroK7j4okhHvXxENge2p8KV5tocLM0ZVX/uovgPbABGpyvaQkMI5povxSDa
OFZqvha/e5BqtpTovN9+RAEwFIyercf0SGFjLyuI9GULEWwfqo4OvdcnE8LdYKjW
CuRLahGajrt19bjbt15LCGRGd4kyFFYDTFy26GggLXDvqnUw7XTn6AU/4Gw3ORw5
fxJf5ELF5wYy1erUOaH8LSRk1WoMgil2g6jZJE19vwKCAQEA0ToUrnq/36WR+hE4
rcqU4uJRdsYPHRlSHSr9T4Qz+TgIGZKf70ka2LcyMyAXtQSwRxjR7RyO0NJBIjnO
RcQ8rbnpz1cVtKNlqTC6FCjKg09rsPuFkNASdxNYOLHcU8njIRQn0Iq/rSfuitcx
XEOHv+YwuoUrbR3Q9iRr1s4x88lb9INH5CiFV0XZJjfIVV0YrB2tvlqlPf6ttFBh
Ub5cnFPuOUAv/csf7KWNOpozvFzW2+2SL9grnilgWxkHVizez8HDv9e1lz7ZOm8N
1QBBhpcKrXiTdM6LzyLKw7mu5o3KVIfujUUgy9adCrH710f2p9pkrGhWv65Jmmvu
HNchEwKCAQBOfRJh2G42WgIqmeEuWvl/NfKDEliESXZVP08cOLqirDtjsz2mYam5
aEl9Cj4ZOcEBP/eeQgG8L2t5fVIe7TFexvPPT1/L3IT03N41kOGJlmAD8/fmoXL2
KGZdAtph7ebbFKZaQn7eoUM1fTrVwWAjHfhoZdZ9CP/+VRoO/r+M6UqBQ8lM2sU1
FSi2oAXM0dNvt2//cd90S/HWlVC0A4ITVlwW3ilSsspDTZtuNqodfUIuVN+p1lcV
V5q0zgq2RaiR4e660DeBa5XHukRUPkN4Z1CccgoTYnhZX54GHcgJ8Iakp25cI1jB
6CbyJnFqGQ0odH/2gmuOII8b3OX8nYxrAoIBAQDFuMaBg7Xa0535v+6NY0iPgF5O
fKEQI9pGlLk8oKOZKLMRqQYba2qWE4jXjUyl0g3iQ1IYynFi3+cayDoMCrBXmbZ5
mGebuBySHYpBv3ajhOf1JV1cl1xivgUxM5LW708kNOuf4/hTZXR3D34kJAhoxS+/
KMkcE4BT8IZIHQ+wIMhmYLAdSQCVVv8x78jN0sZCC0fjqVuyPdYQ8sIc3OHsJZcW
lzewFW72lfsiB/RxWZ/XwXONXeW5Quf+XwbGGboTofyzTxzsYSwn1U9Kt8iaY8zr
z7Z5SQCSf2Js9V9lJcodYswWlxrdtoRKA/WgrvQkZhGGAePTUVoO5Lab29M8
-----END RSA PRIVATE KEY-----`
)

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
	}
}
