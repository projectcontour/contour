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

# Explicitly add "preserveUnknownFields: false" to CRD specs since any CRDs created
# as v1beta1 will have this field set to true, which we don't want going forward, and
# it needs to be explicitly specified in order to be updated/removed. After enough time
# has passed and we're not concerned about folks upgrading from v1beta1 CRDs, we can
# remove the awk call that adds this field to the spec, and rely on the v1 default.
ls "${TEMPDIR}"/*.yaml | xargs cat | sed '/^$/d' \
  | awk '/group: projectcontour.io/{print "  preserveUnknownFields: false"}1' \
  > "${REPO}/examples/contour/01-crds.yaml"
