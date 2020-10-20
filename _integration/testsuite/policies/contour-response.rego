# Copyright Project Contour Authors
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License.  You may obtain
# a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  See the
# License for the specific language governing permissions and limitations
# under the License.

package contour.http.response

# This package contains helper functions for processing the body of
# ingress-conformance-echo server response. The general form of the
# response body is a JSON payload similar to this:
#
# {
#  "path": "/f11887",
#  "host": "echo.projectcontour.io",
#  "method": "GET",
#  "proto": "HTTP/1.1",
#  "headers": {
#   "Accept": [
#    "*/*"
#   ],
#   "Content-Length": [
#    "0"
#   ],
#   "Target-Present": [
#    "true"
#   ],
#   "User-Agent": [
#    "curl/7.72.0"
#   ],
#   "X-Envoy-Expected-Rq-Timeout-Ms": [
#    "15000"
#   ],
#   "X-Envoy-Internal": [
#    "true"
#   ],
#   "X-Forwarded-For": [
#    "172.18.0.1"
#   ],
#   "X-Forwarded-Proto": [
#    "http"
#   ],
#   "X-Request-Id": [
#    "3f37abbf-35f5-423c-aafe-20b5bdaba157"
#   ],
#   "X-Request-Start": [
#    "t=1602828253.669"
#   ]
#  },
#  "namespace": "default",
#  "ingress": "ingress-conformance-echo",
#  "service": "ingress-conformance-echo",
#  "pod": "ingress-conformance-echo-57bcd796dd-qhbpw"
# }

# Return the HTTP response body, or an empty object if there is no body.
body(resp) = value {
  is_null(resp.body)
  value := {}
}

# Return the HTTP response body, or an empty object if there is no body.
body(resp) = value {
  not is_null(resp.body)
  value := resp.body
}

# Get the "service" element from a ingress-conformance-echo response
# body. Returns "" if there is no body or no "service" element in the
# response.
service(resp) = value {
  b := body(resp)
  value := object.get(b, "service", "")
} else = "" {
  true
}

# Get the "pod" element from a ingress-conformance-echo response
# body. Returns "" if there is no body or no "pod" element in the
# response.
pod(resp) = value {
  b := body(resp)
  value := object.get(b, "pod", "")
} else = "" {
  true
}

# Return true if the response status matches.
status_is(resp, expected_code) = true {
  status_code := object.get(resp, "status_code", 0)
  status_code == expected_code
} else = false {
  true
}

# Return true if the response status matches.
status_code(resp) = code {
  code := object.get(resp, "status_code", 0)
} else = 0 {
  true
}

# Return true if the response status is in the 4xx range.
is_4xx(resp) = true {
  status_code := object.get(resp, "status_code", 0)
  status_code >= 400
  status_code < 500
} else = false {
  true
}

# Return true if the response body reports a header with the given value.
body_has_header_value(resp, name, value) = true {
  response_body := body(resp)
  response_headers := object.get(response_body, "headers", {})
  header_values := object.get(response_headers, name, set())

  some v
  header_values[v] == value
} else = false {
  true
}
