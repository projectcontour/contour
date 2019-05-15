// Copyright © 2019 Heptio
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
	"testing"

	accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoy_accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/google/go-cmp/cmp"
)

func TestFileAccessLog(t *testing.T) {
	tests := map[string]struct {
		path string
		want []*envoy_accesslog.AccessLog
	}{
		"stdout": {
			path: "/dev/stdout",
			want: []*envoy_accesslog.AccessLog{{
				Name: util.FileAccessLog,
				ConfigType: &envoy_accesslog.AccessLog_TypedConfig{
					TypedConfig: any(&accesslog_v2.FileAccessLog{
						Path: "/dev/stdout",
					}),
				},
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FileAccessLog(tc.path)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
