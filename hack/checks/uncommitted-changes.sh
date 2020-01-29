#! /usr/bin/env bash

# uncommitted-changes.sh: Exit with an error if there are any uncommitted
# source file changes.

readonly REPO=$(cd $(dirname $0)/../.. && pwd)

cd ${REPO}

if git status -s 2>&1 | grep -E -q '^\s+[MADRCU]'; then \
        echo Uncommitted changes in generated sources: ; \
        git status -s ; \
        exit 1; \
fi

exit 0
