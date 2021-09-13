---
title: Contour Support Policy
layout: page
---

This document describes which versions of Contour are supported by the Contour team.

## Supported releases

Contour is in the process of changing both to quarterly releases and three supported releases.

The first Contour version covered by the quarterly release cadence will be Contour v1.20, scheduled for late October 2021.

At the time it is released, it will be the only supported version, and versions 1.21 and 1.22 will continue supporting back to Contour 1.20.

When Contour 1.23 releases (nine months later), Contour 1.20 will fall out of support.

The following table illustrates how this will work. The given dates are estimates, not guarantees.
They are our best guess as to when each version will be released.

| Version |v1.19 |v1.20|v1.21|v1.22|v1.23|
|---------|--------|-------|-------|-------|-------|
|Q3 2021 (September 2021) | :heavy_check_mark: |
|Q4 2021 (October 2021) | :negative_squared_cross_mark: | :heavy_check_mark: |
|Q1 2022 (January 2021) | :negative_squared_cross_mark: | :heavy_check_mark: |:heavy_check_mark: |
|Q2 2022 (April 2021) | :negative_squared_cross_mark: | :heavy_check_mark: |:heavy_check_mark: |:heavy_check_mark: |
|Q3 2022 (July 2021) | :negative_squared_cross_mark: | :negative_squared_cross_mark: |:heavy_check_mark: |:heavy_check_mark: | :heavy_check_mark: |

## What does a release being "supported" mean?

In short, "supported" means that we will issue fixes for security and other critical bugs for that release's supported lifetime.

However, we will require users to upgrade to the most recent patch release for a version to be supported.

That is:
- The latest patch version in each release is the supported version.
- If you are not running the supported version from your release train, we will ask you to upgrade first if you have any problems.
- When a new patch is cut, that will become the supported version for that release.

So, if Contour 1.20.0 is the only supported version, and we release Contour 1.20.1, then the supported version will change to 1.20.1.

If, later on, the supported versions are 1.21, 1.22, and 1.23, and we release 1.21.1, 1.22.1, and 1.23.1, then the patch releases will be the only supported versions.

## Latest version and the `:latest` tag
The latest stable release is identified by the [Docker tag `:latest`][1].
`:latest` is an alias for {{< param latest_version >}} which is the current stable release.

When required we may release a patch release to address security issues, serious problems with no suitable workaround, or documentation issues.
At that point the patch release will become the :latest stable release.

For example, prior to a patch release version Contour 1.20.0 was the `:latest` stable release.
If Contour 1.20.1 is released, the `:latest` tag will move to that version.

### Additional Resources

- [Contour Compatibility Matrix][2]

[1]: {{< ref "resources/tagging.md" >}}
[2]: {{< ref "resources/compatibility-matrix.md" >}}
