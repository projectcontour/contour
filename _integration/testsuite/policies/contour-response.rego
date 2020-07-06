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

has_testid(resp) = true {
  resp.body.TestId
}

has_testid(resp) = false {
  not resp.body
}

has_testid(resp) = false {
  not resp.body.TestId
}

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

# Get the TestId element from a ingress-conformance-echo response
# body. Returns "" if there is no body or no TestId response.
testid(resp) = value {
  b := body(resp)
  value := object.get(b, "TestId", "")
}

# Return true if the response status matches.
status_is(resp, expected_code) = true {
  status_code := object.get(resp, "status_code", 0)
  status_code == expected_code
} else = false {
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
