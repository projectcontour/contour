#! /usr/bin/env bash

# Latest semver release does not compile, gg controller-tools.
# Override it with this for now.
go install sigs.k8s.io/controller-tools/cmd/controller-gen

TMPDIR=`mktemp -d crd.XXXXXX`

trap "rm -rf $TMPDIR; exit" 0 1 2 15

# Controller-gen seems to use an unstable sort for the order of output of the CRDs
# so, output them to separate files, then concatenate those files.
# That should give a stable sort.
controller-gen crd paths=../apis/... output:dir=$TMPDIR

ls $TMPDIR/*.yaml | xargs cat | sed '/^$/d' > contour/01-crds.yaml
