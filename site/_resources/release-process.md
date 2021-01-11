---
title: Contour Release Process
layout: page
---

- [Minor release process](#minor-release-process)
- [Patch release process](#patch-release-process)

# Minor Release Process

## Overview

A minor release is e.g. `v1.11.0`.

A minor release requires:
- website updates
- a release branch to be created
- YAML to be customized
- a release tag to be created
- an Operator release
- a GitHub release with release notes
- public communication
- cleanup


## Process

### Set environment variables

Set environment variables for use in subsequent steps:

```bash
export CONTOUR_RELEASE_VERSION=v1.11.0
export CONTOUR_RELEASE_VERSION_MAJOR=1
export CONTOUR_RELEASE_VERSION_MINOR=11

export CONTOUR_UPSTREAM_REMOTE_NAME=upstream
export CONTOUR_OPERATOR_UPSTREAM_REMOTE_NAME=upstream
```

### Update the website with release-specific information

1. Check out `main`, ensure it's up to date, and ensure you have a clean working directory.
1. Create a new local feature branch from `main`.
1. Generate a new set of versioned docs:
    
    ```bash
    go run ./hack/release/prepare-release.go $CONTOUR_RELEASE_VERSION
    ```

1. Add the new release to the compatibility matrix (`/site/_resources/compatibility-matrix.md`).
1. Document upgrade instructions for the new release (`/site/_resources/upgrading.md`).
1. Commit all changes, push the branch, and PR it into `main`.

_Note: the PR will probably fail the siteproof check due to [#2032](https://github.com/projectcontour/contour/issues/2032). It's a good idea to scan the CI log for any true issues._

### Branch and tag release

1. Check out `main`, ensure it's up to date, and ensure you have a clean working directory.
1. Create a local release branch:

    ```bash
    git checkout -b release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Push the branch to `github.com/projectcontour/contour`:

    ```bash
    git push --set-upstream ${CONTOUR_UPSTREAM_REMOTE_NAME} release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Update the deployment YAML and create a local tag:

    ```bash
    ./hack/release/make-release-tag.sh main $CONTOUR_RELEASE_VERSION
    ```

1. Push the branch to `github.com/projectcontour/contour`:

    ```bash
    git push ${CONTOUR_UPSTREAM_REMOTE_NAME} release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Push the tag to `github.com/projectcontour/contour`:

    ```bash
    git push ${CONTOUR_UPSTREAM_REMOTE_NAME} ${CONTOUR_RELEASE_VERSION}
    ```

### Release the operator

1. Check out `main`, ensure it's up to date, and ensure you have a clean working directory.
1. Create a local release branch:

    ```bash
    git checkout -b release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Push the branch to `github.com/projectcontour/contour-operator`:

    ```bash
    git push --set-upstream ${CONTOUR_OPERATOR_UPSTREAM_REMOTE_NAME} release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Update the deployment YAML and create a local tag:

    ```bash
    ./hack/release/make-release-tag.sh main $CONTOUR_RELEASE_VERSION
    ```

1. Push the branch to `github.com/projectcontour/contour-operator`:

    ```bash
    git push ${CONTOUR_OPERATOR_UPSTREAM_REMOTE_NAME} release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Push the tag to `github.com/projectcontour/contour-operator`:

    ```bash
    git push ${CONTOUR_OPERATOR_UPSTREAM_REMOTE_NAME} ${CONTOUR_RELEASE_VERSION}
    ```

### Update quickstart YAML redirects

1. Check out `main`, ensure it's up to date, and ensure you have a clean working directory.
1. Create a new local feature branch from `main`.
1. Update `netlify.toml` to redirect quickstart links to the new release branch.
1. Commit all changes, push the branch, and PR it into `main`.

### Do the Github release and write release notes

Now you have a tag pushed to Github, go to the release tab on github, select the tag and write up your release notes.

_Note: Filter on the Github label "release note" and Github milestone which should include any PRs which should be called out in the release notes._ 

### Toot your horn

- Post a blog entry to projectcontour.io
- Post a note to the #contour channel on k8s slack, also update the /topic with the current release number
- Post a note to the #project-contour channel on the vmware slack, also update the /topic with the current release number
- Send an update to the [cncf-contour-users mailing list](https://lists.cncf.io/g/cncf-contour-users/)

### File issues

If you encountered any problems or areas for improvement while executing the release, file issues before you forget.



# Patch Release Process

## Overview

A patch release is e.g. `v1.11.1`.

A patch release requires:
- patches to be cherry-picked onto the existing release branch
- website updates
- YAML to be customized
- a release tag to be created
- an Operator release
- a GitHub release with release notes
- public communication
- cleanup

## Process

### Set environment variables

Set environment variables for use in subsequent steps:

```bash
export CONTOUR_RELEASE_VERSION=v1.11.1
export CONTOUR_RELEASE_VERSION_MAJOR=1
export CONTOUR_RELEASE_VERSION_MINOR=11
export CONTOUR_PREVIOUS_VERSION=v1.11.0

export CONTOUR_UPSTREAM_REMOTE_NAME=upstream
export CONTOUR_OPERATOR_UPSTREAM_REMOTE_NAME=upstream
```

### Cherry-pick relevant commits into release branch

1. Get a list of commit SHAs from `main` to backport.
1. Check out the release branch for the minor version you're patching (i.e. `release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}`), ensure it's up to date, and ensure you have a clean working directory.
1. Create a new local feature branch from the release branch.
1. Cherry-pick each commit from Step 1, fixing any conflicts as needed:

    ```bash
    # repeat for each SHA
    git cherry-pick <SHA>
    ```

1. Commit all changes, push the branch, and PR it into the release branch.

### Update the website with release-specific information

1. Check out `main`, ensure it's up to date, and ensure you have a clean working directory.
1. Create a new local feature branch from `main`.
1. Generate a new set of versioned docs:
    
    ```bash
    go run ./hack/release/prepare-release.go $CONTOUR_PREVIOUS_VERSION $CONTOUR_RELEASE_VERSION
    ```

1. Add the new release to the compatibility matrix (`/site/_resources/compatibility-matrix.md`).
1. Document upgrade instructions for the new release (`/site/_resources/upgrading.md`).
1. Commit all changes, push the branch, and PR it into `main`.

### Update YAML and tag release

1. Check out the release branch, ensure it's up to date, and ensure you have a clean working directory.

1. Update the deployment YAML and create a local tag:

    ```bash
    ./hack/release/make-release-tag.sh $CONTOUR_PREVIOUS_VERSION $CONTOUR_RELEASE_VERSION
    ```

1. Push the branch to `github.com/projectcontour/contour`:

    ```bash
    git push ${CONTOUR_UPSTREAM_REMOTE_NAME} release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Push the tag to `github.com/projectcontour/contour`:

    ```bash
    git push ${CONTOUR_UPSTREAM_REMOTE_NAME} ${CONTOUR_RELEASE_VERSION}
    ```

### Release the operator

1. Get a list of commit SHAs from `main` to backport.
1. Check out the release branch for the minor version you're patching (i.e. `release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}`), ensure it's up to date, and ensure you have a clean working directory.
1. Create a new local feature branch from the release branch.
1. Cherry-pick each commit from Step 1, fixing any conflicts as needed:

    ```bash
    # repeat for each SHA
    git cherry-pick <SHA>
    ```

1. Commit all changes, push the branch, and PR it into the release branch.

1. Check out the release branch, ensure it's up to date, and ensure you have a clean working directory.

1. Update the deployment YAML and create a local tag:

    ```bash
    ./hack/release/make-release-tag.sh $CONTOUR_PREVIOUS_VERSION $CONTOUR_RELEASE_VERSION
    ```

1. Push the branch to `github.com/projectcontour/contour-operator`:

    ```bash
    git push ${CONTOUR_OPERATOR_UPSTREAM_REMOTE_NAME} release-${CONTOUR_RELEASE_VERSION_MAJOR}.${CONTOUR_RELEASE_VERSION_MINOR}
    ```

1. Push the tag to `github.com/projectcontour/contour-operator`:

    ```bash
    git push ${CONTOUR_OPERATOR_UPSTREAM_REMOTE_NAME} ${CONTOUR_RELEASE_VERSION}
    ```

### Do the Github release and write release notes

Now you have a tag pushed to Github, go to the release tab on github, select the tag and write up your release notes.

_Note: Filter on the Github label "release note" and Github milestone which should include any PRs which should be called out in the release notes._ 

### Toot your horn

- Post a note to the #contour channel on k8s slack, also update the /topic with the current release number
- Post a note to the #project-contour channel on the vmware slack, also update the /topic with the current release number
- Send an update to the [cncf-contour-users mailing list](https://lists.cncf.io/g/cncf-contour-users/)

### File issues

If you encountered any problems or areas for improvement while executing the release, file issues before you forget.
