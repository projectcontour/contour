---
title: Contour Tagging Policy
layout: page
---

This document describes Contour's image tagging policy.

## Released versions

`ghcr.io/projectcontour/contour:<SemVer>`

Contour follows the [Semantic Versioning][1] standard for releases.
Each tag in the github.com/projectcontour/contour repository has a matching image. eg. `ghcr.io/projectcontour/contour:{{< param latest_version >}}`

`ghcr.io/projectcontour/contour:v<major>.<minor>`

This tag will point to the latest available patch of the release train mentioned.
That is, it's `:latest` where you're guaranteed to not have a minor version bump.

### Latest

`ghcr.io/projectcontour/contour:latest`

The `latest` tag follows the most recent stable version of Contour.

## Development

`ghcr.io/projectcontour/contour:main`

The `main` tag follows the latest commit to land on the `main` branch.

[1]: http://semver.org/
