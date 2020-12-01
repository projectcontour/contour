---
title: Contour Release Process
layout: page
---

This page documents the process for releasing a new version of Contour or [Contour Operator][1].

The release types are as follows. All are tagged from the same release branch.

- Alpha releases.
- Beta releases.
- RC (Release Candidate) releases.
- Final releases.
- Patch releases.

For minor releases, generally we will not do an Alpha/Beta/RC/Final flow, we will jump straight to cutting the final from main.

## Alpha and beta releases

The steps for an alpha or beta release are

- Tag the head of main with the relevant release tag (in this case `alpha.1`), and push

```sh
$ git tag -a v0.15.0-alpha.1 -m 'contour 1.2.0 alpha 1'
$ git push --tags
```

Once the tag is present on main, Github Actions will build the tag and push it to Docker Hub for you.
Then, you are done until we are ready to branch for an rc or final release.

## Update Docs Site

The documentation site (projectcontour.io) has versioned documentation.

The required changes can be automatically performed by the `hack/release/prepare-release.go` tool.
For example:

```sh
$ go run ./hack/release/prepare-release.go v9.9.9
```

This can be pushed straight back to main.

The tool performs the following steps, which can also be followed to update the site to create the new release:

- In: `site/_data`
  - Make new yml file for version (e.g. v1-1-0-toc.yaml)
  - Add yml file to site/_data/toc-mapping.yml
- Copy `main` directory to the new version (e.g. v1.1.0)
- In: `site/_config.yml`
  - Update `defaults` and add new version
  - Update `collections` to add new version
  - Update `versions` to add new version
  - Update `latest` to change previous version to new version

## Update compatibility matrices

If there has been an Envoy version upgrade, check that the [Envoy Support Matrix](https://projectcontour.io/resources/envoy/) has been updated.

If there has been a Kubernetes client-go upgrade, check that the [Kubernetes Support Matrix](https://projectcontour.io/resources/kubernetes/) has been updated.

## Upgrade instructions

Add upgrade instructions to the [Upgrading](https://projectcontour.io/resources/upgrading/) page.

## Branch for release

As contours main branch is under active development, rc and final releases are made from a branch.
Create a release branch locally, like so

```sh
$ git checkout -b release-1.7
```

Push the release branch to the upstream repository:

```sh
$ git push --set-upstream <YOUR UPSTREAM REMOTE NAME> release-1.7
```

If you are doing a patch release on an existing branch, skip this step and just checkout the branch instead.

This branch is used for all the almost-done release types, rc and final.
Each one of these release types is just a different tag on the release branch.

## Final release

These two steps can be automatically performed by the `hack/release/make-release-tag.sh` tool.
For example:

```sh
$ ./hack/release/make-release-tag.sh v1.2.1 v1.3.1
```

### Updating the example YAMLs

Your final job before doing the last release is to ensure that all the YAMLs in `examples/contour/` are updated.
The Docker tag should be updated from the previous stable release to this new one.

### Tag release from release branch

Tag the head of your release branch with the release tag, and push

```sh
$ git tag -a v1.2.0 -m 'contour 1.2.0'
$ git push --tags
```

## Patch release

### Cherry-pick required commits into the release branch

Get any required changes into the release branch by whatever means you choose.

### Tag patch release from release branch

Tag the head of your release branch with the release tag, and push

```sh
$ git tag -a v1.2.1 -m 'contour 1.2.1'
$ git push --tags
```

## Finishing up

If you've made a production release (that is, a final release or a patch release), you have a couple of things left to do.

### Updating site details

The quickstart url, https://projectcontour.io/quickstart/contour.yaml redirects to the current stable release.
This is controlled by the `[[redirects]]` section in `netlify.toml`. If the definition of `:latest` has changed, update the quickstart redirector to match.

### Do the Github release and write release notes

Now you have a tag pushed to Github, go to the release tab on github, select the tag and write up your release notes.
For patch releases, include the previous release notes below the new ones.

_Note: Filter on the Github label "release note" and Github milestone which should include any PRs which should be called out in the release notes._ 

### Toot your horn

- Post a note to the #contour channel on k8s slack, also update the /topic with the current release number
- Post a note to the #project-contour channel on the vmware slack, also update the /topic with the current release number
- Send an update to the [cncf-contour-users mailing list](https://lists.cncf.io/g/cncf-contour-users/)

[1]: https://github.com/projectcontour/contour-operator
