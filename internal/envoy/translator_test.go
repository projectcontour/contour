// Copyright © 2017 Heptio
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

package envoy

import (
	"io/ioutil"
	"reflect"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/heptio/contour/internal/log/stdlog"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type testClusterCache map[string]*v2.Cluster

func (cc testClusterCache) Add(c *v2.Cluster) {
	cc[c.Name] = c
}

func (cc testClusterCache) Remove(name string) {
	delete(cc, name)
}

func (cc testClusterCache) Values() []*v2.Cluster {
	var r []*v2.Cluster
	for _, v := range cc {
		r = append(r, v)
	}
	return r
}

func TestTranslateService(t *testing.T) {
	tests := []struct {
		name string
		svc  *v1.Service
		want testClusterCache
	}{{
		name: "single service port",
		svc: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}},
			},
		},
		want: testClusterCache{
			"default/simple/80": &v2.Cluster{
				Name: "default/simple/80",
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
					ServiceName: "default/simple/6502",
				},
				ConnectTimeout: &duration.Duration{
					Nanos: 250 * millisecond,
				},
				LbPolicy: v2.Cluster_ROUND_ROBIN,
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			cc := make(testClusterCache)
			tr := &Translator{
				Logger:       stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS),
				ClusterCache: cc,
			}
			tr.translateService(tc.svc)
			if !reflect.DeepEqual(tc.want, tr.ClusterCache) {
				t.Fatalf("translateService(%v): got: %v, want: %v", tc.svc, tr.ClusterCache, tc.want)
			}
		})
	}
}

type testClusterLoadAssignmentCache map[string]*v2.ClusterLoadAssignment

func (cc testClusterLoadAssignmentCache) Add(c *v2.ClusterLoadAssignment) {
	cc[c.ClusterName] = c
}

func (cc testClusterLoadAssignmentCache) Remove(name string) {
	delete(cc, name)
}

func (cc testClusterLoadAssignmentCache) Values() []*v2.ClusterLoadAssignment {
	var r []*v2.ClusterLoadAssignment
	for _, v := range cc {
		r = append(r, v)
	}
	return r
}

func TestTranslateEndpoints(t *testing.T) {
	tests := []struct {
		name string
		ep   *v1.Endpoints
		want testClusterLoadAssignmentCache
	}{{
		name: "simple",
		ep: &v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Subsets: []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: "192.168.183.24",
				}},
				Ports: []v1.EndpointPort{{
					Port: 8080,
				}},
			}},
		},
		want: testClusterLoadAssignmentCache{
			"default/simple/8080": &v2.ClusterLoadAssignment{
				ClusterName: "default/simple/8080",
				Endpoints: []*v2.LocalityLbEndpoints{{
					Locality: &v2.Locality{
						Region:  "ap-southeast-2", // totally a guess
						Zone:    "2b",
						SubZone: "banana", // yeah, need to think of better values here
					},
					LbEndpoints: []*v2.LbEndpoint{{
						Endpoint: &v2.Endpoint{
							Address: &v2.Address{
								Address: &v2.Address_SocketAddress{
									SocketAddress: &v2.SocketAddress{
										Protocol: v2.SocketAddress_TCP,
										Address:  "192.168.183.24",
										PortSpecifier: &v2.SocketAddress_PortValue{
											PortValue: 8080,
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
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			cc := make(testClusterLoadAssignmentCache)
			tr := &Translator{
				Logger: stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS),
				ClusterLoadAssignmentCache: cc,
			}
			tr.translateEndpoints(tc.ep)
			if !reflect.DeepEqual(tc.want, tr.ClusterLoadAssignmentCache) {
				t.Fatalf("translateEndpoints(%v): got: %v, want: %v", tc.ep, tr.ClusterLoadAssignmentCache, tc.want)
			}
		})
	}
}

type testVirtualHostCache map[string]*v2.VirtualHost

func (cc testVirtualHostCache) Add(c *v2.VirtualHost) {
	cc[c.Name] = c
}

func (cc testVirtualHostCache) Remove(name string) {
	delete(cc, name)
}

func (cc testVirtualHostCache) Values() []*v2.VirtualHost {
	var r []*v2.VirtualHost
	for _, v := range cc {
		r = append(r, v)
	}
	return r
}

func TestTranslateIngress(t *testing.T) {
	tests := []struct {
		name string
		ing  *v1beta1.Ingress
		want testVirtualHostCache
	}{{
		name: "default backend",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		want: testVirtualHostCache{
			"default/simple": &v2.VirtualHost{
				Name:    "default/simple",
				Domains: []string{"*"},
				Routes: []*v2.Route{{
					Match: &v2.RouteMatch{
						PathSpecifier: &v2.RouteMatch_Prefix{
							Prefix: "/", // match all
						},
					},
					Action: &v2.Route_Route{
						Route: &v2.RouteAction{
							ClusterSpecifier: &v2.RouteAction_Cluster{
								Cluster: "default/backend/80",
							},
						},
					},
				}},
			},
		},
	}, {
		name: "incorrect ingress class",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "incorrect",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "nginx",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		want: make(testVirtualHostCache), // expected to be empty, the ingress class is ingnored
	}, {
		name: "explicit ingress class",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "correct",
				Namespace: "default",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "contour",
				},
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		},
		want: testVirtualHostCache{
			"default/correct": &v2.VirtualHost{
				Name:    "default/correct",
				Domains: []string{"*"},
				Routes: []*v2.Route{{
					Match: &v2.RouteMatch{
						PathSpecifier: &v2.RouteMatch_Prefix{
							Prefix: "/", // match all
						},
					},
					Action: &v2.Route_Route{
						Route: &v2.RouteAction{
							ClusterSpecifier: &v2.RouteAction_Cluster{
								Cluster: "default/backend/80",
							},
						},
					},
				}},
			},
		},
	}, {
		name: "name based vhost",
		ing: &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: "default",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "httpbin.org",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "httpbin-org",
									ServicePort: intstr.FromInt(80),
								},
							}},
						},
					},
				}},
			},
		},
		want: testVirtualHostCache{
			"default/httpbin/httpbin.org": &v2.VirtualHost{
				Name:    "default/httpbin/httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []*v2.Route{{
					Match: &v2.RouteMatch{
						PathSpecifier: &v2.RouteMatch_Prefix{
							Prefix: "/", // match all
						},
					},
					Action: &v2.Route_Route{
						Route: &v2.RouteAction{
							ClusterSpecifier: &v2.RouteAction_Cluster{
								Cluster: "default/httpbin-org/80",
							},
						},
					},
				}},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			cc := make(testVirtualHostCache)
			tr := &Translator{
				Logger:           stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS),
				VirtualHostCache: cc,
			}
			tr.translateIngress(tc.ing)
			if !reflect.DeepEqual(tc.want, tr.VirtualHostCache) {
				t.Fatalf("translateIngress(%v): got: %v, want: %v", tc.ing, tr.VirtualHostCache, tc.want)
			}
		})
	}
}

