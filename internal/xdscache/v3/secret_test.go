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

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	"google.golang.org/protobuf/proto"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSecretCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_tls_v3.Secret
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: secretmap(
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			want: []proto.Message{
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sc SecretCache
			sc.Update(tc.contents)
			got := sc.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestSecretCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_tls_v3.Secret
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: secretmap(
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			query: []string{"default/secret/0567f551af"},
			want: []proto.Message{
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
		},
		"partial match": {
			contents: secretmap(
				secret("default/secret-a/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			),
			query: []string{"default/secret/0567f551af", "default/secret-b/5397c67313"},
			want: []proto.Message{
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			},
		},
		"no match": {
			contents: secretmap(
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			query: []string{"default/secret-b/5397c67313"},
			want:  nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sc SecretCache
			sc.Update(tc.contents)
			got := sc.Query(tc.query)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestSecretVisit(t *testing.T) {
	tests := map[string]struct {
		objs []interface{}
		want map[string]*envoy_tls_v3.Secret
	}{
		"nothing": {
			objs: nil,
			want: map[string]*envoy_tls_v3.Secret{},
		},
		"unassociated secrets": {
			objs: []interface{}{
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			},
			want: map[string]*envoy_tls_v3.Secret{},
		},
		"simple ingress with secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple ingresses with shared secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"omg.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "omg.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple ingresses with different secrets": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret-a",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 80),
									}},
								},
							},
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"omg.example.com"},
							SecretName: "secret-b",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "omg.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 80),
									}},
								},
							},
						}},
					},
				},
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret-a/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			),
		},
		"simple httpproxy with secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple httpproxies with shared secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www1.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www2.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple httpproxies with different secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www1.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret-a",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www2.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret-b",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			},
			want: secretmap(
				secret("default/secret-a/0567f551af", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sc SecretCache
			sc.OnChange(buildDAG(t, tc.objs...))
			protobuf.ExpectEqual(t, tc.want, sc.values)
		})
	}
}

// buildDAG produces a dag.DAG from the supplied objects.
func buildDAG(t *testing.T, objs ...interface{}) *dag.DAG {
	builder := dag.Builder{
		Source: dag.KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		},
		Processors: []dag.Processor{
			&dag.ListenerProcessor{},
			&dag.IngressProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			},
			&dag.HTTPProxyProcessor{},
		},
	}

	for _, o := range objs {
		builder.Source.Insert(o)
	}
	return builder.Build()
}

// buildDAGFallback produces a dag.DAG from the supplied objects with a fallback cert configured.
func buildDAGFallback(t *testing.T, fallbackCertificate *types.NamespacedName, objs ...interface{}) *dag.DAG {
	builder := dag.Builder{
		Source: dag.KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		},
		Processors: []dag.Processor{
			&dag.ListenerProcessor{
				HTTPAddress:  "0.0.0.0",
				HTTPPort:     8080,
				HTTPSAddress: "0.0.0.0",
				HTTPSPort:    8443,
			},
			&dag.IngressProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			},
			&dag.HTTPProxyProcessor{
				FallbackCertificate: fallbackCertificate,
			},
		},
	}
	for _, o := range objs {
		builder.Source.Insert(o)
	}
	return builder.Build()
}

func secretmap(secrets ...*envoy_tls_v3.Secret) map[string]*envoy_tls_v3.Secret {
	m := make(map[string]*envoy_tls_v3.Secret)
	for _, s := range secrets {
		m[s.Name] = s
	}
	return m
}

func secret(name string, data map[string][]byte) *envoy_tls_v3.Secret {
	return &envoy_tls_v3.Secret{
		Name: name,
		Type: &envoy_tls_v3.Secret_TlsCertificate{
			TlsCertificate: &envoy_tls_v3.TlsCertificate{
				CertificateChain: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_InlineBytes{
						InlineBytes: data[v1.TLSCertKey],
					},
				},
				PrivateKey: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_InlineBytes{
						InlineBytes: data[v1.TLSPrivateKeyKey],
					},
				},
			},
		},
	}
}

// tlssecert creates a new v1.Secret object of type kubernetes.io/tls.
func tlssecret(namespace, name string, data map[string][]byte) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1.SecretTypeTLS,
		Data: data,
	}
}

func backend(name string, port int32) *networking_v1.IngressBackend {
	return &networking_v1.IngressBackend{
		Service: &networking_v1.IngressServiceBackend{
			Name: name,
			Port: networking_v1.ServiceBackendPort{Number: port},
		},
	}
}

