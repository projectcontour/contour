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

package contour.resources

# The `supported' rule lists the resources that are supported by the
# API server.
supported[r]  = true {
    data.resources[r][".versions"]
}

# is_supported returns whether the names resource is supported. This is
# a functional variable of the `supported` rule.
is_supported(resource) = true {
  supported[resource]
} else = false {
  true
}

# The `versions` rule returns an array of GroupVersionKind structures
# for the resource `r`.
versions[r] = v {
    v := data.resources[r][".versions"]
}

# is_present is true if the named resource exists in the data document.
#
# Examples:
#   resources.is_present("httproxies", "proxy-name")
#   resources.is_present("httproxies", "namespace/proxy-name")
is_present(resource, name) = true {
  parts = split(name, "/")
  count(parts) == 2
  # Look up data.resources.$NS.$RESOURCE.$NAME.
  data.resources[parts[0]][resource][parts[1]]
} else = true {
  parts = split(name, "/")
  count(parts) == 1
  data.resources[resource][name]
} else = false {
  true
}

# get returns the named resource from the data document. If the resource
# is not present, an empty object is returned.
#
# Examples:
#   resources.get("secrets", "foo")
#   resources.get("pods", "namespace-name/foo")
get(resource, name) = obj {
  parts = split(name, "/")

  # This is namespace/name syntax.
  count(parts) == 2

  # Look up data.resources.$NS.$RESOURCE.$NAME.
  ns := object.get(data.resources, parts[0], {})
  res := object.get(ns, resource, {})
  obj := object.get(res, parts[1], {})

} else = obj {
  parts = split(name, "/")

  # This is a name in the default namespace.
  count(parts) == 1

  # Look up data.resources.$RESOURCE.$NAME.
  res := object.get(data.resources, resource, {})
  obj := object.get(res, name, {})
} else = obj {
  obj := {}
}

# status returns the status field of the named resource. If the resource
# is not present, and empty object is returned. Implemented in terms of
# 'get', so namespace syntax works for the object name.
#
# Examples:
#   resources.status("httpproxies", "foo")
status(resource, name) = s {
  r := get(resource, name)
  s := object.get(r, "status", {})
}
