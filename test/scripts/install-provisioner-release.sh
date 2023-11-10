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

# install-provisioner-release.sh: Install a specific release of the Contour gateway provisioner.

set -o pipefail
set -o errexit
set -o nounset

readonly KUBECTL=${KUBECTL:-kubectl}
readonly WAITTIME=${WAITTIME:-5m}

readonly PROGNAME=$(basename "$0")
readonly VERS=${1:-}

if [ -z "$VERS" ] ; then
        printf "Usage: %s VERSION\n" $PROGNAME
        exit 1
fi

# Install the Contour version.
${KUBECTL} apply -f "https://projectcontour.io/quickstart/$VERS/contour-gateway-provisioner.yaml"

# Wait for Gateway API admission server to be fully rolled out.
# This is only for backwards compatibility, the Gateway API
# admission server is not included as of Contour 1.27.
if ${KUBECTL} get namespace gateway-system > /dev/null 2>&1; then
        ${KUBECTL} rollout status --timeout="${WAITTIME}" -n gateway-system deployment/gateway-api-admission-server
fi

${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l control-plane=contour-gateway-provisioner deployments --for=condition=Available
