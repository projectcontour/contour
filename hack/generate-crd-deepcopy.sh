#! /usr/bin/env bash

# See
#	- https://github.com/kubernetes/sample-controller/blob/master/hack/update-codegen.sh
#	- https://github.com/kubernetes/code-generator/blob/master/generate-groups.sh
#
# This script does basically the same as the generate-groups script linked
# above. Since Go modules don't download scripts, don't have a vendored
# copy of the script, hence this facsimilie.

set -o errexit
set -o nounset
set -o pipefail

readonly REPO=$(cd $(dirname $0)/.. && pwd)
readonly DESTDIR=${DESTDIR:-$(mktemp -d)}
readonly YEAR=$(date +%Y)

readonly GO111MODULE=on
readonly GOFLAGS=-mod=vendor

export GO111MODULE
export GOFLAGS

readonly CONTOUR="github.com/projectcontour/contour"


boilerplate() {
    cat <<EOF
/*
Copyright © ${YEAR} VMware

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
EOF
}

generator() {
	local -r tool="$1"

	shift

	go run k8s.io/code-generator/cmd/$tool \
		--go-header-file ${DESTDIR}/boilerplate.go.tmpl \
		--output-base ${DESTDIR} \
		"$@"
}

# The output files will be generated to ${DESTDIR}/${CONTOUR}, so make a
# symbolic link back to the current repository to make them update in place
(
	cd ${DESTDIR}
	mkdir -p $(dirname $CONTOUR)
	ln -s ${REPO} $(dirname $CONTOUR)/contour

        boilerplate > ${DESTDIR}/boilerplate.go.tmpl
)

generator \
deepcopy-gen \
	--input-dirs ${CONTOUR}/apis/projectcontour/v1 \
	--output-file-base zz_generated.deepcopy \
	--output-package ${CONTOUR}/apis/projectcontour/v1

generator \
deepcopy-gen \
	--input-dirs ${CONTOUR}/apis/contour/v1beta1 \
	--output-file-base zz_generated.deepcopy \
	--output-package ${CONTOUR}/apis/contour/v1beta1
