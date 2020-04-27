#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/.. && pwd)

cd $REPO

# Generate backwards-compatible CRDs.
go run sigs.k8s.io/controller-tools/cmd/controller-gen \
  crd paths=./apis/... output:dir=${REPO}/config/components/types

# Generate V1 CRDs for Kubernetes 1.6 or later.
go run sigs.k8s.io/controller-tools/cmd/controller-gen \
  crd:crdVersions=v1 paths=./apis/... output:dir=${REPO}/config/components/types-v1
