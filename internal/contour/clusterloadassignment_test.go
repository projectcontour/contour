// Copyright © 2018 Heptio
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

	"github.com/gogo/protobuf/proto"
	"k8s.io/api/core/v1"
)

func TestClusterLoadAssignmentCacheRecomputeClusterLoadAssignment(t *testing.T) {
	tests := map[string]struct {
		oldep, newep *v1.Endpoints
		want         []proto.Message
	}{
		"simple": {
			newep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080)),
			},
		},
		"multiple addresses": {
			newep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"23.23.247.89",
					"50.17.192.147",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(80),
			}),
			want: []proto.Message{
				clusterloadassignment("default/httpbin-org",
					lbendpoint("23.23.247.89", 80),
					lbendpoint("50.17.192.147", 80),
					lbendpoint("50.17.206.192", 80),
					lbendpoint("50.19.99.160", 80),
				),
			},
		},
		"named container port": {
			newep: endpoints("default", "secure", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: []v1.EndpointPort{{
					Name: "https",
					Port: 8443,
				}},
			}),
			want: []proto.Message{
				clusterloadassignment("default/secure/https", lbendpoint("192.168.183.24", 8443)),
			},
		},
		"remove existing": {
			oldep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var cc ClusterLoadAssignmentCache
			cc.recomputeClusterLoadAssignment(tc.oldep, tc.newep)
			got := cc.Values()
			sort.Stable(clusterLoadAssignmentsByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%v\ngot:\n%v", tc.want, got)
			}
		})
	}
}
