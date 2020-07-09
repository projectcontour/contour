#! /usr/bin/env bash

# Copyright Project Contour Authors
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License.  You may obtain
# a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  See the
# License for the specific language governing permissions and limitations
# under the License.

set -o pipefail
set -o errexit
set -o nounset

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly CLUSTERNAME=${CLUSTER:-contour-integration}
readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

# List of tags to apply to the image built from the working directory.
# The "working" tag is applied to unambigiously reference the working
# image, since "master" and "latest" could also come from the Docker
# registry.
readonly TAGS="master latest working"

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::create() {
    ${KIND} create cluster \
        --name "${CLUSTERNAME}" \
        --wait "${WAITTIME}" \
        --config "${REPO}/_integration/testsuite/kind-expose-port.yaml"
}

kind::cluster::load() {
    ${KIND} load docker-image \
        --name "${CLUSTERNAME}" \
        "$@"
}

if kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME already exists"
    exit 2
fi

# Build the current version of Contour.
make -C ${REPO} container IMAGE=docker.io/projectcontour/contour VERSION="v$$"

for t in $TAGS ; do
    docker tag \
        docker.io/projectcontour/contour:"v$$" \
        docker.io/projectcontour/contour:$t
done

# Create a fresh kind cluster.
if ! kind::cluster::exists "$CLUSTERNAME" ; then
  kind::cluster::create
fi

# Push the Contour build image into the cluster.
for t in $TAGS ; do
    kind::cluster::load docker.io/projectcontour/contour:$t
done

# Install Contour.
#
# NOTE(jpeach): The certgen job uses the ":latest" tag with the
# "Latest" pull policy, which forces the kubelet to re-fetch from
# DockerHub, which is why we have to whack the image pull policy.
for y in ${REPO}/examples/contour/*.yaml ; do
  ${KUBECTL} apply -f <(sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' < "$y")
done

${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=envoy pods --for=condition=Ready

# Install cert-manager.
${KUBECTL} apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.14.1/cert-manager.yaml
${KUBECTL} wait --timeout="${WAITTIME}" -n cert-manager -l app=cert-manager deployments --for=condition=Available
