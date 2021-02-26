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

# install-ratelimit-service.sh: Install a rate limit service and configuration
# for Contour.

set -o pipefail
set -o errexit
set -o nounset

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

# Define some rate limiting policies to correspond to
# testsuite/httpproxy/020-global-rate-limiting.yaml.
${KUBECTL} apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ratelimit-config
  namespace: projectcontour
data:
  ratelimit-config.yaml: |
    domain: contour
    descriptors:
      - key: generic_key
        value: vhostlimit
        rate_limit:
          unit: hour
          requests_per_unit: 1
      - key: generic_key
        value: routelimit
        rate_limit:
          unit: hour
          requests_per_unit: 1
      - key: generic_key
        value: tlsvhostlimit
        rate_limit:
          unit: hour
          requests_per_unit: 1
      - key: generic_key
        value: tlsroutelimit
        rate_limit:
          unit: hour
          requests_per_unit: 1
EOF

# Create the ratelimit deployment, service and extension service.
${KUBECTL} apply -f ${REPO}/examples/ratelimit/02-ratelimit.yaml
${KUBECTL} apply -f ${REPO}/examples/ratelimit/03-ratelimit-extsvc.yaml

# Configure the ratelimit extension service with Contour.
${KUBECTL} apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    rateLimitService:
      extensionService: projectcontour/ratelimit
      domain: contour
      failOpen: false  
    # The options below are copied from
    # install-fallback-certificate.sh.
    tls:
      fallback-certificate:
        name: fallback-cert
        namespace: projectcontour
    accesslog-format: envoy
    disablePermitInsecure: false
EOF

# Restart to pick up the new config map.
${KUBECTL} rollout restart deployment -n projectcontour contour

# Wait for contour to restart.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour deployments --for=condition=Available

# Wait for ratelimit service to be ready.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=ratelimit deployments --for=condition=Available
