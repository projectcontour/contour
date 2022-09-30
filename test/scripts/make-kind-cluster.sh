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

readonly MULTINODE_CLUSTER=${MULTINODE_CLUSTER:-"false"}
readonly IPV6_CLUSTER=${IPV6_CLUSTER:-"false"}
readonly NODEIMAGE=${NODEIMAGE:-"docker.io/kindest/node:v1.25.2@sha256:9be91e9e9cdf116809841fc77ebdb8845443c4c72fe5218f3ae9eb57fdb4bace"}
readonly CLUSTERNAME=${CLUSTERNAME:-contour-e2e}
readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}/../.." && pwd)

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::create() {
    local config_file="${REPO}/test/scripts/kind-expose-port.yaml"
    if [[ "${MULTINODE_CLUSTER}" == "true" ]]; then
        config_file="${REPO}/test/scripts/kind-multinode.yaml"
    fi
    if [[ "${IPV6_CLUSTER}" == "true" ]]; then
        config_file="${REPO}/test/scripts/kind-ipv6.yaml"
    fi
    ${KIND} create cluster \
        --name "${CLUSTERNAME}" \
        --image "${NODEIMAGE}" \
        --wait "${WAITTIME}" \
        --config "${config_file}"
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

# Push test images into the cluster. Do this up-front
# so that the first test does not incur the cost of 
# pulling them. Helps avoid flakes.
images=$(grep "Image = " $(find "$REPO/test/e2e" -name "fixtures.go") | awk '{print $3}' | tr -d '"')
for image in ${images}; do
  docker pull "${image}"
  kind::cluster::load "${image}"
done

# Install metallb.
${KUBECTL} apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.5/config/manifests/metallb-native.yaml
${KUBECTL} wait --timeout="${WAITTIME}" -n metallb-system deployment/controller --for=condition=Available

# Apply config with addresses based on docker network IPAM
if [[ "${IPV6_CLUSTER}" == "true" ]]; then
    subnet=$(docker network inspect kind | jq -r '.[].IPAM.Config[].Subnet | select(contains(":"))')
    # Assume default kind network subnet prefix of 64, and choose addresses in that range.
    address_first_blocks=$(echo ${subnet} | awk -F: '{printf "%s:%s:%s:%s",$1,$2,$3,$4}')
    address_range="${address_first_blocks}:ffff:ffff:ffff::-${address_first_blocks}:ffff:ffff:ffff:003f"
else
    subnet=$(docker network inspect kind | jq -r '.[].IPAM.Config[].Subnet | select(contains(":") | not)')
    # Assume default kind network subnet prefix of 16, and choose addresses in that range.
    address_first_octets=$(echo ${subnet} | awk -F. '{printf "%s.%s",$1,$2}')
    address_range="${address_first_octets}.255.200-${address_first_octets}.255.250"
fi

${KUBECTL} apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  namespace: metallb-system
  name: pool
spec:
  addresses:
  - ${address_range}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: pool-advertisement
  namespace: metallb-system
spec:
  ipAddressPools:
  - pool
EOF


# Install cert-manager.
${KUBECTL} apply -f https://github.com/jetstack/cert-manager/releases/download/v1.5.1/cert-manager.yaml
${KUBECTL} wait --timeout="${WAITTIME}" -n cert-manager -l app=cert-manager deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n cert-manager -l app=webhook deployments --for=condition=Available

# Install Gateway API CRDs and webhook.
${KUBECTL} apply -f "${REPO}/examples/gateway/00-crds.yaml"
${KUBECTL} apply -f "${REPO}/examples/gateway/00-namespace.yaml"
${KUBECTL} apply -f "${REPO}/examples/gateway/01-admission_webhook.yaml"
${KUBECTL} apply -f "${REPO}/examples/gateway/02-certificate_config.yaml"
${KUBECTL} wait --timeout="${WAITTIME}" -n gateway-system deployment/gateway-api-admission-server --for=condition=Available

# Install Contour CRDs.
${KUBECTL} apply -f "${REPO}/examples/contour/01-crds.yaml"
