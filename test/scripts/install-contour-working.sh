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

# install-contour-working.sh: Install Contour from the working repo.

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly CLUSTERNAME=${CLUSTERNAME:-contour-e2e}
readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::load::docker() {
    ${KIND} load docker-image \
        --name "${CLUSTERNAME}" \
        "$@"
}

if ! kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME does not exist"
    exit 2
fi

# Wrap sed to deal with GNU and BSD sed flags.
run::sed() {
    local -r vers="$(sed --version < /dev/null 2>&1 | grep -q GNU && echo gnu || echo bsd)"
    case "$vers" in
        gnu) sed -i "$@" ;;
        *) sed -i '' "$@" ;;
    esac
}

# Build the current version of Contour.
VERSION="v$$"
make -C ${REPO} container IMAGE=ghcr.io/projectcontour/contour VERSION=${VERSION}

# Push the Contour build image into the cluster.
kind::cluster::load::docker ghcr.io/projectcontour/contour:${VERSION}

# Install Contour
${KUBECTL} apply -f ${REPO}/examples/contour/00-common.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/01-crds.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-rbac.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-role-contour.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-service-contour.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-service-envoy.yaml

for file in ${REPO}/examples/contour/02-job-certgen.yaml ${REPO}/examples/contour/03-contour.yaml ${REPO}/examples/contour/03-envoy.yaml ; do
  # Set image pull policy to IfNotPresent so kubelet will use the
  # images that we loaded onto the node, rather than trying to pull
  # them from the registry.
  run::sed \
    "-es|imagePullPolicy: Always|imagePullPolicy: IfNotPresent|" \
    "$file"

  # Set the image tag to $VERSION to unambiguously use the image
  # we built above.
  run::sed \
    "-es|image: ghcr.io/projectcontour/contour:.*$|image: ghcr.io/projectcontour/contour:${VERSION}|" \
    "$file"

  ${KUBECTL} apply -f "$file"
done

# The Contour pod won't schedule until this ConfigMap is created, since it's mounted as a volume.
# This is ok to create the config after the Contour deployment.
${KUBECTL} apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    gateway:
      controllerName: projectcontour.io/projectcontour/contour
EOF

# Install fallback cert

${KUBECTL} apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
EOF

${KUBECTL} apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: fallback-cert
  namespace: projectcontour
spec:
  dnsNames:
  - fallback.projectcontour.io
  secretName: fallback-cert
  issuerRef:
    name: selfsigned
    kind: ClusterIssuer
EOF

${KUBECTL} apply -f - <<EOF
apiVersion: projectcontour.io/v1
kind: TLSCertificateDelegation
metadata:
  name: fallback-cert
  namespace: projectcontour
spec:
  delegations:
  - secretName: fallback-cert
    targetNamespaces:
    - "*"
EOF

# Wait for the fallback certificate to issue.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour certificates/fallback-cert --for=condition=Ready

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
      - key: route_limit_key
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

# Wait for Contour and Envoy to report "Ready" status.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=envoy pods --for=condition=Ready
