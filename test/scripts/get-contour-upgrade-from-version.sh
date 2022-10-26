#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

if CURRENT_TAG=$(git describe --tags --exact-match 2>/dev/null); then
  # We are on a tag, so find previous tag to this one.
  git tag -l --sort=-v:refname | grep -v 'alpha\|beta\|rc' | grep -A1 -x $CURRENT_TAG | tail -1
elif git describe --tags --abbrev=0 | grep -q -v v1.2.0; then
  # Note: Contour v1.2.0 was improperly tagged on main so we
  # ignore it to ensure we dont hit that case here.

  # We should be on a release branch with an existing tag.
  # Check the branch name and error if it does not contain release.
  # Tags should not be added to non-release branches.
  if git branch --show-current | grep -q release; then
    git describe --tags --abbrev=0
  else
    echo 'Error: invalid tag on branch'
    exit 1
  fi
else
  # We are on a release branch with no tag created yet, main, or some
  # other checkout, so just use the latest tag.
  # If needed, user can override this version with environment variables.
  git tag -l --sort=-v:refname | grep -v 'alpha\|beta\|rc' | head -1
fi
