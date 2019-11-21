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

package contour

import (
	"testing"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/metrics"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIngressRouteMetrics(t *testing.T) {
	// ir1 is a valid ingressroute
	ir1 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}, {
				Match: "/prefix",
				Delegate: &ingressroutev1.Delegate{
					Name: "delegated",
				}},
			},
		},
	}

	// ir2 is invalid because it contains a service with negative port
	ir2 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	// ir3 is invalid because it lives outside the roots namespace
	ir3 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foobar",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir4 is invalid because its match prefix does not match its parent's (ir1)
	ir4 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/doesnotmatch",
				Services: []ingressroutev1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// ir6 is invalid because it delegates to itself, producing a cycle
	ir6 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "self",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "self",
				},
			}},
		},
	}

	// ir7 delegates to ir8, which is invalid because it delegates back to ir7
	ir7 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "child",
				},
			}},
		},
	}

	ir8 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "child",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "parent",
				},
			}},
		},
	}

	// ir9 is invalid because it has a route that both delegates and has a list of services
	ir9 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "child",
				},
				Services: []ingressroutev1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// ir10 delegates to ir11 and ir 12.
	ir10 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "validChild",
				},
			}, {
				Match: "/bar",
				Delegate: &ingressroutev1.Delegate{
					Name: "invalidChild",
				},
			}},
		},
	}

	ir11 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "validChild",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// ir12 is invalid because it contains an invalid port
	ir12 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			Routes: []ingressroutev1.Route{{
				Match: "/bar",
				Services: []ingressroutev1.Service{{
					Name: "foo",
					Port: 12345678,
				}},
			}},
		},
	}

	// ir13 is invalid because it does not specify and FQDN
	ir13 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Services: []ingressroutev1.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// ir14 delegates tp ir15 but it is invalid because it is missing fqdn
	ir14 := &ingressroutev1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: ingressroutev1.IngressRouteSpec{
			VirtualHost: &projcontour.VirtualHost{},
			Routes: []ingressroutev1.Route{{
				Match: "/foo",
				Delegate: &ingressroutev1.Delegate{
					Name: "validChild",
				},
			}},
		},
	}

	// proxy1 is a valid httpproxy
	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy2 is invalid because it contains a service with negative port
	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	// proxy3 is invalid because it lives outside the roots namespace
	proxy3 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foobar",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	//// proxy4 is invalid because its match prefix does not match its parent's (proxy1)
	//proxy4 := &projcontour.HTTPProxy{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Namespace: "roots",
	//		Name:      "delegated",
	//	},
	//	Spec: projcontour.HTTPProxySpec{
	//		Routes: []projcontour.Route{{
	//			Conditions: []projcontour.Condition{{
	//				Prefix: "/doesnotmatch",
	//			}},
	//			Services: []projcontour.Service{{
	//				Name: "home",
	//				Port: 8080,
	//			}},
	//		}},
	//	},
	//}

	// proxy6 is invalid because it delegates to itself, producing a cycle
	proxy6 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "self",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "self",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	// proxy7 delegates to proxy8, which is invalid because it delegates back to proxy7
	proxy7 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "child",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxy8 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "child",
		},
		Spec: projcontour.HTTPProxySpec{
			Includes: []projcontour.Include{{
				Name: "parent",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	//// proxy9 is invalid because it has a route that both delegates and has a list of services
	//proxy9 := &projcontour.HTTPProxy{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Namespace: "roots",
	//		Name:      "parent",
	//	},
	//	Spec: projcontour.HTTPProxySpec{
	//		VirtualHost: &projcontour.VirtualHost{
	//			Fqdn: "example.com",
	//		},
	//		Includes: []projcontour.Include{{
	//			Name: "child",
	//			Conditions: []projcontour.Condition{{
	//				Prefix: "/foo",
	//			}},
	//		}},
	//		Routes: []projcontour.Route{{
	//			Services: []projcontour.Service{{
	//				Name: "kuard",
	//				Port: 8080,
	//			}},
	//		}},
	//	},
	//}

	// proxy10 delegates to proxy11 and proxy12.
	proxy10 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "validChild",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "invalidChild",
				Conditions: []projcontour.Condition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxy11 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "validChild",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy12 is invalid because it contains an invalid port
	proxy12 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/bar",
				}},
				Services: []projcontour.Service{{
					Name: "foo",
					Port: 12345678,
				}},
			}},
		},
	}

	// proxy13 is invalid because it does not specify and FQDN
	proxy13 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	proxy14 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{},
			Includes: []projcontour.Include{{
				Name: "validChild",
				Conditions: []projcontour.Condition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "foo",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     12345678,
			}},
		},
	}

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "foo",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "home",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	tests := map[string]struct {
		objs           []interface{}
		wantIR         *metrics.RouteMetric
		wantProxy      *metrics.RouteMetric
		rootNamespaces []string
	}{
		"valid ingressroute": {
			objs: []interface{}{ir1, s3},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
			wantProxy: nil,
		},
		"invalid port in service": {
			objs: []interface{}{ir2},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
			wantProxy: nil,
		},
		"root ingressroute outside of roots namespace": {
			objs: []interface{}{ir3},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "finance"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "finance"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "finance"}: 1,
				},
			},
			wantProxy:      nil,
			rootNamespaces: []string{"foo"},
		},
		"delegated route's match prefix does not match parent's prefix": {
			objs: []interface{}{ir1, ir4, s3},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 2,
				},
			},
			wantProxy: nil,
		},
		"root ingressroute does not specify FQDN": {
			objs: []interface{}{ir13},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
			wantProxy: nil,
		},
		"self-edge produces a cycle": {
			objs: []interface{}{ir6},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
			wantProxy: nil,
		},
		"child delegates to parent, producing a cycle": {
			objs: []interface{}{ir7, ir8},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 2,
				},
			},
			wantProxy: nil,
		},
		"route has a list of services and also delegates": {
			objs: []interface{}{ir9},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
			wantProxy: nil,
		},
		"ingressroute is an orphaned route": {
			objs: []interface{}{ir8},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{},
				Valid:   map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Root: map[metrics.Meta]int{},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
			wantProxy: nil,
		},
		"ingressroute delegates to multiple ingressroutes, one is invalid": {
			objs: []interface{}{ir10, ir11, ir12, s1, s2},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots"}:                       1,
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 3,
				},
			},
			wantProxy: nil,
		},
		"invalid parent orphans children": {
			objs: []interface{}{ir14, ir11},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 2,
				},
			},
			wantProxy: nil,
		},
		"multi-parent children is not orphaned when one of the parents is invalid": {
			objs: []interface{}{ir14, ir11, ir10, s2},
			wantIR: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
					{Namespace: "roots"}:                       1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 2,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 3,
				},
			},
			wantProxy: nil,
		},
		"valid proxy": {
			objs:   []interface{}{proxy1, s3},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
		},
		"invalid port in service - proxy": {
			objs:   []interface{}{proxy2},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
		},
		"root proxy outside of roots namespace": {
			objs:   []interface{}{proxy3},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "finance"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "finance"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "finance"}: 1,
				},
			},
			rootNamespaces: []string{"foo"},
		},
		"root proxy does not specify FQDN": {
			objs:   []interface{}{proxy13},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
		},
		"self-edge produces a cycle - proxy": {
			objs:   []interface{}{proxy6},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Valid:    map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
		},
		"child delegates to parent, producing a cycle - proxy": {
			objs:   []interface{}{proxy7, proxy8},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 2,
				},
			},
		},
		"proxy is an orphaned route": {
			objs:   []interface{}{proxy8},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{},
				Valid:   map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Root: map[metrics.Meta]int{},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
			},
		},
		"proxy delegates to multiple proxies, one is invalid": {
			objs:   []interface{}{proxy10, proxy11, proxy12, s1, s2},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots"}:                       1,
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 3,
				},
			},
		},
		"invalid parent orphans children - proxy": {
			objs:   []interface{}{proxy14, proxy11},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Valid: map[metrics.Meta]int{},
				Orphaned: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 2,
				},
			},
		},
		"multi-parent children is not orphaned when one of the parents is invalid - proxy": {
			objs:   []interface{}{proxy14, proxy11, proxy10, s2},
			wantIR: nil,
			wantProxy: &metrics.RouteMetric{
				Invalid: map[metrics.Meta]int{
					{Namespace: "roots"}:                       1,
					{Namespace: "roots", VHost: "example.com"}: 1,
				},
				Valid: map[metrics.Meta]int{
					{Namespace: "roots"}: 1,
				},
				Orphaned: map[metrics.Meta]int{},
				Root: map[metrics.Meta]int{
					{Namespace: "roots"}: 2,
				},
				Total: map[metrics.Meta]int{
					{Namespace: "roots"}: 3,
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := dag.Builder{
				Source: dag.KubernetesCache{
					RootNamespaces: tc.rootNamespaces,
					FieldLogger:    testLogger(t),
				},
			}
			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			gotIR, gotProxy := calculateRouteMetric(dag.Statuses())
			if tc.wantIR != nil {
				assert.Equal(t, *tc.wantIR, gotIR)
			}
			if tc.wantProxy != nil {
				assert.Equal(t, *tc.wantProxy, gotProxy)
			}
		})
	}
}
