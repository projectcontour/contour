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

package dag

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/timeout"
)

func TestDAGInsertGatewayAPI(t *testing.T) {
	kuardService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard2",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardService3 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard3",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	kuardServiceCustomNs := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "custom",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	blogService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blogsvc",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 80, 8080)},
		},
	}

	tlsService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "tlssvc",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("https", "TCP", 443, 8443)},
		},
	}

	tlsAndNonTLSService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "tlsandnontlssvc",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{
				makeServicePort("http", "TCP", 80, 8080),
				makeServicePort("https", "TCP", 443, 8443),
			},
		},
	}

	validClass := &gatewayapi_v1.GatewayClass{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-validClass",
		},
		Spec: gatewayapi_v1.GatewayClassSpec{
			ControllerName: "projectcontour.io/contour",
		},
		Status: gatewayapi_v1.GatewayClassStatus{
			Conditions: []meta_v1.Condition{
				{
					Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
					Status: meta_v1.ConditionTrue,
				},
			},
		},
	}

	gatewayHTTPAllNamespaces := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	gatewayHTTPSameNamespace := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromSame),
					},
				},
			}},
		},
	}

	gatewayHTTPNamespaceSelector := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
						Selector: &meta_v1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "contour",
							},
							MatchExpressions: []meta_v1.LabelSelectorRequirement{{
								Key:      "type",
								Operator: "In",
								Values:   []string{"controller"},
							}},
						},
					},
				},
			}},
		},
	}

	hostname := gatewayapi_v1.Hostname("gateway.projectcontour.io")
	wildcardHostname := gatewayapi_v1.Hostname("*.projectcontour.io")

	gatewayHTTPWithHostname := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Port:     80,
				Hostname: &hostname,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	gatewayHTTPWithWildcardHostname := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Port:     80,
				Hostname: &wildcardHostname,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	gatewayHTTPWithAddresses := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Addresses: []gatewayapi_v1.GatewayAddress{
				{
					Type:  ptr.To(gatewayapi_v1.IPAddressType),
					Value: "1.2.3.4",
				},
			},
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gatewayapi_v1.HTTPProtocolType,
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	gatewayTLSPassthroughAllNamespaces := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Port:     443,
				Protocol: gatewayapi_v1.TLSProtocolType,
				TLS: &gatewayapi_v1.GatewayTLSConfig{
					Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	gatewayTLSPassthroughSameNamespace := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Port:     443,
				Protocol: gatewayapi_v1.TLSProtocolType,
				TLS: &gatewayapi_v1.GatewayTLSConfig{
					Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromSame),
					},
				},
			}},
		},
	}

	gatewayTLSPassthroughNamespaceSelector := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Port:     443,
				Protocol: gatewayapi_v1.TLSProtocolType,
				TLS: &gatewayapi_v1.GatewayTLSConfig{
					Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
				},
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromSelector),
						Selector: &meta_v1.LabelSelector{
							MatchLabels: map[string]string{"matching-label-key": "matching-label-value"},
						},
					},
				},
			}},
		},
	}

	cert1 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ca",
			Namespace: "projectcontour",
		},
		Type: core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			CACertificateKey: []byte(fixture.CERTIFICATE),
		},
	}
	cert2 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ca2",
			Namespace: "projectcontour",
		},
		Type: core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			CACertificateKey: []byte(fixture.EC_CERTIFICATE),
		},
	}

	configMapCert1 := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ca",
			Namespace: "projectcontour",
		},
		Data: map[string]string{
			CACertificateKey: fixture.CERTIFICATE,
		},
	}

	sec1 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "projectcontour",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	sec2 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "tls-cert-namespace",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	gatewayTLSTerminateCertInDifferentNamespace := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Name:     "https",
				Port:     443,
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				TLS: &gatewayapi_v1.GatewayTLSConfig{
					Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
					CertificateRefs: []gatewayapi_v1.SecretObjectReference{
						gatewayapi.CertificateRef(sec2.Name, sec2.Namespace),
					},
				},
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	gatewayHTTPSAllNamespaces := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{{
				Port:     443,
				Protocol: gatewayapi_v1.HTTPSProtocolType,
				TLS: &gatewayapi_v1.GatewayTLSConfig{
					CertificateRefs: []gatewayapi_v1.SecretObjectReference{
						gatewayapi.CertificateRef(sec1.Name, sec1.Namespace),
					},
				},
				AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
					Namespaces: &gatewayapi_v1.RouteNamespaces{
						From: ptr.To(gatewayapi_v1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	gatewayHTTPAndHTTPS := &gatewayapi_v1.Gateway{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GatewaySpec{
			GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
			Listeners: []gatewayapi_v1.Listener{
				{
					Name:     "http-listener",
					Port:     80,
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
				{
					Name:     "https-listener",
					Port:     443,
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						CertificateRefs: []gatewayapi_v1.SecretObjectReference{
							gatewayapi.CertificateRef(sec1.Name, sec1.Namespace),
						},
					},
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
				},
			},
		},
	}

	basicHTTPRoute := &gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
			}},
		},
	}

	basicTLSRoute := &gatewayapi_v1alpha2.TLSRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
			},
			Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
			Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
				BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
			}},
		},
	}

	basicGRPCRoute := &gatewayapi_v1.GRPCRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.GRPCRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
			},
			Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
			Rules: []gatewayapi_v1.GRPCRouteRule{{
				Matches: []gatewayapi_v1.GRPCRouteMatch{{
					Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
				}},
				BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
			}},
		},
	}

	crossNSBackendHTTPRoute := makeHTTPRoute("basic", "default", "", gatewayapi_v1.HTTPRouteRule{
		Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
		BackendRefs: []gatewayapi_v1.HTTPBackendRef{{
			BackendRef: gatewayapi_v1.BackendRef{
				BackendObjectReference: gatewayapi_v1.BackendObjectReference{
					Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
					Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
					Name:      gatewayapi_v1.ObjectName(kuardService.Name),
					Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
				},
				Weight: ptr.To(int32(1)),
			},
		}},
	})

	tests := map[string]struct {
		objs         []any
		gatewayclass *gatewayapi_v1.GatewayClass
		gateway      *gatewayapi_v1.Gateway
		upstreamTLS  *UpstreamTLS
		want         []*Listener
	}{
		"insert basic single route, single hostname": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"gateway with addresses is unsupported": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPWithAddresses,
			objs: []any{
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"gateway without a gatewayclass": {
			gateway: gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway same namespace selector, route in gateway's namespace": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPSameNamespace,
			objs: []any{
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway same namespace selector, route in different namespace": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPSameNamespace,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "different-ns-than-gateway", "test.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway From namespace selector": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPNamespaceSelector,
			objs: []any{
				&core_v1.Namespace{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
				},
				kuardServiceCustomNs,
				makeHTTPRoute("basic", "custom", "test.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardServiceCustomNs))),
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway From namespace selector, not matching": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPNamespaceSelector,
			objs: []any{
				&core_v1.Namespace{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "notmatch",
							"type": "someother",
						},
					},
				},
				kuardServiceCustomNs,
				makeHTTPRoute("basic", "custom", "test.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(),
		},

		"HTTPRoute does not include the gateway in its list of parent refs": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.HTTPRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								gatewayapi.GatewayParentRef("projectcontour", "some-other-gateway"),
								gatewayapi.GatewayParentRef("projectcontour", "some-other-gateway-2"),
							},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.HTTPRouteRule{{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},

		// BEGIN TLSRoute<->Gateway selection test cases
		"TLSRoute: Gateway selects TLSRoutes in all namespaces": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardServiceCustomNs,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "test.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardServiceCustomNs)),
							},
						},
					),
				},
			),
		},
		"TLSRoute: Gateway selects TLSRoutes in same namespace, and route is in the same namespace": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughSameNamespace,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: gatewayTLSPassthroughSameNamespace.Namespace,
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "test.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
					),
				},
			),
		},
		"TLSRoute: Gateway selects TLSRoutes in same namespace, and route is not in the same namespace": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughSameNamespace,
			objs: []any{
				kuardServiceCustomNs,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: kuardServiceCustomNs.Namespace,
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute: Gateway selects TLSRoutes in namespaces matching selector, and route is in a matching namespace": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughNamespaceSelector,
			objs: []any{
				kuardServiceCustomNs,
				&core_v1.Namespace{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:   kuardServiceCustomNs.Namespace,
						Labels: map[string]string{"matching-label-key": "matching-label-value"},
					},
				},
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: kuardServiceCustomNs.Namespace,
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "test.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardServiceCustomNs)),
							},
						},
					),
				},
			),
		},
		"TLSRoute: Gateway selects TLSRoutes in namespaces matching selector, and route is in a non-matching namespace": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughNamespaceSelector,
			objs: []any{
				kuardServiceCustomNs,
				&core_v1.Namespace{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:   kuardServiceCustomNs.Namespace,
						Labels: map[string]string{"matching-label-key": "this-label-value-does-not-match"},
					},
				},
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: kuardServiceCustomNs.Namespace,
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(),
		},

		"TLSRoute: Gateway selects non-TLSRoutes": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces, // selects HTTPRoutes, not TLSRoutes
			objs: []any{
				kuardService,
				basicTLSRoute,
			},
			want: listeners(),
		},

		"TLSRoute: TLSRoute allows Gateways from list, and gateway is not in the list": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("some-other-namespace", "some-other-gateway-name")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"test.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(),
		},
		// END TLSRoute<->Gateway selection test cases

		"TLS Listener with TLS.Mode=Passthrough is invalid if certificateRef is specified": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.TLSProtocolType,
						TLS: &gatewayapi_v1.GatewayTLSConfig{
							Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
							CertificateRefs: []gatewayapi_v1.SecretObjectReference{
								gatewayapi.CertificateRef(sec1.Name, sec1.Namespace),
							},
						},
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				kuardService,
				basicTLSRoute,
			},
			want: listeners(),
		},
		"TLS Listener with TLS.Mode=Terminate is invalid if certificateRef is not specified": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName(validClass.Name),
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.TLSProtocolType,
						TLS: &gatewayapi_v1.GatewayTLSConfig{
							Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
						},
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				sec1,
				kuardService,
				basicTLSRoute,
			},
			want: listeners(),
		},
		"TLS Listener with TLS not defined is invalid": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.TLSProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				kuardService,
				basicTLSRoute,
			},
			want: listeners(),
		},
		"TLSRoute with invalid listener protocol of HTTP": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						TLS: &gatewayapi_v1.GatewayTLSConfig{
							Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
						},
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				kuardService,
				basicTLSRoute,
			},
			want: listeners(),
		},
		"TLSRoute with invalid listener kind": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				basicTLSRoute,
			},
			want: listeners(),
		},
		// Issue: https://github.com/projectcontour/contour/issues/3591
		"one gateway with two httproutes, different hostnames": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				basicHTTPRoute,
				makeHTTPRoute("basic-two", "projectcontour", "another.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("another.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"insert gateway with selector kind that doesn't match": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
							Kinds: []gatewayapi_v1.RouteGroupKind{
								{
									Group: ptr.To(gatewayapi_v1.Group(gatewayapi_v1.GroupName)),
									Kind:  gatewayapi_v1.Kind("INVALID-KIND"),
								},
							},
						},
					}},
				},
			},
			objs: []any{
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"insert gateway with selector group that doesn't match": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
							Kinds: []gatewayapi_v1.RouteGroupKind{
								{
									Group: ptr.To(gatewayapi_v1.Group("invalid-group-name")),
									Kind:  gatewayapi_v1.Kind("HTTPRoute"),
								},
							},
						},
					}},
				},
			},
			objs: []any{
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic multiple routes, single hostname": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				blogService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io",
					makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1),
					makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/blog", "blogsvc", 80, 1),
					gatewayapi_v1.HTTPRouteRule{
						Matches: append(
							gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/another"),
							gatewayapi_v1.HTTPRouteMatch{
								Headers: gatewayapi.HTTPHeaderMatch(gatewayapi_v1.HeaderMatchExact, "X-Foo-Header", "some_value"),
							},
						),
						BackendRefs: gatewayapi.HTTPBackendRef("blogsvc", 80, 1),
					},
				),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost(
							"test.projectcontour.io",
							prefixrouteHTTPRoute("/", service(kuardService)),
							&Route{
								PathMatchCondition: prefixSegment("/blog"),
								Clusters:           clustersWeight(service(blogService)),
								Priority:           1,
							},
							&Route{
								PathMatchCondition: prefixSegment("/another"),
								Clusters:           clustersWeight(service(blogService)),
								Priority:           2,
							},
							&Route{
								PathMatchCondition: prefixString("/"),
								HeaderMatchConditions: []HeaderMatchCondition{
									{Name: "X-Foo-Header", Value: "some_value", MatchType: "exact"},
								},
								Clusters: clustersWeight(service(blogService)),
								Priority: 2,
							},
						),
					),
				},
			),
		},
		"multiple hosts": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.HTTPRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
							"test2.projectcontour.io",
							"test3.projectcontour.io",
							"test4.projectcontour.io",
						},
						Rules: []gatewayapi_v1.HTTPRouteRule{{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
						virtualhost("test2.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
						virtualhost("test3.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
						virtualhost("test4.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"no host defined": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"wildcard hostname": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "*.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("*.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"invalid hostnames - IP": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "192.168.122.1", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(),
		},
		"invalid hostnames - with port": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "default", "test.projectcontour.io:80", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(),
		},
		"invalid hostnames - wildcard label by itself": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "default", "*", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(),
		},
		// If the ServiceName referenced from an HTTPRoute is missing,
		// the route should return an HTTP 500.
		"missing service": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", directResponseRoute("/", http.StatusInternalServerError)),
					),
				},
			),
		},
		// If port is not defined the route will return an HTTP 500.
		"missing port": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				makeHTTPRoute("basic", "default", "", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: []gatewayapi_v1.HTTPBackendRef{
						{
							BackendRef: gatewayapi_v1.BackendRef{
								BackendObjectReference: gatewayapi_v1.BackendObjectReference{
									Kind: ptr.To(gatewayapi_v1.Kind("Service")),
									Name: "kuard",
								},
							},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", directResponseRoute("/", http.StatusInternalServerError)),
					),
				},
			),
		},
		"HTTPRoute references a backend in a different namespace, no ReferenceGrant": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				crossNSBackendHTTPRoute,
			},
			want: listeners(&Listener{
				Name: "http-80",
				VirtualHosts: virtualhosts(
					virtualhost("*", directResponseRoute("/", http.StatusInternalServerError)),
				),
			}),
		},
		"HTTPRoute references a backend in a different namespace, with valid ReferenceGrant": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				crossNSBackendHTTPRoute,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "HTTPRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(&Listener{
				Name:         "http-80",
				VirtualHosts: virtualhosts(virtualhost("*", prefixrouteHTTPRoute("/", service(kuardService)))),
			}),
		},
		"HTTPRoute references a backend in a different namespace, with valid ReferenceGrant (service-specific)": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				crossNSBackendHTTPRoute,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "HTTPRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
							Name: ptr.To(gatewayapi_v1.ObjectName(kuardService.Name)),
						}},
					},
				},
			},
			want: listeners(&Listener{
				Name:         "http-80",
				VirtualHosts: virtualhosts(virtualhost("*", prefixrouteHTTPRoute("/", service(kuardService)))),
			}),
		},
		"HTTPRoute references a backend in a different namespace, with invalid ReferenceGrant (wrong Kind)": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				crossNSBackendHTTPRoute,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "TLSRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(&Listener{
				Name: "http-80",
				VirtualHosts: virtualhosts(
					virtualhost("*", directResponseRoute("/", http.StatusInternalServerError)),
				),
			}),
		},
		"HTTPRoute references a backend in a different namespace, with invalid ReferenceGrant (grant in wrong namespace)": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				crossNSBackendHTTPRoute,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: "some-other-namespace", // would need to be "projectcontour" to be valid
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "HTTPRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(&Listener{
				Name: "http-80",
				VirtualHosts: virtualhosts(
					virtualhost("*", directResponseRoute("/", http.StatusInternalServerError)),
				),
			}),
		},
		"HTTPRoute references a backend in a different namespace, with invalid ReferenceGrant (wrong from namespace)": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				crossNSBackendHTTPRoute,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "HTTPRoute",
							Namespace: "some-other-namespace", // would need to be "default" to be valid
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(&Listener{
				Name: "http-80",
				VirtualHosts: virtualhosts(
					virtualhost("*", directResponseRoute("/", http.StatusInternalServerError)),
				),
			}),
		},
		"HTTPRoute references a backend in a different namespace, with invalid ReferenceGrant (wrong service name)": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				crossNSBackendHTTPRoute,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "HTTPRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
							Name: ptr.To(gatewayapi_v1.ObjectName("some-other-service")), // would need to be "kuard" to be valid.
						}},
					},
				},
			},
			want: listeners(&Listener{
				Name: "http-80",
				VirtualHosts: virtualhosts(
					virtualhost("*", directResponseRoute("/", http.StatusInternalServerError)),
				),
			}),
		},
		"insert basic single route with exact path match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchExact, "/blog", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							exactrouteHTTPRoute("/blog", service(kuardService))),
					),
				},
			),
		},
		"insert basic single route with regular expression path match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchRegularExpression, "/bl+og", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							regexrouteHTTPRoute("/bl+og", service(kuardService))),
					),
				},
			),
		},
		// Single host with single route containing multiple prefixes to the same service.
		"insert basic single route with multiple prefixes, single hostname": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io",
					gatewayapi_v1.HTTPRouteRule{
						Matches: []gatewayapi_v1.HTTPRouteMatch{{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}, {
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/blog"),
							},
						}, {
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/tech"),
							},
						}},
						BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							prefixrouteHTTPRoute("/", service(kuardService)),
							segmentPrefixHTTPRoute("/blog", service(kuardService)),
							segmentPrefixHTTPRoute("/tech", service(kuardService))),
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway with TLS, HTTP protocol is ignored": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1.HTTPProtocolType,
						TLS: &gatewayapi_v1.GatewayTLSConfig{
							CertificateRefs: []gatewayapi_v1.SecretObjectReference{
								gatewayapi.CertificateRef(sec1.Name, sec1.Namespace),
							},
						},
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				sec1,
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(
				&Listener{
					Name:     "http-443",
					Protocol: "http",
					Address:  "0.0.0.0",
					Port:     8443,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							prefixrouteHTTPRoute("/", service(kuardService)),
						)),
					EnableWebsockets: true,
				},
			),
		},
		"insert basic single route, single hostname, gateway with TLS, HTTPS protocol missing certificateRef": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1.HTTPSProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				sec1,
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway with TLS": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPSAllNamespaces,
			objs: []any{
				sec1,
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "test.projectcontour.io",
								Routes: routes(prefixrouteHTTPRoute("/", service(kuardService))),
							},
							Secret: secret(sec1),
						},
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway with missing TLS certificate": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPSAllNamespaces,
			objs: []any{
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway with invalid TLS certificate": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPSAllNamespaces,
			objs: []any{
				&core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tlscert",
						Namespace: "projectcontour",
					},
					Type: core_v1.SecretTypeTLS,
					Data: secretdata("wrong", "wronger"),
				},
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway with TLS & Insecure Listeners": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAndHTTPS,
			objs: []any{
				sec1,
				kuardService,
				basicHTTPRoute,
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "test.projectcontour.io",
								Routes: routes(prefixrouteHTTPRoute("/", service(kuardService))),
							},
							Secret: secret(sec1),
						},
					),
				},
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"TLS Listener Gateway CertificateRef must be type core.Secret": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1.HTTPSProtocolType,
						TLS: &gatewayapi_v1.GatewayTLSConfig{
							CertificateRefs: []gatewayapi_v1.SecretObjectReference{
								{
									Group: ptr.To(gatewayapi_v1.Group("custom")),
									Kind:  ptr.To(gatewayapi_v1.Kind("shhhh")),
									Name:  gatewayapi_v1.ObjectName(sec1.Name),
								},
							},
						},
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				sec1,
				blogService,
				basicHTTPRoute,
			},
			want: listeners(),
		},
		"TLS Listener Gateway CertificateRef must be specified": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1.HTTPSProtocolType,
						TLS:      &gatewayapi_v1.GatewayTLSConfig{},
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{
				sec1,
				blogService,
				basicHTTPRoute,
			},
			want: listeners(),
		},

		// BEGIN TLS CertificateRef + ReferenceGrant tests
		"Gateway references TLS cert in different namespace, with valid ReferenceGrant": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tls-cert-reference-grant",
						Namespace: sec2.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "Gateway",
							Namespace: gatewayapi_v1.Namespace(gatewayTLSTerminateCertInDifferentNamespace.Namespace),
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Secret",
						}},
					},
				},
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "test.projectcontour.io",
								Routes: routes(prefixrouteHTTPRoute("/", service(kuardService))),
							},
							Secret: secret(sec2),
						},
					),
				},
			),
		},
		"Gateway references TLS cert in different namespace, with no ReferenceGrant": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(),
		},
		"Gateway references TLS cert in different namespace, with valid ReferenceGrant (secret-specific)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tls-cert-reference-grant",
						Namespace: sec2.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "Gateway",
							Namespace: gatewayapi_v1.Namespace(gatewayTLSTerminateCertInDifferentNamespace.Namespace),
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Secret",
							Name: ptr.To(gatewayapi_v1.ObjectName(sec2.Name)),
						}},
					},
				},
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "test.projectcontour.io",
								Routes: routes(prefixrouteHTTPRoute("/", service(kuardService))),
							},
							Secret: secret(sec2),
						},
					),
				},
			),
		},
		"Gateway references TLS cert in different namespace, with invalid ReferenceGrant (grant in wrong namespace)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tls-cert-reference-grant",
						Namespace: "wrong-namespace",
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "Gateway",
							Namespace: gatewayapi_v1.Namespace(gatewayTLSTerminateCertInDifferentNamespace.Namespace),
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Secret",
						}},
					},
				},
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(),
		},
		"Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong From namespace)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tls-cert-reference-grant",
						Namespace: sec2.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "Gateway",
							Namespace: gatewayapi_v1.Namespace("wrong-namespace"),
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Secret",
						}},
					},
				},
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(),
		},
		"Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong From kind)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tls-cert-reference-grant",
						Namespace: sec2.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "WrongKind",
							Namespace: gatewayapi_v1.Namespace(gatewayTLSTerminateCertInDifferentNamespace.Namespace),
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Secret",
						}},
					},
				},
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(),
		},
		"Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong To kind)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tls-cert-reference-grant",
						Namespace: sec2.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "Gateway",
							Namespace: gatewayapi_v1.Namespace(gatewayTLSTerminateCertInDifferentNamespace.Namespace),
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "WrongKind",
						}},
					},
				},
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(),
		},
		"Gateway references TLS cert in different namespace, with invalid ReferenceGrant (wrong secret name)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSTerminateCertInDifferentNamespace,
			objs: []any{
				sec2,
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "tls-cert-reference-grant",
						Namespace: sec2.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "Gateway",
							Namespace: gatewayapi_v1.Namespace(gatewayTLSTerminateCertInDifferentNamespace.Namespace),
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Secret",
							Name: ptr.To(gatewayapi_v1.ObjectName("wrong-name")),
						}},
					},
				},
				basicHTTPRoute,
				kuardService,
			},
			want: listeners(),
		},

		// END CertificateRef ReferenceGrant tests

		"No valid hostnames defined": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				makeHTTPRoute("basic", "projectcontour", "*.*.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "blobsvc", 80, 1)),
			},
			want: listeners(),
		},
		"Invalid listener protocol type (TCP)": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.TCPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{basicHTTPRoute},
			want: listeners(),
		},
		"Invalid listener protocol type (UDP)": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1.UDPProtocolType,
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{basicHTTPRoute},
			want: listeners(),
		},
		"Invalid listener protocol type (custom)": {
			gatewayclass: validClass,
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					Listeners: []gatewayapi_v1.Listener{{
						Port:     80,
						Protocol: "projectcontour.io/HTTPUDP",
						AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
							Namespaces: &gatewayapi_v1.RouteNamespaces{
								From: ptr.To(gatewayapi_v1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			objs: []any{basicHTTPRoute},
			want: listeners(),
		},
		"gateway with HTTP and HTTPS listeners, each route selects a different listener": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAndHTTPS,
			objs: []any{
				sec1,
				kuardService,
				blogService,
				&gatewayapi_v1.HTTPRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http-listener", 0),
							},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.HTTPRouteRule{{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
						}},
					},
				},
				&gatewayapi_v1.HTTPRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basictls",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "https-listener", 0),
							},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.HTTPRouteRule{{
							Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
							BackendRefs: gatewayapi.HTTPBackendRef("blogsvc", 80, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "test.projectcontour.io",
								Routes: routes(prefixrouteHTTPRoute("/", service(blogService))),
							},
							Secret: secret(sec1),
						},
					),
				},
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"insert basic single route with single header match and path match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						Headers: gatewayapi.HTTPHeaderMatch(gatewayapi_v1.HeaderMatchExact, "foo", "bar"),
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "foo", Value: "bar", MatchType: "exact", Invert: false},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"insert two routes with single header match, path match and header match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{
						{
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/blog"),
							},
						}, {
							Path: &gatewayapi_v1.HTTPPathMatch{
								Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
								Value: ptr.To("/tech"),
							},
							Headers: gatewayapi.HTTPHeaderMatch(gatewayapi_v1.HeaderMatchExact, "foo", "bar"),
						},
					},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/blog"),
							Clusters:           clustersWeight(service(kuardService)),
						},
						&Route{
							PathMatchCondition: prefixSegment("/tech"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "foo", Value: "bar", MatchType: "exact", Invert: false},
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"insert two routes with single header match without explicit path match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Headers: gatewayapi.HTTPHeaderMatch(gatewayapi_v1.HeaderMatchRegularExpression, "foo", "^abc$"),
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "foo", Value: "^abc$", MatchType: "regex", Invert: false},
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"insert route with multiple header matches including multiple for the same key": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Headers: []gatewayapi_v1.HTTPHeaderMatch{
							{Name: "header-1", Value: "value-1"},
							{Name: "header-2", Value: "value-2"},
							{Name: "header-1", Value: "value-3"},
							{Name: "HEADER-1", Value: "value-4"},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "header-1", Value: "value-1", MatchType: "exact"},
								{Name: "header-2", Value: "value-2", MatchType: "exact"},
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"route with HTTP method match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						Method: ptr.To(gatewayapi_v1.HTTPMethodGet),
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: ":method", Value: "GET", MatchType: "exact"},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"insert single route with single query param match without type specified and path match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
							{
								Name:  "param-1",
								Value: "value-1",
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							QueryParamMatchConditions: []QueryParamMatchCondition{
								{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"insert single route with single query param match with type specified and path match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-1",
								Value: "value-1",
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							QueryParamMatchConditions: []QueryParamMatchCondition{
								{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"insert single route with multiple query param matches including multiple for the same key": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-1",
								Value: "value-1",
							},
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-2",
								Value: "value-2",
							},
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-1",
								Value: "value-3",
							},
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "Param-1",
								Value: "value-4",
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							QueryParamMatchConditions: []QueryParamMatchCondition{
								{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
								{Name: "param-2", Value: "value-2", MatchType: QueryParamMatchTypeExact},
								{Name: "Param-1", Value: "value-4", MatchType: QueryParamMatchTypeExact},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"insert single route with multiple query param matches including multiple for the same key and mixed types": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-1",
								Value: "value-1",
							},
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-2",
								Value: "value-2",
							},
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-1",
								Value: "value-3",
							},
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchRegularExpression),
								Name:  "Param-1",
								Value: "value-4",
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							QueryParamMatchConditions: []QueryParamMatchCondition{
								{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
								{Name: "param-2", Value: "value-2", MatchType: QueryParamMatchTypeExact},
								{Name: "Param-1", Value: "value-4", MatchType: QueryParamMatchTypeRegex},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},

		"insert single route with duplicate key param matches with mixed types, ignores the second type": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchExact),
								Name:  "param-1",
								Value: "value-1",
							},
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchRegularExpression),
								Name:  "param-1",
								Value: "value-2",
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							QueryParamMatchConditions: []QueryParamMatchCondition{
								{Name: "param-1", Value: "value-1", MatchType: QueryParamMatchTypeExact},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},

		"insert single route with single query param match with regex type specified and path match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: []gatewayapi_v1.HTTPRouteMatch{{
						Path: &gatewayapi_v1.HTTPPathMatch{
							Type:  ptr.To(gatewayapi_v1.PathMatchPathPrefix),
							Value: ptr.To("/"),
						},
						QueryParams: []gatewayapi_v1.HTTPQueryParamMatch{
							{
								Type:  ptr.To(gatewayapi_v1.QueryParamMatchRegularExpression),
								Name:  "query-param-regex",
								Value: "value-%d-[a-zA-Z0-9]",
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							QueryParamMatchConditions: []QueryParamMatchCondition{
								{Name: "query-param-regex", Value: "value-%d-[a-zA-Z0-9]", MatchType: QueryParamMatchTypeRegex},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"Route rule with request header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					Filters: []gatewayapi_v1.HTTPRouteFilter{
						{
							Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
									{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
									{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
								},
								Add: []gatewayapi_v1.HTTPHeader{
									{Name: "custom-header-add", Value: "foo-bar"},
								},
								Remove: []string{"x-remove"},
							},
						},
						{
							// Second instance of filter should be ignored.
							Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
									{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "ignored"},
									{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar-ignored.com"},
								},
								Add: []gatewayapi_v1.HTTPHeader{
									{Name: "custom-header-add", Value: "ignored"},
								},
								Remove: []string{"x-remove-ignored"},
							},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustersWeight(service(kuardService)),
							RequestHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"Custom-Header-Set": "foo-bar", // Verify the header key is canonicalized.
								},
								Add: map[string]string{
									"Custom-Header-Add": "foo-bar", // Verify the header key is canonicalized.
								},
								Remove:      []string{"X-Remove"}, // Verify the header key is canonicalized.
								HostRewrite: "bar.com",
							},
						},
					)),
				},
			),
		},
		"Route rule with response header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					Filters: []gatewayapi_v1.HTTPRouteFilter{
						{
							Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
							ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
									{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
									{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
								},
								Add: []gatewayapi_v1.HTTPHeader{
									{Name: "custom-header-add", Value: "foo-bar"},
								},
								Remove: []string{"x-remove"},
							},
						},
						{
							// Second instance of filter should be ignored.
							Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
							ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
									{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "ignored"},
									{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar-ignored.com"},
								},
								Add: []gatewayapi_v1.HTTPHeader{
									{Name: "custom-header-add", Value: "ignored"},
								},
								Remove: []string{"x-remove-ignored"},
							},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustersWeight(service(kuardService)),
							ResponseHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"Custom-Header-Set": "foo-bar", // Verify the header key is canonicalized.
									"Host":              "bar.com", // Host header isn't significant in a response so it can be set.
								},
								Add: map[string]string{
									"Custom-Header-Add": "foo-bar", // Verify the header key is canonicalized.
								},
								Remove: []string{"X-Remove"}, // Verify the header key is canonicalized.
							},
						},
					)),
				},
			),
		},
		"HTTP backend with request header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: []gatewayapi_v1.HTTPBackendRef{
						{
							BackendRef: gatewayapi_v1.BackendRef{
								BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								Weight:                 ptr.To(int32(1)),
							},
							Filters: []gatewayapi_v1.HTTPRouteFilter{
								{
									Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "foo-bar"},
										},
										Remove: []string{"x-remove"},
									},
								},
								{
									// Second instance of filter should be ignored.
									Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "ignored"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar-ignored.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "ignored"},
										},
										Remove: []string{"x-remove-ignored"},
									},
								},
							},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeaders(map[string]string{"Custom-Header-Set": "foo-bar"}, map[string]string{"Custom-Header-Add": "foo-bar"}, []string{"X-Remove"}, "bar.com", nil, nil, nil, service(kuardService)),
						},
					)),
				},
			),
		},
		"HTTP backend with response header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: []gatewayapi_v1.HTTPBackendRef{
						{
							BackendRef: gatewayapi_v1.BackendRef{
								BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								Weight:                 ptr.To(int32(1)),
							},
							Filters: []gatewayapi_v1.HTTPRouteFilter{
								{
									Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "foo-bar"},
										},
										Remove: []string{"x-remove"},
									},
								},
								{
									// Second instance of filter should be ignored.
									Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "ignored"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar-ignored.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "ignored"},
										},
										Remove: []string{"x-remove-ignored"},
									},
								},
							},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeaders(nil, nil, nil, "", map[string]string{"Custom-Header-Set": "foo-bar", "Host": "bar.com"}, map[string]string{"Custom-Header-Add": "foo-bar"}, []string{"X-Remove"}, service(kuardService)),
						},
					)),
				},
			),
		},
		"Route rule with invalid request header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
							Set: []gatewayapi_v1.HTTPHeader{
								{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
								{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
							},
							Add: []gatewayapi_v1.HTTPHeader{
								{Name: "!invalid-header-add", Value: "foo-bar"},
							},
						},
					}},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustersWeight(service(kuardService)),
							RequestHeadersPolicy: &HeadersPolicy{
								Set:         map[string]string{"Custom-Header-Set": "foo-bar"},
								Add:         map[string]string{}, // Invalid header should not be set.
								HostRewrite: "bar.com",
							},
						},
					)),
				},
			),
		},
		"HTTP backend with invalid request header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: []gatewayapi_v1.HTTPBackendRef{
						{
							BackendRef: gatewayapi_v1.BackendRef{
								BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								Weight:                 ptr.To(int32(1)),
							},
							Filters: []gatewayapi_v1.HTTPRouteFilter{{
								Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
									Set: []gatewayapi_v1.HTTPHeader{
										{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
										{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
									},
									Add: []gatewayapi_v1.HTTPHeader{
										{Name: "!invalid-header-add", Value: "foo-bar"},
									},
								},
							}},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeaders(map[string]string{"Custom-Header-Set": "foo-bar"}, map[string]string{}, nil, "bar.com", nil, nil, nil, service(kuardService)),
						},
					)),
				},
			),
		},
		"HTTP backend with invalid response header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: []gatewayapi_v1.HTTPBackendRef{
						{
							BackendRef: gatewayapi_v1.BackendRef{
								BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
								Weight:                 ptr.To(int32(1)),
							},
							Filters: []gatewayapi_v1.HTTPRouteFilter{{
								Type: gatewayapi_v1.HTTPRouteFilterResponseHeaderModifier,
								ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
									Set: []gatewayapi_v1.HTTPHeader{
										{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
										{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
									},
									Add: []gatewayapi_v1.HTTPHeader{
										{Name: "!invalid-header-add", Value: "foo-bar"},
									},
								},
							}},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeaders(nil, nil, nil, "", map[string]string{"Custom-Header-Set": "foo-bar", "Host": "bar.com"}, map[string]string{}, nil, service(kuardService)),
						},
					)),
				},
			),
		},
		"HTTPRoute rule with request redirect filter": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gatewayapi_v1.HTTPRequestRedirectFilter{
							Scheme:     ptr.To("https"),
							Hostname:   ptr.To(gatewayapi_v1.PreciseHostname("envoyproxy.io")),
							Port:       ptr.To(gatewayapi_v1.PortNumber(443)),
							StatusCode: ptr.To(301),
						},
					}},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Redirect: &Redirect{
								Scheme:     "https",
								Hostname:   "envoyproxy.io",
								PortNumber: 443,
								StatusCode: 301,
							},
						},
					)),
				},
			),
		},
		"HTTPRoute rule with request redirect filter with multiple matches": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: append(
						gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/another-match")...,
					),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gatewayapi_v1.HTTPRequestRedirectFilter{
							Scheme:     ptr.To("https"),
							Hostname:   ptr.To(gatewayapi_v1.PreciseHostname("envoyproxy.io")),
							Port:       ptr.To(gatewayapi_v1.PortNumber(443)),
							StatusCode: ptr.To(301),
						},
					}},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Redirect: &Redirect{
								Scheme:     "https",
								Hostname:   "envoyproxy.io",
								PortNumber: 443,
								StatusCode: 301,
							},
						},
						&Route{
							PathMatchCondition: prefixSegment("/another-match"),
							Redirect: &Redirect{
								Scheme:     "https",
								Hostname:   "envoyproxy.io",
								PortNumber: 443,
								StatusCode: 301,
							},
						},
					)),
				},
			),
		},
		"HTTPRoute rule with request redirect filter with ReplacePrefixMatch to another value": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gatewayapi_v1.HTTPRequestRedirectFilter{
							Path: &gatewayapi_v1.HTTPPathModifier{
								Type:               gatewayapi_v1.PrefixMatchHTTPPathModifier,
								ReplacePrefixMatch: ptr.To("/replacement"),
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/prefix"),
							Redirect: &Redirect{
								PathRewritePolicy: &PathRewritePolicy{
									PrefixRewrite: "/replacement",
								},
							},
						},
					)),
				},
			),
		},
		"HTTPRoute rule with request redirect filter with ReplacePrefixMatch to \"/\"": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gatewayapi_v1.HTTPRequestRedirectFilter{
							Path: &gatewayapi_v1.HTTPPathModifier{
								Type:               gatewayapi_v1.PrefixMatchHTTPPathModifier,
								ReplacePrefixMatch: ptr.To("/"),
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/prefix"),
							Redirect: &Redirect{
								PathRewritePolicy: &PathRewritePolicy{
									PrefixRegexRemove: "^/prefix/*",
								},
							},
						},
					)),
				},
			),
		},
		"HTTPRoute rule with request redirect filter with ReplaceFullPath": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gatewayapi_v1.HTTPRequestRedirectFilter{
							Path: &gatewayapi_v1.HTTPPathModifier{
								Type:            gatewayapi_v1.FullPathHTTPPathModifier,
								ReplaceFullPath: ptr.To("/replacement"),
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/prefix"),
							Redirect: &Redirect{
								PathRewritePolicy: &PathRewritePolicy{
									FullPathRewrite: "/replacement",
								},
							},
						},
					)),
				},
			),
		},
		"HTTPRoute rule with request mirror filter": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
						RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
							BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
						},
					}},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						withMirror(prefixrouteHTTPRoute("/", service(kuardService)), []*Service{service(kuardService2)}, 100))),
				},
			),
		},
		"HTTPRoute rule with two request mirror filters": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					Filters: []gatewayapi_v1.HTTPRouteFilter{
						{
							Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
							},
						},
						{
							Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
							RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
								BackendRef: gatewayapi.ServiceBackendObjectRef("kuard3", 8080),
							},
						},
					},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						withMirror(prefixrouteHTTPRoute("/", service(kuardService)), []*Service{service(kuardService2), service(kuardService3)}, 100))),
				},
			),
		},
		"HTTPRoute rule with request mirror filter with multiple matches": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: append(
						gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
						gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/another-match")...,
					),
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterRequestMirror,
						RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
							BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
						},
					}},
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						withMirror(prefixrouteHTTPRoute("/", service(kuardService)), []*Service{service(kuardService2)}, 100),
						withMirror(segmentPrefixHTTPRoute("/another-match", service(kuardService)), []*Service{service(kuardService2)}, 100),
					)),
				},
			),
		},
		"HTTPRoute rule with URLRewrite filter with ReplacePrefixMatch to another value": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gatewayapi_v1.HTTPURLRewriteFilter{
							Path: &gatewayapi_v1.HTTPPathModifier{
								Type:               gatewayapi_v1.PrefixMatchHTTPPathModifier,
								ReplacePrefixMatch: ptr.To("/replacement"),
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/prefix"),
							PathRewritePolicy: &PathRewritePolicy{
								PrefixRewrite: "/replacement",
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"HTTPRoute rule with URLRewrite filter with ReplacePrefixMatch to \"/\"": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gatewayapi_v1.HTTPURLRewriteFilter{
							Path: &gatewayapi_v1.HTTPPathModifier{
								Type:               gatewayapi_v1.PrefixMatchHTTPPathModifier,
								ReplacePrefixMatch: ptr.To("/"),
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/prefix"),
							PathRewritePolicy: &PathRewritePolicy{
								PrefixRegexRemove: "^/prefix/*",
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"HTTPRoute rule with URLRewrite filter with ReplaceFullPath": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gatewayapi_v1.HTTPURLRewriteFilter{
							Path: &gatewayapi_v1.HTTPPathModifier{
								Type:            gatewayapi_v1.FullPathHTTPPathModifier,
								ReplaceFullPath: ptr.To("/replacement"),
							},
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/prefix"),
							PathRewritePolicy: &PathRewritePolicy{
								FullPathRewrite: "/replacement",
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"HTTPRoute rule with URLRewrite filter with Hostname rewrite": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{{
						Type: gatewayapi_v1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gatewayapi_v1.HTTPURLRewriteFilter{
							Hostname: ptr.To(gatewayapi_v1.PreciseHostname("rewritten.com")),
						},
					}},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition:   prefixSegment("/prefix"),
							RequestHeadersPolicy: &HeadersPolicy{HostRewrite: "rewritten.com"},
							Clusters:             clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"HTTPRoute rule with RequestHeadersModifier and URLRewrite filter with Hostname rewrite": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "test.projectcontour.io", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/prefix"),
					Filters: []gatewayapi_v1.HTTPRouteFilter{
						{
							Type: gatewayapi_v1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
								Set: []gatewayapi_v1.HTTPHeader{
									{
										Name:  "Host",
										Value: "requestheader.rewritten.com",
									},
								},
							},
						},
						{
							Type: gatewayapi_v1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gatewayapi_v1.HTTPURLRewriteFilter{
								Hostname: ptr.To(gatewayapi_v1.PreciseHostname("url.rewritten.com")),
							},
						},
					},
					BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixSegment("/prefix"),
							RequestHeadersPolicy: &HeadersPolicy{
								Add:         map[string]string{},
								HostRewrite: "url.rewritten.com",
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},

		"HTTPRoute rule with request timeout": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("5s", ""),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							&Route{
								PathMatchCondition: prefixString("/"),
								Clusters:           clustersWeight(service(kuardService)),
								TimeoutPolicy: RouteTimeoutPolicy{
									ResponseTimeout: timeout.DurationSetting(5 * time.Second),
								},
							},
						),
					),
				},
			),
		},
		"HTTPRoute rule with 0s (disabled) request timeout": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("0s", ""),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							&Route{
								PathMatchCondition: prefixString("/"),
								Clusters:           clustersWeight(service(kuardService)),
								TimeoutPolicy: RouteTimeoutPolicy{
									ResponseTimeout: timeout.DisabledSetting(),
								},
							},
						),
					),
				},
			),
		},
		"HTTPRoute rule with request and backendRequest timeout": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("5s", "4s"),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							&Route{
								PathMatchCondition: prefixString("/"),
								Clusters:           clustersWeight(service(kuardService)),
								TimeoutPolicy: RouteTimeoutPolicy{
									ResponseTimeout: timeout.DurationSetting(4 * time.Second),
								},
							},
						),
					),
				},
			),
		},

		"HTTPRoute rule with backendRequest timeout only": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("", "5s"),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							&Route{
								PathMatchCondition: prefixString("/"),
								Clusters:           clustersWeight(service(kuardService)),
								TimeoutPolicy: RouteTimeoutPolicy{
									ResponseTimeout: timeout.DurationSetting(5 * time.Second),
								},
							},
						),
					),
				},
			),
		},
		"HTTPRoute rule with 0s (disabled) backendRequest timeout": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("", "0s"),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							&Route{
								PathMatchCondition: prefixString("/"),
								Clusters:           clustersWeight(service(kuardService)),
								TimeoutPolicy: RouteTimeoutPolicy{
									ResponseTimeout: timeout.DisabledSetting(),
								},
							},
						),
					),
				},
			),
		},
		"HTTPRoute rule with invalid request timeout": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("invalid", ""),
			},
			want: listeners(),
		},
		"HTTPRoute rule with invalid backend request timeout": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("5s", "invalid"),
			},
			want: listeners(),
		},

		"HTTPRoute rule with backend request timeout greater than request timeout": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				makeHTTPRouteWithTimeouts("5s", "6s"),
			},
			want: listeners(),
		},

		"HTTPRoute with BackendTLSPolicy": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			upstreamTLS: &UpstreamTLS{
				MinimumProtocolVersion: "1.2",
				MaximumProtocolVersion: "1.2",
			},
			objs: []any{
				tlsService,
				configMapCert1,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
				&gatewayapi_v1alpha3.BackendTLSPolicy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
						TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
									Kind: "Service",
									Name: "tlssvc",
								},
							},
						},
						Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
							CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
								Kind: "ConfigMap",
								Name: gatewayapi_v1.ObjectName(configMapCert1.Name),
							}},
							Hostname: "example.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", routeCluster("/",
							&Cluster{
								Weight: 1,
								Upstream: &Service{
									Protocol: "tls",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      tlsService.Name,
										ServiceNamespace: tlsService.Namespace,
										ServicePort:      tlsService.Spec.Ports[0],
										HealthPort:       tlsService.Spec.Ports[0],
									},
								},
								Protocol: "tls",
								UpstreamValidation: &PeerValidationContext{
									CACertificates: []*Secret{
										caSecret(cert1),
									},
									SubjectNames: []string{"example.com"},
								},
								UpstreamTLS: &UpstreamTLS{
									MinimumProtocolVersion: "1.2",
									MaximumProtocolVersion: "1.2",
								},
							},
						)),
					),
				},
			),
		},
		"HTTPRoute with BackendTLSPolicy and CA cert in secret": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			upstreamTLS: &UpstreamTLS{
				MinimumProtocolVersion: "1.2",
				MaximumProtocolVersion: "1.2",
			},
			objs: []any{
				tlsService,
				cert1,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
				&gatewayapi_v1alpha3.BackendTLSPolicy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
						TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
									Kind: "Service",
									Name: "tlssvc",
								},
							},
						},
						Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
							CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
								Kind: "Secret",
								Name: gatewayapi_v1.ObjectName(cert1.Name),
							}},
							Hostname: "example.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", routeCluster("/",
							&Cluster{
								Weight: 1,
								Upstream: &Service{
									Protocol: "tls",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      tlsService.Name,
										ServiceNamespace: tlsService.Namespace,
										ServicePort:      tlsService.Spec.Ports[0],
										HealthPort:       tlsService.Spec.Ports[0],
									},
								},
								Protocol: "tls",
								UpstreamValidation: &PeerValidationContext{
									CACertificates: []*Secret{
										caSecret(cert1),
									},
									SubjectNames: []string{"example.com"},
								},
								UpstreamTLS: &UpstreamTLS{
									MinimumProtocolVersion: "1.2",
									MaximumProtocolVersion: "1.2",
								},
							},
						)),
					),
				},
			),
		},
		"HTTPRoute with BackendTLSPolicy using multiple certs": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			upstreamTLS: &UpstreamTLS{
				MinimumProtocolVersion: "1.2",
				MaximumProtocolVersion: "1.2",
			},
			objs: []any{
				tlsService,
				cert1,
				cert2,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
				&gatewayapi_v1alpha3.BackendTLSPolicy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
						TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
									Kind: "Service",
									Name: "tlssvc",
								},
							},
						},
						Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
							CACertificateRefs: []gatewayapi_v1.LocalObjectReference{
								{
									Kind: "Secret",
									Name: gatewayapi_v1.ObjectName(cert1.Name),
								},
								{
									Kind: "Secret",
									Name: gatewayapi_v1.ObjectName(cert2.Name),
								},
							},
							Hostname: "example.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", routeCluster("/",
							&Cluster{
								Weight: 1,
								Upstream: &Service{
									Protocol: "tls",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      tlsService.Name,
										ServiceNamespace: tlsService.Namespace,
										ServicePort:      tlsService.Spec.Ports[0],
										HealthPort:       tlsService.Spec.Ports[0],
									},
								},
								Protocol: "tls",
								UpstreamValidation: &PeerValidationContext{
									CACertificates: []*Secret{
										caSecret(cert1),
										caSecret(cert2),
									},
									SubjectNames: []string{"example.com"},
								},
								UpstreamTLS: &UpstreamTLS{
									MinimumProtocolVersion: "1.2",
									MaximumProtocolVersion: "1.2",
								},
							},
						)),
					),
				},
			),
		},
		"HTTPRoute with BackendTLSPolicy and sectionName set": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			upstreamTLS: &UpstreamTLS{
				MinimumProtocolVersion: "1.2",
				MaximumProtocolVersion: "1.2",
			},
			objs: []any{
				tlsAndNonTLSService,
				cert1,
				makeHTTPRoute("tls-basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/tls", "tlsandnontlssvc", 443, 1)),
				&gatewayapi_v1.HTTPRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "non-tls-basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Rules: []gatewayapi_v1.HTTPRouteRule{{
							Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/non-tls"),
							BackendRefs: gatewayapi.HTTPBackendRefs(
								gatewayapi.HTTPBackendRef("tlsandnontlssvc", 80, 1),
							),
						}},
					},
				},
				&gatewayapi_v1alpha3.BackendTLSPolicy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
						TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
									Kind: "Service",
									Name: "tlsandnontlssvc",
								},
								SectionName: ptr.To(gatewayapi_v1.SectionName("https")),
							},
						},
						Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
							CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
								Kind: "Secret",
								Name: gatewayapi_v1.ObjectName(cert1.Name),
							}},
							Hostname: "example.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*",
							&Route{
								PathMatchCondition: prefixSegment("/tls"),
								Clusters: []*Cluster{
									{
										Weight: 1,
										Upstream: &Service{
											Protocol: "tls",
											Weighted: WeightedService{
												Weight:           1,
												ServiceName:      tlsAndNonTLSService.Name,
												ServiceNamespace: tlsAndNonTLSService.Namespace,
												ServicePort:      tlsAndNonTLSService.Spec.Ports[1],
												HealthPort:       tlsAndNonTLSService.Spec.Ports[1],
											},
										},
										Protocol: "tls",
										UpstreamValidation: &PeerValidationContext{
											CACertificates: []*Secret{
												caSecret(cert1),
											},
											SubjectNames: []string{"example.com"},
										},
										UpstreamTLS: &UpstreamTLS{
											MinimumProtocolVersion: "1.2",
											MaximumProtocolVersion: "1.2",
										},
									},
								},
							},
							&Route{
								PathMatchCondition: prefixSegment("/non-tls"),
								Clusters: []*Cluster{
									{
										Weight: 1,
										Upstream: &Service{
											Weighted: WeightedService{
												Weight:           1,
												ServiceName:      tlsAndNonTLSService.Name,
												ServiceNamespace: tlsAndNonTLSService.Namespace,
												ServicePort:      tlsAndNonTLSService.Spec.Ports[0],
												HealthPort:       tlsAndNonTLSService.Spec.Ports[0],
											},
										},
									},
								},
							},
						),
					),
				},
			),
		},
		"HTTPRoute with invalid BackendTLSPolicy certRef secret": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			upstreamTLS: &UpstreamTLS{
				MinimumProtocolVersion: "1.2",
				MaximumProtocolVersion: "1.2",
			},
			objs: []any{
				tlsService,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
				&gatewayapi_v1alpha3.BackendTLSPolicy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
						TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
									Kind: "Service",
									Name: "tlssvc",
								},
							},
						},
						Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
							CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
								Kind: "Secret",
								Name: gatewayapi_v1.ObjectName(cert1.Name),
							}},
							Hostname: "example.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", routeCluster("/",
							&Cluster{
								Weight: 1,
								Upstream: &Service{
									Protocol: "",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      tlsService.Name,
										ServiceNamespace: tlsService.Namespace,
										ServicePort:      tlsService.Spec.Ports[0],
										HealthPort:       tlsService.Spec.Ports[0],
									},
								},
							},
						)),
					),
				},
			),
		},
		"HTTPRoute with invalid BackendTLSPolicy certRef configmap": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			upstreamTLS: &UpstreamTLS{
				MinimumProtocolVersion: "1.2",
				MaximumProtocolVersion: "1.2",
			},
			objs: []any{
				tlsService,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
				&gatewayapi_v1alpha3.BackendTLSPolicy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
						TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
									Kind: "Service",
									Name: "tlssvc",
								},
							},
						},
						Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
							CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
								Kind: "ConfigMap",
								Name: gatewayapi_v1.ObjectName(cert1.Name),
							}},
							Hostname: "example.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", routeCluster("/",
							&Cluster{
								Weight: 1,
								Upstream: &Service{
									Protocol: "",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      tlsService.Name,
										ServiceNamespace: tlsService.Namespace,
										ServicePort:      tlsService.Spec.Ports[0],
										HealthPort:       tlsService.Spec.Ports[0],
									},
								},
							},
						)),
					),
				},
			),
		},
		"HTTPRoute with invalid BackendTLSPolicy certRef kind": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			upstreamTLS: &UpstreamTLS{
				MinimumProtocolVersion: "1.2",
				MaximumProtocolVersion: "1.2",
			},
			objs: []any{
				tlsService,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "tlssvc", 443, 1)),
				&gatewayapi_v1alpha3.BackendTLSPolicy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha3.BackendTLSPolicySpec{
						TargetRefs: []gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayapi_v1alpha2.LocalPolicyTargetReference{
									Kind: "Service",
									Name: "tlssvc",
								},
							},
						},
						Validation: gatewayapi_v1alpha3.BackendTLSPolicyValidation{
							CACertificateRefs: []gatewayapi_v1.LocalObjectReference{{
								Kind: "Invalid",
								Name: gatewayapi_v1.ObjectName(cert1.Name),
							}},
							Hostname: "example.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", routeCluster("/",
							&Cluster{
								Weight: 1,
								Upstream: &Service{
									Protocol: "",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      tlsService.Name,
										ServiceNamespace: tlsService.Namespace,
										ServicePort:      tlsService.Spec.Ports[0],
										HealthPort:       tlsService.Spec.Ports[0],
									},
								},
							},
						)),
					),
				},
			),
		},
		"different weights for multiple forwardTos": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				makeHTTPRoute("basic", "projectcontour", "", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: gatewayapi.HTTPBackendRefs(
						gatewayapi.HTTPBackendRef("kuard", 8080, 5),
						gatewayapi.HTTPBackendRef("kuard2", 8080, 10),
						gatewayapi.HTTPBackendRef("kuard3", 8080, 15),
					),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixrouteHTTPRoute("/",
							&Service{
								Weighted: WeightedService{
									Weight:           5,
									ServiceName:      kuardService.Name,
									ServiceNamespace: kuardService.Namespace,
									ServicePort:      kuardService.Spec.Ports[0],
									HealthPort:       kuardService.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           10,
									ServiceName:      kuardService2.Name,
									ServiceNamespace: kuardService2.Namespace,
									ServicePort:      kuardService2.Spec.Ports[0],
									HealthPort:       kuardService.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           15,
									ServiceName:      kuardService3.Name,
									ServiceNamespace: kuardService3.Namespace,
									ServicePort:      kuardService3.Spec.Ports[0],
									HealthPort:       kuardService.Spec.Ports[0],
								},
							},
						)),
					),
				},
			),
		},
		"one service weight zero w/weights for other forwardTos": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				makeHTTPRoute("basic", "projectcontour", "", gatewayapi_v1.HTTPRouteRule{
					Matches: gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
					BackendRefs: gatewayapi.HTTPBackendRefs(
						gatewayapi.HTTPBackendRef("kuard", 8080, 5),
						gatewayapi.HTTPBackendRef("kuard2", 8080, 0),
						gatewayapi.HTTPBackendRef("kuard3", 8080, 15),
					),
				}),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixrouteHTTPRoute("/",
							&Service{
								Weighted: WeightedService{
									Weight:           5,
									ServiceName:      kuardService.Name,
									ServiceNamespace: kuardService.Namespace,
									ServicePort:      kuardService.Spec.Ports[0],
									HealthPort:       kuardService.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           0,
									ServiceName:      kuardService2.Name,
									ServiceNamespace: kuardService2.Namespace,
									ServicePort:      kuardService2.Spec.Ports[0],
									HealthPort:       kuardService.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           15,
									ServiceName:      kuardService3.Name,
									ServiceNamespace: kuardService3.Namespace,
									ServicePort:      kuardService3.Spec.Ports[0],
									HealthPort:       kuardService.Spec.Ports[0],
								},
							},
						)),
					),
				},
			),
		},
		"weight of zero for a single forwardTo results in 500": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchPathPrefix, "/", "kuard", 8080, 0)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("*", directResponseRouteService("/", http.StatusInternalServerError, &Service{
							Weighted: WeightedService{
								Weight:           0,
								ServiceName:      kuardService.Name,
								ServiceNamespace: kuardService.Namespace,
								ServicePort:      kuardService.Spec.Ports[0],
								HealthPort:       kuardService.Spec.Ports[0],
							},
						})),
					),
				},
			),
		},
		"basic TLSRoute": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
					),
				},
			),
		},
		"TLSRoute references a backend in a different namespace, no ReferenceGrant": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute references a backend in a different namespace, with valid ReferenceGrant": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							},
						}},
					},
				},
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "TLSRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
					),
				},
			),
		},
		"TLSRoute references a backend in a different namespace, with valid ReferenceGrant (service-specific)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							},
						}},
					},
				},
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "TLSRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
							Name: ptr.To(gatewayapi_v1.ObjectName(kuardService.Name)),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
					),
				},
			),
		},
		"TLSRoute references a backend in a different namespace, with invalid ReferenceGrant (wrong Kind)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							},
						}},
					},
				},
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "HTTPRoute", // would need to be TLSRoute to be valid
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute references a backend in a different namespace, with invalid ReferenceGrant (grant in wrong namespace)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							},
						}},
					},
				},
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: "some-other-namespace", // would have to be "projectcontour" to be valid
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "TLSRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute references a backend in a different namespace, with invalid ReferenceGrant (wrong from namespace)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							},
						}},
					},
				},
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "TLSRoute",
							Namespace: "some-other-namespace", // would have to be "default" to be valid
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute references a backend in a different namespace, with invalid ReferenceGrant (wrong service name)": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayapi_v1.BackendRef{
								{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							},
						}},
					},
				},
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1beta1.ReferenceGrantSpec{
						From: []gatewayapi_v1beta1.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "TLSRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1beta1.ReferenceGrantTo{{
							Kind: "Service",
							Name: ptr.To(gatewayapi_v1.ObjectName("some-other-service")), // would have to be "kuard" to be valid
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute with multiple SNIs": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"tcp.projectcontour.io",
							"another.projectcontour.io",
							"thing.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "another.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "thing.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
					),
				},
			),
		},
		"TLSRoute with multiple SNIs, one is invalid": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"tcp.projectcontour.io",
							"*.*.another.projectcontour.io",
							"thing.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "thing.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
					),
				},
			),
		},
		"TLSRoute with multiple SNIs, all are invalid": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"tcp.*.projectcontour.io",
							"*.*.another.projectcontour.io",
							"!!thing.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute without any hostnames specified results in '*' match all": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "*",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(service(kuardService)),
							},
						},
					),
				},
			),
		},
		"TLSRoute with missing forwardTo service": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
						}},
					},
				},
			},
			want: listeners(),
		},
		"TLSRoute with multiple weighted ForwardTos": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRefs(
								gatewayapi.TLSRouteBackendRef("kuard", 8080, ptr.To(int32(1))),
								gatewayapi.TLSRouteBackendRef("kuard2", 8080, ptr.To(int32(2))),
								gatewayapi.TLSRouteBackendRef("kuard3", 8080, ptr.To(int32(3))),
							),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(
									weightedService(kuardService, 1),
									weightedService(kuardService2, 2),
									weightedService(kuardService3, 3),
								),
							},
						},
					),
				},
			),
		},
		"TLSRoute with multiple weighted ForwardTos and one zero weight": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRefs(
								gatewayapi.TLSRouteBackendRef("kuard", 8080, ptr.To(int32(1))),
								gatewayapi.TLSRouteBackendRef("kuard2", 8080, ptr.To(int32(0))),
								gatewayapi.TLSRouteBackendRef("kuard3", 8080, ptr.To(int32(3))),
							),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(
									weightedService(kuardService, 1),
									weightedService(kuardService2, 0),
									weightedService(kuardService3, 3),
								),
							},
						},
					),
				},
			),
		},
		"TLSRoute with multiple unweighted ForwardTos all default to 1": {
			gatewayclass: validClass,
			gateway:      gatewayTLSPassthroughAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				&gatewayapi_v1alpha2.TLSRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{"tcp.projectcontour.io"},
						Rules: []gatewayapi_v1alpha2.TLSRouteRule{{
							BackendRefs: gatewayapi.TLSRouteBackendRefs(
								gatewayapi.TLSRouteBackendRef("kuard", 8080, nil),
								gatewayapi.TLSRouteBackendRef("kuard2", 8080, nil),
								gatewayapi.TLSRouteBackendRef("kuard3", 8080, nil),
							),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "tcp.projectcontour.io",
							},
							TCPProxy: &TCPProxy{
								Clusters: clustersWeight(
									weightedService(kuardService, 1),
									weightedService(kuardService2, 1),
									weightedService(kuardService3, 1),
								),
							},
						},
					),
				},
			),
		},
		"insert gateway listener with host": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPWithHostname,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "", makeHTTPRouteRule(gatewayapi_v1.PathMatchExact, "/blog", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("gateway.projectcontour.io",
							exactrouteHTTPRoute("/blog", service(kuardService))),
					),
				},
			),
		},
		"insert gateway listener with host, httproute with host": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPWithWildcardHostname,
			objs: []any{
				kuardService,
				makeHTTPRoute("basic", "projectcontour", "http.projectcontour.io", makeHTTPRouteRule(gatewayapi_v1.PathMatchExact, "/blog", "kuard", 8080, 1)),
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("http.projectcontour.io",
							exactrouteHTTPRoute("/blog", service(kuardService))),
					),
				},
			),
		},
		"GRPCRoute: insert basic single grpc route, single hostname": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				basicGRPCRoute,
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", exactrouteGRPCRoute("/io.projectcontour/Login", grpcService(kuardService, "h2c"))),
					),
				},
			),
		},
		"GRPCRoute: insert basic single route, single hostname, gateway same namespace selector, route in different namespace": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPSameNamespace,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "different-ns-than-gateway",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		"GRPCRoute: route does not include the gateway in its list of parent refs": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								gatewayapi.GatewayParentRef("projectcontour", "some-other-gateway"),
							},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		"GRPCRoute: gateway with HTTP and HTTPS listeners, each route selects a different listener": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAndHTTPS,
			objs: []any{
				sec1,
				kuardService,
				blogService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "http-listener", 0),
							},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						}},
					},
				},
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basictls",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{
								gatewayapi.GatewayListenerParentRef("projectcontour", "contour", "https-listener", 0),
							},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("blogsvc", 80, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "test.projectcontour.io",
								Routes: routes(exactrouteGRPCRoute("/io.projectcontour/Login", grpcService(blogService, "h2"))),
							},
							Secret: secret(sec1),
						},
					),
				},
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", exactrouteGRPCRoute("/io.projectcontour/Login", grpcService(kuardService, "h2c"))),
					),
				},
			),
		},
		"GRPCRoute: insert basic single route with single method match and exact header match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method:  gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
								Headers: gatewayapi.GRPCHeaderMatch(gatewayapi_v1.GRPCHeaderMatchExact, "version", "2"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: exact("/io.projectcontour/Login"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "version", Value: "2", MatchType: "exact", Invert: false},
							},
							Clusters: clustersWeight(grpcService(kuardService, "h2c")),
						}),
					),
				},
			),
		},
		"GRPCRoute: insert basic single route with single method match and regular expression header match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method:  gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
								Headers: gatewayapi.GRPCHeaderMatch(gatewayapi_v1.GRPCHeaderMatchRegularExpression, "version", "2+"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: exact("/io.projectcontour/Login"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "version", Value: "2+", MatchType: "regex", Invert: false},
							},
							Clusters: clustersWeight(grpcService(kuardService, "h2c")),
						}),
					),
				},
			),
		},
		"GRPCRoute: insert basic single route with no method match and single header match": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Headers: gatewayapi.GRPCHeaderMatch(gatewayapi_v1.GRPCHeaderMatchExact, "version", "2"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: &PrefixMatchCondition{Prefix: "/"},
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "version", Value: "2", MatchType: "exact", Invert: false},
							},
							Clusters: clustersWeight(grpcService(kuardService, "h2c")),
						}),
					),
				},
			),
		},
		"GRPCRoute: insert basic single route with no matches": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches:     []gatewayapi_v1.GRPCRouteMatch{{}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: &PrefixMatchCondition{Prefix: "/"},
							Clusters:           clustersWeight(grpcService(kuardService, "h2c")),
						}),
					),
				},
			),
		},
		"GRPCRoute: Route rule with request header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
							Filters: []gatewayapi_v1.GRPCRouteFilter{
								{
									Type: gatewayapi_v1.GRPCRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "foo-bar"},
										},
										Remove: []string{"x-remove"},
									},
								},
								{
									// Second instance of filter should be ignored.
									Type: gatewayapi_v1.GRPCRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "ignored"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar-ignored.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "ignored"},
										},
										Remove: []string{"x-remove-ignored"},
									},
								},
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: exact("/io.projectcontour/Login"),
							Clusters:           clustersWeight(grpcService(kuardService, "h2c")),
							RequestHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"Custom-Header-Set": "foo-bar", // Verify the header key is canonicalized.
								},
								Add: map[string]string{
									"Custom-Header-Add": "foo-bar", // Verify the header key is canonicalized.
								},
								Remove:      []string{"X-Remove"}, // Verify the header key is canonicalized.
								HostRewrite: "bar.com",
							},
						},
					)),
				},
			),
		},
		"GRPCRoute: Route rule with response header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
							Filters: []gatewayapi_v1.GRPCRouteFilter{
								{
									Type: gatewayapi_v1.GRPCRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "foo-bar"},
										},
										Remove: []string{"x-remove"},
									},
								},
								{
									// Second instance of filter should be ignored.
									Type: gatewayapi_v1.GRPCRouteFilterResponseHeaderModifier,
									ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "ignored"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar-ignored.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "custom-header-add", Value: "ignored"},
										},
										Remove: []string{"x-remove-ignored"},
									},
								},
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: exact("/io.projectcontour/Login"),
							Clusters:           clustersWeight(grpcService(kuardService, "h2c")),
							ResponseHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"Custom-Header-Set": "foo-bar", // Verify the header key is canonicalized.
									"Host":              "bar.com", // Host header isn't significant in a response so it can be set.
								},
								Add: map[string]string{
									"Custom-Header-Add": "foo-bar", // Verify the header key is canonicalized.
								},
								Remove: []string{"X-Remove"}, // Verify the header key is canonicalized.
							},
						},
					)),
				},
			),
		},
		"GRPCRoute: Route rule with invalid request header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{
							{
								Matches: []gatewayapi_v1.GRPCRouteMatch{{
									Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
								}},
								BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
								Filters: []gatewayapi_v1.GRPCRouteFilter{{
									Type: gatewayapi_v1.GRPCRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
										Set: []gatewayapi_v1.HTTPHeader{
											{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
											{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
										},
										Add: []gatewayapi_v1.HTTPHeader{
											{Name: "!invalid-header-add", Value: "foo-bar"},
										},
									},
								}},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: exact("/io.projectcontour/Login"),
							Clusters:           clustersWeight(grpcService(kuardService, "h2c")),
							RequestHeadersPolicy: &HeadersPolicy{
								Set:         map[string]string{"Custom-Header-Set": "foo-bar"},
								Add:         map[string]string{}, // Invalid header should not be set.
								HostRewrite: "bar.com",
							},
						},
					)),
				},
			),
		},
		"GRPCRoute: HTTP backend with invalid response header modifier": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{
							{
								Matches: []gatewayapi_v1.GRPCRouteMatch{{
									Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
								}},
								BackendRefs: []gatewayapi_v1.GRPCBackendRef{
									{
										BackendRef: gatewayapi_v1.BackendRef{
											BackendObjectReference: gatewayapi.ServiceBackendObjectRef("kuard", 8080),
											Weight:                 ptr.To(int32(1)),
										},
										Filters: []gatewayapi_v1.GRPCRouteFilter{{
											Type: gatewayapi_v1.GRPCRouteFilterResponseHeaderModifier,
											ResponseHeaderModifier: &gatewayapi_v1.HTTPHeaderFilter{
												Set: []gatewayapi_v1.HTTPHeader{
													{Name: gatewayapi_v1.HTTPHeaderName("custom-header-set"), Value: "foo-bar"},
													{Name: gatewayapi_v1.HTTPHeaderName("Host"), Value: "bar.com"},
												},
												Add: []gatewayapi_v1.HTTPHeader{
													{Name: "!invalid-header-add", Value: "foo-bar"},
												},
											},
										}},
									},
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: exact("/io.projectcontour/Login"),
							Clusters:           clusterHeaders(nil, nil, nil, "", map[string]string{"Custom-Header-Set": "foo-bar", "Host": "bar.com"}, map[string]string{}, nil, grpcService(kuardService, "h2c")),
						},
					)),
				},
			),
		},
		"GRPCRoute: rule with request mirror filter": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
							Filters: []gatewayapi_v1.GRPCRouteFilter{{
								Type: gatewayapi_v1.GRPCRouteFilterRequestMirror,
								RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
									BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
								},
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						withMirror(exactrouteGRPCRoute("/io.projectcontour/Login", grpcService(kuardService, "h2c")), []*Service{grpcService(kuardService2, "h2c")}, 100))),
				},
			),
		},
		"GRPCRoute: rule with two request mirror filters": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				kuardService2,
				kuardService3,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Hostnames: []gatewayapi_v1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: gatewayapi.GRPCRouteBackendRef("kuard", 8080, 1),
							Filters: []gatewayapi_v1.GRPCRouteFilter{
								{
									Type: gatewayapi_v1.GRPCRouteFilterRequestMirror,
									RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
										BackendRef: gatewayapi.ServiceBackendObjectRef("kuard2", 8080),
									},
								},
								{
									Type: gatewayapi_v1.GRPCRouteFilterRequestMirror,
									RequestMirror: &gatewayapi_v1.HTTPRequestMirrorFilter{
										BackendRef: gatewayapi.ServiceBackendObjectRef("kuard3", 8080),
									},
								},
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						withMirror(exactrouteGRPCRoute("/io.projectcontour/Login", grpcService(kuardService, "h2c")), []*Service{grpcService(kuardService2, "h2c"), grpcService(kuardService3, "h2c")}, 100))),
				},
			),
		},

		"GRPCRoute: references a backend in a different namespace, with valid ReferenceGrant": {
			gatewayclass: validClass,
			gateway:      gatewayHTTPAllNamespaces,
			objs: []any{
				kuardService,
				&gatewayapi_v1.GRPCRoute{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1.GRPCRouteSpec{
						CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
							ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
						},
						Rules: []gatewayapi_v1.GRPCRouteRule{{
							Matches: []gatewayapi_v1.GRPCRouteMatch{{
								Method: gatewayapi.GRPCMethodMatch(gatewayapi_v1.GRPCMethodMatchExact, "io.projectcontour", "Login"),
							}},
							BackendRefs: []gatewayapi_v1.GRPCBackendRef{{
								BackendRef: gatewayapi_v1.BackendRef{
									BackendObjectReference: gatewayapi_v1.BackendObjectReference{
										Kind:      ptr.To(gatewayapi_v1alpha2.Kind("Service")),
										Namespace: ptr.To(gatewayapi_v1.Namespace(kuardService.Namespace)),
										Name:      gatewayapi_v1.ObjectName(kuardService.Name),
										Port:      ptr.To(gatewayapi_v1alpha2.PortNumber(8080)),
									},
									Weight: ptr.To(int32(1)),
								},
							}},
						}},
					},
				},
				&gatewayapi_v1beta1.ReferenceGrant{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "foo",
						Namespace: kuardService.Namespace,
					},
					Spec: gatewayapi_v1alpha2.ReferenceGrantSpec{
						From: []gatewayapi_v1alpha2.ReferenceGrantFrom{{
							Group:     gatewayapi_v1.GroupName,
							Kind:      "GRPCRoute",
							Namespace: "default",
						}},
						To: []gatewayapi_v1alpha2.ReferenceGrantTo{{
							Kind: "Service",
						}},
					},
				},
			},
			want: listeners(&Listener{
				Name:         "http-80",
				VirtualHosts: virtualhosts(virtualhost("*", exactrouteGRPCRoute("/io.projectcontour/Login", grpcService(kuardService, "h2c")))),
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					gatewayclass: tc.gatewayclass,
					gateway:      tc.gateway,
					FieldLogger:  fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{
						HTTPAddress:  "0.0.0.0",
						HTTPSAddress: "0.0.0.0",
					},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
						UpstreamTLS: tc.upstreamTLS,
					},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			for _, l := range dag.Listeners {
				got[l.Port] = l
			}

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				want[v.Port] = v
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestDAGInsert(t *testing.T) {
	// The DAG is insensitive to ordering, adding an ingress, then a service,
	// should have the same result as adding a service, then an ingress.

	sec1 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	// Invalid cert in the secret
	secInvalid := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata("wrong", "wronger"),
	}

	// weird secret with a blank ca.crt that
	// cert manager creates. #1644
	sec3 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: core_v1.SecretTypeTLS,
		Data: map[string][]byte{
			CACertificateKey:         []byte(""),
			core_v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
			core_v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
		},
	}

	sec4 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "root",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	fallbackCertificateSecret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "default",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	fallbackCertificateSecretRootNamespace := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "root",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	cert1 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ca",
			Namespace: "default",
		},
		Type: core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			CACertificateKey: []byte(fixture.CERTIFICATE),
		},
	}

	cert2 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ca",
			Namespace: "caCertOriginalNs",
		},
		Type: core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			CACertificateKey: []byte(fixture.CERTIFICATE),
		},
	}

	crl := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "crl",
			Namespace: "default",
		},
		Type: core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			CRLKey: []byte(fixture.CRL),
		},
	}

	i1V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	i1aV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	// i2V1 is functionally identical to i1V1
	i2V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}

	// i2aV1 is missing a http key from the spec.rule.
	// see issue 606
	i2aV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "test1.test.com",
			}},
		},
	}

	// i3V1 is similar to i2V1 but includes a hostname on the ingress rule
	i3V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "kuard.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}
	// i4V1 is like i1V1 except it uses a named service port
	i4V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i5V1 is functionally identical to i2V1
	i5V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}
	// i6V1 contains two named vhosts which point to the same service
	// one of those has TLS
	i6V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "a.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}, {
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}

	i6aV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "a.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}, {
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}

	i6bV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}

	i6cV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
				"kubernetes.io/ingress.allow-http":         "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}

	// i7V1 contains a single vhost with two paths
	i7V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-paths",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}, {
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	// i8V1 is identical to i7V1 but uses multiple IngressRules
	i8V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}, {
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}
	// i9V1 is identical to i8V1 but disables non TLS connections
	i9V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}, {
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	i10aV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/tls-minimum-protocol-version": "1.3",
				"projectcontour.io/tls-maximum-protocol-version": "1.3",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i11V1 has a websocket route
	i11V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "websocket",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/websocket-routes": "/ws1 , /ws2",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}, {
							Path:    "/ws1",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i12aV1 has an invalid timeout
	i12aV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/request-timeout": "peanut",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i12bV1 has a reasonable timeout
	i12bV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/request-timeout": "1m30s", // 90 seconds y'all
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i12cV1 has an unreasonable timeout
	i12cV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/request-timeout": "infinite",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i12dV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "peanut",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i12eV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "1m30s", // 90 seconds y'all
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i12fV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "infinite",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i13_v1 a and b are a pair of ingressesv1 for the same vhost
	// they represent a tricky way over 'overlaying' routes from one
	// ingress onto another
	i13aV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("app-service", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	i13bV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: *backendv1("challenge-service", intstr.FromInt(8009)),
						}},
					},
				},
			}},
		},
	}

	i3aV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(80))),
			}},
		},
	}

	i14V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/retry-on":        "gateway-error",
				"projectcontour.io/num-retries":     "6",
				"projectcontour.io/per-try-timeout": "10s",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i15V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "regex",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/[^/]+/invoices(/.*|/?)", // issue 1243
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i15InvalidRegexV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "regex",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "^\\/(?!\\/)(.*?)",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i16V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "wildcards",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				// No hostname.
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}, {
				// Allow wildcard as first label.
				// K8s will only allow hostnames with wildcards of this form.
				Host: "*.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i17V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host:             "example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}

	// i18V1 is  use secret from another namespace using annotation projectcontour.io/tls-cert-namespace
	i18V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "tls-from-other-ns-annotation",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/tls-cert-namespace": sec4.Namespace,
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: sec4.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "kuard.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}

	iPathMatchTypesV1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "pathmatchtypes",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{
							{
								PathType: (*networking_v1.PathType)(ptr.To("Exact")),
								Path:     "/exact",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(ptr.To("Exact")),
								Path:     "/exact_with_regex/.*",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(ptr.To("Prefix")),
								Path:     "/prefix",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(ptr.To("Prefix")),
								Path:     "/prefix_trailing_slash/",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(ptr.To("Prefix")),
								Path:     "/prefix_with_regex/.*",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(ptr.To("ImplementationSpecific")),
								Path:     "/implementation_specific",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(ptr.To("ImplementationSpecific")),
								Path:     "/implementation_specific_with_regex/.*",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
						},
					},
				},
			}},
		},
	}

	// s3a and b have http/2 protocol annotations
	s3a := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2c": "80,http",
			},
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 80, 8888)},
		},
	}

	s3b := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      s3a.Name,
			Namespace: s3a.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2": "80,http",
			},
		},
		Spec: s3a.Spec,
	}

	s3c := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      s3b.Name,
			Namespace: s3b.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "80,http",
			},
		},
		Spec: s3b.Spec,
	}

	sec13 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	s13a := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "app-service",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	s13b := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "challenge-service",
			Namespace: "nginx-ingress",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8009, 8080)},
		},
	}

	proxyMultipleBackends := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}, {
					Name: "kuarder",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTLS12 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName:             sec1.Name,
					MinimumProtocolVersion: "1.2",
					MaximumProtocolVersion: "1.2",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTLS13 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName:             sec1.Name,
					MinimumProtocolVersion: "1.3",
					MaximumProtocolVersion: "1.3",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTLSInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName:             sec1.Name,
					MinimumProtocolVersion: "0.999",
					MaximumProtocolVersion: "1.4",
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyWeightsTwoRoutesDiffWeights := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_v1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 90,
				}},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/b",
				}},
				Services: []contour_v1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 60,
				}},
			}},
		},
	}

	proxyWeightsOneRouteDiffWeights := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_v1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   8080,
					Weight: 60,
				}},
			}},
		},
	}

	proxyRetryPolicyValidTimeout := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				RetryPolicy: &contour_v1.RetryPolicy{
					NumRetries:    6,
					PerTryTimeout: "10s",
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyRetryPolicyInvalidTimeout := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				RetryPolicy: &contour_v1.RetryPolicy{
					NumRetries:    6,
					PerTryTimeout: "please",
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyRetryPolicyZeroRetries := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				RetryPolicy: &contour_v1.RetryPolicy{
					NumRetries:    0,
					PerTryTimeout: "10s",
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTimeoutPolicyInvalidResponse := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &contour_v1.TimeoutPolicy{
					Response: "peanut",
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTimeoutPolicyValidResponse := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &contour_v1.TimeoutPolicy{
					Response: "1m30s",
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTimeoutPolicyInfiniteResponse := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &contour_v1.TimeoutPolicy{
					Response: "infinite",
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyWildcardFQDN := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "wildcard",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "*.projectcontour.io",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	s1 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	// s1a carries the tls annotation
	s1a := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "8080",
			},
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	// s1b carries all four ingress annotations{
	s1b := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/max-connections":          "9000",
				"projectcontour.io/max-pending-requests":     "4096",
				"projectcontour.io/max-requests":             "404",
				"projectcontour.io/max-retries":              "7",
				"projectcontour.io/per-host-max-connections": "45",
			},
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	// s2 is like s1 but with a different name
	s2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuarder",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	// s2a is like s1 but with a different name again.
	// used in testing override priority.
	s2a := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuardest",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	// s3 is like s1 but has a different port
	s3 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 9999, 8080)},
		},
	}

	s4 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	s9 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}

	s10 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "tls-passthrough",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("https", "TCP", 443, 443), makeServicePort("http", "TCP", 80, 80)},
		},
	}

	s11 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blog",
			Namespace: "it",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("blog", "TCP", 8080)},
		},
	}

	s12 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "teama",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	s13 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "teamb",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	s14 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			ExternalName: "externalservice.io",
			Ports: []core_v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
			Type: core_v1.ServiceTypeExternalName,
		},
	}

	proxyDelegatedTLSSecret := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: s10.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_v1.TLS{
					SecretName: "projectcontour/ssl-cert", // not delegated
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s10.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxy1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy1a tcp forwards traffic to default/kuard:8080 by TLS pass-through it.
	proxy1a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			},
		},
	}

	// proxy1b is a straight HTTP forward, no conditions.
	proxy1b := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy1c is a straight forward, with prefix and header conditions.
	proxy1c := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:    "x-request-id",
						Present: true,
					},
				}, {
					Prefix: "/kuard",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "e-tag",
						Contains: "abcdef",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:        "x-timeout",
						NotContains: "infinity",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "digest-auth",
						Exact: "scott",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "digest-password",
						NotExact: "tiger",
					},
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy1d tcp forwards secure traffic to default/kuard:8080 by TLS pass-through it,
	// insecure traffic is 301 upgraded.
	proxy1d := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy1e tcp forwards secure traffic to default/kuard:8080 by TLS pass-through it,
	// insecure traffic is not 301 upgraded because of the permitInsecure: true annotation.
	proxy1e := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_v1.Route{{
				PermitInsecure: true,
				Services: []contour_v1.Service{{
					Name: s10.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: s10.Name,
					Port: 443,
				}},
			},
		},
	}

	// proxy1f is identical to proxy1 and ir1, except for a different service.
	// Used to test priority when importing ir then httproxy.
	proxy1f := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s2a.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy2a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "kubesystem",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:    "x-request-id",
						Present: true,
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:        "x-timeout",
						NotContains: "infinity",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "digest-auth",
						Exact: "scott",
					},
				}},
				Name:      "kuard",
				Namespace: "default",
			}, {
				// This second include has a similar set of conditions with
				// slight differences which should still ensure there is a
				// route programmed.
				Conditions: []contour_v1.MatchCondition{{
					Header: &contour_v1.HeaderMatchCondition{
						Name:    "x-request-id",
						Present: true,
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:       "x-timeout",
						NotPresent: true,
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:  "digest-auth",
						Exact: "scott",
					},
				}},
				Name:      "kuard",
				Namespace: "default",
			}},
		},
	}

	proxy2b := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/kuard",
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "e-tag",
						Contains: "abcdef",
					},
				}, {
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "digest-password",
						NotExact: "tiger",
					},
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy2c := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				HealthCheckPolicy: &contour_v1.HTTPHealthCheckPolicy{
					Path: "/healthz",
				},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy2d is a proxy with two routes that have the same prefix and a Contains header
	// condition on the same header, differing only in the value of the condition.
	proxy2d := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{
						{
							Header: &contour_v1.HeaderMatchCondition{
								Name:     "e-tag",
								Contains: "abc",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
				{
					Conditions: []contour_v1.MatchCondition{
						{
							Header: &contour_v1.HeaderMatchCondition{
								Name:     "e-tag",
								Contains: "def",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
			},
		},
	}

	// proxy2e is a proxy with two routes that both have a condition on the same
	// header, one using Contains and one using NotContains.
	proxy2e := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{
				{
					Conditions: []contour_v1.MatchCondition{
						{
							Header: &contour_v1.HeaderMatchCondition{
								Name:     "e-tag",
								Contains: "abc",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
				{
					Conditions: []contour_v1.MatchCondition{
						{
							Header: &contour_v1.HeaderMatchCondition{
								Name:        "e-tag",
								NotContains: "abc",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
			},
		},
	}

	// proxy6 has TLS and does not specify min tls version
	proxy6 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy17 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: cert1.Name,
						SubjectName:   "example.com",
					},
				}},
			}},
		},
	}
	protocolh2 := "h2"
	proxy17h2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name:     "kuard",
					Port:     8080,
					Protocol: &protocolh2,
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: cert1.Name,
						SubjectName:   "example.com",
					},
				}},
			}},
		},
	}
	proxy17UpstreamCACertDelegation := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: fmt.Sprintf("%s/%s", cert2.Namespace, cert2.Name),
						SubjectName:   "example.com",
					},
				}},
			}},
		},
	}

	// proxy18 is downstream validation, HTTP route
	proxy18 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: cert1.Name,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy19 is downstream validation, TCP proxying
	proxy19 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: cert1.Name,
					},
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy10 has a websocket route
	proxy10 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/websocket",
				}},
				EnableWebsockets: true,
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy10b has a websocket route w/multiple upstreams
	proxy10b := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/websocket",
				}},
				EnableWebsockets: true,
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy12 tests mirroring
	proxy12 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	// proxy13 has two mirrors, invalid.
	proxy13 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}, {
					// it is legal to mention a service more that
					// once, however it is not legal for more than one
					// service to be marked as mirror.
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	// proxy20 is downstream validation, skip cert validation
	proxy20 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						SkipClientCertValidation: true,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy21 is downstream validation, skip cert validation, with a CA
	proxy21 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						SkipClientCertValidation: true,
						CACertificate:            cert1.Name,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy22 is downstream validation with CRL check.
	proxy22 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             cert1.Name,
						CertificateRevocationList: crl.Name,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy22 is downstream validation with CRL check but only for leaf-certificate.
	proxy23 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             cert1.Name,
						CertificateRevocationList: crl.Name,
						OnlyVerifyLeafCertCrl:     true,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy24 is downstream validation, optional cert validation
	proxy24 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate:             cert1.Name,
						OptionalClientCertificate: true,
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy25 is downstream validation, fwd client cert details
	proxy25 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_v1.DownstreamValidation{
						CACertificate: cert1.Name,
						ForwardClientCertificate: &contour_v1.ClientCertificateDetails{
							Subject: true,
							Cert:    true,
							Chain:   true,
							DNS:     true,
							URI:     true,
						},
					},
				},
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// invalid because tcpproxy both includes another and
	// has a list of services.
	proxy37 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Include: &contour_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: "roots",
				},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// Invalid because tcpproxy neither includes another httpproxy
	// nor has a list of services.
	proxy37a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{},
		},
	}

	// proxy38 is invalid when combined with proxy39
	// as the latter is a root.
	proxy38 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Include: &contour_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy39 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy39broot is a valid TCPProxy which includes to another TCPProxy
	proxy39broot := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "root",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Include: &contour_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy39brootplural := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "root",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				IncludesDeprecated: &contour_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy39bchild := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy40 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// issue 2309, each route must have at least one service
	proxy41 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "missing-service",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "missing-service.example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: nil, // missing
			}},
		},
	}

	proxy100 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "marketingwww",
				Namespace: "marketing",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy100a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100b := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/infotech",
				}},
				Services: []contour_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100c := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: contour_v1.HTTPProxySpec{
			Includes: []contour_v1.Include{{
				Name:      "marketingit",
				Namespace: "it",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/it",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/infotech",
				}},
				Services: []contour_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}, {
				Services: []contour_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100d := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "marketingit",
			Namespace: "it",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy101 and proxy101a test inclusion without a specified namespace.
	proxy101 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy101a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy101.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// invalid because two prefix conditions on route.
	proxy102 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// invalid because two prefix conditions on include.
	proxy103 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "www",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
			}},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy103a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "www",
			Namespace: "teama",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s12.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy104 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy104a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy104.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy105 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy105a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy106 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/kuarder/",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy106a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy107 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy107a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/withavengeance",
				}},
				Services: []contour_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy108 and proxy108a test duplicate conditions on include
	proxy108 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "root",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteama",
				Namespace: "teamb",
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy108a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blogteama",
			Namespace: "teama",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s12.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy108b := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "blogteamb",
			Namespace: "teamb",
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: s13.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyReplaceHostHeaderRoute := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "bar.com",
					}},
				},
			}},
		},
	}

	proxyReplaceHostHeaderService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
					RequestHeadersPolicy: &contour_v1.HeadersPolicy{
						Set: []contour_v1.HeaderValue{{
							Name:  "Host",
							Value: "bar.com",
						}},
					},
				}},
			}},
		},
	}

	proxyReplaceHostHeaderMultiple := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "bar.com",
					}, {
						Name:  "x-header",
						Value: "bar.com",
					}, {
						Name:  "y-header",
						Value: "zed.com",
					}},
				},
			}},
		},
	}

	proxyReplaceNonHostHeader := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "x-header",
						Value: "bar.com",
					}},
				},
			}},
		},
	}

	proxyReplaceHeaderEmptyValue := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name: "x-header",
					}},
				},
			}},
		},
	}

	proxyCookieLoadBalancer := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: "Cookie",
				},
			}},
		},
	}

	proxyLoadBalancerHashPolicyHeader := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_v1.RequestHashPolicy{
						{
							Terminal: true,
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "X-Some-Header",
							},
						},
						{
							// Lower case but duplicated, should be ignored.
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "x-some-header",
							},
						},
						{
							HeaderHashOptions: nil,
						},
						{
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "X-Some-Other-Header",
							},
						},
						{
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "",
							},
						},
					},
				},
			}},
		},
	}

	proxyLoadBalancerHashPolicyQueryParameter := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_v1.RequestHashPolicy{
						{
							Terminal: true,
							QueryParameterHashOptions: &contour_v1.QueryParameterHashOptions{
								ParameterName: "something",
							},
						},
						{
							QueryParameterHashOptions: nil,
						},
						{
							QueryParameterHashOptions: &contour_v1.QueryParameterHashOptions{
								ParameterName: "other",
							},
						},
						{
							QueryParameterHashOptions: &contour_v1.QueryParameterHashOptions{
								ParameterName: "",
							},
						},
					},
				},
			}},
		},
	}

	proxyLoadBalancerHashPolicySourceIP := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_v1.RequestHashPolicy{
						{
							// Ensure header hash policies and source IP hashing
							// can coexist.
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "X-Some-Header",
							},
						},
						{
							Terminal:     true,
							HashSourceIP: true,
						},
						{
							// Duplicate should be ignored.
							HashSourceIP: true,
						},
					},
				},
			}},
		},
	}

	proxyLoadBalancerHashPolicyAllInvalid := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_v1.RequestHashPolicy{
						{
							HeaderHashOptions: nil,
						},
						{
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "",
							},
						},
						{
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "X-Foo",
							},
							HashSourceIP: true,
						},
						{
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "X-Foo",
							},
							QueryParameterHashOptions: &contour_v1.QueryParameterHashOptions{
								ParameterName: "something",
							},
						},
						{
							HeaderHashOptions: &contour_v1.HeaderHashOptions{
								HeaderName: "X-Foo",
							},
							QueryParameterHashOptions: &contour_v1.QueryParameterHashOptions{
								ParameterName: "something",
							},
							HashSourceIP: true,
						},
						{
							QueryParameterHashOptions: &contour_v1.QueryParameterHashOptions{
								ParameterName: "something",
							},
							HashSourceIP: true,
						},
						{
							QueryParameterHashOptions: nil,
						},
						{
							QueryParameterHashOptions: &contour_v1.QueryParameterHashOptions{
								ParameterName: "",
							},
						},
					},
				},
			}},
		},
	}

	// proxy109 has a route that rewrites headers.
	proxy109 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				RequestHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "In-Foo",
						Value: "bar",
					}},
					Remove: []string{
						"In-Baz",
					},
				},
				ResponseHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Out-Foo",
						Value: "bar",
					}},
					Remove: []string{
						"Out-Baz",
					},
				},
			}},
		},
	}
	// proxy111 has a route that rewrites headers.
	proxy111 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				ResponseHeadersPolicy: &contour_v1.HeadersPolicy{
					Set: []contour_v1.HeaderValue{{
						Name:  "Host",
						Value: "bar.baz",
					}},
				},
			}},
		},
	}
	// proxy112 has a route that rewrites headers.
	proxy112 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
					ResponseHeadersPolicy: &contour_v1.HeadersPolicy{
						Set: []contour_v1.HeaderValue{{
							Name:  "Host",
							Value: "bar.baz",
						}},
					},
				}},
			}},
		},
	}

	// Invalid because has exact in include match conditions
	proxy113 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example113-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example113.com",
			},
			Includes: []contour_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_v1.MatchCondition{{
					Exact: "/kuarder",
				}},
			}},
		},
	}

	proxy114 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example114-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example114.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "proxy114a",
				Namespace: s1.Namespace,
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name:      "proxy114b",
				Namespace: s2.Namespace,
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxy114a := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "proxy114a",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Exact: "/exact",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy114b := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "proxy114b",
			Namespace: s2.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/prefix",
				}},
				Services: []contour_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	cookieRewritePoliciesRoute := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
					{
						Name: "some-cookie",
						PathRewrite: &contour_v1.CookiePathRewrite{
							Value: "/foo",
						},
						DomainRewrite: &contour_v1.CookieDomainRewrite{
							Value: "example.com",
						},
						Secure:   ptr.To(true),
						SameSite: ptr.To("Strict"),
					},
					{
						Name:     "some-other-cookie",
						SameSite: ptr.To("Lax"),
						Secure:   ptr.To(false),
					},
				},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
			}},
		},
	}

	cookieRewritePoliciesService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
					CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
						{
							Name: "some-cookie",
							PathRewrite: &contour_v1.CookiePathRewrite{
								Value: "/foo",
							},
							DomainRewrite: &contour_v1.CookieDomainRewrite{
								Value: "example.com",
							},
							Secure:   ptr.To(true),
							SameSite: ptr.To("Strict"),
						},
						{
							Name:     "some-other-cookie",
							SameSite: ptr.To("Lax"),
						},
					},
				}},
			}},
		},
	}

	duplicateCookieRewritePoliciesRoute := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
					{
						Name:   "some-cookie",
						Secure: ptr.To(true),
					},
					{
						Name:     "some-cookie",
						SameSite: ptr.To("Lax"),
					},
				},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
			}},
		},
	}

	duplicateCookieRewritePoliciesService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
					CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
						{
							Name:   "some-cookie",
							Secure: ptr.To(true),
						},
						{
							Name:     "some-cookie",
							SameSite: ptr.To("Lax"),
						},
					},
				}},
			}},
		},
	}

	emptyCookieRewritePolicyRoute := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
					{
						Name: "some-cookie",
					},
				},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
			}},
		},
	}

	emptyCookieRewritePolicyService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "nginx",
					Port: 80,
					CookieRewritePolicies: []contour_v1.CookieRewritePolicy{
						{
							Name: "some-cookie",
						},
					},
				}},
			}},
		},
	}

	protocol := "h2c"
	proxy110 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name:     "kuard",
					Port:     8080,
					Protocol: &protocol,
				}},
			}},
		},
	}

	ingressExternalNameService := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "externalname",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1(s14.GetName(), intstr.FromInt(80)),
						}},
					},
				},
			}},
		},
	}

	proxyExternalNameService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: s14.GetName(),
					Port: 80,
				}},
			}},
		},
	}

	tcpProxyExternalNameService := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_v1.TLS{
					SecretName: sec1.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name:     s14.GetName(),
					Port:     80,
					Protocol: ptr.To("tls"),
				}},
			},
		},
	}

	// Invalid proxy because the regex match is in the includes block.
	proxyInvalidRegexPath := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-invalid-regexpath-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.invalid.regexpath.com",
			},
			Includes: []contour_v1.Include{{
				Name: "path",
				Conditions: []contour_v1.MatchCondition{{
					Regex: "/.*/path",
				}},
			}},
		},
	}

	proxyRegexPath := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-valid-regexpath-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.valid.regexpath.com",
			},
			Includes: []contour_v1.Include{{
				Name:      "child1",
				Namespace: s1.Namespace,
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name:      "child2",
				Namespace: s2.Namespace,
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	child1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "child1",
			Namespace: s1.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Regex: "/regex/.*",
				}},
				Services: []contour_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	child2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "child2",
			Namespace: s2.Namespace,
		},
		Spec: contour_v1.HTTPProxySpec{
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/prefix",
				}},
				Services: []contour_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	tests := map[string]struct {
		objs                         []any
		disablePermitInsecure        bool
		enableExternalNameSvc        bool
		fallbackCertificateName      string
		fallbackCertificateNamespace string
		want                         []*Listener
	}{
		"ingressv1: insert ingress w/ default backend w/o matching service": {
			objs: []any{
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ default backend": {
			objs: []any{
				i1V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ single unnamed backend w/o matching service": {
			objs: []any{
				i2V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ single unnamed backend": {
			objs: []any{
				i2V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress with missing spec.rule.http key": {
			objs: []any{
				i2aV1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ host name and single backend w/o matching service": {
			objs: []any{
				i3V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ host name and single backend": {
			objs: []any{
				i3V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert non matching service then ingress w/ default backend": {
			objs: []any{
				s2,
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ default backend then matching service with wrong port": {
			objs: []any{
				i1V1,
				s3,
			},
			want: listeners(),
		},
		"ingressv1: insert unnamed ingress w/ single backend then matching service with wrong port": {
			objs: []any{
				i2V1,
				s3,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ default backend then matching service w/ named port": {
			objs: []any{
				i4V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service w/ named port then ingress w/ default backend": {
			objs: []any{
				s1,
				i4V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ single unnamed backend w/ named service port then service": {
			objs: []any{
				i5V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service then ingress w/ single unnamed backend w/ named service port": {
			objs: []any{
				s1,
				i5V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert secret": {
			objs: []any{
				sec1,
			},
			want: listeners(),
		},
		"ingressv1: insert secret then ingress w/o tls": {
			objs: []any{
				sec1,
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, secret then ingress w/o tls": {
			objs: []any{
				s1,
				sec1,
				i1V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},

		"ingressv1: insert secret then ingress w/ tls": {
			objs: []any{
				sec1,
				i3V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, secret then ingress w/ tls": {
			objs: []any{
				s1,
				sec1,
				i3V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("kuard.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service w/ secret with w/ blank ca.crt": {
			objs: []any{
				s1,
				sec3, // issue 1644
				i3V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("kuard.example.com", sec3, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert invalid secret then ingress w/o tls": {
			objs: []any{
				secInvalid,
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, invalid secret then ingress w/o tls": {
			objs: []any{
				s1,
				secInvalid,
				i1V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert invalid secret then ingress w/ tls": {
			objs: []any{
				secInvalid,
				i3V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, invalid secret then ingress w/ tls": {
			objs: []any{
				s1,
				secInvalid,
				i3V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two vhosts": {
			objs: []any{
				i6V1,
			},
			want: nil, // no matching service
		},
		"ingressv1: insert ingress w/ two vhosts then matching service": {
			objs: []any{
				i6V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service then ingress w/ two vhosts": {
			objs: []any{
				s1,
				i6V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two vhosts then service then secret": {
			objs: []any{
				i6V1,
				s1,
				sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service then secret then ingress w/ two vhosts": {
			objs: []any{
				s1,
				sec1,
				i6V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two paths then one service": {
			objs: []any{
				i7V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", service(s1)),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two paths then services": {
			objs: []any{
				i7V1,
				s2,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
						),
					),
				},
			),
		},
		"ingressv1: insert two services then ingress w/ two ingress rules": {
			objs: []any{
				s1, s2, i8V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two paths httpAllowed: false": {
			objs: []any{
				i9V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ two paths httpAllowed: false then tls and service": {
			objs: []any{
				i9V1,
				sec1,
				s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("b.example.com", sec1,
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
						),
					),
				},
			),
		},
		"ingressv1: insert default ingress httpAllowed: false": {
			objs: []any{
				i1aV1,
			},
			want: listeners(),
		},
		"ingressv1: insert default ingress httpAllowed: false then tls and service": {
			objs: []any{
				i1aV1, sec1, s1,
			},
			want: listeners(), // default ingress cannot be tls
		},
		"ingressv1: insert ingress w/ two vhosts httpAllowed: false": {
			objs: []any{
				i6aV1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ two vhosts httpAllowed: false then tls and service": {
			objs: []any{
				i6aV1, sec1, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ force-ssl-redirect: true": {
			objs: []any{
				i6bV1, sec1, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},

		"ingressv1: insert ingress w/ force-ssl-redirect: true and allow-http: false": {
			objs: []any{
				i6cV1, sec1, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},

		"insert httpproxy with tls version 1.2": {
			objs: []any{
				proxyTLS12, s1, sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								Routes: routes(
									routeUpgrade("/", service(s1)),
								),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.2",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},

		"insert httpproxy with tls version 1.3": {
			objs: []any{
				proxyTLS13, s1, sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "foo.com",
								Routes: routes(
									routeUpgrade("/", service(s1)),
								),
							},
							MinTLSVersion: "1.3",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},

		"insert httpproxy with invalid tls version": {
			objs: []any{
				proxyTLSInvalid, s1, sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},

		"insert httpproxy referencing two backends, one missing": {
			objs: []any{
				proxyMultipleBackends, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s2))),
					),
				},
			),
		},
		"insert httpproxy with a wildcard fqdn": {
			objs: []any{
				proxyWildcardFQDN, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*.projectcontour.io",
							&Route{
								PathMatchCondition: prefixString("/"),
								HeaderMatchConditions: []HeaderMatchCondition{
									{Name: ":authority", Value: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.projectcontour\\.io(:[0-9]+)?", MatchType: "regex", Invert: false},
								},
								Clusters: clusters(service(s1)),
							}),
					),
				},
			),
		},
		"insert httpproxy referencing two backends": {
			objs: []any{
				proxyMultipleBackends, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1), service(s2))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ tls min proto annotation": {
			objs: []any{
				i10aV1,
				sec1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "b.example.com",
								Routes: routes(
									prefixroute("/", service(s1)),
								),
							},
							MinTLSVersion: "1.3",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},

		"ingressv1: insert ingress w/ websocket route annotation": {
			objs: []any{
				i11V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", service(s1)),
							routeWebsocket("/ws1", service(s1)),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ invalid legacy timeout annotation": {
			objs: []any{
				i12aV1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ invalid timeout annotation": {
			objs: []any{
				i12dV1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
						}),
					),
				},
			),
		},
		"insert httpproxy w/ invalid timeoutpolicy": {
			objs: []any{
				proxyTimeoutPolicyInvalidResponse,
				s1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ valid legacy timeout annotation": {
			objs: []any{
				i12bV1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: RouteTimeoutPolicy{
								ResponseTimeout: timeout.DurationSetting(90 * time.Second),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ valid timeout annotation": {
			objs: []any{
				i12eV1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: RouteTimeoutPolicy{
								ResponseTimeout: timeout.DurationSetting(90 * time.Second),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy w/ valid timeoutpolicy": {
			objs: []any{
				proxyTimeoutPolicyValidResponse,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy:      RouteTimeoutPolicy{ResponseTimeout: timeout.DurationSetting(90 * time.Second)},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ legacy infinite timeout annotation": {
			objs: []any{
				i12cV1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: RouteTimeoutPolicy{
								ResponseTimeout: timeout.DisabledSetting(),
							},
						}),
					),
				},
			),
		},

		"ingressv1: insert ingress w/ infinite timeout annotation": {
			objs: []any{
				i12fV1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: RouteTimeoutPolicy{
								ResponseTimeout: timeout.DisabledSetting(),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy w/ infinite timeoutpolicy": {
			objs: []any{
				proxyTimeoutPolicyInfiniteResponse,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy:      RouteTimeoutPolicy{ResponseTimeout: timeout.DisabledSetting()},
						}),
					),
				},
			),
		},
		"insert httpproxy with missing tls delegation should not present port 80": {
			objs: []any{
				s10, proxyDelegatedTLSSecret,
			},
			want: listeners(), // no listeners, ir19 is invalid
		},
		"insert httpproxy with retry annotations": {
			objs: []any{
				proxyRetryPolicyValidTimeout,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    6,
								PerTryTimeout: timeout.DurationSetting(10 * time.Second),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy with invalid PerTryTimeout": {
			objs: []any{
				proxyRetryPolicyInvalidTimeout,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    6,
								PerTryTimeout: timeout.DefaultSetting(),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy with zero retry count": {
			objs: []any{
				proxyRetryPolicyZeroRetries,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    1,
								PerTryTimeout: timeout.DurationSetting(10 * time.Second),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress with timeout policy": {
			objs: []any{
				i14V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "gateway-error",
								NumRetries:    6,
								PerTryTimeout: timeout.DurationSetting(10 * time.Second),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress with regex route": {
			objs: []any{
				i15V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: regex("/[^/]+/invoices(/.*|/?)"),
							Clusters:           clustermap(s1),
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress with invalid regex route": {
			objs: []any{
				i15InvalidRegexV1,
				s1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress with various path match types": {
			objs: []any{
				iPathMatchTypesV1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							&Route{
								PathMatchCondition: exact("/exact"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: exact("/exact_with_regex/.*"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: prefixSegment("/prefix"),
								Clusters:           clustermap(s1),
							},
							&Route{
								// Trailing slash is stripped.
								PathMatchCondition: prefixSegment("/prefix_trailing_slash"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: prefixSegment("/prefix_with_regex/.*"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: prefixString("/implementation_specific"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: regex("/implementation_specific_with_regex/.*"),
								Clusters:           clustermap(s1),
							},
						),
					),
				},
			),
		},
		"ingressv1: insert ingress with wildcard hostnames": {
			objs: []any{
				s1,
				i16V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
						virtualhost("*.example.com", &Route{
							PathMatchCondition: &PrefixMatchCondition{Prefix: "/"},
							HeaderMatchConditions: []HeaderMatchCondition{
								{
									Name:      ":authority",
									MatchType: HeaderMatchTypeRegex,
									Value:     "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com(:[0-9]+)?",
								},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress overlay": {
			objs: []any{
				i13aV1, i13bV1, sec13, s13a, s13b,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeUpgrade("/", service(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", service(s13b)),
						),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("example.com", sec13,
							routeUpgrade("/", service(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", service(s13b)),
						),
					),
				},
			),
		},
		"ingressv1: h2c service annotation": {
			objs: []any{
				i3aV1, s3a,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Protocol: "h2c",
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s3a.Name,
									ServiceNamespace: s3a.Namespace,
									ServicePort:      s3a.Spec.Ports[0],
									HealthPort:       s3a.Spec.Ports[0],
								},
							}),
						),
					),
				},
			),
		},
		"ingressv1: h2 service annotation": {
			objs: []any{
				i3aV1, s3b,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Protocol: "h2",
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s3b.Name,
									ServiceNamespace: s3b.Namespace,
									ServicePort:      s3b.Spec.Ports[0],
									HealthPort:       s3b.Spec.Ports[0],
								},
							}),
						),
					),
				},
			),
		},
		"ingressv1: tls service annotation": {
			objs: []any{
				i3aV1, s3c,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Protocol: "tls",
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s3c.Name,
									ServiceNamespace: s3c.Namespace,
									ServicePort:      s3c.Spec.Ports[0],
									HealthPort:       s3c.Spec.Ports[0],
								},
							}),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress then service w/ upstream annotations": {
			objs: []any{
				i1V1,
				s1b,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s1b.Name,
									ServiceNamespace: s1b.Namespace,
									ServicePort:      s1b.Spec.Ports[0],
									HealthPort:       s1b.Spec.Ports[0],
								},
								CircuitBreakers: CircuitBreakers{
									MaxConnections:        9000,
									MaxPendingRequests:    4096,
									MaxRequests:           404,
									MaxRetries:            7,
									PerHostMaxConnections: 45,
								},
							}),
						),
					),
				},
			),
		},

		"insert httpproxy with two routes to the same service": {
			objs: []any{
				proxyWeightsTwoRoutesDiffWeights, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/a", &Cluster{
								Upstream: &Service{
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s1.Name,
										ServiceNamespace: s1.Namespace,
										ServicePort:      s1.Spec.Ports[0],
										HealthPort:       s1.Spec.Ports[0],
									},
								},
								Weight: 90,
							}),
							routeCluster("/b", &Cluster{
								Upstream: &Service{
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s1.Name,
										ServiceNamespace: s1.Namespace,
										ServicePort:      s1.Spec.Ports[0],
										HealthPort:       s1.Spec.Ports[0],
									},
								},
								Weight: 60,
							}),
						),
					),
				},
			),
		},
		"insert httpproxy with one routes to the same service with two different weights": {
			objs: []any{
				proxyWeightsOneRouteDiffWeights, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/a",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
									Weight: 90,
								}, &Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
									Weight: 60,
								},
							),
						),
					),
				},
			),
		},
		"insert httproxy": {
			objs: []any{
				proxy1, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httproxy w/o condition": {
			objs: []any{
				proxy1b, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httproxy with invalid include": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
						},
						Includes: []contour_v1.Include{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/finance",
							}},
							Name:      "non-existent",
							Namespace: "non-existent",
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/finance"),
							DirectResponse: &DirectResponse{
								StatusCode: http.StatusBadGateway,
							},
						}),
					),
				},
			),
		},
		"insert httproxy with include references another root": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
						},
						Includes: []contour_v1.Include{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/finance",
							}},
							Name:      "other-root",
							Namespace: "default",
						}},
					},
				},
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "other-root",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example2.com",
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/finance"),
							DirectResponse: &DirectResponse{
								StatusCode: http.StatusBadGateway,
							},
						}),
					),
				},
			),
		},
		"insert httproxy w/ conditions": {
			objs: []any{
				proxy1c, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/kuard"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "x-request-id", MatchType: "present"},
								{Name: "e-tag", Value: "abcdef", MatchType: "contains"},
								{Name: "x-timeout", Value: "infinity", MatchType: "contains", Invert: true},
								{Name: "digest-auth", Value: "scott", MatchType: "exact"},
								{Name: "digest-password", Value: "tiger", MatchType: "exact", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httproxy w/ multiple routes with a Contains condition on the same header": {
			objs: []any{
				proxy2d, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "abc", MatchType: "contains"},
							},
							Clusters: clusters(service(s1)),
						}, &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "def", MatchType: "contains"},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},

		"insert httproxy w/ multiple routes with condition on the same header, one Contains and one NotContains": {
			objs: []any{
				proxy2e, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "abc", MatchType: "contains"},
							},
							Clusters: clusters(service(s1)),
						}, &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "abc", MatchType: "contains", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httproxy w/ included conditions": {
			objs: []any{
				proxy2a, proxy2b, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/kuard"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "x-request-id", MatchType: "present"},
								{Name: "x-timeout", Value: "infinity", MatchType: "contains", Invert: true},
								{Name: "digest-auth", Value: "scott", MatchType: "exact"},
								{Name: "e-tag", Value: "abcdef", MatchType: "contains"},
								{Name: "digest-password", Value: "tiger", MatchType: "exact", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}, &Route{
							PathMatchCondition: prefixString("/kuard"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "x-request-id", MatchType: "present"},
								{Name: "x-timeout", MatchType: "present", Invert: true},
								{Name: "digest-auth", Value: "scott", MatchType: "exact"},
								{Name: "e-tag", Value: "abcdef", MatchType: "contains"},
								{Name: "digest-password", Value: "tiger", MatchType: "exact", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httpproxy w/ healthcheck": {
			objs: []any{
				proxy2c, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/", &Cluster{
								Upstream: service(s1),
								HTTPHealthCheckPolicy: &HTTPHealthCheckPolicy{
									Path: "/healthz",
								},
							}),
						),
					),
				},
			),
		},
		"insert httpproxy with mirroring route": {
			objs: []any{
				proxy12, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							withMirror(prefixroute("/", service(s1)), []*Service{service(s2)}, 100),
						),
					),
				},
			),
		},
		"insert httpproxy with two mirrors": {
			objs: []any{
				proxy13, s1, s2,
			},
			want: listeners(),
		},
		"insert httpproxy with websocket route and prefix rewrite": {
			objs: []any{
				proxy10, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
							routeWebsocket("/websocket", service(s1)),
						),
					),
				},
			),
		},
		"insert httpproxy with multiple upstreams prefix rewrite route and websockets along one path": {
			objs: []any{
				proxy10b, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
							routeWebsocket("/websocket", service(s1)),
						),
					),
				},
			),
		},

		"insert httpproxy with protocol and service": {
			objs: []any{
				proxy110, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeProtocol("/", protocol, service(s1))),
					),
				},
			),
		},

		"insert httpproxy without tls version": {
			objs: []any{
				proxy6, s1, sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},

		"insert httpproxy expecting upstream verification": {
			objs: []any{
				cert1, proxy17, s1a,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Protocol: "tls",
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1a.Name,
											ServiceNamespace: s1a.Namespace,
											ServicePort:      s1a.Spec.Ports[0],
											HealthPort:       s1a.Spec.Ports[0],
										},
									},
									Protocol: "tls",
									UpstreamValidation: &PeerValidationContext{
										CACertificates: []*Secret{
											caSecret(cert1),
										},
										SubjectNames: []string{"example.com"},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with h2 expecting upstream verification": {
			objs: []any{
				cert1, proxy17h2, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1a.Name,
											ServiceNamespace: s1a.Namespace,
											ServicePort:      s1a.Spec.Ports[0],
											HealthPort:       s1a.Spec.Ports[0],
										},
									},
									Protocol: "h2",
									UpstreamValidation: &PeerValidationContext{
										CACertificates: []*Secret{
											caSecret(cert1),
										},
										SubjectNames: []string{"example.com"},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy expecting upstream verification, no certificate": {
			objs: []any{
				proxy17, s1a,
			},
			want: listeners(), // no listeners, missing certificate
		},
		"insert httpproxy expecting upstream verification, no annotation on service": {
			objs: []any{
				cert1, proxy17, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
						),
					),
				},
			),
		},
		"insert httpproxy expecting upstream verification, CA secret in different namespace is not delegated": {
			objs: []any{
				cert2, proxy17UpstreamCACertDelegation, s1a,
			},
			want: listeners(),
		},
		"insert httpproxy expecting upstream verification, CA secret in different namespace is delegated": {
			objs: []any{
				cert2, s1a,
				&contour_v1.TLSCertificateDelegation{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "CACertDelagation",
						Namespace: cert2.Namespace,
					},
					Spec: contour_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_v1.CertificateDelegation{{
							SecretName:       cert2.Name,
							TargetNamespaces: []string{"*"},
						}},
					},
				},
				proxy17UpstreamCACertDelegation,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Protocol: "tls",
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1a.Name,
											ServiceNamespace: s1a.Namespace,
											ServicePort:      s1a.Spec.Ports[0],
											HealthPort:       s1a.Spec.Ports[0],
										},
									},
									Protocol: "tls",
									UpstreamValidation: &PeerValidationContext{
										CACertificates: []*Secret{
											caSecret(cert2),
										},
										SubjectNames: []string{"example.com"},
									},
								},
							),
						),
					),
				},
			),
		},

		"insert httpproxy with downstream verification": {
			objs: []any{
				cert1, proxy18, s1, sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								Routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificates: []*Secret{
									caSecret(cert1),
								},
							},
						},
					),
				},
			),
		},

		"insert httpproxy w/ tcpproxy in tls termination mode w/ downstream verification": {
			objs: []any{
				cert1, proxy19, s1, sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificates: []*Secret{
									caSecret(cert1),
								},
							},
						},
					),
				},
			),
		},

		"insert httpproxy w/ tls termination mode w/ skip cert verification": {
			objs: []any{
				proxy20, s1, sec1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								Routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								SkipClientCertValidation: true,
							},
						},
					),
				},
			),
		},

		"insert httpproxy w/ tls termination mode w/ skip cert verification and a ca": {
			objs: []any{
				proxy21, s1, sec1, cert1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								Routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								SkipClientCertValidation: true,
								CACertificates: []*Secret{
									caSecret(cert1),
								},
							},
						},
					),
				},
			),
		},

		"insert httpproxy w/ tls termination with client validation and CRL check": {
			objs: []any{
				proxy22, s1, sec1, cert1, crl,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								Routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificates: []*Secret{
									caSecret(cert1),
								},
								CRL: crlSecret(crl),
							},
						},
					),
				},
			),
		},

		"insert httpproxy w/ tls termination with client validation and CRL check but only for leaf-certificate": {
			objs: []any{
				proxy23, s1, sec1, cert1, crl,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								Routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificates: []*Secret{
									caSecret(cert1),
								},
								CRL:                   crlSecret(crl),
								OnlyVerifyLeafCertCrl: true,
							},
						},
					),
				},
			),
		},

		"insert httpproxy w/ tls termination with client cert forwarding": {
			objs: []any{
				proxy25, s1, sec1, cert1, crl,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								Routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificates: []*Secret{
									caSecret(cert1),
								},
								ForwardClientCertificate: &ClientCertificateDetails{
									Subject: true,
									Cert:    true,
									Chain:   true,
									DNS:     true,
									URI:     true,
								},
							},
						},
					),
				},
			),
		},

		"insert httpproxy w/ tls termination with optional client validation": {
			objs: []any{
				proxy24, s1, sec1, cert1, crl,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
								Routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificates: []*Secret{
									caSecret(cert1),
								},
								OptionalClientCertificate: true,
							},
						},
					),
				},
			),
		},
		"insert httpproxy with downstream verification, missing ca certificate": {
			objs: []any{
				proxy18, s1, sec1,
			},
			want: listeners(),
		},
		"insert httpproxy with invalid tcpproxy": {
			objs: []any{proxy37, s1},
			want: listeners(),
		},
		"insert httpproxy with empty tcpproxy": {
			objs: []any{proxy37a, s1},
			want: listeners(),
		},
		"insert httpproxy w/ tcpproxy w/ missing include": {
			objs: []any{proxy38, s1},
			want: listeners(),
		},
		"insert httpproxy w/ tcpproxy w/ includes another root": {
			objs: []any{proxy38, proxy39, s1},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "www.example.com", // this is proxy39, not proxy38
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert httpproxy w/tcpproxy w/include": {
			objs: []any{proxy39broot, proxy39bchild, s1},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "www.example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		// Issue #2218
		"insert httpproxy w/tcpproxy w/include plural": {
			objs: []any{proxy39brootplural, proxy39bchild, s1},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "www.example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert httpproxy w/ tcpproxy w/ includes valid child": {
			objs: []any{proxy38, proxy40, s1},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "passthrough.example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert httproxy w/ route w/ no services": {
			objs: []any{proxy41, s1},
			want: listeners(), // expect empty, route is invalid so vhost is invalid
		},
		"insert httpproxy with pathPrefix include": {
			objs: []any{
				proxy100, proxy100a, s1, s4,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/blog",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s4.Name,
											ServiceNamespace: s4.Namespace,
											ServicePort:      s4.Spec.Ports[0],
											HealthPort:       s4.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with pathPrefix include, child adds to pathPrefix": {
			objs: []any{
				proxy100, proxy100b, s1, s4,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							&Route{
								PathMatchCondition: prefixString("/blog/infotech"),
								Clusters: []*Cluster{
									{
										Upstream: &Service{
											Weighted: WeightedService{
												Weight:           1,
												ServiceName:      s4.Name,
												ServiceNamespace: s4.Namespace,
												ServicePort:      s4.Spec.Ports[0],
												HealthPort:       s4.Spec.Ports[0],
											},
										},
									},
								},
							},
						),
					),
				},
			),
		},
		"insert httpproxy with pathPrefix include, child adds to pathPrefix, delegates again": {
			objs: []any{
				proxy100, proxy100c, proxy100d, s1, s4, s11,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							&Route{
								PathMatchCondition: prefixString("/blog/infotech"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s4.Name,
											ServiceNamespace: s4.Namespace,
											ServicePort:      s4.Spec.Ports[0],
											HealthPort:       s4.Spec.Ports[0],
										},
									},
								}},
							},
							routeCluster("/blog",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s4.Name,
											ServiceNamespace: s4.Namespace,
											ServicePort:      s4.Spec.Ports[0],
											HealthPort:       s4.Spec.Ports[0],
										},
									},
								},
							),
							&Route{
								PathMatchCondition: prefixString("/blog/it/foo"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s11.Name,
											ServiceNamespace: s11.Namespace,
											ServicePort:      s11.Spec.Ports[0],
											HealthPort:       s11.Spec.Ports[0],
										},
									},
								}},
							},
						),
					),
				},
			),
		},
		"insert httpproxy with no namespace for include": {
			objs: []any{
				proxy101, proxy101a, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
											HealthPort:       s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, no prefix condition on included proxy": {
			objs: []any{
				proxy104, proxy104a, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
											HealthPort:       s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, / on included proxy": {
			objs: []any{
				proxy105, proxy105a, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
											HealthPort:       s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, full prefix on included proxy": {
			objs: []any{
				proxy107, proxy107a, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder/withavengeance",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
											HealthPort:       s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"invalid httpproxy with exact condition in include match conditions": {
			objs: []any{proxy113, s1},
			want: listeners(),
		},
		"httpproxy with include, exact and prefix on included proxy": {
			objs: []any{
				proxy114, proxy114a, proxy114b, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example114.com",
							routeClusterExact("/foo/exact",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/bar/prefix",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
											HealthPort:       s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},

		"insert httpproxy with include ending with /, / on included proxy": {
			objs: []any{
				proxy106, proxy106a, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
											HealthPort:       s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
											HealthPort:       s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with multiple prefix conditions on route": {
			objs: []any{
				proxy102, s1,
			},
			want: listeners(),
		},
		"insert httpproxy with multiple prefix conditions on include": {
			objs: []any{
				proxy103, proxy103a, s1, s12,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						// route on root proxy is served, includes is ignored since condition is invalid
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httpproxy duplicate conditions on include": {
			objs: []any{
				proxy108, proxy108a, proxy108b, s1, s12, s13,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							// route on root proxy is served
							prefixroute("/", service(s1)),
							// route for first valid include is served
							&Route{
								PathMatchCondition: prefixString("/blog"),
								HeaderMatchConditions: []HeaderMatchCondition{
									{Name: "x-header", Value: "abc", MatchType: "contains"},
								},
								Clusters: clusters(service(s12)),
							},
						),
					),
				},
			),
		},
		"insert proxy with tcp forward without TLS termination w/ passthrough": {
			objs: []any{
				proxy1a, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		// issue 1952
		"insert proxy with tcp forward without TLS termination w/ passthrough and 301 upgrade of port 80": {
			objs: []any{
				proxy1d, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com",
							routeUpgrade("/", service(s1)),
						),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert proxy with tcp forward without TLS termination w/ passthrough without 301 upgrade of port 80": {
			objs: []any{
				proxy1e, s10,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s10.Name,
											ServiceNamespace: s10.Namespace,
											ServicePort:      s10.Spec.Ports[1],
											HealthPort:       s10.Spec.Ports[1],
										},
									},
								},
							),
						),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "kuard.example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s10),
								),
							},
							MinTLSVersion: "", // tls passthrough does not specify a TLS version; that's the domain of the backend
						},
					),
				},
			),
		},

		"insert httpproxy with route-level header manipulation": {
			objs: []any{
				proxy109, s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeHeaders("/", map[string]string{
								"In-Foo": "bar",
							}, []string{"In-Baz"}, map[string]string{
								"Out-Foo": "bar",
							}, []string{"Out-Baz"}, service(s1)),
						),
					),
				},
			),
		},

		// issue 1399
		"service shared across ingress and httpproxy tcpproxy": {
			objs: []any{
				sec1,
				s9,
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"example.com"},
							SecretName: s1.Name,
						}},
						Rules: []networking_v1.IngressRule{{
							Host:             "example.com",
							IngressRuleValue: ingressrulev1value(backendv1(s9.Name, intstr.FromInt(80))),
						}},
					},
				},
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						TCPProxy: &contour_v1.TCPProxy{
							Services: []contour_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
					),
				},
			),
		},

		// issue 1954
		"httpproxy tcpproxy + permitinsecure": {
			objs: []any{
				sec1,
				s9,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						Routes: []contour_v1.Route{{
							PermitInsecure: true,
							Services: []contour_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						}},
						TCPProxy: &contour_v1.TCPProxy{
							Services: []contour_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						// not upgraded because the route is permitInsecure: true
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
					),
				},
			),
		},
		// issue 1954
		"httpproxy tcpproxy + tlspassthrough + permitinsecure": {
			objs: []any{
				s9,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								Passthrough: true,
							},
						},
						Routes: []contour_v1.Route{{
							PermitInsecure: true,
							Services: []contour_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						}},
						TCPProxy: &contour_v1.TCPProxy{
							Services: []contour_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						// not upgraded because the route is permitInsecure: true
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							MinTLSVersion: "",
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
					),
				},
			),
		},
		"HTTPProxy request redirect policy": {
			objs: []any{
				s1,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "redirect",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "projectcontour.io",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
								Scheme:     ptr.To("https"),
								Hostname:   ptr.To("envoyproxy.io"),
								Port:       ptr.To(int32(443)),
								StatusCode: ptr.To(contour_v1.RedirectResponseCode(301)),
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Redirect: &Redirect{
								Scheme:     "https",
								Hostname:   "envoyproxy.io",
								PortNumber: 443,
								StatusCode: 301,
							},
						},
					)),
				},
			),
		},
		"HTTPProxy request redirect policy - no services": {
			objs: []any{
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "redirect",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "projectcontour.io",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
								Scheme:     ptr.To("https"),
								Hostname:   ptr.To("envoyproxy.io"),
								Port:       ptr.To(int32(443)),
								StatusCode: ptr.To(contour_v1.RedirectResponseCode(301)),
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Redirect: &Redirect{
								Scheme:     "https",
								Hostname:   "envoyproxy.io",
								PortNumber: 443,
								StatusCode: 301,
							},
						},
					)),
				},
			),
		},
		"HTTPProxy request redirect policy with multiple matches": {
			objs: []any{
				s1, s2,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "redirect",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "projectcontour.io",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_v1.Service{{
								Name: s2.Name,
								Port: 8080,
							}},
						}, {
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/blog",
							}},
							RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
								Scheme:     ptr.To("https"),
								Hostname:   ptr.To("envoyproxy.io"),
								Port:       ptr.To(int32(443)),
								StatusCode: ptr.To(contour_v1.RedirectResponseCode(301)),
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s2.Name,
										ServiceNamespace: s2.Namespace,
										ServicePort:      s2.Spec.Ports[0],
										HealthPort:       s2.Spec.Ports[0],
									},
								},
							}},
						},
						&Route{
							PathMatchCondition: prefixString("/blog"),
							Redirect: &Redirect{
								Scheme:     "https",
								Hostname:   "envoyproxy.io",
								PortNumber: 443,
								StatusCode: 301,
							},
						},
					)),
				},
			),
		},
		"HTTPProxy DirectResponse policy - code 200": {
			objs: []any{
				s1,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "direct-response",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "projectcontour.io",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
								StatusCode: 200,
								Body:       "success",
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							DirectResponse: &DirectResponse{
								StatusCode: 200,
								Body:       "success",
							},
						},
					)),
				},
			),
		},
		"HTTPProxy DirectResponse policy - no body": {
			objs: []any{
				s1,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "direct-response",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "projectcontour.io",
						},
						Routes: []contour_v1.Route{{
							Conditions: []contour_v1.MatchCondition{{
								Prefix: "/",
							}},
							DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
								StatusCode: 503,
							},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							DirectResponse: &DirectResponse{
								StatusCode: 503,
							},
						},
					)),
				},
			),
		},
		"HTTPProxy DirectResponse policy with multiple matches": {
			objs: []any{
				s1,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "direct-response",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "projectcontour.io",
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{{
									Prefix: "/",
								}},
								Services: []contour_v1.Service{{
									Name: s1.Name,
									Port: 8080,
								}},
							}, {
								Conditions: []contour_v1.MatchCondition{{
									Prefix: "/direct",
								}},
								DirectResponsePolicy: &contour_v1.HTTPDirectResponsePolicy{
									StatusCode: 404,
									Body:       "page not found",
								},
							}, {
								Conditions: []contour_v1.MatchCondition{{
									Prefix: "/redirect",
								}},
								RequestRedirectPolicy: &contour_v1.HTTPRequestRedirectPolicy{
									Scheme:     ptr.To("https"),
									Hostname:   ptr.To("envoyproxy.io"),
									Port:       ptr.To(int32(443)),
									StatusCode: ptr.To(contour_v1.RedirectResponseCode(301)),
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(virtualhost("projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s1.Name,
										ServiceNamespace: s1.Namespace,
										ServicePort:      s1.Spec.Ports[0],
										HealthPort:       s1.Spec.Ports[0],
									},
								},
							}},
						},
						&Route{
							PathMatchCondition: prefixString("/direct"),
							DirectResponse: &DirectResponse{
								StatusCode: 404,
								Body:       "page not found",
							},
						},
						&Route{
							PathMatchCondition: prefixString("/redirect"),
							Redirect: &Redirect{
								Scheme:     "https",
								Hostname:   "envoyproxy.io",
								PortNumber: 443,
								StatusCode: 301,
							},
						},
					)),
				},
			),
		},

		"ingressv1: Ingress then HTTPProxy with identical details, except referencing s2a": {
			objs: []any{
				i17V1,
				proxy1f,
				s1,
				s2a,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s2a))),
					),
				},
			),
		},
		"ingressv1: insert service, secret, then ingress w/ tls and delegation annotation (missing delegation)": {
			objs: []any{
				s1,
				sec4,
				i18V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service, secret, delegation, then ingress w/ tls and delegation annotation": {
			objs: []any{
				s1,
				sec4,
				&contour_v1.TLSCertificateDelegation{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "CertDelagation",
						Namespace: sec4.Namespace,
					},
					Spec: contour_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_v1.CertificateDelegation{{
							SecretName:       sec4.Name,
							TargetNamespaces: []string{"*"},
						}},
					},
				},
				i18V1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						securevirtualhost("kuard.example.com", sec4, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert ingress with externalName service": {
			objs: []any{
				ingressExternalNameService,
				s14,
			},
			enableExternalNameSvc: true,
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									ExternalName: "externalservice.io",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s14.Name,
										ServiceNamespace: s14.Namespace,
										ServicePort:      s14.Spec.Ports[0],
										HealthPort:       s14.Spec.Ports[0],
									},
								},
							}},
						}),
					),
				},
			),
		},
		"insert ingress with externalName service, but externalName services disabled": {
			objs: []any{
				ingressExternalNameService,
				s14,
			},
			enableExternalNameSvc: false,
			want:                  listeners(),
		},
		"insert proxy with externalName service": {
			objs: []any{
				proxyExternalNameService,
				s14,
			},
			enableExternalNameSvc: true,
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									ExternalName: "externalservice.io",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s14.Name,
										ServiceNamespace: s14.Namespace,
										ServicePort:      s14.Spec.Ports[0],
										HealthPort:       s14.Spec.Ports[0],
									},
								},
								SNI: "externalservice.io",
							}},
						}),
					),
				},
			),
		},
		"insert tcp proxy with externalName service": {
			objs: []any{
				tcpProxyExternalNameService,
				s14,
				sec1,
			},
			enableExternalNameSvc: true,
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: []*Cluster{{
									Upstream: &Service{
										ExternalName: "externalservice.io",
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s14.Name,
											ServiceNamespace: s14.Namespace,
											ServicePort:      s14.Spec.Ports[0],
											HealthPort:       s14.Spec.Ports[0],
										},
									},
									Protocol: "tls",
									SNI:      "externalservice.io",
								}},
							},
							MinTLSVersion: "1.2",
							MaxTLSVersion: "1.3",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},

		"insert proxy with replace header policy - route - host header": {
			objs: []any{
				proxyReplaceHostHeaderRoute,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: service(s9),
								SNI:      "bar.com",
							}},
							RequestHeadersPolicy: &HeadersPolicy{
								HostRewrite: "bar.com",
							},
						}),
					),
				},
			),
		},
		"insert proxy with replace header policy - route - host header - externalName": {
			objs: []any{
				proxyReplaceHostHeaderRoute,
				s14,
			},
			enableExternalNameSvc: true,
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									ExternalName: "externalservice.io",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s14.Name,
										ServiceNamespace: s14.Namespace,
										ServicePort:      s14.Spec.Ports[0],
										HealthPort:       s14.Spec.Ports[0],
									},
								},
								SNI: "bar.com",
							}},
							RequestHeadersPolicy: &HeadersPolicy{
								HostRewrite: "bar.com",
							},
						}),
					),
				},
			),
		},
		"insert proxy with replace header policy - service - host header": {
			objs: []any{
				proxyReplaceHostHeaderService,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s9.Name,
										ServiceNamespace: s9.Namespace,
										ServicePort:      s9.Spec.Ports[0],
										HealthPort:       s9.Spec.Ports[0],
									},
								},
								SNI: "bar.com",
								RequestHeadersPolicy: &HeadersPolicy{
									HostRewrite: "bar.com",
								},
							}},
						}),
					),
				},
			),
		},
		"insert proxy with replace header policy - service - host header - externalName": {
			objs: []any{
				proxyReplaceHostHeaderService,
				s14,
			},
			enableExternalNameSvc: true,
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									ExternalName: "externalservice.io",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s14.Name,
										ServiceNamespace: s14.Namespace,
										ServicePort:      s14.Spec.Ports[0],
										HealthPort:       s14.Spec.Ports[0],
									},
								},
								SNI: "bar.com",
								RequestHeadersPolicy: &HeadersPolicy{
									HostRewrite: "bar.com",
								},
							}},
						}),
					),
				},
			),
		},
		"insert proxy with response header policy - route - host header": {
			objs: []any{
				proxy111,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with response header policy - service - host header": {
			objs: []any{
				proxy112,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with replace header policy - host header multiple": {
			objs: []any{
				proxyReplaceHostHeaderMultiple,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: service(s9),
								SNI:      "bar.com",
							}},
							RequestHeadersPolicy: &HeadersPolicy{
								HostRewrite: "bar.com",
								Set: map[string]string{
									"X-Header": "bar.com",
									"Y-Header": "zed.com",
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with request headers policy - not host header": {
			objs: []any{
				proxyReplaceNonHostHeader,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s9),
							RequestHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"X-Header": "bar.com",
								},
							},
						}),
					),
				},
			),
		},

		"insert proxy with request headers policy - empty value": {
			objs: []any{
				proxyReplaceHeaderEmptyValue,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s9),
							RequestHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"X-Header": "",
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with cookie rewrite policies on route": {
			objs: []any{
				cookieRewritePoliciesRoute,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/foo"),
							Clusters:           clustermap(s9),
							CookieRewritePolicies: []CookieRewritePolicy{
								{
									Name:     "some-cookie",
									Path:     ptr.To("/foo"),
									Domain:   ptr.To("example.com"),
									Secure:   2,
									SameSite: ptr.To("Strict"),
								},
								{
									Name:     "some-other-cookie",
									SameSite: ptr.To("Lax"),
									Secure:   1,
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with cookie rewrite policies on service": {
			objs: []any{
				cookieRewritePoliciesService,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/foo"),
							Clusters: []*Cluster{
								{
									Upstream: service(s9),
									CookieRewritePolicies: []CookieRewritePolicy{
										{
											Name:     "some-cookie",
											Path:     ptr.To("/foo"),
											Domain:   ptr.To("example.com"),
											Secure:   2,
											SameSite: ptr.To("Strict"),
										},
										{
											Name:     "some-other-cookie",
											SameSite: ptr.To("Lax"),
										},
									},
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with duplicate cookie rewrite policies on route": {
			objs: []any{
				duplicateCookieRewritePoliciesRoute,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with duplicate cookie rewrite policies on service": {
			objs: []any{
				duplicateCookieRewritePoliciesService,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with empty cookie rewrite policy on route": {
			objs: []any{
				emptyCookieRewritePolicyRoute,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with empty cookie rewrite policy on service": {
			objs: []any{
				emptyCookieRewritePolicyService,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with cookie load balancing strategy": {
			objs: []any{
				proxyCookieLoadBalancer,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "Cookie"},
							},
							RequestHashPolicies: []RequestHashPolicy{
								{
									CookieHashOptions: &CookieHashOptions{
										CookieName: "X-Contour-Session-Affinity",
										TTL:        time.Duration(0),
										Path:       "/",
									},
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with load balancer hash source ip": {
			objs: []any{
				proxyLoadBalancerHashPolicySourceIP,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "RequestHash"},
							},
							RequestHashPolicies: []RequestHashPolicy{
								{
									HeaderHashOptions: &HeaderHashOptions{
										HeaderName: "X-Some-Header",
									},
								},
								{
									Terminal:     true,
									HashSourceIP: true,
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with load balancer request header hash policies": {
			objs: []any{
				proxyLoadBalancerHashPolicyHeader,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "RequestHash"},
							},
							RequestHashPolicies: []RequestHashPolicy{
								{
									Terminal: true,
									HeaderHashOptions: &HeaderHashOptions{
										HeaderName: "X-Some-Header",
									},
								},
								{
									HeaderHashOptions: &HeaderHashOptions{
										HeaderName: "X-Some-Other-Header",
									},
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with load balancer request query parameter hash policies": {
			objs: []any{
				proxyLoadBalancerHashPolicyQueryParameter,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "RequestHash"},
							},
							RequestHashPolicies: []RequestHashPolicy{
								{
									Terminal: true,
									QueryParameterHashOptions: &QueryParameterHashOptions{
										ParameterName: "something",
									},
								},
								{
									QueryParameterHashOptions: &QueryParameterHashOptions{
										ParameterName: "other",
									},
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with all invalid request hash policies": {
			objs: []any{
				proxyLoadBalancerHashPolicyAllInvalid,
				s9,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "RoundRobin"},
							},
						}),
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "example.com",
								Routes: routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							MaxTLSVersion:       "1.3",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecret),
						},
					),
				},
			),
		},

		"httpproxy with fallback certificate enabled - cert delegation not configured": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "root",
			objs: []any{
				sec4,
				s9,
				fallbackCertificateSecret,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(),
		},
		"httpproxy with fallback certificate enabled - cert delegation configured all namespaces": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "root",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecretRootNamespace,
				&contour_v1.TLSCertificateDelegation{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "fallbackcertdelegation",
						Namespace: "root",
					},
					Spec: contour_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_v1.CertificateDelegation{{
							SecretName:       "fallbacksecret",
							TargetNamespaces: []string{"*"},
						}},
					},
				},
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "example.com",
								Routes: routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							MaxTLSVersion:       "1.3",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecretRootNamespace),
						},
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled - cert delegation configured single namespaces": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "root",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecretRootNamespace,
				&contour_v1.TLSCertificateDelegation{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "fallbackcertdelegation",
						Namespace: "root",
					},
					Spec: contour_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_v1.CertificateDelegation{{
							SecretName:       "fallbacksecret",
							TargetNamespaces: []string{"default"},
						}},
					},
				},
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "example.com",
								Routes: routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							MaxTLSVersion:       "1.3",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecretRootNamespace),
						},
					),
				},
			),
		},

		"httpproxy with fallback certificate enabled - no tls secret": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: nil,
		},
		"httpproxy with fallback certificate enabled along with ClientValidation": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								EnableFallbackCertificate: true,
								ClientValidation: &contour_v1.DownstreamValidation{
									CACertificate: cert1.Name,
								},
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: nil,
		},
		"httpproxy with fallback certificate enabled - another not enabled": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx-disabled",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "projectcontour.io",
							TLS: &contour_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
						virtualhost("projectcontour.io", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "example.com",
								Routes: routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							MaxTLSVersion:       "1.3",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecret),
						},
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "projectcontour.io",
								Routes: routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							MaxTLSVersion:       "1.3",
							Secret:              secret(sec1),
							FallbackCertificate: nil,
						},
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled - bad fallback cert": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "badnamespaces",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "example.com",
								Routes: routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							MaxTLSVersion:       "1.3",
							Secret:              secret(sec1),
							FallbackCertificate: nil,
						},
					),
				},
			),
		},

		"httpproxy with fallback certificate disabled - fallback cert specified": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []any{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_v1.Route{{
							Services: []contour_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:   "example.com",
								Routes: routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							MaxTLSVersion:       "1.3",
							Secret:              secret(sec1),
							FallbackCertificate: nil,
						},
					),
				},
			),
		},
		"httpproxy with tcpproxy with multiple services, no explicit weights": {
			objs: []any{
				s1, s2, s9,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "weighted-tcpproxy",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								Passthrough: true,
							},
						},
						TCPProxy: &contour_v1.TCPProxy{
							Services: []contour_v1.Service{
								{Name: s1.Name, Port: int(s1.Spec.Ports[0].Port)},
								{Name: s2.Name, Port: int(s2.Spec.Ports[0].Port)},
								{Name: s9.Name, Port: int(s9.Spec.Ports[0].Port)},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s1), service(s2), service(s9)),
							},
						},
					),
				},
			),
		},
		"httpproxy with tcpproxy with multiple weighted services": {
			objs: []any{
				s1, s2, s9,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "weighted-tcpproxy",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								Passthrough: true,
							},
						},
						TCPProxy: &contour_v1.TCPProxy{
							Services: []contour_v1.Service{
								{Name: s1.Name, Port: int(s1.Spec.Ports[0].Port), Weight: 1},
								{Name: s2.Name, Port: int(s2.Spec.Ports[0].Port), Weight: 2},
								{Name: s9.Name, Port: int(s9.Spec.Ports[0].Port), Weight: 3},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: []*Cluster{
									{Upstream: service(s1), Weight: 1},
									{Upstream: service(s2), Weight: 2},
									{Upstream: service(s9), Weight: 3},
								},
							},
						},
					),
				},
			),
		},

		"httpproxy with tcpproxy with multiple services, some weighted, some not": {
			objs: []any{
				s1, s2, s9,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "weighted-tcpproxy",
						Namespace: "default",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_v1.TLS{
								Passthrough: true,
							},
						},
						TCPProxy: &contour_v1.TCPProxy{
							Services: []contour_v1.Service{
								{Name: s1.Name, Port: int(s1.Spec.Ports[0].Port), Weight: 1},
								{Name: s2.Name, Port: int(s2.Spec.Ports[0].Port), Weight: 0},
								{Name: s9.Name, Port: int(s9.Spec.Ports[0].Port), Weight: 3},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: HTTPS_LISTENER_NAME,
					Port: 8443,
					SecureVirtualHosts: securevirtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name: "example.com",
							},
							TCPProxy: &TCPProxy{
								Clusters: []*Cluster{
									{Upstream: service(s1), Weight: 1},
									{Upstream: service(s2), Weight: 0},
									{Upstream: service(s9), Weight: 3},
								},
							},
						},
					),
				},
			),
		},
		"invalid httpproxy with regex in include block": {
			objs: []any{proxyInvalidRegexPath, s1},
			want: listeners(),
		},
		"valid httpproxy with include, prefix and regex on included proxy": {
			objs: []any{
				proxyRegexPath, child1, child2, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.valid.regexpath.com",
							routeClusterRegex("/foo/regex/.*",
								&Cluster{
									Upstream: service(s1),
								},
							),
							routeCluster("/bar/prefix",
								&Cluster{
									Upstream: service(s2),
								},
							),
						),
					),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					FieldLogger: fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger:               fixture.NewTestLogger(t),
						EnableExternalNameService: tc.enableExternalNameSvc,
					},
					&HTTPProxyProcessor{
						EnableExternalNameService: tc.enableExternalNameSvc,
						DisablePermitInsecure:     tc.disablePermitInsecure,
						FallbackCertificate: &types.NamespacedName{
							Name:      tc.fallbackCertificateName,
							Namespace: tc.fallbackCertificateNamespace,
						},
					},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			for _, l := range dag.Listeners {
				got[l.Port] = l
			}

			want := make(map[int]*Listener)
			for _, l := range tc.want {
				want[l.Port] = l
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestGatewayWithHTTPProxyAndIngress(t *testing.T) {
	kuardService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "projectcontour",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}
	sec1 := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "secret",
			Namespace: "projectcontour",
		},
		Type: core_v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	tests := map[string]struct {
		gateway *gatewayapi_v1.Gateway
		objs    []any
		want    []*Listener
	}{
		"HTTPProxy attached to HTTP-only Gateway": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-proxy",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "kuard.projectcontour.io",
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{
										Prefix: "/",
									},
								},
								Services: []contour_v1.Service{
									{
										Name: "kuard",
										Port: 8080,
									},
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", prefixroute("/", service(kuardService))),
					),
				},
			),
		},
		"HTTPProxy attached to Gateway with multiple HTTP listeners": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "http-2",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     81,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-proxy",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "kuard.projectcontour.io",
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{
										Prefix: "/",
									},
								},
								Services: []contour_v1.Service{
									{
										Name: "kuard",
										Port: 8080,
									},
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		"HTTPProxy attached to Gateway with HTTP and HTTPS listener": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     443,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-proxy",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "kuard.projectcontour.io",
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{
										Prefix: "/",
									},
								},
								Services: []contour_v1.Service{
									{
										Name: "kuard",
										Port: 8080,
									},
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", prefixroute("/", service(kuardService))),
					),
				},
			),
		},
		"HTTPProxy with TLS attached to Gateway with HTTP and HTTPS listener": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     443,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				sec1,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-proxy",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "kuard.projectcontour.io",
							TLS: &contour_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{
										Prefix: "/",
									},
								},
								Services: []contour_v1.Service{
									{
										Name: "kuard",
										Port: 8080,
									},
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", routeUpgrade("/", service(kuardService))),
					),
				},
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: []*SecureVirtualHost{
						securevirtualhost("kuard.projectcontour.io", sec1, routeUpgrade("/", service(kuardService))),
					},
				},
			),
		},
		"HTTPProxy with TLS attached to Gateway with HTTP and HTTPS listener using projectcontour.io/https protocol": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi.ContourHTTPSProtocolType,
							Port:     443,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				sec1,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-proxy",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "kuard.projectcontour.io",
							TLS: &contour_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{
										Prefix: "/",
									},
								},
								Services: []contour_v1.Service{
									{
										Name: "kuard",
										Port: 8080,
									},
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", routeUpgrade("/", service(kuardService))),
					),
				},
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: []*SecureVirtualHost{
						securevirtualhost("kuard.projectcontour.io", sec1, routeUpgrade("/", service(kuardService))),
					},
				},
			),
		},
		"HTTPProxy with TLS attached to Gateway with no HTTPS listener": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				sec1,
				&contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-proxy",
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: "kuard.projectcontour.io",
							TLS: &contour_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						Routes: []contour_v1.Route{
							{
								Conditions: []contour_v1.MatchCondition{
									{
										Prefix: "/",
									},
								},
								Services: []contour_v1.Service{
									{
										Name: "kuard",
										Port: 8080,
									},
								},
							},
						},
					},
				},
			},
			want: nil,
		},

		"Ingress attached to HTTP-only Gateway": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-ingress",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{
							{
								Host: "kuard.projectcontour.io",
								IngressRuleValue: networking_v1.IngressRuleValue{
									HTTP: &networking_v1.HTTPIngressRuleValue{
										Paths: []networking_v1.HTTPIngressPath{
											{
												Backend: *backendv1("kuard", intstr.FromInt(8080)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", prefixroute("/", service(kuardService))),
					),
				},
			),
		},
		"Ingress attached to Gateway with multiple HTTP listeners": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "http-2",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     81,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-ingress",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{
							{
								Host: "kuard.projectcontour.io",
								IngressRuleValue: networking_v1.IngressRuleValue{
									HTTP: &networking_v1.HTTPIngressRuleValue{
										Paths: []networking_v1.HTTPIngressPath{
											{
												Backend: *backendv1("kuard", intstr.FromInt(8080)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		"Ingress attached to Gateway with HTTP and HTTPS listener": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     443,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-ingress",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{
							{
								Host: "kuard.projectcontour.io",
								IngressRuleValue: networking_v1.IngressRuleValue{
									HTTP: &networking_v1.HTTPIngressRuleValue{
										Paths: []networking_v1.HTTPIngressPath{
											{
												Backend: *backendv1("kuard", intstr.FromInt(8080)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", prefixroute("/", service(kuardService))),
					),
				},
			),
		},
		"Ingress with TLS attached to Gateway with HTTP and HTTPS listener": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi_v1.HTTPSProtocolType,
							Port:     443,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
							TLS: &gatewayapi_v1.GatewayTLSConfig{
								Mode: ptr.To(gatewayapi_v1.TLSModePassthrough),
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				sec1,
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-ingress",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{
							{
								Host: "kuard.projectcontour.io",
								IngressRuleValue: networking_v1.IngressRuleValue{
									HTTP: &networking_v1.HTTPIngressRuleValue{
										Paths: []networking_v1.HTTPIngressPath{
											{
												Backend: *backendv1("kuard", intstr.FromInt(8080)),
											},
										},
									},
								},
							},
						},
						TLS: []networking_v1.IngressTLS{
							{
								SecretName: sec1.Name,
								Hosts:      []string{"kuard.projectcontour.io"},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", prefixroute("/", service(kuardService))),
					),
				},
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: []*SecureVirtualHost{
						securevirtualhost("kuard.projectcontour.io", sec1, prefixroute("/", service(kuardService))),
					},
				},
			),
		},
		"Ingress with TLS attached to Gateway with HTTP and HTTPS listener using projectcontour.io/https protocol": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "https-1",
							Protocol: gatewayapi.ContourHTTPSProtocolType,
							Port:     443,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				sec1,
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-ingress",
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{
							{
								Host: "kuard.projectcontour.io",
								IngressRuleValue: networking_v1.IngressRuleValue{
									HTTP: &networking_v1.HTTPIngressRuleValue{
										Paths: []networking_v1.HTTPIngressPath{
											{
												Backend: *backendv1("kuard", intstr.FromInt(8080)),
											},
										},
									},
								},
							},
						},
						TLS: []networking_v1.IngressTLS{
							{
								SecretName: sec1.Name,
								Hosts:      []string{"kuard.projectcontour.io"},
							},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Name: "http-80",
					VirtualHosts: virtualhosts(
						virtualhost("kuard.projectcontour.io", prefixroute("/", service(kuardService))),
					),
				},
				&Listener{
					Name: "https-443",
					SecureVirtualHosts: []*SecureVirtualHost{
						securevirtualhost("kuard.projectcontour.io", sec1, prefixroute("/", service(kuardService))),
					},
				},
			),
		},
		"Ingress with TLS (with HTTP not allowed) attached to Gateway with no HTTPS listener": {
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: "contour-gc",
					Listeners: []gatewayapi_v1.Listener{
						{
							Name:     "http-1",
							Protocol: gatewayapi_v1.HTTPProtocolType,
							Port:     80,
							AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
								Namespaces: &gatewayapi_v1.RouteNamespaces{
									From: ptr.To(gatewayapi_v1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			objs: []any{
				kuardService,
				sec1,
				&networking_v1.Ingress{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "projectcontour",
						Name:      "kuard-ingress",
						Annotations: map[string]string{
							"kubernetes.io/ingress.allow-http": "false",
						},
					},
					Spec: networking_v1.IngressSpec{
						Rules: []networking_v1.IngressRule{
							{
								Host: "kuard.projectcontour.io",
								IngressRuleValue: networking_v1.IngressRuleValue{
									HTTP: &networking_v1.HTTPIngressRuleValue{
										Paths: []networking_v1.HTTPIngressPath{
											{
												Backend: *backendv1("kuard", intstr.FromInt(8080)),
											},
										},
									},
								},
							},
						},
						TLS: []networking_v1.IngressTLS{
							{
								SecretName: sec1.Name,
								Hosts:      []string{"kuard.projectcontour.io"},
							},
						},
					},
				},
			},
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gc := &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "contour-gc",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "projectcontour.io/gateway-controller",
				},
			}

			builder := Builder{
				Source: KubernetesCache{
					gatewayclass: gc,
					gateway:      tc.gateway,
					FieldLogger:  fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{
						HTTPAddress:  "0.0.0.0",
						HTTPSAddress: "0.0.0.0",
					},
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			for _, l := range dag.Listeners {
				got[l.Port] = l
			}

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				want[v.Port] = v
			}
			assert.Equal(t, want, got)
		})
	}
}

func backendv1(name string, port intstr.IntOrString) *networking_v1.IngressBackend {
	var v1port networking_v1.ServiceBackendPort

	switch port.Type {
	case intstr.Int:
		v1port = networking_v1.ServiceBackendPort{
			Number: port.IntVal,
		}
	case intstr.String:
		v1port = networking_v1.ServiceBackendPort{
			Name: port.StrVal,
		}
	}

	return &networking_v1.IngressBackend{
		Service: &networking_v1.IngressServiceBackend{
			Name: name,
			Port: v1port,
		},
	}
}

func ingressrulev1value(backend *networking_v1.IngressBackend) networking_v1.IngressRuleValue {
	return networking_v1.IngressRuleValue{
		HTTP: &networking_v1.HTTPIngressRuleValue{
			Paths: []networking_v1.HTTPIngressPath{{
				Backend: *backend,
			}},
		},
	}
}

func TestDAGRootNamespaces(t *testing.T) {
	proxy1 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed1",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy2 is like proxy1, but in a different namespace
	proxy2 := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed2",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example2.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	s2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "allowed1",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080)},
		},
	}

	s3 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "allowed2",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080)},
		},
	}

	tests := map[string]struct {
		rootNamespaces []string
		objs           []any
		want           int
	}{
		"nil root httpproxy namespaces": {
			objs: []any{proxy1, s2},
			want: 1,
		},
		"empty root httpproxy namespaces": {
			objs: []any{proxy1, s2},
			want: 1,
		},
		"single root namespace with root httpproxy": {
			rootNamespaces: []string{"allowed1"},
			objs:           []any{proxy1, s2},
			want:           1,
		},
		"multiple root namespaces, one with a root httpproxy": {
			rootNamespaces: []string{"foo", "allowed1", "bar"},
			objs:           []any{proxy1, s2},
			want:           1,
		},
		"multiple root namespaces, each with a root httpproxy": {
			rootNamespaces: []string{"foo", "allowed1", "allowed2"},
			objs:           []any{proxy1, proxy2, s2, s3},
			want:           2,
		},
		"root httpproxy defined outside single root namespaces": {
			rootNamespaces: []string{"foo"},
			objs:           []any{proxy1},
			want:           0,
		},
		"root httpproxy defined outside multiple root namespaces": {
			rootNamespaces: []string{"foo", "bar"},
			objs:           []any{proxy1},
			want:           0,
		},
		"two root httpproxy, one inside root namespace, one outside": {
			rootNamespaces: []string{"foo", "allowed2"},
			objs:           []any{proxy1, proxy2, s3},
			want:           1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: tc.rootNamespaces,
					FieldLogger:    fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			var got int
			if l := dag.Listeners[HTTP_LISTENER_NAME]; l != nil {
				got = len(l.VirtualHosts)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuilderRunsProcessorsInOrder(t *testing.T) {
	var got []string

	b := Builder{
		Processors: []Processor{
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "foo") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "bar") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "baz") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "abc") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "def") }),
		},
	}

	b.Build()

	assert.Equal(t, []string{"foo", "bar", "baz", "abc", "def"}, got)
}

