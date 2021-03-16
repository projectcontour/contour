---
title: Contour Tagging Policy
layout: page
---

This document describes Contour's image tagging policy.

## Released versions

`docker.io/projectcontour/contour:<SemVer>`

Contour follows the [Semantic Versioning][1] standard for releases.
Each tag in the github.com/projectcontour/contour repository has a matching image. eg. `docker.io/projectcontour/contour:{{ site.latest }}`

### Latest

`docker.io/projectcontour/contour:latest`

The `latest` tag follows the most recent stable version of Contour.

## Development

`docker.io/projectcontour/contour:main`

The `main` tag follows the latest commit to land on the `main` branch.

[1]: http://semver.org/
