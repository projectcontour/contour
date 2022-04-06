#! /usr/bin/env bash

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

set -o pipefail
set -o errexit
set -o nounset

readonly KUBECTL=${KUBECTL:-kubectl}
readonly IMG=${CONTOUR_E2E_IMAGE:-ghcr.io/projectcontour/contour:main}

echo "Using Contour image: ${IMG}"

${KUBECTL} apply -f examples/gateway-provisioner/00-common.yaml
${KUBECTL} apply -f examples/gateway-provisioner/01-roles.yaml
${KUBECTL} apply -f examples/gateway-provisioner/02-rolebindings.yaml
${KUBECTL} apply -f <(sed "s|ghcr.io/projectcontour/contour:main|$IMG|g; s|imagePullPolicy: Always|imagePullPolicy: IfNotPresent|g" examples/gateway-provisioner/03-gateway-provisioner.yaml)

${KUBECTL} apply -f - <<EOF
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1alpha2
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-provisioner
EOF

cd $(mktemp -d)
git clone https://github.com/kubernetes-sigs/gateway-api
cd gateway-api
go test ./conformance -gateway-class=contour
