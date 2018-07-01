// +build yolo

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

package contour

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTranslatorAddService(t *testing.T) {
	tests := []struct {
		name string
		svc  *v1.Service
		want []proto.Message
	}{{
		name: "single service port",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []proto.Message{
			cluster("default/simple/80", "default/simple"),
		},
	}, {
		name: "long namespace and service name",
		svc: service(
			"beurocratic-company-test-domain-1",
			"tiny-cog-department-test-instance",
			v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			},
		),
		want: []proto.Message{
			cluster(
				"beurocratic-company-test-domain-1/tiny-cog-depa-52e801/80",
				"beurocratic-company-test-domain-1/tiny-cog-department-test-instance", // ServiceName is not subject to the 60 char limit
			),
		},
	}, {
		name: "single named service port",
		svc: service("default", "simple", v1.ServicePort{
			Name:       "http",
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []proto.Message{
			cluster("default/simple/80", "default/simple/http"),
			cluster("default/simple/http", "default/simple/http"),
		},
	}, {
		name: "two service ports",
		svc: service("default", "simple", v1.ServicePort{
			Name:       "http",
			Protocol:   "TCP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}, v1.ServicePort{
			Name:       "alt",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromString("9001"),
		}),
		want: []proto.Message{
			cluster("default/simple/80", "default/simple/http"),
			cluster("default/simple/8080", "default/simple/alt"),
			cluster("default/simple/alt", "default/simple/alt"),
			cluster("default/simple/http", "default/simple/http"),
		},
	}, {
		// TODO(dfc) I think this is impossible, the apiserver would require
		// these ports to be named.
		name: "one tcp service, one udp service",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "UDP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}, v1.ServicePort{
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromString("9001"),
		}),
		want: []proto.Message{
			cluster("default/simple/8080", "default/simple"),
		},
	}, {
		name: "one udp service",
		svc: service("default", "simple", v1.ServicePort{
			Protocol:   "UDP",
			Port:       80,
			TargetPort: intstr.FromInt(6502),
		}),
		want: []proto.Message{},
	}}

	log := testLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tr.OnAdd(tc.svc)
			got := contents(&tr.ClusterCache)
			sort.Stable(clusterByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
		})
	}
}

func TestTranslatorUpdateService(t *testing.T) {
	tests := map[string]struct {
		oldObj *v1.Service
		newObj *v1.Service
		want   []proto.Message
	}{
		"remove named service port": {
			oldObj: service("default", "kuard",
				v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
				v1.ServicePort{
					Name:       "https",
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			),
			newObj: service("default", "kuard",
				v1.ServicePort{
					Name:       "https",
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			),
			want: []proto.Message{
				&v2.Cluster{
					Name: "default/kuard/443",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard/https",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				}, &v2.Cluster{
					Name: "default/kuard/https",
					Type: v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig:   apiconfigsource("contour"), // hard coded by initconfig
						ServiceName: "default/kuard/https",
					},
					ConnectTimeout: 250 * time.Millisecond,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
				},
			},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tr.OnAdd(tc.oldObj)
			tr.OnUpdate(tc.oldObj, tc.newObj)
			got := contents(&tr.ClusterCache)
			sort.Stable(clusterByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
		})
	}
}

func TestTranslatorRemoveService(t *testing.T) {
	tests := map[string]struct {
		setup func(*Translator)
		svc   *v1.Service
		want  []proto.Message
	}{
		"remove existing": {
			setup: func(tr *Translator) {
				tr.OnAdd(service("default", "simple", v1.ServicePort{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}))
			},
			svc: service("default", "simple", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []proto.Message{},
		},
		"remove named": {
			setup: func(tr *Translator) {
				tr.OnAdd(service("default", "simple", v1.ServicePort{
					Name:       "kevin",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}))
			},
			svc: service("default", "simple", v1.ServicePort{
				Name:       "kevin",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []proto.Message{},
		},
		"remove different": {
			setup: func(tr *Translator) {
				tr.OnAdd(service("default", "simple", v1.ServicePort{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}))
			},
			svc: service("default", "different", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []proto.Message{
				cluster("default/simple/80", "default/simple"),
			},
		},
		"remove non existant": {
			setup: func(*Translator) {},
			svc: service("default", "simple", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(6502),
			}),
			want: []proto.Message{},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Translator{
				FieldLogger: log,
			}
			tc.setup(tr)
			tr.OnDelete(tc.svc)
			got := contents(&tr.ClusterCache)
			sort.Stable(clusterByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
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

func cluster(name, servicename string) *v2.Cluster {
	return &v2.Cluster{
		Name: name,
		Type: v2.Cluster_EDS,
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig:   apiconfigsource("contour"),
			ServiceName: servicename,
		},
		ConnectTimeout: 250 * time.Millisecond,
		LbPolicy:       v2.Cluster_ROUND_ROBIN,
	}
}
