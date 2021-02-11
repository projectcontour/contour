#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

declare -r -a TARGETS=(
	${REPO}/apis
	${REPO}/site/_metrics
	${REPO}/examples/render
	${REPO}/examples/contour
	${REPO}/site/docs/main/config/api-reference.html
)

if git status -s ${TARGETS[@]} 2>&1 | grep -E -q '^\s+[MADRCU]'
then
	echo Uncommitted changes in generated sources:
	git status -s ${TARGETS[@]}
	exit 1
fi
