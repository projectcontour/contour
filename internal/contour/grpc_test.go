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
	"context"
	"reflect"
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/heptio/contour/internal/log/stdlog"
)

func TestGRPCAPIFetchClusters(t *testing.T) {
	tests := []struct {
		name      string
		services  []*v1.Service
		endpoints []*v1.Endpoints
		ingresses []*v1beta1.Ingress
		req       v2.DiscoveryRequest
		want      v2.DiscoveryResponse
	}{}

	var w discardWriter
	l := stdlog.New(w, w, 0)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ds DataSource
			for _, s := range tc.services {
				ds.AddService(s)
			}
			for _, e := range tc.endpoints {
				ds.AddEndpoints(e)
			}
			for _, i := range tc.ingresses {
				ds.AddIngress(i)
			}
			api := &grpcAPI{
				Logger:     l,
				DataSource: &ds,
			}
			got, err := api.FetchClusters(context.TODO(), &tc.req)
			checkErr(t, err)
			if !reflect.DeepEqual(&tc.want, got) {
				t.Fatalf("expected: %q, got %q", tc.want, got)
			}
		})
	}
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
