package k8s

import (
	"testing"

	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
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
		{"Pod", &v1.Pod{}},
		{"Ingress", &v1beta1.Ingress{}},
		{"HTTPProxy", &projectcontour.HTTPProxy{}},
		{"TLSCertificateDelegation", &projectcontour.TLSCertificateDelegation{}},
		{"Foo", &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.projectcontour.io/v1",
				"kind":       "Foo",
			}},
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.Kind, KindOf(c.Obj))
	}
}

func TestVersionOf(t *testing.T) {
	cases := []struct {
		Version string
		Obj     interface{}
	}{
		{"v1", &v1.Secret{}},
		{"v1", &v1.Service{}},
		{"v1", &v1.Endpoints{}},
		{"networking.k8s.io/v1beta1", &v1beta1.Ingress{}},
		{"projectcontour.io/v1", &projectcontour.HTTPProxy{}},
		{"projectcontour.io/v1", &projectcontour.TLSCertificateDelegation{}},
		{"test.projectcontour.io/v1", &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.projectcontour.io/v1",
				"kind":       "Foo",
			}},
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.Version, VersionOf(c.Obj))
	}
}
