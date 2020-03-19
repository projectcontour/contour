---
title: Contour Release Process
layout: page
---

This page documents the process for releasing a new version of Contour.

The release types are as follows. All are tagged from the same release branch.

- Alpha releases.
- Beta releases.
- RC (Release Candidate) releases.
- Final releases.
- Patch releases.

## Alpha and beta releases

The steps for an alpha or beta release are

- Tag the head of master with the relevant release tag (in this case `alpha.1`), and push

```sh
$ git tag -a v0.15.0-alpha.1 -m 'contour 1.2.0 alpha 1'
$ git push --tags
```

Once the tag is present on master, Github Actions will build the tag and push it to Docker Hub for you.
Then, you are done until we are ready to branch for an rc or final release.

## Update Docs Site

The documentation site (projectcontour.io) has versioned documentation. The following steps can be followed to update the site to create the new release:

- In: `site/_data`
  - Make new yml file for version (e.g. v1-1-0-toc.yaml)
  - Add yml file to site/_data/toc-mapping.yml
- Copy `master` directory to the new version (e.g. v1.1.0)
- In: `site/_config.yml`
  - Update `defaults` and add new version
  - Update `collections` to add new version
  - Update `versions` to add new version
  - Update `latest` to change previous version to new version

These changes can be automatically performed by the `hack/release/prepare-release.go` tool.
For example:

```sh
$ go run ./hack/release/prepare-release.go v9.9.9
```

## Branch for release

As contours master branch is under active development, rc and final releases are made from a branch.
Create a release branch locally, like so

```sh
$ git checkout -b release-0.15
```

If you are doing a patch release on an existing branch, skip this step and just checkout the branch instead.

This branch is used for all the almost-done release types, rc and final.
Each one of these release types is just a different tag on the release branch.

## Final release

### Updating the example YAMLs

Your final job before doing the last release is to ensure that all the YAMLs in `examples/contour/` are updated.
The Docker tag should be updated from the previous stable release to this new one.

### Tag release from release branch

Tag the head of your release branch with the release tag, and push

```sh
$ git tag -a v1.2.0 -m 'contour 1.2.0'
$ git push --tags
```

These two steps can be automatically performed by the `hack/release/make-release-tag.sh` tool.
For example:

```sh
$ ./hack/release/make-release-tag.sh v1.2.1 v1.3.1
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

## Updating the `:latest` tag

If you've cut a new production-ready release, you'll need to update `:latest` on Docker Hub as well.

Firstly, you'll need to be:

- logged in to Docker Hub
- A member of the projectcontour Docker Hub org with push rights.

Once that's all true, do the following steps:

```shell
$ docker login
Authenticating with existing credentials...
Login Succeeded
# The make target will do what you need
# If you don't set the version, it will be a noop
$ make tag-latest REGISTRY=docker.io/projectcontour LATEST_VERSION=v1.0.0
docker pull docker.io/projectcontour/contour:v1.0.0
v1.0.0: Pulling from docker.io/projectcontour/contour
Digest: sha256:7af8d77b3fcdbebec31abd1059aedacc119b3561b933976402c87f31a309ec53
Status: Image is up to date for projectcontour/contour:v1.0.0
docker.io/projectcontour/contour:v1.0.0
docker tag docker.io/projectcontour/contour:v1.0.0 docker.io/projectcontour/contour:latest
docker push docker.io/projectcontour/contour:latest
The push refers to repository [docker.io/projectcontour/contour]
43ef43ac3a59: Layer already exists
latest: digest: sha256:7af8d77b3fcdbebec31abd1059aedacc119b3561b933976402c87f31a309ec53 size: 527
```

## Finishing up

If you've made a production release (that is, a final release or a patch release), you have a couple of things left to do.

### Updating site details

The quickstart url, https://projectcontour.io/quickstart/contour.yaml redirects to the current stable release.
This is controlled by a line in `site/_redirects`. If the definition of `:latest` has changed, update the quickstart redirector to match.

You also need to set the variable `latest` in `site/_config.yml` to the released version for the site to work correctly.

### Do the Github release and write release notes

Now you have a tag pushed to Github, go to the release tab on github, select the tag and write up your release notes.
For patch releases, include the previous release notes below the new ones.

_Note: Filter on the Github label "release note" and Github milestone which should include any PRs which should be called out in the release notes._ 

### Toot your horn

- Post a note to the #contour channel on k8s slack, also update the /topic with the current release number
- Post a note to the #project-contour channel on the vmware slack, also update the /topic with the current release number
- Send an email to the project-contour mailing list
