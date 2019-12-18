#! /usr/bin/env bash

GOPATH=${GOPATH:-$(go env GOPATH)}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/.. && pwd)

readonly GENDOC="${GOPATH}/src/github.com/ahmetb/gen-crd-api-reference-docs"

gendoc::build() {
    go get -u -d github.com/ahmetb/gen-crd-api-reference-docs

    (
        cd $GENDOC
        go build .
    )
}

# Exec the doc generator. Note that we use custom templates to inject
# the CSS styles that make the output look better on the Contour site.
gendoc::exec() {
    local readonly prefix=${GENDOC}

    $prefix/gen-crd-api-reference-docs \
        -template-dir $REPO/site/_data/template \
        -config $prefix/example-config.json \
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
