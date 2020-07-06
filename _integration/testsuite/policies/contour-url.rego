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

package contour.http.client.url

import data.contour.http.client

# Generate a HTTP URL from the `target_addr` and `target_http_port`
# parameters.
http(path) = url {
    startswith(path, "/")
    url := sprintf("http://%s:%d%s", [
             client.target_addr, client.target_http_port, path
    ])
} else = url {
    url := sprintf("http://%s:%d/%s", [
             client.target_addr, client.target_http_port, path
    ])
}

# Generate a HTTPS URL from the `target_addr` and `target_https_port`
# parameters.
https(path) = url {
    startswith(path, "/")
    url := sprintf("https://%s:%d%s", [
             client.target_addr, client.target_https_port, path
    ])
} else = url {
    url := sprintf("https://%s:%d/%s", [
             client.target_addr, client.target_https_port, path
    ])
}
