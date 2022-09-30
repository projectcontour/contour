#! /usr/bin/env bash

readonly PROGNAME=$(basename "$0")
readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}/../.." && pwd)

readonly TOKEN=${TOKEN:-}

readonly DOCKER=${DOCKER:-docker}
readonly LABELS=${LABELS:-${REPO}/site/data/github-labels.yaml}

set -o errexit
set -o nounset
set -o pipefail

labelsync() {
    $DOCKER run \
        --rm \
        --volume "${REPO}:/$(basename "${REPO}")" \
        gcr.io/k8s-prow/label_sync:latest \
    "$@"
}

path::absolute() {
    local -r p="$1"
    local dir

    dir=$(cd "$(dirname "$p")" && pwd)

    echo "${dir}/$(basename "$p")"
}

# NOTE: $TOKEN has to be a file that is inside $REPO.
if [ -z "${TOKEN}" ]; then
    echo "$PROGNAME: missing \$TOKEN"
    exit 2
fi

# Make the path absolute.
yaml=$(path::absolute "${LABELS}")

# Remove up to the basename of the repo. We have have an absolute path
# within the repository, which will resolve within the container.
yaml=${yaml##$(dirname "${REPO}")}

# Treat the token the same as the YAML path, which requires it to be a
# file within the repository.
token=$(path::absolute "${TOKEN}")
token=${token##$(dirname "${REPO}")}

labelsync \
    --debug \
    --confirm \
    --orgs projectcontour \
    --skip projectcontour/toc,projectcontour/yages \
    --config "${yaml}" \
    --token "${token}"
