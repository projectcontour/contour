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

package featuretests

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/envoy"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestMirrorPolicy(t *testing.T) {
	rh, c, done := setup(t, func(reh *contour.EventHandler) {
		reh.Builder.Source.IngressClass = "linkerd"
	})
	defer done()

	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	svc2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: svc1.Namespace,
		},
		Spec: svc1.Spec,
	}
	rh.OnAdd(svc1)
	rh.OnAdd(svc2)

	p1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: svc1.Namespace,
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{Fqdn: "example.com"},
			Routes: []projcontour.Route{{
				Conditions: prefixCondition("/"),
				Services: []projcontour.Service{{
					Name: svc1.Name,
					Port: 8080,
				}, {
					Name:   svc2.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}
	rh.OnAdd(p1)

	c.Request(routeType).Equals(&v2.DiscoveryResponse{
		Resources: resources(t,
			envoy.RouteConfiguration("ingress_http",
				envoy.VirtualHost(p1.Spec.VirtualHost.Fqdn,
					envoy.Route(envoy.RoutePrefix("/"),
						withMirrorPolicy(routeCluster("default/kuard/8080/da39a3ee5e"), "default/kuarder/8080/da39a3ee5e")),
				),
			),
			envoy.RouteConfiguration("ingress_https"),
		),
		TypeUrl: routeType,
	})
}
