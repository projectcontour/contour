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
	"testing"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v2 "github.com/envoyproxy/go-control-plane/api"
)

func TestClusterCacheRecomputeService(t *testing.T) {
	tests := map[string]struct {
		oldObj *v1.Service
		newObj *v1.Service
		want   []*v2.Cluster
	}{
		"add unnamed service": {
			oldObj: nil,
			newObj: service("default", "kuard",
				v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			),
			want: []*v2.Cluster{{
				Name: "default/kuard/443",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}},
		},
		"name previously unnamed port": {
			oldObj: service("default", "kuard",
				v1.ServicePort{
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
			want: []*v2.Cluster{{
				Name: "default/kuard/443",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}, {
				Name: "default/kuard/https",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}},
		},
		"remove name from port": {
			oldObj: service("default", "kuard",
				v1.ServicePort{
					Name:       "https",
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			),
			newObj: service("default", "kuard",
				v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			),
			want: []*v2.Cluster{{
				Name: "default/kuard/443",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}},
		},
		"update service port": {
			oldObj: service("default", "kuard",
				v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			),
			newObj: service("default", "kuard",
				v1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
			),
			want: []*v2.Cluster{{
				Name: "default/kuard/8080",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}, {
				Name: "default/kuard/http",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}},
		},
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
			want: []*v2.Cluster{{
				Name: "default/kuard/443",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}, {
				Name: "default/kuard/https",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8443",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}},
		},
		"update, remove, and rename a named port": {
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
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8000),
				},
			),
			want: []*v2.Cluster{{
				Name: "default/kuard/443",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "default/kuard/8000",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var cc ClusterCache
			cc.recomputeService(tc.oldObj, tc.newObj)
			got := cc.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%v\ngot:\n%v\n", tc.want, got)
			}
		})
	}

}
