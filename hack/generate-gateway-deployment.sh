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
  (cd "${REPO}" && ls $f) | awk '{printf "#       %s\n", $0}'
done
echo "#"
echo

# certgen uses the ':latest' image tag, so it always needs to be pulled. Everything
# else correctly uses versioned image tags so we should use IfNotPresent and updates
# the Contour config for Gateway API.
for y in "${REPO}/examples/contour/"*.yaml ; do
    echo # Ensure we have at least one newline between joined fragments.
    case $y in
    */01-contour-config.yaml)
        sed 's|# gateway:|gateway:|g ; s|#   controllerName: projectcontour.io/projectcontour/contour|  controllerName: projectcontour.io/projectcontour/contour|g ; s|#   name: contour|  name: contour|g ; s|#   namespace: projectcontour|  namespace: projectcontour|g' < "$y"
        ;;
    */02-job-certgen.yaml)
        cat "$y"
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
