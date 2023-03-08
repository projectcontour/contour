#!/usr/bin/env bash

# set -o pipefail
set -o errexit
set -o nounset

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

# Set the image tag to match the current Contour version.
VERSION="v$$"

# Build the Contour Provisioner image.
make -C ${REPO} container IMAGE=ghcr.io/projectcontour/contour VERSION=${VERSION}

# Push the Contour Provisioner image into the cluster.
kind::cluster::load::docker ghcr.io/projectcontour/contour:${VERSION}

for file in ${REPO}/examples/gateway-provisioner/00-common.yaml ${REPO}/examples/gateway-provisioner/01-roles.yaml ${REPO}/examples/gateway-provisioner/02-rolebindings.yaml ${REPO}/examples/gateway-provisioner/03-gateway-provisioner.yaml ; do
  # Set image pull policy to IfNotPresent so kubelet will use the
  # images that we loaded onto the node, rather than trying to pull
  # them from the registry.
  run::sed \
    "-es|imagePullPolicy: Always|imagePullPolicy: IfNotPresent|" \
    "$file"

  # Set the image tag to $VERSION to unambiguously use the image
  # we built above.
  run::sed \
    "-es|image: ghcr.io/projectcontour/contour:.*$|image: ghcr.io/projectcontour/contour:${VERSION}|" \
    "$file"

  # Add an item to the args field with the value --contour-image=ghcr.io/projectcontour/contour:${VERSION}
  run::sed \
    "-e 's/- --ingress-class=contour/- --ingress-class=contour\n        - --contour-image=ghcr.io\/projectcontour\/contour:${VERSION}/' \
    "$file"

  ${KUBECTL} apply -f "$file"
done

# Wait for the provisioner to report "Ready" status.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l control-plane=contour-gateway-provisioner deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l control-plane=contour-gateway-provisioner pods --for=condition=Ready
