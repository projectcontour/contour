#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/.. && pwd)
readonly PROGNAME=$(basename $0)


readonly TARGET="${REPO}/examples/render/contour.yaml"

exec >$TARGET

cat <<EOF
# This file is generated from the individual YAML files by $PROGNAME. Do not
# edit this file directly but instead edit the source files and re-render.
#
# Generated from:
EOF

(cd ${REPO} && ls examples/contour/*.yaml) | \
  awk '{printf "#       %s\n", $0}'

echo "#"
echo

cat ${REPO}/examples/contour/*.yaml | \
  sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g'

