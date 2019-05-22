package contour

import (
	"reflect"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
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
				secret("default/secret/cd1b506996", "cert", "key"),
			),
			want: []proto.Message{
				secret("default/secret/cd1b506996", "cert", "key"),
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
				secret("default/secret/cd1b506996", "cert", "key"),
			),
			query: []string{"default/secret/cd1b506996"},
			want: []proto.Message{
				secret("default/secret/cd1b506996", "cert", "key"),
			},
		},
		"partial match": {
			contents: secretmap(
				secret("default/secret-a/ff2a9f58ca", "cert-a", "key-a"),
				secret("default/secret-b/0a068be4ba", "cert-b", "key-b"),
			),
			query: []string{"default/secret/cd1b506996", "default/secret-b/0a068be4ba"},
			want: []proto.Message{
				secret("default/secret-b/0a068be4ba", "cert-b", "key-b"),
			},
		},
		"no match": {
			contents: secretmap(
				secret("default/secret/cd1b506996", "cert", "key"),
			),
			query: []string{"default/secret-b/0a068be4ba"},
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-a",
						Namespace: "default",
					},
					Data: secretdata("cert", "key"),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-b",
						Namespace: "default",
					},
					Data: secretdata("cert", "key"),
				},
			},
			want: map[string]*auth.Secret{},
		},
		"simple ingress with secret": {
			objs: []interface{}{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("cert", "key"),
				},
			},
			want: secretmap(
				secret("default/secret/cd1b506996", "cert", "key"),
			),
		},
		"multiple ingresses with shared secret": {
			objs: []interface{}{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("cert", "key"),
				},
			},
			want: secretmap(
				secret("default/secret/cd1b506996", "cert", "key"),
			),
		},
		"multiple ingresses with different secrets": {
			objs: []interface{}{
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
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-a",
						Namespace: "default",
					},
					Data: secretdata("cert-a", "key-a"),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-b",
						Namespace: "default",
					},
					Data: secretdata("cert-b", "key-b"),
				},
			},
			want: secretmap(
				secret("default/secret-a/ff2a9f58ca", "cert-a", "key-a"),
				secret("default/secret-b/0a068be4ba", "cert-b", "key-b"),
			),
		},
		"simple ingressroute with secret": {
			objs: []interface{}{
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
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("cert", "key"),
				},
			},
			want: secretmap(
				secret("default/secret/cd1b506996", "cert", "key"),
			),
		},
		"multiple ingressroutes with shared secret": {
			objs: []interface{}{
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
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
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
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: secretdata("cert", "key"),
				},
			},
			want: secretmap(
				secret("default/secret/cd1b506996", "cert", "key"),
			),
		},
		"multiple ingressroutes with different secret": {
			objs: []interface{}{
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
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
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
						Routes: []ingressroutev1.Route{
							{
								Services: []ingressroutev1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-a",
						Namespace: "default",
					},
					Data: secretdata("cert-a", "key-a"),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-b",
						Namespace: "default",
					},
					Data: secretdata("cert-b", "key-b"),
				},
			},
			want: secretmap(
				secret("default/secret-a/ff2a9f58ca", "cert-a", "key-a"),
				secret("default/secret-b/0a068be4ba", "cert-b", "key-b"),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reh := ResourceEventHandler{
				FieldLogger: testLogger(t),
				Notifier:    new(nullNotifier),
				Metrics:     metrics.NewMetrics(prometheus.NewRegistry()),
			}
			for _, o := range tc.objs {
				reh.OnAdd(o)
			}
			root := reh.Build()
			got := visitSecrets(root)
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%+v\ngot:\n%+v", tc.want, got)
			}
		})
	}
}

func secretmap(secrets ...*auth.Secret) map[string]*auth.Secret {
	m := make(map[string]*auth.Secret)
	for _, s := range secrets {
		m[s.Name] = s
	}
	return m
}

func secret(name, cert, key string) *auth.Secret {
	return &auth.Secret{
		Name: name,
		Type: &auth.Secret_TlsCertificate{
			TlsCertificate: &auth.TlsCertificate{
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(key),
					},
				},
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(cert),
					},
				},
			},
		},
	}
}
