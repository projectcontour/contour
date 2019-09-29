#!/bin/bash -e
#
# Copyright © 2019 VMware
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

VERSION=$(go list -m all | grep k8s.io/code-generator | rev | cut -d"-" -f1 | cut -d" " -f1 | rev)
TMP_DIR=$(mktemp -d)
git clone https://github.com/kubernetes/code-generator.git ${TMP_DIR}
(cd ${TMP_DIR} && git reset --hard ${VERSION})

${TMP_DIR}/generate-groups.sh \
  all \
  github.com/projectcontour/contour/apis/generated \
  github.com/projectcontour/contour/apis \
  "contour:v1beta1 projectcontour:v1" \
  --output-base . \
  --go-header-file hack/boilerplate.go.tmpl \
  $@

# Copy the generated.deepcopy to the api packages
rm -rf apis/generated
cp -r github.com/projectcontour/contour/apis/generated apis/
mv github.com/projectcontour/contour/apis/contour/v1beta1/zz_generated.deepcopy.go apis/contour/v1beta1
mv github.com/projectcontour/contour/apis/projectcontour/v1/zz_generated.deepcopy.go apis/projectcontour/v1
rm -rf github.com
