#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}"/.. && pwd)
readonly PROGNAME=$(basename "$0")
readonly TARGET="${REPO}/examples/render/contour-gateway.yaml"

exec > >(git stripspace >"$TARGET")

cat <<EOF
# This file is generated from the individual YAML files by $PROGNAME. Do not
# edit this file directly but instead edit the source files and re-render.
#
# Generated from:
EOF

for f in "examples/contour/"*.yaml "examples/gateway/"*.yaml ; do
  case $f in
  */03-envoy-deployment.yaml)
    # skip
    ;;
  *)
    (cd "${REPO}" && ls $f) | awk '{printf "#       %s\n", $0}'
    ;;
  esac
done

echo

for y in "${REPO}/examples/contour/"*.yaml ; do
    echo # Ensure we have at least one newline between joined fragments.
    case $y in
    */03-envoy-deployment.yaml)
        # skip
        ;;
    */01-contour-config.yaml)
        sed 's|# gateway:|gateway:|g ; s|#   controllerName: projectcontour.io/projectcontour/contour|  controllerName: projectcontour.io/projectcontour/contour|g' < "$y"
        ;;
    *)
        sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' < "$y"
        ;;
    esac
done

for y in "${REPO}/examples/gateway/"*.yaml ; do
    echo # Ensure we have at least one newline between joined fragments.
    case $y in
    */00-crds.yaml)
        # Since the Gateway CRDs are generated, the manifest does not start with "---".
        echo "---"
    esac
    cat "$y"
done
