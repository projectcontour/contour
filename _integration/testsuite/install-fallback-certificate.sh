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

# install-fallback-certificate.sh: Install a fallback certificate
# configuration for Contour.

set -o pipefail
set -o errexit
set -o nounset

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

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

${KUBECTL} apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    tls:
      fallback-certificate:
        name: fallback-cert
        namespace: projectcontour
    accesslog-format: envoy
    disablePermitInsecure: false
EOF

# Wait for the fallback certificate to issue.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour certificates/fallback-cert --for=condition=Ready

# Restart to pick up the new config map.
${KUBECTL} rollout restart deployment -n projectcontour contour

# Wait for contour to restart.
${KUBECTL} wait --timeout="${WAITTIME}" -n projectcontour -l app=contour deployments --for=condition=Available
