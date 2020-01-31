#! /usr/bin/env bash

# Checks if the files changed in the last commit are contained within
# specifed directories.  This is used to determine if a job should
# run based on changes within a commit PATHS_TO_SEARCH can be a single
# path "site" or multiple paths "site hack", response if for any path
# specified.

# Add to a job within .travis.yml
#      env:
#        - SEARCH_DIRECTORIES="site"
#      install:
#        - ./hack/travis/travis-check-files-changed.sh $SEARCH_DIRECTORIES ; RETURN_CODE=$? ; if [ $RETURN_CODE -eq 137 ]; then travis_terminate 0; elif [ $RETURN_CODE -ne 0 ]; then travis_terminate $RETURN_CODE; fi

set -o errexit
set -o nounset
set -o pipefail

# 1. Make sure the paths to search are not empty
if [ $# -eq 0 ]; then
    echo "usage: $0 DIRECTORY [DIRECTORY...]"
    exit 1
fi

# 2. Get the latest commit
readonly LATEST_COMMIT=$(git rev-parse HEAD)

# 3. Get the latest commit in the searched paths
readonly LATEST_COMMIT_IN_PATH=$(git log -1 --format=format:%H --full-diff "$@")

if [ $LATEST_COMMIT != $LATEST_COMMIT_IN_PATH ]; then
    echo "Exiting this job because code in the following paths have not changed:"
    echo $@
    exit 137
fi

echo "Changes detected in the following paths:"
echo $@
exit 0
