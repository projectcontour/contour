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

# make-kind-cluster.sh: build a kind cluster and install a working copy
# of Contour into it.

set -o pipefail
set -o errexit
set -o nounset

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly NODEIMAGE=${NODEIMAGE:-"docker.io/kindest/node:v1.20.2"}
readonly CLUSTERNAME=${CLUSTERNAME:-contour-integration}
readonly WAITTIME=${WAITTIME:-5m}
readonly GATEWAY_API_VERSION=v0.2.0

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}/../.." && pwd)

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::create() {
    ${KIND} create cluster \
        --name "${CLUSTERNAME}" \
        --image "${NODEIMAGE}" \
        --wait "${WAITTIME}" \
        --config "${REPO}/_integration/testsuite/kind-expose-port.yaml"
}

kind::cluster::load() {
    ${KIND} load docker-image \
        --name "${CLUSTERNAME}" \
        "$@"
}

if kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME already exists"
    echo exit 2
fi

# Create a fresh kind cluster.
if ! kind::cluster::exists "$CLUSTERNAME" ; then
  kind::cluster::create

  # Print the k8s version for verification
  ${KUBECTL} version
fi

# Push test images into the cluster.
for i in $(find "$HERE" -name "*.yaml" -print0 | xargs -0 awk '$1=="image:"{print $2}')
do
    docker pull "$i"
    kind::cluster::load "$i"
done

# Install cert-manager.
${KUBECTL} apply -f https://github.com/jetstack/cert-manager/releases/download/v1.1.0/cert-manager.yaml
${KUBECTL} wait --timeout="${WAITTIME}" -n cert-manager -l app=cert-manager deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n cert-manager -l app=webhook deployments --for=condition=Available

# Install Gateway API CRDs.
${KUBECTL} kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=${GATEWAY_API_VERSION}" | ${KUBECTL} apply -f -
