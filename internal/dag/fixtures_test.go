package dag

import (
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Fixtures struct {
	secrets       map[string]*v1.Secret
	services      map[string]*v1.Service
	ingresses     map[string]*v1beta1.Ingress
	ingressRoutes map[string]*ingressroutev1.IngressRoute
	httpProxies   map[string]*projcontour.HTTPProxy
}

func (f *Fixtures) Secret(s string) *v1.Secret {
	return f.secrets[s]
}

func (f *Fixtures) Service(s string) *v1.Service {
	return f.services[s]
}

func (f *Fixtures) Ingress(s string) *v1beta1.Ingress {
	return f.ingresses[s]
}

func (f *Fixtures) IngressRoute(s string) *ingressroutev1.IngressRoute {
	return f.ingressRoutes[s]
}

func (f *Fixtures) HTTPProxy(s string) *projcontour.HTTPProxy {
	return f.httpProxies[s]
}

func NewFixtures() *Fixtures {
	f := Fixtures{
		secrets:       map[string]*v1.Secret{},
		services:      map[string]*v1.Service{},
		ingresses:     map[string]*v1beta1.Ingress{},
		ingressRoutes: map[string]*ingressroutev1.IngressRoute{},
		httpProxies:   map[string]*projcontour.HTTPProxy{},
	}

	f.secrets["sec1"] = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}

	return &f
}
