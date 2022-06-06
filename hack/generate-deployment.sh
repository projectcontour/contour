#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}"/.. && pwd)
readonly PROGNAME=$(basename "$0")

# Target defines the file which the rendered file will be outputted.
TARGET=""
SKIP_FILE=""

# MODE should be "deployment" or "daemonset"
readonly MODE="$1"

case $MODE in
  deployment)
      TARGET="${REPO}/examples/render/contour-deployment.yaml"
      SKIP_FILE="*/03-envoy.yaml"
      ;;
  daemonset)
      TARGET="${REPO}/examples/render/contour.yaml"
      SKIP_FILE="*/03-envoy-deployment.yaml"
      ;;
esac

# Files defines the set of source files to render together.
readonly FILES="examples/contour/*.yaml
examples/deployment/03-envoy-deployment.yaml"

exec > >(git stripspace >"$TARGET")

cat <<EOF
# This file is generated from the individual YAML files by $PROGNAME. Do not
# edit this file directly but instead edit the source files and re-render.
#
# Generated from:
EOF

for f in $FILES; do

   case $f in
   $SKIP_FILE)
      # skip this file
      ;;
   *)
   (ls "$f") | awk '{printf "#       %s\n", $0}'
      ;;
    esac
done

echo

for y in $FILES ; do
    echo # Ensure we have at least one newline between joined fragments.
    case $y in
    $SKIP_FILE)
       # skip this file
        ;;
    *)
        cat $y
        ;;
    esac
done

