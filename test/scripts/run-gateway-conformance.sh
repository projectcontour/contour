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
export CONTOUR_IMG=${CONTOUR_E2E_IMAGE:-ghcr.io/projectcontour/contour:main}

echo "Using Contour image: ${CONTOUR_IMG}"
echo "Using Gateway API version: ${GATEWAY_API_VERSION}"

${KUBECTL} apply -f examples/gateway-provisioner/00-common.yaml
${KUBECTL} apply -f examples/gateway-provisioner/01-roles.yaml
${KUBECTL} apply -f examples/gateway-provisioner/02-rolebindings.yaml
${KUBECTL} apply -f <(cat examples/gateway-provisioner/03-gateway-provisioner.yaml | \
    yq eval '.spec.template.spec.containers[0].image = env(CONTOUR_IMG)' - | \
    yq eval '.spec.template.spec.containers[0].imagePullPolicy = "IfNotPresent"' - | \
    yq eval '.spec.template.spec.containers[0].args += "--contour-image="+env(CONTOUR_IMG)' -)

${KUBECTL} apply -f - <<EOF
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-controller
EOF


# If we're running conformance tests for the same Gateway API version
# that we're using via go.mod, use our own test driver (via `go test`)
# so we can opt into additional supported features that we know we support.
# Otherwise, we're likely running the `main` conformance tests for a nightly
# build, where we have to clone the upstream repo to be able to run that
# version of the tests, but lose the ability to opt into tests for additional
# supported features since that's not exposed via flag.
GO_MOD_GATEWAY_API_VERSION=$(grep "sigs.k8s.io/gateway-api" go.mod | awk '{print $2}')

if [ "$GATEWAY_API_VERSION" = "$GO_MOD_GATEWAY_API_VERSION" ]; then
  go test -timeout=20m -tags conformance ./test/conformance/gatewayapi --gateway-class=contour
else 
  cd $(mktemp -d)
  git clone https://github.com/kubernetes-sigs/gateway-api
  cd gateway-api
  git checkout "${GATEWAY_API_VERSION}"
  # TODO: Keep the list of skipped features in sync with
  # test/conformance/gatewayapi/gateway_conformance_test.go.
  # Can implement with the -skip flag available with go 1.20
  # or if Gateway API supports skipping tests via custom flag.
  go test -timeout=20m ./conformance -gateway-class=contour -all-features
fi
