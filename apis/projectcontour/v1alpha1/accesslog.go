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

package v1alpha1

import (
	"fmt"
	"regexp"
	"strings"
)

// DefaultFields are fields that will be included by default when JSON logging is enabled.
var DefaultFields = AccessLogFields([]string{
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
})

// DEFAULT_ACCESS_LOG_TYPE is the default access log format.
// nolint:revive
const DEFAULT_ACCESS_LOG_TYPE = EnvoyAccessLog

// jsonFields is the canonical translation table for JSON fields to Envoy log template formats,
// used for specifying fields for Envoy to log when JSON logging is enabled.
var jsonFields = map[string]string{
	"@timestamp":            "%START_TIME%",
	"ts":                    "%START_TIME%",
	"authority":             "%REQ(:AUTHORITY)%",
	"method":                "%REQ(:METHOD)%",
	"path":                  "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%",
	"request_id":            "%REQ(X-REQUEST-ID)%",
	"uber_trace_id":         "%REQ(UBER-TRACE-ID)%",
	"upstream_service_time": "%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%",
	"user_agent":            "%REQ(USER-AGENT)%",
	"x_forwarded_for":       "%REQ(X-FORWARDED-FOR)%",
	"x_trace_id":            "%REQ(X-TRACE-ID)%",
}

// envoySimpleOperators is the list of known supported Envoy log template keywords that do not
// have arguments nor require canonical translations.
var envoySimpleOperators = map[string]struct{}{
	"BYTES_RECEIVED":                                {},
	"BYTES_SENT":                                    {},
	"CONNECTION_ID":                                 {},
	"CONNECTION_TERMINATION_DETAILS":                {},
	"DOWNSTREAM_DIRECT_REMOTE_ADDRESS":              {},
	"DOWNSTREAM_DIRECT_REMOTE_ADDRESS_WITHOUT_PORT": {},
	"DOWNSTREAM_LOCAL_ADDRESS":                      {},
	"DOWNSTREAM_LOCAL_ADDRESS_WITHOUT_PORT":         {},
	"DOWNSTREAM_LOCAL_PORT":                         {},
	"DOWNSTREAM_LOCAL_SUBJECT":                      {},
	"DOWNSTREAM_LOCAL_URI_SAN":                      {},
	"DOWNSTREAM_PEER_CERT":                          {},
	"DOWNSTREAM_PEER_CERT_V_END":                    {},
	"DOWNSTREAM_PEER_CERT_V_START":                  {},
	"DOWNSTREAM_PEER_FINGERPRINT_1":                 {},
	"DOWNSTREAM_PEER_FINGERPRINT_256":               {},
	"DOWNSTREAM_PEER_ISSUER":                        {},
	"DOWNSTREAM_PEER_SERIAL":                        {},
	"DOWNSTREAM_PEER_SUBJECT":                       {},
	"DOWNSTREAM_PEER_URI_SAN":                       {},
	"DOWNSTREAM_REMOTE_ADDRESS":                     {},
	"DOWNSTREAM_REMOTE_ADDRESS_WITHOUT_PORT":        {},
	"DOWNSTREAM_TLS_CIPHER":                         {},
	"DOWNSTREAM_TLS_SESSION_ID":                     {},
	"DOWNSTREAM_TLS_VERSION":                        {},
	"DURATION":                                      {},
	"GRPC_STATUS":                                   {},
	"HOSTNAME":                                      {},
	"LOCAL_REPLY_BODY":                              {},
	"PROTOCOL":                                      {},
	"REQUESTED_SERVER_NAME":                         {},
	"REQUEST_DURATION":                              {},
	"RESPONSE_CODE":                                 {},
	"RESPONSE_CODE_DETAILS":                         {},
	"RESPONSE_DURATION":                             {},
	"RESPONSE_FLAGS":                                {},
	"RESPONSE_TX_DURATION":                          {},
	"ROUTE_NAME":                                    {},
	"START_TIME":                                    {},
	"UPSTREAM_CLUSTER":                              {},
	"UPSTREAM_HOST":                                 {},
	"UPSTREAM_LOCAL_ADDRESS":                        {},
	"UPSTREAM_TRANSPORT_FAILURE_REASON":             {},
}

