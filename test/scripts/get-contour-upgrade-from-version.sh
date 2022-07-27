#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

if CURRENT_TAG=$(git describe --tags --exact-match 2>/dev/null); then
  # We are on a tag, so find previous tag to this one.
  git tag -l --sort=-v:refname | grep -v 'alpha\|beta\|rc' | grep -A1 -x $CURRENT_TAG | grep -v "$CURRENT_TAG" | head -1
elif git branch --show-current | grep -q release; then
  # We are on a release branch, so find tag.
  git describe --tags --abbrev=0
else
  # We are likely on main or some other checkout, just use latest tag.
  # If needed, user can override this version with environment variables.
  git tag -l --sort=-v:refname | grep -v 'alpha\|beta\|rc' | head -1
fi
