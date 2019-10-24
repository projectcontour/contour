---
title: Supported version policy
layout: page
---

This document describes which versions of Contour are supported by the Contour team.

## Stable release

Only the latest stable release is supported.
The latest stable release is identified by the [Docker tag `:latest`]({% link _resources/tagging.md %}).

When required we may release a patch release to address security issues, serious problems with no suitable workaround, or documentation issues.
At that point the patch release will become the :latest stable release.

For example, prior to a patch release version Contour 1.0.0 was the `:latest` stable release.
If Contour 1.0.1 is release, the `:latest` tag will move to that version.

No support is offered for major, minor, or patch releases older than the `:latest` stable release.
