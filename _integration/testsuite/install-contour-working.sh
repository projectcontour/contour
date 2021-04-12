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

readonly CLUSTERNAME=${CLUSTERNAME:-contour-integration}
readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

# List of tags to apply to the image built from the working directory.
# The "working" tag is applied to unambigiously reference the working
# image, since "main" and "latest" could also come from the Docker
# registry.
readonly TAGS="main latest working"

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::load() {
    ${KIND} load docker-image \
        --name "${CLUSTERNAME}" \
        "$@"
}

if ! kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME does not exist"
    exit 2
fi

# Build the current version of Contour.
make -C ${REPO} container IMAGE=docker.io/projectcontour/contour VERSION="v$$"

for t in $TAGS ; do
    docker tag \
        docker.io/projectcontour/contour:"v$$" \
        docker.io/projectcontour/contour:$t
done

# Push the Contour build image into the cluster.
for t in $TAGS ; do
    kind::cluster::load docker.io/projectcontour/contour:$t
done


# Install Contour

${KUBECTL} apply -f ${REPO}/examples/contour/00-common.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/01-crds.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-rbac.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-role-contour.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-service-contour.yaml
${KUBECTL} apply -f ${REPO}/examples/contour/02-service-envoy.yaml

# Manifests use the "Always" image pull policy, which forces the kubelet to re-fetch from
# DockerHub, which is why we have to update policy to `IfNotPresent`.
${KUBECTL} apply -f <(sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' < ${REPO}/examples/contour/02-job-certgen.yaml)
${KUBECTL} apply -f <(sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' < ${REPO}/examples/contour/03-contour.yaml)
${KUBECTL} apply -f <(sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' < ${REPO}/examples/contour/03-envoy.yaml)

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
      name: contour
      namespace: projectcontour
    rateLimitService:
      extensionService: projectcontour/ratelimit
      domain: contour
      failOpen: false
    tls:
      fallback-certificate:
        name: fallback-cert
        namespace: projectcontour
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

# Create a GatewayClass
${KUBECTL} apply -f - <<EOF
apiVersion: networking.x-k8s.io/v1alpha1 
kind: GatewayClass
metadata:
  name: contour-class
spec:
  controller: projectcontour.io/ingress-controller
EOF


${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour deployments --for=condition=Available
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=envoy pods --for=condition=Ready