// nolint:revive
const (
	// CERTIFICATE generated by
	// openssl genrsa -out example-key.pem 2048
	// openssl req -new -x509 -days 18250 -key example-key.pem -sha256 -subj "/CN=www.example.com" -out example.pem
	CERTIFICATE = `-----BEGIN CERTIFICATE-----
MIIDFzCCAf+gAwIBAgIUZULFakfIJl0qaJXAVPCz2nzvB38wDQYJKoZIhvcNAQEL
BQAwGjEYMBYGA1UEAwwPd3d3LmV4YW1wbGUuY29tMCAXDTIyMDgxOTExMDkxNVoY
DzIwNzIwODA2MTEwOTE1WjAaMRgwFgYDVQQDDA93d3cuZXhhbXBsZS5jb20wggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDS9S4d/ea6wqiib8UeyHMptoks
w+q2DNuF75NQHLh5Z2rUnE/N8/KVhpIx81QdId1maWS0b3392hCRRFY3sDlMpRk/
1uoQgzLdk8pjw1JiqoDpvTiKZsADmVuUcCdHLNEzYtcLWBv0VyyNyE5pdrnVnMbx
w1aiQ8w2lcBCJQ8Y4DAc5oKlvBu49aUsvfFZwjL6Cr1qafQiYylqQcz7zqGBYjXc
iMzN+4fE1XQlw1iy6XmVZiHQr8Sb7EBI+g0iJapgNv7tBunzywSvAYK8N42QQOll
1sKEVf7thoNEmJTIUFo6m57Fys7LQ/B8in5JwBU+1FjqNWLJ1Gj+zIc93oc3AgMB
AAGjUzBRMB0GA1UdDgQWBBS/BZ2Uu1Y0//Um8bOqyyWz9LnPvzAfBgNVHSMEGDAW
gBS/BZ2Uu1Y0//Um8bOqyyWz9LnPvzAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3
DQEBCwUAA4IBAQCTZ6ZDi7aU7NjZdGWNLrRCBEt+FcD+mdvtRcaSp2K7m+WObnWl
rDM7V/s8ziu8ffwfwEbaBKYVLO7Mww8ke0WclBp1sq6A5AWy1sBCQYJuPCdOJNY0
fLaZObhUSQvNGw1wAXkgczrsOa/5QII356UsLiqhninXWYTMvNehab4+QW6Dldqo
EyxKgX2Ls984ZN5CDvvXfRnkeQW1/K705ReZq8qmtmCwU5wHYy0IoJGNapeX45VY
6s2n5I5CpH4L9Ua4NLgqphjC/QYK4q71GHTZD89mfTsmE+0flgFDS+wrv5SusK8u
CY2iW9j8VptZU8LVs9FrhgecEtfXbTA3MeSo
-----END CERTIFICATE-----`

	RSA_PRIVATE_KEY = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0vUuHf3musKoom/FHshzKbaJLMPqtgzbhe+TUBy4eWdq1JxP
zfPylYaSMfNUHSHdZmlktG99/doQkURWN7A5TKUZP9bqEIMy3ZPKY8NSYqqA6b04
imbAA5lblHAnRyzRM2LXC1gb9FcsjchOaXa51ZzG8cNWokPMNpXAQiUPGOAwHOaC
pbwbuPWlLL3xWcIy+gq9amn0ImMpakHM+86hgWI13IjMzfuHxNV0JcNYsul5lWYh
0K/Em+xASPoNIiWqYDb+7Qbp88sErwGCvDeNkEDpZdbChFX+7YaDRJiUyFBaOpue
xcrOy0PwfIp+ScAVPtRY6jViydRo/syHPd6HNwIDAQABAoIBAEf36RHGSu6v9gPk
iaUk0VULtuSUuf/9hu68es873Rtd0q5R3U/vx3SHglyUHMALi5Kipf6AgsUVnc1R
OPCqqAGj2WdUFGopuDKrdsJuIi8S6APVz/I3d45CxWFwmZXIjl4vfBmcp3zGOKbu
DQIhxOhBIgXclDOrWYHNuNdX+TyMr9cBe9bPtsWmN6Wl8V26kLzboQ3bIsM9taL+
sizVNrLL4U7oqbsKVhmI1m78+VaucQeK+XuyvuT6toOxY/Mgidcp4Mwq06aOTFug
N/u+VyXNen3Vk6/kMTxbaM9aQ05cnOrKqrgJAhiFGb+lcqvvGlwGxNqGcBABYkqf
ejhNdEECgYEA/3shOSl4GmEGZZXTARo0bDnelSpSJgmlStBeAGraq+lm/4IrTCCB
2tmJkEs6uiCEH7vkbbI/lLlhNjM2gKDovP3UwTQHGfiHlcIYYICMIpbp7+Ar9+ex
4KPXmgqqhTKC4xQYGoUE6INlpZcQ4blA4AtQbhgcC4Cp8FD/QbXp0xECgYEA02Ll
EAxzHZBNoK/5oPL0LiDUW/zPdOWCQCW8oGW5gDUvCLQ/lntegL8Qzubt7PZ4xgeg
m2ENTDcp1Zfn9s0T1V2T9Sba8gShUCvm9nLVCj4OQ3lwDufwHFuhKFpj1BVnHD5y
9yhXfyrFgvhamepEjq6LZUCrL1HxZqgOezv6JccCgYBqM1oFNArcFFcfZV+YRrdi
AdBX+4a4jyvp5KIe1ExgSB7rucWb2KuCOQmpNMyN0LR7qJR1UTKC9WjGqhVO9RSq
c228fo8xKZHbHBscCnO2cTt/3pUIcYUM167pNuPZiLzF/nVimMcIjI51fk2jN2oT
eECP82+9DFgYMONbAm7XsQKBgH/Y3ztenDz0Ks8Vv3/FkUNY3bco5vwHV0ieyj+k
ZpYRFHpKMe88fEKXzH2mk53uz8rNkCiJgTZoYqfpcQUGsYkpSLRLpL4daMcJVm4V
s523PH84sjqBsuojzQuP57K8oxkk9/ld79VctAprVLikRISbMnmxrBc5kywIVoHY
G4m/AoGBAPV0wuytURR3CbPHz28wrKRh/xnHrW+Fp3ooRfj8Pr/4zDVsqqNz2gA3
yVb6kAEpg2ON9NOWSfoUfC+THitOnRm8pKL1QL7oiq/2+s3IiK+jSevEU7TUfjio
1LwtUqv1MbKdv7TgkU0YQ99iLocWF4F4oWF6AX86/BL9y7gcbE0y
-----END RSA PRIVATE KEY-----`

	CERTIFICATE_2 = `-----BEGIN CERTIFICATE-----
MIIDJTCCAg2gAwIBAgIJAP5VPT9oymF8MA0GCSqGSIb3DQEBBQUAMCkxJzAlBgNV
BAMMHmV4cGxvc2l2ZS1kaWFycmhlYS5leGFtcGxlLmNvbTAeFw0xOTEyMDUwMjEw
MDlaFw0yOTEyMDIwMjEwMDlaMCkxJzAlBgNVBAMMHmV4cGxvc2l2ZS1kaWFycmhl
YS5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM2V
AaUUqC1/s3b7Rr1D6YkN99dLlwgMvwE+KzPXHfupn0tMdNsSfFjeO7I+mULFwLf+
fBxzUhdn9Vx6k7/INVCl6ktPKD/nhcgW2pfideDN9sCoIzAyPOL6ysV7IkYz5UJC
FTU5zGUW9X7xcRqKyQyvXEQPc/sx8fVIDr5Aw5K64a000NTLfjcTWqEWrXqk7wdg
u7+7ggazxMGJQEyjwpDQVToQH5yIdbyjgB4rniykqGYzeixTzHeQCf7d/kYTHPrW
v9970zo6PC8Q99mFl75BR7Hdy3X9DSAcz050MwkCXSgwoYxGnWHCrI+Ynndf+RO5
YDMyXbEAT1zbD2O2ER8CAwEAAaNQME4wHQYDVR0OBBYEFBSXzvnpiVwzs9iiPABO
D0SbE8PuMB8GA1UdIwQYMBaAFBSXzvnpiVwzs9iiPABOD0SbE8PuMAwGA1UdEwQF
MAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAFQ1sHL2yqAOekYTWO4ZzDGKWbkFPjPM
hEP3ZFOUabXeR3q/n8yzb1ngFCQj/alSf6otPXZqrfdXtN4TPGhx50kf9zmX94r2
lWP+TcHyMQOJGXJQgTv1yGAgruJ638GZAiHJ0q3b3hrRVLaHwFcBaYRwtPGA9CgX
gfKxSdfTxUcuSz4gPIB3JDfRbO5E45rSZJokbjuYm5ZI5DtjSRCA6w//lDKqJGiM
6aUNFwejPFZJ1MkBgQ1muYifzRoXZ/pUwUMlEzThChe3r2nXznCbpK9LtJ6KOX0Y
dIpyQrOkc9wdjNlS2+h2IYwmJujZNoISX53P7bbAqm01guWYDASz+Wc=
-----END CERTIFICATE-----`

	RSA_PRIVATE_KEY_2 = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAzZUBpRSoLX+zdvtGvUPpiQ3310uXCAy/AT4rM9cd+6mfS0x0
2xJ8WN47sj6ZQsXAt/58HHNSF2f1XHqTv8g1UKXqS08oP+eFyBbal+J14M32wKgj
MDI84vrKxXsiRjPlQkIVNTnMZRb1fvFxGorJDK9cRA9z+zHx9UgOvkDDkrrhrTTQ
1Mt+NxNaoRateqTvB2C7v7uCBrPEwYlATKPCkNBVOhAfnIh1vKOAHiueLKSoZjN6
LFPMd5AJ/t3+RhMc+ta/33vTOjo8LxD32YWXvkFHsd3Ldf0NIBzPTnQzCQJdKDCh
jEadYcKsj5ied1/5E7lgMzJdsQBPXNsPY7YRHwIDAQABAoIBAGQNC6LivcJ+7sGO
IuxDm+mGscLG1/cj9OVO80mkfMQY1hyYVhq0EW5SoazqyD317gfdw1s5SI95mbDr
OvLQJhpc1PzXxlfrfnFVpgbbQNEqi9dRPObc3EL/GSYo+hI+eWnYrWec/HuKQ+oG
6SuotZYF1hqNhr2Onhnoerxe2o+SohNgy01oUhI+dWsjKbRYobhofxNJ3FWGCyR/
7Xh7RcatHvHlOM6JQ8rFjxB9X5WMhIhkc/O3dZLX8AKWIbfGCN0eudLUdqU+WV9X
/rDfXj7aRhKKt3bUHvcIx+H6ieCrdifYXz8bBquJxTVPctS/pI9NoIREPwC0iYPE
+NyzDcECgYEA9sy1chbhTixHFyXC+C5fT0Sj3HZq4L1be9QA1q+XjcOncPnEf75t
YmbhXP5RnAUB8RW1bnU3n7Vty6RzfXz6PC0qoPFm6xrKZ4p+yn92yI9/6a4VSObt
IlBU3VHx0kxoSk/c/H//JTGGwa1RVGLN6MV8W+t8dTYWK+MDd5wztP8CgYEA1T7x
yb9SitDFcaT6oApEye5FWSC+CkB5BNomEOtOeDkZRoBcO72x4TXRmOT7oVFU5F3x
0qmnlPpWjWiMnNBzh85fugKquwRjXFzWMSpVMPlHm7sTCNO5CKb+RKbK68qUvMzc
Y4BGCufCvvaQU/FxidtdKeRzZlVfABG+pFfbA+ECgYBBzrX3FPjAwne2SWBikuLh
HRlgWMcI5BT3wMD0fd+4clo8eq0Vru411d7zz/Bs3Lz2zuYQ7PqHAHalXVVaOa/z
yctbHONnfz5HO5uxXSmMMw9VfRC53rGOe8MVPJtxiuQoJIF1Zp/fCAS5sgBEsw/a
qIYPcIxAKMriquaqxyDWewKBgQCXIIre/haThq3HgrKUJXLm4VSIe+ny/gpGZAxC
RWFRVrYQ/vte42tjPm8SuoWSqD9PsTymndHEhT497XBp2llmT94Lx8QT0mJQnQK3
yVai5KfZOFWfFd22whLFuKdrQCD1RQKUCd6Z7/JWwAs9Uomyt6JpBBy805gGRo0j
j5gKQQKBgBTuxL1gbWJjmuRXPQrgbg7csn2UaDvKkbz3ibrSnD9BOHvm9aC2c3nY
IPjdKA1DW7i9vFcKVdwqxOnWT2/wYv2FPkV+HrxWh8KX7ZwCAWveo60g53Sr457m
s9pb3b/IYa6Tnxo6cPdhwZ3CrLlq/1IopES1SmvaS4dgMFmf/0vk
-----END RSA PRIVATE KEY-----`
)

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
	}
}
