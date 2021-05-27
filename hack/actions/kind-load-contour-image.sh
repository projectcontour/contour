#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly KIND=${KIND:-kind}

readonly CLUSTERNAME=${CLUSTERNAME:-contour-e2e}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::load::archive() {
    ${KIND} load image-archive \
        --name "${CLUSTERNAME}" \
        "$@"
}

if ! kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME does not exist"
    exit 2
fi

kind::cluster::load::archive "$(ls ${REPO}/image/contour-*.tar)"
