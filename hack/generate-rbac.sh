#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly REPO=$(cd "$(dirname "$0")/.." && pwd)
readonly PROGNAME=$(basename "$0")

readonly GO111MODULE=on

export GO111MODULE

exec >"${REPO}/examples/contour/02-role-contour.yaml"

cat <<EOF
# The following ClusterRole is generated from kubebuilder RBAC tags by
# $PROGNAME. Do not edit this file directly but instead edit the source
# files and re-render.
EOF

exec go run sigs.k8s.io/controller-tools/cmd/controller-gen \
    rbac:roleName=contour \
    output:stdout \
    paths="./internal/k8s"
