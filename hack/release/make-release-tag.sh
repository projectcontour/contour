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

# If pushing the tag failed, then we might have already committed the
# YAML updates. The "git commit" will fail if there are no changes, so
# make sure that there are changes to commit before we do it.
if git status -s examples/contour 2>&1 | grep -E -q '^\s+[MADRCU]'; then
    git commit -s -m "Update Contour Docker image to $NEWVERS." \
        examples/contour/03-contour.yaml \
        examples/contour/03-envoy.yaml \
        examples/render/contour.yaml
fi

git tag -F - "$NEWVERS" <<EOF
Tag $NEWVERS release.

$(git shortlog "$OLDVERS..HEAD")
EOF

printf "Created tag '%s'\n" "$NEWVERS"

# People set up their remotes in different ways, so we need to check
# which one to dry run against. Choose a remote name that pushes to the
# projectcontour org repository (i.e. not the user's Github fork).
readonly REMOTE=$(git remote -v | awk '$2~/projectcontour\/contour/ && $3=="(push)" {print $1}' | head -n 1)
if [ -n "$REMOTE" ]; then
    printf "%s: unable to determine remote for %s\n" "$PROGNAME" "projectcontour/contour"
    exit 1
fi

printf "Testing whether tag '%s' can be pushed\n" "$NEWVERS"
git push --dry-run "$REMOTE" "$NEWVERS"

printf "Run 'git push %s %s' to push the tag if you are happy\n" "$REMOTE" "$NEWVERS"
