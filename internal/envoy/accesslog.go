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
	accesslogv2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
)

// FileAccessLog returns a new file based access log filter.
func FileAccessLog(path string) []*accesslog.AccessLog {
	return []*accesslog.AccessLog{{
		Name: util.FileAccessLog,
		ConfigType: &accesslog.AccessLog_TypedConfig{
			TypedConfig: any(&accesslogv2.FileAccessLog{
				Path: path,
				// TODO(dfc) FileAccessLog_Format elided.
			}),
		},
	}}
}
