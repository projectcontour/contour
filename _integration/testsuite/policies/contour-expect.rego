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

package contour.http.expect

import data.contour.http.response
import data.builtin.result

# response_status_is(response_object, status_code)
#
# Checks whether the response has the wanted HTTP status.
response_status_is(response_object, wanted) = r {
  response.status_code(response_object) == wanted
  r := result.Passf("got expected status %d", [wanted])
} else = r {
  r := result.Errorf("got status %d, wanted %d", [response.status_code(response_object), wanted])
}

# response_status_not(response_object, status_code)
#
# Checks whether the response has the unwanted HTTP status.
response_status_not(response_object, unwanted) = r {
  response.status_code(response_object) == unwanted
  r := result.Errorf("got status %d, wanted anything but %d",
          [response.status_code(response_object), unwanted])
} else = r {
  r := result.Passf("got unwanted status %d", [unwanted])
}

# response_service_is(response_object, status_code)
#
# Checks whether the response has the wanted service name field.
response_service_is(response_object, wanted) = r {
  response.service(response_object) == wanted
  r := result.Passf("got expected service name %q", [wanted])
} else = r {
  r := result.Errorf("got service name %q, wanted %q", [response.service(response_object), wanted])
}

# response_header_is(response_object, header_name, header_value)
#
# Checks whether the response body contains the matching header value.
response_header_is(response_object, header_name, header_value) = r {
  # Pass if the header is there and contains the wanted value.
  response_body := response.body(response_object)
  response_headers := object.get(response_body, "headers", {})
  values := response_headers[header_name]

  # Assert that there is some key v, which yields the value we want.
  some v
  values[v] == header_value

  r := result.Passf("value of response header %q matches %q", [header_name, header_value])
} else  = r {
  # Error on any other values.
  response_body := response.body(response_object)
  response_headers := object.get(response_body, "headers", {})
  values := response_headers[header_name]
  r := result.Errorf("value of response header %q is %q, wanted %q", [header_name, values, header_value])
} else  = r {
  # Error if the header isn't present.
  response_body := response.body(response_object)
  response_headers := response_body["headers"]
  r := result.Errorf("header %q is not present in the response body", [header_name])
} else = r {
  # Failsafe error.
  r := result.Errorf("response header %q unmatched:\n%s", [
    header_name, yaml.marshal(object.get(response.body(response_object), "Headers", {}))
  ])
}
#
# response_header_has_prefix(response_object, header_name, header_prefix)
#
# Checks whether the response body contains the matching header value.
response_header_has_prefix(response_object, header_name, header_prefix) = r {
  # Pass if the header is there and contains the wanted value.
  response_body := response.body(response_object)
  response_headers := object.get(response_body, "headers", {})
  values := response_headers[header_name]

  # Assert that there is some key v, which yields the value we want.
  some v
  startswith(values[v], header_prefix)

  r := result.Passf("value of response header %q has prefix %q", [header_name, header_prefix])
} else  = r {
  # Error on any other values.
  response_body := response.body(response_object)
  response_headers := object.get(response_body, "headers", {})
  values := response_headers[header_name]
  r := result.Errorf("value of response header %q is %q, wanted prefix %q", [header_name, values, header_prefix])
} else  = r {
  # Error if the header isn't present.
  response_body := response.body(response_object)
  response_headers := response_body["headers"]
  r := result.Errorf("header %q is not present in the response body", [header_name])
} else = r {
  # Failsafe error.
  r := result.Errorf("response header %q unmatched:\n%s", [
    header_name, yaml.marshal(object.get(response.body(response_object), "Headers", {}))
  ])
}

# response_header_does_not_have(response_object, header_name)
#
# Checks whether the response body does not contain the matching header.
response_header_does_not_have(response_object, header_name) = r {
  # Pass if the header is there and contains the wanted value.
  response_body := response.body(response_object)
  response_headers := object.get(response_body, "headers", {})
  values := response_headers[header_name]
  r := result.Passf("value of response header %q was not removed", [header_name])
} else  = r {
  # Error if the header is present.
  response_body := response.body(response_object)
  response_headers := object.get(response_body, "headers", {})
  values := response_headers[header_name]
  r := result.Errorf("value of response header %q is %q, wanted removed", [header_name, values])
}
