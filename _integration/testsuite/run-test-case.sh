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

readonly TESTER=${TESTER:-integration-tester}

readonly ADDRESS=${ADDRESS:-"127.0.0.1"}
readonly HTTP_PORT=${HTTP_PORT:-"9080"}
readonly HTTPS_PORT=${HTTPS_PORT:-"9443"}

exec "$TESTER" run \
    --format=tap \
    --fixtures "${HERE}/fixtures" \
    --policies "${HERE}/policies" \
    --param proxy.address="$ADDRESS" \
    --param proxy.http_port="$HTTP_PORT" \
    --param proxy.https_port="$HTTPS_PORT" \
    --watch pods,secrets \
    "$@"
