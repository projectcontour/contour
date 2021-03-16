---
title: Contour Support Policy
layout: page
---

This document describes which versions of Contour are supported by the Contour team.

## Stable release

Only the latest stable release is supported.

The latest stable release is identified by the [Docker tag `:latest`][1].
`:latest` is an alias for {{ site.github.latest_release.tag_name }} which is the current stable release.

When required we may release a patch release to address security issues, serious problems with no suitable workaround, or documentation issues.
At that point the patch release will become the :latest stable release.

For example, prior to a patch release version Contour 1.0.0 was the `:latest` stable release.
If Contour 1.0.1 is release, the `:latest` tag will move to that version.

No support is offered for major, minor, or patch releases older than the `:latest` stable release.

### Additional Resources

- [Envoy Support Matrix][2]
- [Kubernetes Support Matrix][3]

[1]: {% link _resources/tagging.md %}
[2]: {% link _resources/envoy.md %}
[3]: {% link _resources/kubernetes.md %}