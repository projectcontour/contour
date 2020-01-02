#! /usr/bin/env bash

set -o errexit

GOPATH=${GOPATH:-$(go env GOPATH)}

# "go env" doesn't print anything if GOBIN is the default, so we
# have to manually default it.
GOBIN=${GOBIN:-$(go env GOBIN)}
GOBIN=${GOBIN:-${GOPATH}/bin}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/.. && pwd)

gendoc::build() {
    go install github.com/ahmetb/gen-crd-api-reference-docs
}

# Exec the doc generator. Note that we use custom templates to inject
# the CSS styles that make the output look better on the Contour site.
gendoc::exec() {
    local readonly confdir="${REPO}/site/_data/refdocs"

    ${GOBIN}/gen-crd-api-reference-docs \
        -template-dir ${confdir} \
        -config ${confdir}/config.json \
        "$@"
}

gendoc::build

# Fake up a GOPATH so that the current working directory can be
# imported by the documentation generator.
GOPATH=$(mktemp -d)
mkdir -p ${GOPATH}/src/github.com/projectcontour
ln -s $REPO ${GOPATH}/src/github.com/projectcontour/contour

gendoc::exec \
    -api-dir github.com/projectcontour/contour/apis/projectcontour \
    -out-file $REPO/site/docs/master/api-reference.html
