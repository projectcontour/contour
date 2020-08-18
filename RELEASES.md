# Versioning and Release
This document describes the versioning and release process of Contour. This document is a living document, contents will be updated according to each release.

## Releases
Contour releases will be versioned using dotted triples, similar to [Semantic Version](http://semver.org/). For this specific document, we will refer to the respective components of this triple as `<major>.<minor>.<patch>`. The version number may have additional information, such as "-rc1,-rc2,-rc3" to mark release candidate builds for earlier access. Such releases will be considered as "pre-releases".

### Major and Minor Releases
Major and minor releases of Contour will be branched from main when the release reaches to `RC (release candidate)` state. The release cadence is around once a month, If for any reason this release cadence has to be adjusted (for example due to open source events), we will communicate it clearly on Slack, Twitter, and distribution lists. There is no mandated timeline for major versions and there are currently no criteria for shipping a new major version (i.e. Contour 2.0.0). You can find additional resources on the [release process](https://projectcontour.io/resources/release-process/) portion of our website.

### Patch releases
Patch releases are based on the major/minor release branch. There is no specific release cadence for patch releases as we already release monthly. However, we will create patch releases to address critical community and security issues (for example to address high severity security issues in Contour or in Envoy). We will patch release only the latest release of Contour. The Contour team only maintains a single release branch.

### Release Support Matrix
We support only the latest Contour [release](https://github.com/projectcontour/contour/releases). The can find the Contour Support Policy [here](https://projectcontour.io/resources/support/) 
* [Envoy Support Matrix](https://projectcontour.io/resources/envoy/)
* [Kubernetes Support Matrix](https://projectcontour.io/resources/kubernetes/)

### Upgrade path 
The upgrade path for Contour, including upgrade instructions, is documented [here](https://projectcontour.io/resources/upgrading/). Each Contour version also requires a specific release of Envoy, documented in the upgrading portion of our website.

### Next Release and Prioritized Backlog
The activity for the next release is tracked in the [up-to-date project board](https://github.com/orgs/projectcontour/projects/2). If your issue is not present the backlog, please reach out to the maintainers to add the issue to the project board. You may need to install the ZenHub browser plugin for this link to be visible.
