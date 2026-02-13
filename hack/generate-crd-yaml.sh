#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}/.." && pwd)
readonly TEMPDIR=$(mktemp -d crd-XXXXXX)

# Optional first arg is the paths pattern.
readonly PATHS="${1:-"./apis/..."}"

trap 'rm -rf "$TEMPDIR"; exit' 0 1 2 15

cd "${REPO}"

echo "controller-gen version: "
go run sigs.k8s.io/controller-tools/cmd/controller-gen --version

# Controller-gen seems to use an unstable sort for the order of output of the CRDs
# so, output them to separate files, then concatenate those files.
# That should give a stable sort.
go run sigs.k8s.io/controller-tools/cmd/controller-gen \
  crd:crdVersions=v1 "paths=${PATHS}" "output:dir=${TEMPDIR}"

# Remove "error" from required fields in load balancer status.
# For details, see:
# - https://github.com/projectcontour/contour/issues/7391
# - https://github.com/kubernetes-sigs/controller-tools/pull/944#issuecomment-3314629362
# This workaround can be removed if the upstream Kubernetes type resolves the conflicting markers.
readonly HTTPPROXY_CRD="${TEMPDIR}/projectcontour.io_httpproxies.yaml"
readonly PATCH_PATH="/spec/versions/0/schema/openAPIV3Schema/properties/status/properties/loadBalancer/properties/ingress/items/properties/ports/items/required/0"

kubectl::patch() {
  kubectl patch -f "$HTTPPROXY_CRD" --local --type=json -p "$1" "${@:2}"
}

kubectl::patch "[{\"op\": \"test\", \"path\": \"$PATCH_PATH\", \"value\": \"error\"}]" --dry-run=client > /dev/null || {
  echo "Error: CRD structure has changed. The workaround for issue #7391 may no longer be needed or needs updating."
  exit 1
}

kubectl::patch "[{\"op\": \"remove\", \"path\": \"$PATCH_PATH\"}]" -o yaml > "$HTTPPROXY_CRD.tmp" && { echo "---"; cat "$HTTPPROXY_CRD.tmp"; } > "$HTTPPROXY_CRD"

# Explicitly add "preserveUnknownFields: false" to CRD specs since any CRDs created
# as v1beta1 will have this field set to true, which we don't want going forward, and
# it needs to be explicitly specified in order to be updated/removed. After enough time
# has passed and we're not concerned about folks upgrading from v1beta1 CRDs, we can
# remove the awk call that adds this field to the spec, and rely on the v1 default.
ls "${TEMPDIR}"/*.yaml | xargs cat | sed '/^$/d' \
  | awk '/group: projectcontour.io/{print "  preserveUnknownFields: false"}1' \
  > "${REPO}/examples/contour/01-crds.yaml"
