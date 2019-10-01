package contour

import (
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSecretCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*auth.Secret
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: secretmap(
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			want: []proto.Message{
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sc SecretCache
			sc.Update(tc.contents)
			got := sc.Contents()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestSecretCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*auth.Secret
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: secretmap(
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			query: []string{"default/secret/28337303ac"},
			want: []proto.Message{
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
		},
		"partial match": {
			contents: secretmap(
				secret("default/secret-a/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5fe9f5601b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			),
			query: []string{"default/secret/28337303ac", "default/secret-b/5fe9f5601b"},
			want: []proto.Message{
				secret("default/secret-b/5fe9f5601b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			},
		},
		"no match": {
			contents: secretmap(
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			query: []string{"default/secret-b/5fe9f5601b"},
			want:  nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sc SecretCache
			sc.Update(tc.contents)
			got := sc.Query(tc.query)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestSecretVisit(t *testing.T) {
	tests := map[string]struct {
		objs []interface{}
		want map[string]*auth.Secret
	}{
		"nothing": {
			objs: nil,
			want: map[string]*auth.Secret{},
		},
		"unassociated secrets": {
			objs: []interface{}{
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			},
			want: map[string]*auth.Secret{},
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
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
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
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"omg.example.com"},
							SecretName: "secret",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
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
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret-a",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				&v1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: v1beta1.IngressSpec{
						TLS: []v1beta1.IngressTLS{{
							Hosts:      []string{"omg.example.com"},
							SecretName: "secret-b",
						}},
						Backend: &v1beta1.IngressBackend{
							ServiceName: "kuard",
							ServicePort: intstr.FromInt(8080),
						},
					},
				},
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret-a/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5fe9f5601b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			),
		},
		"simple ingressroute with secret": {
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
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &ingressroutev1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []ingressroutev1.Route{{
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple ingressroutes with shared secret": {
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
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &ingressroutev1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []ingressroutev1.Route{{
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.other.com",
							TLS: &ingressroutev1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []ingressroutev1.Route{{
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple ingressroutes with different secret": {
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
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &ingressroutev1.TLS{
								SecretName: "secret-a",
							},
						},
						Routes: []ingressroutev1.Route{{
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&ingressroutev1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: ingressroutev1.IngressRouteSpec{
						VirtualHost: &ingressroutev1.VirtualHost{
							Fqdn: "www.other.com",
							TLS: &ingressroutev1.TLS{
								SecretName: "secret-b",
							},
						},
						Routes: []ingressroutev1.Route{{
							Services: []ingressroutev1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret-a/28337303ac", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5fe9f5601b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := buildDAG(t, tc.objs...)
			got := visitSecrets(root)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

// buildDAG produces a dag.DAG from the supplied objects.
func buildDAG(t *testing.T, objs ...interface{}) *dag.DAG {
	builder := dag.Builder{
		Source: dag.KubernetesCache{
			FieldLogger: testLogger(t),
		},
	}

	for _, o := range objs {
		builder.Source.Insert(o)
	}
	return builder.Build()
}

func secretmap(secrets ...*auth.Secret) map[string]*auth.Secret {
	m := make(map[string]*auth.Secret)
	for _, s := range secrets {
		m[s.Name] = s
	}
	return m
}

func secret(name string, data map[string][]byte) *auth.Secret {
	return &auth.Secret{
		Name: name,
		Type: &auth.Secret_TlsCertificate{
			TlsCertificate: &auth.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: data[v1.TLSCertKey],
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
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

	CERTIFICATE_2 = `-----BEGIN CERTIFICATE-----
MIICMzCCAZygAwIBAgIJALiPnVsvq8dsMA0GCSqGSIb3DQEBBQUAMFMxCzAJBgNV
BAYTAlVTMQwwCgYDVQQIEwNmb28xDDAKBgNVBAcTA2ZvbzEMMAoGA1UEChMDZm9v
MQwwCgYDVQQLEwNmb28xDDAKBgNVBAMTA2ZvbzAeFw0xMzAzMTkxNTQwMTlaFw0x
ODAzMTgxNTQwMTlaMFMxCzAJBgNVBAYTAlVTMQwwCgYDVQQIEwNmb28xDDAKBgNV
BAcTA2ZvbzEMMAoGA1UEChMDZm9vMQwwCgYDVQQLEwNmb28xDDAKBgNVBAMTA2Zv
bzCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEAzdGfxi9CNbMf1UUcvDQh7MYB
OveIHyc0E0KIbhjK5FkCBU4CiZrbfHagaW7ZEcN0tt3EvpbOMxxc/ZQU2WN/s/wP
xph0pSfsfFsTKM4RhTWD2v4fgk+xZiKd1p0+L4hTtpwnEw0uXRVd0ki6muwV5y/P
+5FHUeldq+pgTcgzuK8CAwEAAaMPMA0wCwYDVR0PBAQDAgLkMA0GCSqGSIb3DQEB
BQUAA4GBAJiDAAtY0mQQeuxWdzLRzXmjvdSuL9GoyT3BF/jSnpxz5/58dba8pWen
v3pj4P3w5DoOso0rzkZy2jEsEitlVM2mLSbQpMM+MUVQCQoiG6W9xuCFuxSrwPIS
pAqEAuV4DNoxQKKWmhVv+J0ptMWD25Pnpxeq5sXzghfJnslJlQND
-----END CERTIFICATE-----`

	RSA_PRIVATE_KEY = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----`
)

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
	}
}
