#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/.. && pwd)

ls ${REPO}/examples/contour/*.yaml | \
  xargs cat ${REPO}/examples/render/gen-warning.yaml | \
  sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' \
  > ${REPO}/examples/render/contour.yaml
