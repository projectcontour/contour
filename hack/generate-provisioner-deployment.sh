#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}"/.. && pwd)
readonly PROGNAME=$(basename "$0")
readonly TARGET="${REPO}/examples/render/contour-gateway-provisioner.yaml"

exec > >(git stripspace >"$TARGET")

# FILES defines the set of source files to render together.
readonly FILES="examples/contour/01-crds.yaml
examples/gateway/00-crds.yaml
examples/gateway-provisioner/*.yaml"

# Write file header listing individual files used.
cat <<EOF
# This file is generated from the individual YAML files by $PROGNAME. Do not
# edit this file directly but instead edit the source files and re-render.
#
# Generated from:
EOF

for f in $FILES; do
   (ls "$f") | awk '{printf "#       %s\n", $0}'
done

# Insert newline.
echo

# Concatenate files together.
for y in $FILES ; do
    # Ensure we have at least one newline between joined fragments.
    echo 

    # Since the Gateway YAMLs are pulled from the Gateway API repo, the manifests do not start with "---".
    case $y in
    */gateway/00-crds.yaml)  
      echo "---"
      ;;

    esac
      
    # Write the file contents.
    cat $y
done

