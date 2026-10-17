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

# Reuse a single development tag so repeated builds do not leave uniquely-tagged images behind.
readonly VERSION="dev"

# Build the Contour Provisioner image.
make -C ${REPO} container IMAGE=ghcr.io/projectcontour/contour VERSION=${VERSION}

# Push the Contour Provisioner image into the cluster.
kind::cluster::load::docker ghcr.io/projectcontour/contour:${VERSION}

# Install the Gateway provisioner using the loaded image.
export CONTOUR_IMG=ghcr.io/projectcontour/contour:${VERSION}

${KUBECTL} apply -f examples/gateway-provisioner/00-common.yaml
${KUBECTL} apply -f examples/gateway-provisioner/01-roles.yaml
${KUBECTL} apply -f examples/gateway-provisioner/02-rolebindings.yaml
${KUBECTL} apply -f <(cat examples/gateway-provisioner/03-gateway-provisioner.yaml | \
    yq eval '.spec.template.spec.containers[0].image = env(CONTOUR_IMG)' - | \
    yq eval '.spec.template.spec.containers[0].imagePullPolicy = "IfNotPresent"' - | \
    yq eval '.spec.template.spec.containers[0].args += "--contour-image="+env(CONTOUR_IMG)' -)

# The image tag does not change, so explicitly restart the provisioner and its managed workloads.
${KUBECTL} rollout restart -n projectcontour deployment/contour-gateway-provisioner

managed_workloads=$(
    ${KUBECTL} get deployments,daemonsets \
        --all-namespaces \
        --selector=app.kubernetes.io/managed-by=contour-gateway-provisioner \
        --output=jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.kind}{"/"}{.metadata.name}{"\n"}{end}'
)
while read -r namespace workload ; do
    if [[ -n "${workload}" ]]; then
        ${KUBECTL} rollout restart -n "${namespace}" "${workload}"
    fi
done <<< "${managed_workloads}"

# Wait for the provisioner to report "Ready" status.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l control-plane=contour-gateway-provisioner deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l control-plane=contour-gateway-provisioner pods --for=condition=Ready
