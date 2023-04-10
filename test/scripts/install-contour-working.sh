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

# install-contour-working.sh: Install Contour from the working repo.

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly CLUSTERNAME=${CLUSTERNAME:-contour-e2e}
readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::load::docker() {
    ${KIND} load docker-image \
        --name "${CLUSTERNAME}" \
        "$@"
}

if ! kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME does not exist"
    exit 2
fi

# Set the image tag to match the current git hash + dirty flag.
VERSION=$(git describe --exclude="*" --always --dirty)

# Build the image.
make -C ${REPO} container IMAGE=ghcr.io/projectcontour/contour VERSION=${VERSION}

# Push the Contour build image into the cluster.
kind::cluster::load::docker ghcr.io/projectcontour/contour:${VERSION}

# Install Contour
${KUBECTL} apply -f ${REPO}/examples/contour/00-common.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/01-contour-config.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/01-crds.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-rbac.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-role-contour.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-service-contour.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-service-envoy.yaml

for file in ${REPO}/examples/contour/02-job-certgen.yaml ${REPO}/examples/contour/03-contour.yaml ${REPO}/examples/contour/03-envoy.yaml ; do
  # Set image pull policy to IfNotPresent so kubelet will use the
  # images that we loaded onto the node, rather than trying to pull
  # them from the registry.
  # Set the image tag to $VERSION to unambiguously use the image
  # we built above.
  sed \
    "-es|imagePullPolicy: Always|imagePullPolicy: IfNotPresent|" \
    "-es|image: ghcr.io/projectcontour/contour:.*$|image: ghcr.io/projectcontour/contour:${VERSION}|" \
    "$file" | \
  ${KUBECTL} apply -f -
done

# Wait for Contour and Envoy to report "Ready" status.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=envoy pods --for=condition=Ready
