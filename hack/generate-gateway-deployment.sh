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
        sed 's|# gateway:|gateway:|g ; s|#   controllerName: projectcontour.io/gateway-controller|  controllerName: projectcontour.io/gateway-controller|g' < "$y"
        ;;
    *)
        cat $y
        ;;
    esac
done

for y in "${REPO}/examples/gateway/"*.yaml ; do
    echo # Ensure we have at least one newline between joined fragments.
    
    # Since the Gateway YAMLs are pulled from the Gateway API repo, the manifests do not start with "---".
    case $y in
    */00-crds.yaml)  
      echo "---"
      ;;

    */00-namespace.yaml)
      echo "---"
      ;;

    */01-admission_webhook.yaml)  
      echo "---"
      ;;

    */02-certificate_config.yaml)  
      echo "---"
      ;;

    esac
    
    cat "$y"
done
