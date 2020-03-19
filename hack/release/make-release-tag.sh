#! /usr/bin/env bash

# make-release-tag.sh: This script assumes that you are on a branch and have
# otherwise prepared the release. It rewrites the Docker image version in
# the deployment YAML, then created a tag with a message containing the
# shortlog from the previous versio.

readonly PROGNAME=$(basename "$0")
readonly OLDVERS="$1"
readonly NEWVERS="$2"

if [ -z "$OLDVERS" ] || [ -z "$NEWVERS" ]; then
    printf "Usage: %s OLDVERS NEWVERS\n" PROGNAME
    exit 1
fi

set -o errexit
set -o nounset
set -o pipefail

readonly IMG="docker.io/projectcontour/contour:$NEWVERS"


if [ -n "$(git tag --list "$NEWVERS")" ]; then
    printf "%s: tag '%s' already exists\n" "$PROGNAME" "$NEWVERS"
    exit 1
fi

# NOTE(jpeach): this will go away or change once we move to kustomize
# since at that point the versioned image name will appear exactly once.
for f in examples/contour/03-envoy.yaml examples/contour/03-contour.yaml ; do
    case $(uname -s) in
    Darwin)
        sed -i '' "-es|docker.io/projectcontour/contour:master|$IMG|" "$f"
        ;;
    Linux)
        sed -i "-es|docker.io/projectcontour/contour:master|$IMG|" "$f"
        ;;
    *)
        printf "Unsupported system '%s'" "$(uname -s)"
        exit 2
        ;;
    esac
done

make generate

git commit -s -m "Update Contour Docker image to $NEWVERS." \
    examples/contour/03-contour.yaml \
    examples/contour/03-envoy.yaml \
    examples/render/contour.yaml

git tag -F - "$NEWVERS" <<EOF
Tag $NEWVERS release.

$(git shortlog "$OLDVERS..HEAD")
EOF

printf "Created tag '%s'\n" "$NEWVERS"
printf "Run '%s' to push the tag if you are happy\n" "git push --tags"
