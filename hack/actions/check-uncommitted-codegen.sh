#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

declare -r -a DIRS=(
	${REPO}/apis
	${REPO}/site/_metrics
	${REPO}/examples/render
	${REPO}/examples/contour
)

if git status -s ${DIRS[@]} 2>&1 | grep -E -q '^\s+[MADRCU]'
then
	echo Uncommitted changes in generated sources:
	git status -s ${DIRS[@]}
	exit 1
fi