func TestHTTPProxyConficts(t *testing.T) {
	type testcase struct {
		objs          []any
		wantListeners []*Listener
		wantStatus    map[types.NamespacedName]contour_v1.DetailedCondition
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					FieldLogger: fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&HTTPProxyProcessor{},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			gotListeners := make(map[int]*Listener)
			for _, l := range dag.Listeners {
				gotListeners[l.Port] = l
			}

			want := make(map[int]*Listener)
			for _, l := range tc.wantListeners {
				want[l.Port] = l
			}
			assert.Equal(t, want, gotListeners)

			gotStatus := make(map[types.NamespacedName]contour_v1.DetailedCondition)
			for _, pu := range dag.StatusCache.GetProxyUpdates() {
				gotStatus[pu.Fullname] = *pu.Conditions[status.ValidCondition]
			}

			assert.Equal(t, tc.wantStatus, gotStatus)
		})
	}

	existingService1 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "existing-service-1",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	existingService2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "existing-service-2",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	run(t, "root proxy with no route conditions refers to a missing service", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Routes: []contour_v1.Route{{
						Services: []contour_v1.Service{{
							Name: "missing-service",
							Port: 8080,
						}},
					}},
				},
			},
		},
		wantListeners: listeners(
			&Listener{
				Name: HTTP_LISTENER_NAME,
				Port: 8080,
				VirtualHosts: virtualhosts(
					virtualhost("example.com", directResponseRoute("/", http.StatusServiceUnavailable)),
				),
			},
		),
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "root-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "default/missing-service" not found`),
		},
	})

	run(t, "root proxy with no route conditions refers to a missing include", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Includes: []contour_v1.Include{{
						Name:      "missing-httpproxy",
						Namespace: "default",
					}},
				},
			},
		},
		wantListeners: listeners(), // No listeners and direct response since we have no route conditions to program.
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "root-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "IncludeNotFound", `include default/missing-httpproxy not found`),
		},
	})

	run(t, "root proxy with prefix route condition refers to a missing include", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Includes: []contour_v1.Include{{
						Conditions: []contour_v1.MatchCondition{{
							Prefix: "/",
						}},
						Name:      "missing-child-proxy",
						Namespace: "default",
					}},
				},
			},
		},
		wantListeners: listeners(
			&Listener{
				Name: HTTP_LISTENER_NAME,
				Port: 8080,
				VirtualHosts: virtualhosts(
					virtualhost("example.com", directResponseRoute("/", http.StatusBadGateway)),
				),
			},
		),
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "root-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "IncludeNotFound", `include default/missing-child-proxy not found`),
		},
	})

	run(t, "root proxy refers to two services, one is missing", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Routes: []contour_v1.Route{
						{
							Conditions: []contour_v1.MatchCondition{{Prefix: "/"}},
							Services: []contour_v1.Service{{
								Name: "missing-service",
								Port: 8080,
							}},
						}, {
							Conditions: []contour_v1.MatchCondition{{Prefix: "/valid"}},
							Services: []contour_v1.Service{{
								Name: "existing-service-1",
								Port: 8080,
							}},
						},
					},
				},
			},
			existingService1,
		},
		wantListeners: listeners(
			&Listener{
				Name: HTTP_LISTENER_NAME,
				Port: 8080,
				VirtualHosts: virtualhosts(
					virtualhost("example.com",
						directResponseRoute("/", http.StatusServiceUnavailable),
						prefixroute("/valid", service(existingService1))),
				),
			},
		),
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "root-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "default/missing-service" not found`),
		},
	})

	run(t, "root proxy refers to three services with weights, one is missing", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Routes: []contour_v1.Route{
						{
							Conditions: []contour_v1.MatchCondition{{Prefix: "/"}},
							Services: []contour_v1.Service{{
								Name:   "missing-service",
								Port:   8080,
								Weight: 50,
							}, {
								Name:   "existing-service-1",
								Port:   8080,
								Weight: 30,
							}, {
								Name:   "existing-service-2",
								Port:   8080,
								Weight: 20,
							}},
						},
					},
				},
			},
			existingService1,
			existingService2,
		},
		wantListeners: listeners(
			&Listener{
				Name: HTTP_LISTENER_NAME,
				Port: 8080,
				VirtualHosts: virtualhosts(
					virtualhost("example.com",
						routeCluster("/",
							&Cluster{
								Upstream: service(existingService1),
								Weight:   30,
							}, &Cluster{
								Upstream: service(existingService2),
								Weight:   20,
							},
						),
					),
				),
			},
		),
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "root-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "default/missing-service" not found`),
		},
	})

	run(t, "root proxy with two includes, one refers to a missing child proxy", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Includes: []contour_v1.Include{{
						Conditions: []contour_v1.MatchCondition{{
							Prefix: "/",
						}},
						Name:      "missing-child-proxy",
						Namespace: "default",
					}, {
						Conditions: []contour_v1.MatchCondition{{
							Prefix: "/valid",
						}},
						Name:      "valid-child-proxy",
						Namespace: "default",
					}},
				},
			},
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "valid-child-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					Routes: []contour_v1.Route{{
						Services: []contour_v1.Service{{
							Name: "existing-service-1",
							Port: 8080,
						}},
					}},
				},
			},
			existingService1,
		},
		wantListeners: listeners(
			&Listener{
				Name: HTTP_LISTENER_NAME,
				Port: 8080,
				VirtualHosts: virtualhosts(
					virtualhost("example.com",
						directResponseRoute("/", http.StatusBadGateway),
						prefixroute("/valid", service(existingService1)),
					),
				),
			},
		),
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "valid-child-proxy", Namespace: "default"}: fixture.NewValidCondition().Valid(),
			{Name: "root-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeIncludeError, "IncludeNotFound", `include default/missing-child-proxy not found`),
		},
	})

	run(t, "root proxy includes child proxy that refers to a missing service", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Includes: []contour_v1.Include{{
						Name:       "invalid-child-proxy",
						Namespace:  "default",
						Conditions: []contour_v1.MatchCondition{{Prefix: "/missing"}},
					}},
				},
			},
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "invalid-child-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					Routes: []contour_v1.Route{{
						Services: []contour_v1.Service{{
							Name: "missing-service",
							Port: 8080,
						}},
					}},
				},
			},
		},
		wantListeners: listeners(
			&Listener{
				Name: HTTP_LISTENER_NAME,
				Port: 8080,
				VirtualHosts: virtualhosts(
					virtualhost("example.com", directResponseRoute("/missing", http.StatusServiceUnavailable)),
				),
			},
		),
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "invalid-child-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "default/missing-service" not found`),
			{Name: "root-proxy", Namespace: "default"}: fixture.NewValidCondition().Valid(),
		},
	})

	run(t, "root proxy includes two child proxies, one refers to a missing service", testcase{
		objs: []any{
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "root-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "example.com",
					},
					Includes: []contour_v1.Include{{
						Name:       "invalid-child-proxy",
						Namespace:  "default",
						Conditions: []contour_v1.MatchCondition{{Prefix: "/missing"}},
					}, {
						Name:       "valid-child-proxy",
						Namespace:  "default",
						Conditions: []contour_v1.MatchCondition{{Prefix: "/existing"}},
					}},
				},
			},
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "invalid-child-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					Routes: []contour_v1.Route{{
						Services: []contour_v1.Service{{
							Name: "missing-service",
							Port: 8080,
						}},
					}},
				},
			},
			&contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "valid-child-proxy",
					Namespace: "default",
				},
				Spec: contour_v1.HTTPProxySpec{
					Routes: []contour_v1.Route{{
						Services: []contour_v1.Service{{
							Name: "existing-service-1",
							Port: 8080,
						}},
					}},
				},
			},
			existingService1,
		},
		wantListeners: listeners(
			&Listener{
				Name: HTTP_LISTENER_NAME,
				Port: 8080,
				VirtualHosts: []*VirtualHost{
					virtualhost("example.com",
						directResponseRoute("/missing", http.StatusServiceUnavailable),
						prefixroute("/existing", service(existingService1))),
				},
			},
		),
		wantStatus: map[types.NamespacedName]contour_v1.DetailedCondition{
			{Name: "invalid-child-proxy", Namespace: "default"}: fixture.NewValidCondition().
				WithError(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "default/missing-service" not found`),
			{Name: "valid-child-proxy", Namespace: "default"}: fixture.NewValidCondition().Valid(),
			{Name: "root-proxy", Namespace: "default"}:        fixture.NewValidCondition().Valid(),
		},
	})
}

func TestDefaultHeadersPolicies(t *testing.T) {
	// i2V1 is functionally identical to i1V1
	i2V1 := &networking_v1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}

	proxyMultipleBackends := &contour_v1.HTTPProxy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_v1.Route{{
				Conditions: []contour_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}, {
					Name: "kuarder",
					Port: 8080,
				}},
			}},
		},
	}

	s1 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	// s2 is like s1 but with a different name
	s2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuarder",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	tests := []struct {
		name            string
		objs            []any
		want            []*Listener
		ingressReqHp    *HeadersPolicy
		ingressRespHp   *HeadersPolicy
		httpProxyReqHp  *HeadersPolicy
		httpProxyRespHp *HeadersPolicy
		wantErr         error
	}{
		{
			name: "empty is fine",
		}, {
			name: "ingressv1: insert ingress w/ single unnamed backend",
			objs: []any{
				i2V1,
				s1,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeadersUnweighted(map[string]string{"Custom-Header-Set": "foo-bar"}, nil, []string{"K-Nada"}, "", service(s1)),
						},
						),
					),
				},
			),
			ingressReqHp: &HeadersPolicy{
				// Add not currently siupported
				// Add: map[string]string{
				// 	"Custom-Header-Add": "foo-bar",
				// },
				Set: map[string]string{
					"Custom-Header-Set": "foo-bar",
				},
				Remove: []string{"K-Nada"},
			},
			ingressRespHp: &HeadersPolicy{
				// Add not currently siupported
				// Add: map[string]string{
				// 	"Custom-Header-Add": "foo-bar",
				// },
				Set: map[string]string{
					"Custom-Header-Set": "foo-bar",
				},
				Remove: []string{"K-Nada"},
			},
		}, {
			name: "insert httpproxy referencing two backends",
			objs: []any{
				proxyMultipleBackends, s1, s2,
			},
			want: listeners(
				&Listener{
					Name: HTTP_LISTENER_NAME,
					Port: 8080,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeadersUnweighted(map[string]string{"Custom-Header-Set": "foo-bar"}, nil, []string{"K-Nada"}, "", service(s1), service(s2)),
						},
						),
					),
				},
			),
			httpProxyReqHp: &HeadersPolicy{
				// Add not currently siupported
				// Add: map[string]string{
				// 	"Custom-Header-Add": "foo-bar",
				// },
				Set: map[string]string{
					"Custom-Header-Set": "foo-bar",
				},
				Remove: []string{"K-Nada"},
			},
			httpProxyRespHp: &HeadersPolicy{
				// Add not currently siupported
				// Add: map[string]string{
				// 	"Custom-Header-Add": "foo-bar",
				// },
				Set: map[string]string{
					"Custom-Header-Set": "foo-bar",
				},
				Remove: []string{"K-Nada"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					FieldLogger: fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&ListenerProcessor{},
					&IngressProcessor{
						FieldLogger:           fixture.NewTestLogger(t),
						RequestHeadersPolicy:  tc.ingressReqHp,
						ResponseHeadersPolicy: tc.ingressRespHp,
					},
					&HTTPProxyProcessor{
						RequestHeadersPolicy:  tc.httpProxyReqHp,
						ResponseHeadersPolicy: tc.httpProxyRespHp,
					},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			for _, l := range dag.Listeners {
				got[l.Port] = l
			}

			want := make(map[int]*Listener)
			for _, l := range tc.want {
				want[l.Port] = l
			}
			assert.Equal(t, want, got)
		})
	}
}

func routes(routes ...*Route) map[string]*Route {
	if len(routes) == 0 {
		return nil
	}
	m := make(map[string]*Route)
	for _, r := range routes {
		m[conditionsToString(r)] = r
	}
	return m
}

func directResponseRoute(prefix string, statusCode uint32) *Route {
	return &Route{
		PathMatchCondition: prefixString(prefix),
		DirectResponse:     &DirectResponse{StatusCode: statusCode},
	}
}

func directResponseRouteService(prefix string, statusCode uint32, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: prefixString(prefix),
		DirectResponse:     &DirectResponse{StatusCode: statusCode},
		Clusters:           clustersWeight(services...),
	}
}

func prefixroute(prefix string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           clusters(services...),
	}
}

func prefixrouteHTTPRoute(prefix string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           clustersWeight(services...),
	}
}

func segmentPrefixHTTPRoute(prefix string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: prefixSegment(prefix),
		Clusters:           clustersWeight(services...),
	}
}

func exactrouteHTTPRoute(path string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: &ExactMatchCondition{Path: path},
		Clusters:           clustersWeight(services...),
	}
}

func regexrouteHTTPRoute(path string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: &RegexMatchCondition{Regex: path},
		Clusters:           clustersWeight(services...),
	}
}

func exactrouteGRPCRoute(path string, first *Service, rest ...*Service) *Route {
	return exactrouteHTTPRoute(path, first, rest...)
}

func routeProtocol(prefix, protocol string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)

	cs := clusters(services...)
	for _, c := range cs {
		c.Protocol = protocol
	}
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           cs,
	}
}

func routeCluster(prefix string, first *Cluster, rest ...*Cluster) *Route {
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           append([]*Cluster{first}, rest...),
	}
}

func routeClusterExact(path string, first *Cluster, rest ...*Cluster) *Route {
	return &Route{
		PathMatchCondition: exact(path),
		Clusters:           append([]*Cluster{first}, rest...),
	}
}

func routeClusterRegex(regexp string, first *Cluster, rest ...*Cluster) *Route {
	return &Route{
		PathMatchCondition: regex(regexp),
		Clusters:           append([]*Cluster{first}, rest...),
	}
}

func routeUpgrade(prefix string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.HTTPSUpgrade = true
	return r
}

func routeWebsocket(prefix string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.Websocket = true
	return r
}

func routeHeaders(prefix string, requestSet map[string]string, requestRemove []string, responseSet map[string]string, responseRemove []string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.RequestHeadersPolicy = &HeadersPolicy{
		Set:    requestSet,
		Remove: requestRemove,
	}
	r.ResponseHeadersPolicy = &HeadersPolicy{
		Set:    responseSet,
		Remove: responseRemove,
	}
	return r
}

func clusterHeaders(requestSet, requestAdd map[string]string, requestRemove []string, hostRewrite string, responseSet, responseAdd map[string]string, responseRemove []string, services ...*Service) (c []*Cluster) {
	var requestHeadersPolicy *HeadersPolicy
	if requestSet != nil || requestAdd != nil || requestRemove != nil || hostRewrite != "" {
		requestHeadersPolicy = &HeadersPolicy{
			Set:         requestSet,
			Add:         requestAdd,
			Remove:      requestRemove,
			HostRewrite: hostRewrite,
		}
	}
	var responseHeadersPolicy *HeadersPolicy
	if responseSet != nil || responseAdd != nil || responseRemove != nil {
		responseHeadersPolicy = &HeadersPolicy{
			Set:    responseSet,
			Add:    responseAdd,
			Remove: responseRemove,
		}
	}
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream:              s,
			Protocol:              s.Protocol,
			RequestHeadersPolicy:  requestHeadersPolicy,
			ResponseHeadersPolicy: responseHeadersPolicy,
			Weight:                s.Weighted.Weight,
		})
	}
	return c
}

func clusterHeadersUnweighted(headersSet, headersAdd map[string]string, headersRemove []string, hostRewrite string, services ...*Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
			Protocol: s.Protocol,
			RequestHeadersPolicy: &HeadersPolicy{
				Set:         headersSet,
				Add:         headersAdd,
				Remove:      headersRemove,
				HostRewrite: hostRewrite,
			},
			ResponseHeadersPolicy: &HeadersPolicy{
				Set:         headersSet,
				Add:         headersAdd,
				Remove:      headersRemove,
				HostRewrite: hostRewrite,
			},
		})
	}
	return c
}

func clusters(services ...*Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
			Protocol: s.Protocol,
		})
	}
	return c
}

func clustersWeight(services ...*Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
			Protocol: s.Protocol,
			Weight:   s.Weighted.Weight,
		})
	}
	return c
}

func service(s *core_v1.Service) *Service {
	return weightedService(s, 1)
}

func appProtcolService(s *core_v1.Service, protocol string, portIndex ...int) *Service {
	idx := 0
	if len(portIndex) > 0 {
		idx = portIndex[0]
	}
	return &Service{
		Weighted: WeightedService{
			Weight:           1,
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[idx],
			HealthPort:       s.Spec.Ports[idx],
		},
		Protocol: protocol,
	}
}

func weightedService(s *core_v1.Service, weight uint32) *Service {
	return &Service{
		Weighted: WeightedService{
			Weight:           weight,
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0],
			HealthPort:       s.Spec.Ports[0],
		},
	}
}

func grpcService(s *core_v1.Service, protocol string) *Service {
	return &Service{
		Protocol: protocol,
		Weighted: WeightedService{
			Weight:           1,
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0],
			HealthPort:       s.Spec.Ports[0],
		},
	}
}

func healthService(s *core_v1.Service) *Service {
	return weightedHealthService(s, 1)
}

func weightedHealthService(s *core_v1.Service, weight uint32) *Service {
	return &Service{
		Weighted: WeightedService{
			Weight:           weight,
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0],
			HealthPort:       s.Spec.Ports[1],
		},
	}
}

func clustermap(services ...*core_v1.Service) []*Cluster {
	var c []*Cluster
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: service(s),
		})
	}
	return c
}

func secret(s *core_v1.Secret) *Secret {
	return &Secret{
		Object:         s,
		ValidTLSSecret: &SecretValidationStatus{},
	}
}

func caSecret(s *core_v1.Secret) *Secret {
	return &Secret{
		Object:        s,
		ValidCASecret: &SecretValidationStatus{},
	}
}

func crlSecret(s *core_v1.Secret) *Secret {
	return &Secret{
		Object:         s,
		ValidCRLSecret: &SecretValidationStatus{},
	}
}

func virtualhosts(vx ...*VirtualHost) []*VirtualHost {
	return vx
}

func securevirtualhosts(vx ...*SecureVirtualHost) []*SecureVirtualHost {
	return vx
}

func virtualhost(name string, first *Route, rest ...*Route) *VirtualHost {
	return &VirtualHost{
		Name:   name,
		Routes: routes(append([]*Route{first}, rest...)...),
	}
}

func securevirtualhost(name string, sec *core_v1.Secret, first *Route, rest ...*Route) *SecureVirtualHost {
	return &SecureVirtualHost{
		VirtualHost: VirtualHost{
			Name:   name,
			Routes: routes(append([]*Route{first}, rest...)...),
		},
		MinTLSVersion: "1.2",
		MaxTLSVersion: "1.3",
		Secret:        secret(sec),
	}
}

func listeners(ls ...*Listener) []*Listener {
	var v []*Listener

	for _, listener := range ls {
		switch listener.Name {
		case HTTP_LISTENER_NAME:
			listener.RouteConfigName = "ingress_http"
			listener.Protocol = "http"
		case HTTPS_LISTENER_NAME:
			listener.RouteConfigName = "https"
			listener.FallbackCertRouteConfigName = "ingress_fallbackcert"
			listener.Protocol = "https"
		case "http-80":
			listener.Protocol = "http"
			listener.Address = "0.0.0.0"
			listener.Port = 8080
			listener.EnableWebsockets = true
		case "https-443":
			listener.Protocol = "https"
			listener.Address = "0.0.0.0"
			listener.Port = 8443
			listener.EnableWebsockets = true
		}
	}

	v = append(v, ls...)
	return v
}

func prefixString(prefix string) MatchCondition {
	return &PrefixMatchCondition{Prefix: prefix, PrefixMatchType: PrefixMatchString}
}

func prefixSegment(prefix string) MatchCondition {
	return &PrefixMatchCondition{Prefix: prefix, PrefixMatchType: PrefixMatchSegment}
}
func exact(path string) MatchCondition  { return &ExactMatchCondition{Path: path} }
func regex(regex string) MatchCondition { return &RegexMatchCondition{Regex: regex} }

func withMirror(r *Route, mirrors []*Service, weight int64) *Route {
	for _, mirror := range mirrors {
		r.MirrorPolicies = append(r.MirrorPolicies, &MirrorPolicy{
			Cluster: &Cluster{
				Upstream: mirror,
			},
			Weight: weight,
		})
	}
	return r
}

func makeHTTPRouteTimeouts(request, backendRequest string) *gatewayapi_v1.HTTPRouteTimeouts {
	httpRouteTimeouts := &gatewayapi_v1.HTTPRouteTimeouts{}

	if request != "" {
		httpRouteTimeouts.Request = ptr.To(gatewayapi_v1.Duration(request))
	}
	if backendRequest != "" {
		httpRouteTimeouts.BackendRequest = ptr.To(gatewayapi_v1.Duration(backendRequest))
	}

	return httpRouteTimeouts
}

func makeHTTPRouteWithTimeouts(request, backendRequest string) *gatewayapi_v1.HTTPRoute {
	return &gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "basic",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
			},
			Hostnames: []gatewayapi_v1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1.HTTPRouteRule{{
				Matches:     gatewayapi.HTTPRouteMatch(gatewayapi_v1.PathMatchPathPrefix, "/"),
				BackendRefs: gatewayapi.HTTPBackendRef("kuard", 8080, 1),
				Timeouts:    makeHTTPRouteTimeouts(request, backendRequest),
			}},
		},
	}
}

func makeHTTPRoute(name, namespace, hostname string, firstRule gatewayapi_v1.HTTPRouteRule, additionalRules ...gatewayapi_v1.HTTPRouteRule) *gatewayapi_v1.HTTPRoute {
	rules := []gatewayapi_v1.HTTPRouteRule{firstRule}
	if len(additionalRules) > 0 {
		rules = append(rules, additionalRules...)
	}
	var hostnames []gatewayapi_v1.Hostname
	if hostname != "" {
		hostnames = []gatewayapi_v1.Hostname{
			gatewayapi_v1.Hostname(hostname),
		}
	}
	return &gatewayapi_v1.HTTPRoute{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayapi_v1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapi_v1.CommonRouteSpec{
				ParentRefs: []gatewayapi_v1.ParentReference{gatewayapi.GatewayParentRef("projectcontour", "contour")},
			},
			Hostnames: hostnames,
			Rules:     rules,
		},
	}
}

func makeHTTPRouteRule(pathType gatewayapi_v1.PathMatchType, pathValue, serviceName string, port uint16, weight int32) gatewayapi_v1.HTTPRouteRule {
	return gatewayapi_v1.HTTPRouteRule{
		Matches:     gatewayapi.HTTPRouteMatch(pathType, pathValue),
		BackendRefs: gatewayapi.HTTPBackendRef(serviceName, port, weight),
	}
}
