#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly REPO=$(cd "$(dirname "$0")/.." && pwd)
readonly YEAR=$(date +%Y)

# Optional first aarg is the paths pattern.
readonly PATHS="${1:-"./apis/..."}"

readonly GO111MODULE=on

export GO111MODULE

boilerplate() {
    cat <<EOF
/*
Copyright Project Contour Authors

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

readonly HEADER=$(mktemp)

boilerplate > "${HEADER}"

echo "controller-gen version: "
go run sigs.k8s.io/controller-tools/cmd/controller-gen --version

exec go run sigs.k8s.io/controller-tools/cmd/controller-gen \
    "object:headerFile=${HEADER}" \
    "paths=${PATHS}"
