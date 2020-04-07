#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly REPO=$(cd ${HERE}/../.. && pwd)

readonly KIND=${KIND:-${HOME}/bin/kind}
readonly KUBECTL=${KIND:-${HOME}/bin/kubectl}

$KIND create cluster --config ${REPO}/examples/kind/kind-expose-port.yaml --wait 2m
$KIND load docker-image docker.io/projectcontour/contour:master
$KIND load docker-image docker.io/projectcontour/contour:latest

$KUBECTL apply -f $REPO/examples/render/contour.yaml
$KUBECTL wait --timeout=2m -n projectcontour -l app=contour deployments --for=condition=Available
$KUBECTL wait --timeout=2m -n projectcontour -l app=envoy pods --for=condition=Ready

git clone --depth=1 https://jpeach:$GITHUB_TOKEN@github.com/jpeach/contour-testsuite.git

$HOME/bin/modden run \
    --fixtures contour-testsuite/fixtures \
    --policies contour-testsuite/policies \
    --param proxy.address=127.0.0.1 \
    --watch pods,secrets \
    contour-testsuite/contour/httpproxy/$1

Result=$?

$KIND delete cluster

exit $Result
