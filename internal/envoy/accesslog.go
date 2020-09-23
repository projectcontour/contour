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

package envoy

//JSONFields is the canonical translation table for JSON fields to Envoy log template formats,
//used for specifying fields for Envoy to log when JSON logging is enabled.
//Only fields specified in this map may be used for JSON logging.
var JSONFields = map[string]string{
	"@timestamp":                "%START_TIME%",
	"ts":                        "%START_TIME%",
	"authority":                 "%REQ(:AUTHORITY)%",
	"bytes_received":            "%BYTES_RECEIVED%",
	"bytes_sent":                "%BYTES_SENT%",
	"downstream_local_address":  "%DOWNSTREAM_LOCAL_ADDRESS%",
	"downstream_remote_address": "%DOWNSTREAM_REMOTE_ADDRESS%",
	"duration":                  "%DURATION%",
	"method":                    "%REQ(:METHOD)%",
	"path":                      "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%",
	"protocol":                  "%PROTOCOL%",
	"request_id":                "%REQ(X-REQUEST-ID)%",
	"requested_server_name":     "%REQUESTED_SERVER_NAME%",
	"response_code":             "%RESPONSE_CODE%",
	"response_flags":            "%RESPONSE_FLAGS%",
	"uber_trace_id":             "%REQ(UBER-TRACE-ID)%",
	"upstream_cluster":          "%UPSTREAM_CLUSTER%",
	"upstream_host":             "%UPSTREAM_HOST%",
	"upstream_local_address":    "%UPSTREAM_LOCAL_ADDRESS%",
	"upstream_service_time":     "%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%",
	"user_agent":                "%REQ(USER-AGENT)%",
	"x_forwarded_for":           "%REQ(X-FORWARDED-FOR)%",
	"x_trace_id":                "%REQ(X-TRACE-ID)%",
}

// DefaultFields are fields that will be included by default when JSON logging is enabled.
var DefaultFields = []string{
	"@timestamp",
	"authority",
	"bytes_received",
	"bytes_sent",
	"downstream_local_address",
	"downstream_remote_address",
	"duration",
	"method",
	"path",
	"protocol",
	"request_id",
	"requested_server_name",
	"response_code",
	"response_flags",
	"uber_trace_id",
	"upstream_cluster",
	"upstream_host",
	"upstream_local_address",
	"upstream_service_time",
	"user_agent",
	"x_forwarded_for",
}
