#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

if CURRENT_TAG=$(git describe --tags --exact-match 2>/dev/null); then
  # We are on a tag, so find previous tag to this one:
  # - list them sorted in reverse semver order
  # - find the current tag + the 5 previous (enough to filter out pre-release tags and still be left with a previous release tag)
  # - exclude the current tag (which may be a pre-release or not)
  # - exclude all remaining pre-release tags
  # - get the highest remaining tag
  git tag -l --sort=-v:refname | grep -A5 -x $CURRENT_TAG | grep -v -x $CURRENT_TAG | grep -v 'alpha\|beta\|rc' | head -n 1
elif git describe --tags --abbrev=0 | grep -q -v v1.2.0; then
  # Note: Contour v1.2.0 was improperly tagged on main so we
  # ignore it to ensure we dont hit that case here.

  # We have a tag in our commit history, so we should
  # be on a release branch or a feature branch from a
  # release branch, with an existing tag.
  git describe --tags --abbrev=0
else
  # We are on a release branch with no tag created yet, main, or some
  # other checkout, so just use the latest tag.
  # If needed, user can override this version with environment variables.
  git tag -l --sort=-v:refname | grep -v 'alpha\|beta\|rc' | head -1
fi
