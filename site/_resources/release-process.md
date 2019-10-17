---
title: Release process
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
$ git tag -a v0.15.0-alpha.1 -m 'contour 0.15.0 alpha 1'
$ git push --tags
```

Once the tag is present on master, Github Actions will build the tag and push it to Docker Hub for you.
Then, you are done until we are ready to branch for an rc or final release.

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
$ git tag -a v0.15.0 -m 'contour 0.15.0'
$ git push --tags
```

## Patch release

### Cherry-pick required commits into the release branch

Get any required changes into the release branch by whatever means you choose.

### Tag patch release from release branch

Tag the head of your release branch with the release tag, and push

```sh
$ git tag -a v0.15.1 -m 'contour 0.15.1'
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
$ env LATEST_VERSION=v1.0.0 make tag-latest
docker pull projectcontour/contour:v1.0.0
v1.0.0: Pulling from projectcontour/contour
Digest: sha256:7af8d77b3fcdbebec31abd1059aedacc119b3561b933976402c87f31a309ec53
Status: Image is up to date for projectcontour/contour:v1.0.0
docker.io/projectcontour/contour:v1.0.0
docker tag projectcontour/contour:v1.0.0 projectcontour/contour:latest
docker push projectcontour/contour:latest
The push refers to repository [docker.io/projectcontour/contour]
43ef43ac3a59: Layer already exists
latest: digest: sha256:7af8d77b3fcdbebec31abd1059aedacc119b3561b933976402c87f31a309ec53 size: 527
```

## Finishing up

If you've made a production release (that is, a final release or a patch release), you have a couple of things left to do.

### Updating quickstart URL

The quickstart url, https://projectcontour.io/quickstart/contour.yaml redirects to the current stable release.
This is controlled by a line in `site/_redirects`. If the definition of `:latest` has changed, update the quickstart redirector to match.

### Do the Github release and write release notes

Now you have a tag pushed to Github, go to the release tab on github, select the tag and write up your release notes. For patch releases, include the previous release notes below the new ones.

### Toot your horn

- Post a note to the #contour channel on k8s slack, also update the /topic with the current release number
- Post a note to the #project-contour channel on the vmware slack, also update the /topic with the current release number
- Send an email to the project-contour mailing list
