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

# Create the Contour namespace and service account.
${KUBECTL} apply -f ${REPO}/examples/contour/namespace.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/service-account.yaml

# Set the image tag to match the current Contour version.
VERSION="v$$"

# Build the Contour Provisioner image.
make -C ${REPO}/provisioner container IMAGE=ghcr.io/projectcontour/contour-provisioner VERSION=${VERSION}

# Push the Contour Provisioner image into the cluster.
kind::cluster::load::docker ghcr.io/projectcontour/contour-provisioner:${VERSION}

# Install the Contour Provisioner.
${KUBECTL} apply -f ${REPO}/provisioner/deploy/00-crds.yaml
${KUBECTL} apply -f ${REPO}/provisioner/deploy/01-rbac.yaml
${KUBECTL} apply -f ${REPO}/provisioner/deploy/02-deployment.yaml
${KUBECTL} apply -f ${REPO}/provisioner/deploy/02-service.yaml

# Wait for the provisioner to report "Ready" status.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour-provisioner deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour-provisioner pods --for=condition=Ready
