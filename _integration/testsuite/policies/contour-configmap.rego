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

package contour.resources.configmap

import  data.contour.resources

# get_data returns the .data field of the named ConfigMap resource. If
# the resource is not present or does not have a .data field, and empty
# object is returned.
#
# Examples:
#   configmap.data("foo")
#   configmap.data("namespace-name/foo")
get_data(name) = obj {
  # Get the named configmap resource.
  config := resources.get("configmaps", name)

  # Get the .data field.
  obj := object.get(config, "data", {})
} else = obj {
  obj := {}
}
