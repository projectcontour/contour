---
title: Contour Release Process
layout: page
---

- [Minor release process][1]
- [Patch release process][2]
- [RC release process][3]

# Minor Release Process

## Overview

A minor release is e.g. `v1.11.0`.

A minor release requires:
- website updates
- a release branch to be created
- YAML to be customized
- an update to dependabot config
- a release tag to be created
- a GitHub release with release notes
- submitting a Gateway API conformance report (if applicable)
- public communication
- cleanup


## Process

### Set environment variables

Set environment variables for use in subsequent steps:

```bash
export CONTOUR_RELEASE_VERSION=v1.11.0
export CONTOUR_RELEASE_VERSION_MAJOR=1
export CONTOUR_RELEASE_VERSION_MINOR=11

export KUBERNETES_MIN_VERSION=1.20
export KUBERNETES_MAX_VERSION=1.22

export CONTOUR_UPSTREAM_REMOTE_NAME=upstream
```

### Update the website with release-specific information

1. Check out `main`, ensure it's up to date, and ensure you have a clean working directory.
1. Create a new local feature branch from `main`.
1. Generate a new set of versioned docs, plus a changelog:

    ```bash
    go run ./hack/release/prepare-release.go $CONTOUR_RELEASE_VERSION $KUBERNETES_MIN_VERSION $KUBERNETES_MAX_VERSION
    ```
1. Proofread the release notes and do any reordering, rewording, reformatting necessary, including editing or deleting the "Deprecation and Removal Notices" section.
1. Add the new release to the compatibility matrix (`site/content/resources/compatibility-matrix.md`).
1. Add the new release to the compatibility YAML (`/versions.yaml`). Be sure to mark this new version as supported and mark oldest currently supported version as no longer supported.
1. Update `.github/workflows/trivy-scan.yaml` to add new release branch and remove oldest listed (should always be 3 latest branches listed).
1. Commit all changes, push the branch, and PR it into `main`.

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

### Update Dependabot configuration

Update the `.github/dependabot.yml` file to update the target branches for Dependabot scanning.
The latest minor (N) and two previous minors should be scanned (N-1 and N-2).

### Submit a Gateway API conformance report

If the compatible Gateway API version has been bumped in this release, a conformance report should be reported upstream.
Currently the process is manual but we have automated running the conformance suite and report generation in CI on release tags.
Steps to submit a conformance report are:

1. Find the corresponding `Build and push a release` GitHub Action workflow run for this release from [here][6]
1. Download the `gateway-conformance-report` artifact from the workflow run and unzip
1. Check it for accuracy, and make a PR like [this one][7] to the `gateway-api` repo with the content of the report

### Update quickstart YAML redirects

1. Check out `main`, ensure it's up to date, and ensure you have a clean working directory.
1. Create a new local feature branch from `main`.
1. Update `netlify.toml` to redirect Contour quickstart links to the new release branch.
1. Commit all changes, push the branch, and PR it into `main`.

### Do the Github release

Now you have a tag pushed to Github, go to the release tab on github, select the tag and paste in the release notes that were generated previously.

# Patch Release Process

## Overview

A patch release is e.g. `v1.11.1`.

A patch release requires:
- patches to be cherry-picked onto the existing release branch
- website updates
- YAML to be customized
- a release tag to be created
- a GitHub release with release notes
- submitting a Gateway API conformance report (if applicable)
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
1. Create release notes in `changelogs/CHANGELOG-<version>.md`.
1. Add the new release to the compatibility matrix (`/site/content/resources/compatibility-matrix.md`).
1. Add the new release to the compatibility YAML (`/versions.yaml`).
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

### Submit a Gateway API conformance report

If the compatible Gateway API version has been bumped in this release, a conformance report should be reported upstream.
Currently the process is manual but we have automated running the conformance suite and report generation in CI on release tags.
Steps to submit a conformance report are:

1. Find the corresponding `Build and push a release` GitHub Action workflow run for this release from [here][6]
1. Download the `gateway-conformance-report` artifact from the workflow run and unzip
1. Check it for accuracy, and make a PR like [this one][7] to the `gateway-api` repo with the content of the report

### Do the Github release

Now you have a tag pushed to Github, go to the release tab on github, select the tag and paste in the release notes that were generated previously.

# RC Release process

## Overview

An release-candidate release is e.g. `v1.23.0-rc.1`.

A release-candidate requires:
- a branch off of `main` for the release-candidate
- YAML to be customized
- a release tag to be created
- a GitHub release with release notes
- public communication
- cleanup

## Process

### Set environment variables

Set environment variables for use in subsequent steps:

```bash
export CONTOUR_RELEASE_VERSION=v1.23.0-rc.1
export CONTOUR_PREVIOUS_VERSION=v1.22.1

export KUBERNETES_MIN_VERSION=1.23
export KUBERNETES_MAX_VERSION=1.25

export CONTOUR_UPSTREAM_REMOTE_NAME=upstream
```

### Create a branch for the release

Typically we will branch off of `main` to cut an RC of an upcoming release and add the release tag on that branch.
This process is easier than managing a `release-*` branch and having to cherry-pick changes over from `main` for additional commits.

1. Make sure `main` is up to date
1. Create a new local branch from `main` (e.g. `release-1.23.0-rc.1`)
1. Commit all changes, push the branch to `github.com/projectcontour/contour`.

### Generate the RC changelog

1. Ensure the RC branch you created above is clean and up to date.
1. Generate a changelog (the `prepare-release.go` script should detect the version is an RC and only generate a changelog):

    ```bash
    go run ./hack/release/prepare-release.go $CONTOUR_RELEASE_VERSION $KUBERNETES_MIN_VERSION $KUBERNETES_MAX_VERSION
    ```
1. Proofread the release notes and do any reordering, rewording, reformatting necessary.
1. Commit all changes, push the branch to `github.com/projectcontour/contour`.

### Update YAML and tag release

1. Check out the RC branch, ensure it's up to date, and ensure you have a clean working directory.

1. Update the deployment YAML and create a local tag:

```bash
./hack/release/make-release-tag.sh $CONTOUR_PREVIOUS_VERSION $CONTOUR_RELEASE_VERSION
```

1. Push the branch to `github.com/projectcontour/contour` (e.g. `git push upstream release-1.23.0-rc.1`).

1. Push the tag to `github.com/projectcontour/contour`:

```bash
git push ${CONTOUR_UPSTREAM_REMOTE_NAME} ${CONTOUR_RELEASE_VERSION}
```

### Do the Github release

Now you have a tag pushed to Github, go to the release tab on github, select the tag and paste in the release notes that were generated previously.

# Release announcements

After any release, a few communications should be sent out to notify users.

- Post a note to the #contour channel on k8s slack, also update the /topic with the current release number
- Send an update to the [Project Contour mailing list][4]

# File issues

If you encountered any problems or areas for improvement while executing the release, file issues before you forget.

[1]: #minor-release-process
[2]: #patch-release-process
[3]: #rc-release-process
[4]: https://groups.google.com/g/project-contour
[6]: https://github.com/projectcontour/contour/actions/workflows/build_tag.yaml
[7]: https://github.com/kubernetes-sigs/gateway-api/pull/2884