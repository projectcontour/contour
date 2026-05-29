---
title: Contour Support Policy
layout: page
---

This document describes which versions of Contour are supported by the Contour team.

## Supported releases

Contour supports a single release track at a time. New releases are made on-demand rather than following a fixed schedule.

When a new release is published, the previous release is no longer supported. For example, when Contour 1.34 is released, Contour 1.33 will no longer be supported.

## What does a release being "supported" mean?

In short, "supported" means that Contour will issue fixes for security and other critical bugs for that release's supported lifetime.

However, the project will require users to upgrade to the most recent patch release for a version to be supported.
So, if Contour 1.20.0 is the only supported version, and Contour 1.20.1 is released, then the supported version will change to 1.20.1.

## Latest version and the `:latest` tag
The latest stable release is identified by the [Docker tag `:latest`][1].
`:latest` is an alias for {{< param latest_version >}} which is the current stable release.

`:latest` is always guaranteed to point to the highest available `:<major>.<minor>.<patch>` release.
When a new `:<major>.<minor>` release track is out the `:latest` tag will move along.

For example, prior to a patch release version Contour 1.20.0 was the `:latest` stable release.
If Contour 1.20.1 is released, the `:latest` tag will move to that version.

### Additional Resources

- [Contour Compatibility Matrix][2]

[1]: {{< ref "resources/tagging.md" >}}
[2]: {{< ref "resources/compatibility-matrix.md" >}}
