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

package json

import (
	"reflect"
	"testing"

	"github.com/heptio/contour/internal/envoy"
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceToClusters(t *testing.T) {
	tests := []struct {
		name string
		s    *v1.Service
		want []envoy.Cluster
		err  error
	}{{
		name: "single service port",
		s: &v1.Service{
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
		want: []envoy.Cluster{{
			Name:             "default/simple/80",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/6502",
		}},
	}, {
		name: "long namespace and service name",
		s: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tiny-cog-department-test-instance",
				Namespace: "beurocratic-company-test-domain-1",
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
		want: []envoy.Cluster{{
			Name:             "beurocratic-company-test-domain-1/tiny-cog-depa-52e801/80",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "beurocratic-company-test-domain-1/tiny-cog-department-test-instance/6502", // ServiceName is not subject to the 60 char limit
		}},
	}, {
		name: "single named service port",
		s: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}},
			},
		},
		want: []envoy.Cluster{{
			Name:             "default/simple/http",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/6502",
		}, {
			Name:             "default/simple/80",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/6502",
		}},
	}, {
		name: "two service ports",
		s: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}, {
					Name:       "alt",
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: intstr.FromString("9001"),
				}},
			},
		},
		want: []envoy.Cluster{{
			Name:             "default/simple/http",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/6502",
		}, {
			Name:             "default/simple/80",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/6502",
		}, {
			Name:             "default/simple/alt",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/9001",
		}, {
			Name:             "default/simple/8080",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/9001",
		}},
	}, {
		name: "one tcp service, one udp service",
		s: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Protocol:   "UDP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}, {
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: intstr.FromString("9001"),
				}},
			},
		},
		want: []envoy.Cluster{{
			Name:             "default/simple/8080",
			Type:             "sds",
			ConnectTimeoutMs: 250,
			LBType:           "round_robin",
			ServiceName:      "default/simple/9001",
		}},
	}, {
		name: "one udp service",
		s: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": "simple",
				},
				Ports: []v1.ServicePort{{
					Protocol:   "UDP",
					Port:       80,
					TargetPort: intstr.FromInt(6502),
				}},
			},
		},
		err: errors.New("service default/simple: no usable ServicePorts"),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ServiceToClusters(tc.s)
			if err != nil && tc.err == nil {
				t.Fatal(err)
			}
			switch {
			case err != nil:
				if err.Error() != tc.err.Error() {
					t.Fatalf("got error: %v, want: %v", err, tc.err)
				}
			default:
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("got: %#v, want: %#v", got, tc.want)
				}
			}
		})
	}
}

func TestValidateService(t *testing.T) {
	tests := []struct {
		name string
		s    *v1.Service
		want error
	}{{
		name: "missing Service.Meta.Name",
		s: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "",
			},
		},
		want: errors.New("Service.Meta.Name is blank"),
	}, {
		name: "missing Service.Meta.Namespace",
		s: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "simple",
			},
		},
		want: errors.New("Service.Meta.Namespace is blank"),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateService(tc.s)
			if tc.want != nil && got == nil || got.Error() != tc.want.Error() {
				t.Errorf("got: %v, expected: %v", tc.want, got)
			}
		})
	}
}
