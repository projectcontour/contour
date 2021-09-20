#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

GOPATH=${GOPATH:-$(go env GOPATH)}

# "go env" doesn't print anything if GOBIN is the default, so we
# have to manually default it.
GOBIN=${GOBIN:-$(go env GOBIN)}
GOBIN=${GOBIN:-${GOPATH}/bin}

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}/.." && pwd)

# Optional first arg is the package root to scan for documentation.
readonly PKGROOT="${1:-github.com/projectcontour/contour/apis/projectcontour}"

gendoc::build() {
    go install github.com/ahmetb/gen-crd-api-reference-docs
}

# Exec the doc generator. Note that we use custom templates to inject
# the CSS styles that make the output look better on the Contour site.
gendoc::exec() {
    local -r confdir="${REPO}/hack/api-docs-config/refdocs"

    "${GOBIN}"/gen-crd-api-reference-docs \
        -template-dir "${confdir}" \
        -config "${confdir}/config.json" \
        "$@"
}

gendoc::build

# Fake up a GOPATH so that the current working directory can be
# imported by the documentation generator.
GOPATH=$(mktemp -d)
mkdir -p "${GOPATH}/src/github.com/projectcontour"
ln -s "${REPO}" "${GOPATH}/src/github.com/projectcontour/contour"

gendoc::exec \
    -api-dir "${PKGROOT}" \
    -out-file "${REPO}/site/content/docs/main/config/api-reference.html"
