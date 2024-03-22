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

package config

import (
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// DefaultFields are fields that will be included by default when JSON logging is enabled.
var DefaultFields = AccessLogFields(contour_v1alpha1.DefaultAccessLogJSONFields)

// DEFAULT_ACCESS_LOG_TYPE is the default access log format.
// nolint:revive
const DEFAULT_ACCESS_LOG_TYPE AccessLogType = AccessLogType(contour_v1alpha1.DefaultAccessLogType)
