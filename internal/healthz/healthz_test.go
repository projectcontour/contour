package healthz

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/json"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v2 "github.com/envoyproxy/go-control-plane/api"
)

func TestHealthz(t *testing.T) {
	want := "ok"
	path := "/healthz"
	got := request(t, path, Healthz)
	if got != want {
		t.Fatalf("%q: expected: %q, got %q", path, want, got)
	}
}

func TestRegister(t *testing.T) {
	ns := "testNS"
	endpoint := "ahost:1234"
	ds := json.DataSource{}
	translator := contour.Translator{}
	Register(&translator, &ds, ns, endpoint)

	testDataSource(t, &ds, ns)
	testTranslator(t, &translator, ns)
}

func testDataSource(t *testing.T, ds *json.DataSource, ns string) {
	var ingresses []*v1beta1.Ingress
	ds.IngressCache.Each(func(i *v1beta1.Ingress) {
		ingresses = append(ingresses, i)
	})
	iwant := []*v1beta1.Ingress{{
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
								ServicePort: intstr.FromInt(1234),
							},
						}},
					}},
			}},
		},
	}}
	if !reflect.DeepEqual(ingresses, iwant) {
		t.Fatalf("ingresscache: expected: %q, got %q", iwant, ingresses)
	}

	var services []*v1.Service
	ds.ServiceCache.Each(func(s *v1.Service) {
		services = append(services, s)
	})
	swant := []*v1.Service{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour_healthz",
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   v1.ProtocolTCP,
				Port:       int32(1234),
				TargetPort: intstr.FromInt(1234),
			}},
		},
	}}
	if !reflect.DeepEqual(services, swant) {
		t.Fatalf("servicecache: expected: %q, got %q", swant, services)
	}

	var endpoints []*v1.Endpoints
	ds.EndpointsCache.Each(func(e *v1.Endpoints) {
		endpoints = append(endpoints, e)
	})
	ewant := []*v1.Endpoints{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour_healthz",
			Namespace: ns,
		},
		Subsets: []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "ahost",
			}},
			Ports: []v1.EndpointPort{{
				Port: int32(1234),
			}},
		}},
	}}
	if !reflect.DeepEqual(endpoints, ewant) {
		t.Fatalf("ingress: expected: %q, got %q", ewant, endpoints)
	}
}

func testTranslator(t *testing.T, translator *contour.Translator, ns string) {
	clusters := translator.ClusterCache.Values()
	cwant := []*v2.Cluster{{
		Name: ns + "/contour_healthz/1234",
		Type: v2.Cluster_EDS,
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig: &v2.ConfigSource{
				ConfigSourceSpecifier: &v2.ConfigSource_ApiConfigSource{
					ApiConfigSource: &v2.ApiConfigSource{
						ApiType:     v2.ApiConfigSource_GRPC,
						ClusterName: []string{"xds_cluster"},
					},
				},
			},
			ServiceName: ns + "/contour_healthz/1234",
		},
		ConnectTimeout: 250 * time.Millisecond,
		LbPolicy:       v2.Cluster_ROUND_ROBIN,
	}}
	if !reflect.DeepEqual(clusters, cwant) {
		t.Fatalf("clusters: expected: %q, got %q", cwant, clusters)
	}
	clusterLoads := translator.ClusterLoadAssignmentCache.Values()
	clwant := []*v2.ClusterLoadAssignment{{
		ClusterName: ns + "/contour_healthz/1234",
		Endpoints: []*v2.LocalityLbEndpoints{{
			Locality: &v2.Locality{
				Region:  "ap-southeast-2",
				Zone:    "2b",
				SubZone: "banana",
			},
			LbEndpoints: []*v2.LbEndpoint{{
				Endpoint: &v2.Endpoint{
					Address: &v2.Address{
						Address: &v2.Address_SocketAddress{
							SocketAddress: &v2.SocketAddress{
								Protocol: v2.SocketAddress_TCP,
								Address:  "ahost",
								PortSpecifier: &v2.SocketAddress_PortValue{
									PortValue: uint32(1234),
								},
							},
						},
					},
				},
			}},
		}},
		Policy: &v2.ClusterLoadAssignment_Policy{
			DropOverload: 0.0,
		},
	}}
	if !reflect.DeepEqual(clusterLoads, clwant) {
		t.Fatalf("clusterloadassignment: expected: %q, got %q", clwant, clusterLoads)
	}
	vhosts := translator.VirtualHostCache.HTTP.Values()
	vhwant := []*v2.VirtualHost{{
		Name:    "localhost",
		Domains: []string{"localhost"},
		Routes: []*v2.Route{{
			Match:  &v2.RouteMatch{PathSpecifier: &v2.RouteMatch_Prefix{Prefix: "/"}},
			Action: &v2.Route_Route{Route: &v2.RouteAction{ClusterSpecifier: &v2.RouteAction_Cluster{Cluster: ns + "/contour_healthz/1234"}}},
		}},
	}}
	if !reflect.DeepEqual(vhosts, vhwant) {
		t.Fatalf("http virtualhosts: expected: %q, got %q", vhwant, vhosts)
	}
	vhosts = translator.VirtualHostCache.HTTPS.Values()
	vhwant = []*v2.VirtualHost{}
	if !reflect.DeepEqual(vhosts, vhwant) {
		t.Fatalf("https virtualhosts: expected: %q, got %q", vhwant, vhosts)
	}
}

func request(t *testing.T, path string, h http.HandlerFunc) string {
	r := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("%v: got %d, want: 200", path, w.Code)
	}
	return w.Body.String()
}
