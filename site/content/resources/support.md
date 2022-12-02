---
title: Contour Support Policy
layout: page
---

This document describes which versions of Contour are supported by the Contour team.

## Supported releases

Contour is changing both to quarterly releases and three supported releases.

The first Contour version covered by the quarterly release cadence is Contour v1.20, released in Jan 2022.

At the time it is released, it will be the only supported version, and versions 1.21 and 1.22 will continue supporting back to Contour 1.20.

When Contour 1.23 releases, Contour 1.20 will fall out of support.

The following table illustrates how this will work. The given dates are estimates, not guarantees.
They are our best guess as to when each version will be released.

| Version | v1.19 | v1.20 | v1.21 | v1.22 | v1.23 |
|---------|-------|-------|-------|-------|-------|
| Q4 2021 | ✅     |
| Q1 2022 | ❌     | ✅     |
| Q2 2022 | ❌     | ✅     | ✅     |
| Q3 2022 | ❌     | ✅     | ✅     | ✅     |
| Q4 2022 | ❌     | ❌     | ✅     | ✅     | ✅     |

## What does a release being "supported" mean?

In short, "supported" means that Contour will issue fixes for security and other critical bugs for that release's supported lifetime.

However, the project will require users to upgrade to the most recent patch release for a version to be supported.

That is:
- The latest patch version in each release is the supported version.
- If you are not running the supported version from your release train, you'll need to upgrade first if you have any problems.
- When a new patch is cut, that will become the supported version for that release.

So, if Contour 1.20.0 is the only supported version, and Contour 1.20.1 is released, then the supported version will change to 1.20.1.

If, later on, the supported versions are 1.21, 1.22, and 1.23, and 1.21.1, 1.22.1, and 1.23.1 are released, then the patch releases will be the only supported versions.

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