func TestHashname(t *testing.T) {
	tests := []struct {
		name string
		l    int
		s    []string
		want string
	}{
		{name: "empty s", l: 99, s: nil, want: ""},
		{name: "single element", l: 99, s: []string{"alpha"}, want: "alpha"},
		{name: "long single element, hashed", l: 12, s: []string{"gammagammagamma"}, want: "0d350ea5c204"},
		{name: "single element, truncated", l: 4, s: []string{"alpha"}, want: "8ed3"},
		{name: "two elements, truncated", l: 19, s: []string{"gammagamma", "betabeta"}, want: "ga-edf159/betabeta"},
		{name: "three elements", l: 99, s: []string{"alpha", "beta", "gamma"}, want: "alpha/beta/gamma"},
		{name: "issue/25", l: 60, s: []string{"default", "my-sevice-name", "my-very-very-long-service-host-name.my.domainname"}, want: "default/my-sevice-name/my-very-very--665863"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hashname(tc.l, append([]string{}, tc.s...)...)
			if got != tc.want {
				t.Fatalf("hashname(%d, %q): got %q, want %q", tc.l, tc.s, got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		l      int
		s      string
		suffix string
		want   string
	}{
		{name: "no truncate", l: 10, s: "quijibo", suffix: "a8c5e6", want: "quijibo"},
		{name: "limit", l: len("quijibo"), s: "quijibo", suffix: "a8c5e6", want: "quijibo"},
		{name: "truncate some", l: 6, s: "quijibo", suffix: "a8c5", want: "q-a8c5"},
		{name: "truncate suffix", l: 4, s: "quijibo", suffix: "a8c5", want: "a8c5"},
		{name: "truncate more", l: 3, s: "quijibo", suffix: "a8c5", want: "a8c"},
		{name: "long single element, truncated", l: 9, s: "gammagamma", suffix: "0d350e", want: "ga-0d350e"},
		{name: "long single element, truncated", l: 12, s: "gammagammagamma", suffix: "0d350e", want: "gamma-0d350e"},
		{name: "issue/25", l: 60 / 3, s: "my-very-very-long-service-host-name.my.domainname", suffix: "a8c5e6", want: "my-very-very--a8c5e6"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.l, tc.s, tc.suffix)
			if got != tc.want {
				t.Fatalf("hashname(%d, %q, %q): got %q, want %q", tc.l, tc.s, tc.suffix, got, tc.want)
			}
		})
	}
}
