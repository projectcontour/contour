#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

mock_dirs=$(find ${REPO} -name mocks -type d)

declare -r -a TARGETS=(
	${REPO}/apis
	${REPO}/site/content/guides/metrics
	${REPO}/examples/render
	${REPO}/examples/contour
	${REPO}/examples/gateway
	${REPO}/examples/gateway-provisioner
	${REPO}/site/content/docs/main/config/api-reference.html
	${mock_dirs}
)

if git status -s ${TARGETS[@]} 2>&1 | grep -E -q '^\s+[MADRCU]'
then
	echo Uncommitted changes in generated sources:
	git status -s ${TARGETS[@]}
	git --no-pager diff
	exit 1
fi
