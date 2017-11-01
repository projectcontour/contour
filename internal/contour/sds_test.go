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
	"testing"

	"github.com/heptio/contour/internal/envoy"
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEndpointsToSDSHosts(t *testing.T) {
	tests := []struct {
		name string
		e    *v1.Endpoints
		port int
		want []*envoy.SDSHost
	}{{
		name: "simple",
		e: &v1.Endpoints{
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
		port: 8080,
		want: []*envoy.SDSHost{{
			IPAddress: "192.168.183.24",
			Port:      8080,
		}},
	}, {
		name: "multiple addresses",
		e: &v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin-org",
				Namespace: "default",
			},
			Subsets: []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: "23.23.247.89",
				}, {
					IP: "50.17.192.147",
				}, {
					IP: "50.17.206.192",
				}, {
					IP: "50.19.99.160",
				}},
				Ports: []v1.EndpointPort{{
					Port: 80,
				}},
			}},
		},
		port: 80,
		want: []*envoy.SDSHost{{
			IPAddress: "23.23.247.89",
			Port:      80,
		}, {
			IPAddress: "50.17.192.147",
			Port:      80,
		}, {
			IPAddress: "50.17.206.192",
			Port:      80,
		}, {
			IPAddress: "50.19.99.160",
			Port:      80,
		}},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EndpointsToSDSHosts(tc.e, tc.port)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got: %#v, want: %#v", got, tc.want)
			}
		})
	}
}

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name string
		e    *v1.Endpoints
		want error
	}{{
		name: "missing Endpoints.Meta.Name",
		e: &v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name: "",
			},
		},
		want: errors.New("Endpoints.Meta.Name is blank"),
	}, {
		name: "missing Endpoints.Meta.Namespace",
		e: &v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name: "simple",
			},
		},
		want: errors.New("Endpoints.Meta.Namespace is blank"),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateEndpoints(tc.e)
			if tc.want != nil && got == nil || got.Error() != tc.want.Error() {
				t.Errorf("got: %v, expected: %v", tc.want, got)
			}
		})
	}
}
