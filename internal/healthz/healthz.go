package healthz

import (
	"net"
	"net/http"
	"strconv"

	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/json"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Healthz(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("ok"))
}

func Register(t *contour.Translator, ds *json.DataSource, ns, healthzEndpoint string) error {
	host, p, err := net.SplitHostPort(healthzEndpoint)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return err
	}
	e := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour_healthz",
			Namespace: ns,
		},
		Subsets: []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: host,
			}},
			Ports: []v1.EndpointPort{{
				Port: int32(port),
			}},
		}},
	}
	s := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour_healthz",
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   v1.ProtocolTCP,
				Port:       int32(port),
				TargetPort: intstr.FromInt(port),
			}},
		},
	}
	i := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour_healthz",
			Namespace: ns,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "localhost",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "contour_healthz",
								ServicePort: intstr.FromInt(port),
							},
						}},
					}},
			}},
		},
	}

	t.OnAdd(e)
	t.OnAdd(s)
	t.OnAdd(i)

	ds.AddEndpoints(e)
	ds.AddService(s)
	ds.AddIngress(i)
	return nil
}