// envoyComplexOperators is the list of known Envoy log template keywords that require
// arguments.
var envoyComplexOperators = map[string]struct{}{
	"REQ":               {},
	"RESP":              {},
	"START_TIME":        {},
	"TRAILER":           {},
	"REQ_WITHOUT_QUERY": {},
}

// AccessLogType is the name of a supported access logging mechanism.
type AccessLogType string

func (a AccessLogType) Validate() error {
	switch a {
	case EnvoyAccessLog, JSONAccessLog:
		return nil
	default:
		return fmt.Errorf("invalid access log format %q", a)
	}
}

const EnvoyAccessLog AccessLogType = "envoy"
const JSONAccessLog AccessLogType = "json"

type AccessLogFields []string

func (a AccessLogFields) Validate() error {

	for key, val := range a.AsFieldMap() {
		if val == "" {
			return fmt.Errorf("invalid JSON log field name %s", key)
		}

		if jsonFields[key] == val {
			continue
		}

		err := parseAccessLogFormat(val)
		if err != nil {
			return fmt.Errorf("invalid JSON field: %s", err)
		}
	}

	return nil
}

func (a AccessLogFields) AsFieldMap() map[string]string {
	fieldMap := map[string]string{}

	for _, val := range a {
		parts := strings.SplitN(val, "=", 2)

		if len(parts) == 1 {
			operator, foundInFieldMapping := jsonFields[val]
			_, isSimpleOperator := envoySimpleOperators[strings.ToUpper(val)]

			switch {
			case isSimpleOperator && !foundInFieldMapping:
				// Operator name is known to be simple, upcase and wrap it in percents.
				fieldMap[val] = fmt.Sprintf("%%%s%%", strings.ToUpper(val))
			case foundInFieldMapping:
				// Operator name has a known mapping, store the result of the mapping.
				fieldMap[val] = operator
			default:
				// Operator name not found, save as emptystring and let validation catch it later.
				fieldMap[val] = ""
			}
		} else {
			// Value is a full key:value pair, store it as is.
			fieldMap[parts[0]] = parts[1]
		}
	}

	return fieldMap
}

// TODO: Commented code since it's not yet in use, but makes the linter unhappy.
// func validateAccessLogFormatString(format string) error {
//	// Empty format means use default format, defined by Envoy.
//	if format == "" {
//		return nil
//	}
//	err := parseAccessLogFormat(format)
//	if err != nil {
//		return fmt.Errorf("invalid access log format: %s", err)
//	}
//	if !strings.HasSuffix(format, "\n") {
//		return fmt.Errorf("invalid access log format: must end in newline")
//	}
//	return nil
//}

// commandOperatorRegexp parses the command operators used in Envoy access log config
//
// Capture Groups:
// Given string "the start time is %START_TIME(%s):3% wow!"
//
//   0. Whole match "%START_TIME(%s):3%"
//   1. Full operator: "START_TIME(%s):3%"
//   2. Operator Name: "START_TIME"
//   3. Arguments: "(%s)"
//   4. Truncation length: ":3"
var commandOperatorRegexp = regexp.MustCompile(`%(([A-Z_]+)(\([^)]+\)(:[0-9]+)?)?%)?`)

func parseAccessLogFormat(format string) error {

	// FindAllStringSubmatch will always return a slice with matches where every slice is a slice
	// of submatches with length of 5 (number of capture groups + 1).
	tokens := commandOperatorRegexp.FindAllStringSubmatch(format, -1)
	if len(tokens) == 0 {
		return nil
	}

	for _, f := range tokens {
		op := f[2]
		if op == "" {
			return fmt.Errorf("invalid Envoy format: %s", f)
		}

		_, okSimple := envoySimpleOperators[op]
		_, okComplex := envoyComplexOperators[op]
		if !okSimple && !okComplex {
			return fmt.Errorf("invalid Envoy format: %s, invalid Envoy operator: %s", f, op)
		}

		if (op == "REQ" || op == "RESP" || op == "TRAILER" || op == "REQ_WITHOUT_QUERY") && f[3] == "" {
			return fmt.Errorf("invalid Envoy format: %s, arguments required for operator: %s", f, op)
		}

		// START_TIME cannot not have truncation length.
		if op == "START_TIME" && f[4] != "" {
			return fmt.Errorf("invalid Envoy format: %s, operator %s cannot have truncation length", f, op)
		}
	}

	return nil
}
