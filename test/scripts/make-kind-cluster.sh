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
readonly SKIP_GATEWAY_API_INSTALL=${SKIP_GATEWAY_API_INSTALL:-"false"}
readonly NODEIMAGE=${NODEIMAGE:-"docker.io/kindest/node:v1.32.0@sha256:c48c62eac5da28cdadcf560d1d8616cfa6783b58f0d94cf63ad1bf49600cb027"}
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

# Install metallb.
${KUBECTL} apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.9/config/manifests/metallb-native.yaml
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

# Wrap application of metallb config in retry loop to minimize
# flakes due to webhook not being ready.
success=false
for n in {1..60}; do
if ${KUBECTL} apply -f - <<EOF
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
then
  success=true
  break
fi
echo "Applying metallb configuration failed, retrying (${n} of 60)"
sleep 1
done

if [ $success != "true" ]; then
  echo "Applying metallb configuration failed"
  exit 1
fi

# Install cert-manager.
CERT_MANAGER_VERSION=$(go list -m all | grep github.com/cert-manager/cert-manager | awk '{print $2}')

${KUBECTL} apply -f https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
${KUBECTL} wait --timeout="${WAITTIME}" -n cert-manager -l app=cert-manager deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n cert-manager -l app=webhook deployments --for=condition=Available

if [[ "${SKIP_GATEWAY_API_INSTALL}" != "true" ]]; then
  # Install Gateway API CRDs.
  ${KUBECTL} apply -f "${REPO}/examples/gateway/00-crds.yaml"
fi

# Install Contour CRDs.
${KUBECTL} apply -f "${REPO}/examples/contour/01-crds.yaml"
