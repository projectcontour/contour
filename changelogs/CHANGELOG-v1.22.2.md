We are delighted to present version v1.22.2 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# Minor Changes

## Bump Envoy to v1.23.3

Bumps Envoy to security patch version 1.23.3.
See Envoy release notes [here](https://www.envoyproxy.io/docs/envoy/v1.23.3/version_history/v1.23/v1.23.3).

(#4897, @sunjayBhatia)

# Other Changes
- Various updates to dependencies for security updates, upgrade to Go 1.19.3, and bump go module version to go 1.17. (#4882, #4884, @sunjayBhatia)

# Installing and Upgrading
For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Compatible Kubernetes Versions

Contour v1.22.2 is tested against Kubernetes 1.22 through 1.24.

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
