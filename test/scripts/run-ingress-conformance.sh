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

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

readonly KUBECTL=${KUBECTL:-kubectl}
readonly SONOBUOY=${SONOBUOY:-sonobuoy}

readonly INGRESS_CLASS=${INGRESS_CLASS:-contour}
# Latest images can be found here: https://console.cloud.google.com/gcr/images/k8s-staging-ingressconformance/GLOBAL/ingress-controller-conformance
readonly INGRESS_CONFORMANCE_IMAGE=${INGRESS_CONFORMANCE_IMAGE:-gcr.io/k8s-staging-ingressconformance/ingress-controller-conformance@sha256:148a649b7d009e8544e2631950b0c05a41cf9a50ede39e20b76bdaaf2ffb873b}

# Set the Ingress Status Address so conformance test pods are reachable in tests
# This multiline sed command is for compatibility across MacOS and GNU sed
${KUBECTL} apply -f <(sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' ${REPO}/examples/contour/03-contour.yaml | sed 's/\s*- serve/&\
        - --ingress-status-address=envoy.projectcontour/')
${KUBECTL} rollout status -n projectcontour deployment contour

${SONOBUOY} run \
    --skip-preflight \
    --kube-conformance-image=${INGRESS_CONFORMANCE_IMAGE} \
    --plugin e2e \
    --plugin-env e2e.INGRESS_CLASS=${INGRESS_CLASS} \
    --plugin-env e2e.WAIT_FOR_STATUS_TIMEOUT=${WAIT_FOR_STATUS_TIMEOUT:-5m} \
    --plugin-env e2e.TEST_TIMEOUT=${TEST_TIMEOUT:-20m} \
    --wait

${SONOBUOY} retrieve

readonly RESULTS_DIR=$(mktemp -d)
tar xzvf *_sonobuoy_*.tar.gz -C ${RESULTS_DIR}
cat ${RESULTS_DIR}/plugins/e2e/results/global/*
grep -r -q failed ${RESULTS_DIR}/plugins/e2e/results/global && echo "***** FAILED INGRESS CONFORMANCE *****" && exit 1