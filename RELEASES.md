# Versioning and Release
This document describes the versioning and release process of Contour. This document is a living document, contents will be updated according to each release.

## Releases
Contour releases will be versioned using dotted triples, similar to [Semantic Version](http://semver.org/). The project refers to the respective components of this triple as `<major>.<minor>.<patch>`. The version number may have additional information, such as "-rc1,-rc2,-rc3" to mark release candidate builds for earlier access. Such releases will be considered as "pre-releases".

### Major and Minor Releases
Major and minor releases of Contour will be branched from main when the release reaches to `RC (release candidate)` state. The release cadence is currently once a month, but is migrating to quarterly as of the October 2021 release (which will be Contour 1.20).

If for any reason this release cadence has to be adjusted (for example due to open source events), the project will communicate it clearly on Slack, Twitter, and distribution lists. There is no mandated timeline for major versions and there are currently no criteria for shipping a new major version (i.e. Contour 2.0.0). You can find additional resources on the [release process](https://projectcontour.io/resources/release-process/) portion of our website.

### Patch releases
Patch releases are based on the major/minor release branch. There is no specific release cadence for patch releases. However, the project will create patch releases to address critical community and security issues (for example to address high severity security issues in Contour or in Envoy).The project will issue patch releases for all supported versions of Contour.

### Release Support Matrix
Per the [Contour support policy](https://projectcontour.io/resources/support/), the project is in the process of transitioning to supporting three Contour releases. Please see the support policy page to see what versions are currently supported.

Also, please consult the [Contour Compatibility Matrix](https://projectcontour.io/resources/compatibility-matrix/) for details of what each version of Contour requires for each of its dependencies like Envoy, Kubernetes, and so on.

Both of these details are also available in a machine-consumable (YAML) format via [versions.yaml](https://github.com/projectcontour/contour/blob/main/versions.yaml).


### Upgrade path 
The upgrade path for Contour, including upgrade instructions, is documented [here](https://projectcontour.io/resources/upgrading/). Each Contour version also requires a specific release of Envoy, documented in the upgrading portion of our website.

### Next Release and Prioritized Backlog
The activity for the next release is tracked in the [up-to-date project board](https://github.com/orgs/projectcontour/projects/2). If your issue is not present the backlog, please reach out to the maintainers to add the issue to the project board.
