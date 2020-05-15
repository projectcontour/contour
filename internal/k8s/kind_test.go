package k8s

import (
	"testing"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestKindOf(t *testing.T) {
	cases := []struct {
		Kind string
		Obj  interface{}
	}{
		{"Secret", &v1.Secret{}},
		{"Service", &v1.Service{}},
		{"Endpoints", &v1.Endpoints{}},
		{"", &v1.Pod{}},
		{"Ingress", &v1beta1.Ingress{}},
		{"IngressRoute", &ingressroutev1.IngressRoute{}},
		{"HTTPProxy", &projectcontour.HTTPProxy{}},
		{"TLSCertificateDelegation", &ingressroutev1.TLSCertificateDelegation{}},
		{"TLSCertificateDelegation", &projectcontour.TLSCertificateDelegation{}},
		{"Foo", &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.projectcontour.io/v1",
				"kind":       "Foo",
			}},
		},
	}

	for _, c := range cases {
		kindOf := KindOf(c.Obj)
		if kindOf != c.Kind {
			t.Errorf("got %q for KindOf(%T), wanted %q",
				kindOf, c.Obj, c.Kind)
		}
	}
}
