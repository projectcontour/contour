We are delighted to present version v1.21.2 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# Minor Changes

## Bump Envoy to v1.22.6

Bumps Envoy to security patch version 1.22.6.
See Envoy release notes [here](https://www.envoyproxy.io/docs/envoy/v1.22.6/version_history/current).

(#4889, @sunjayBhatia)

# Other Changes
- Various updates to dependencies for security updates, fixes for CI, upgrade to Go 1.18.8, and bump go module version to go 1.17. (#4885, @sunjayBhatia)

# Installing and Upgrading
For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Compatible Kubernetes Versions

Contour v1.21.2 is tested against Kubernetes 1.21 through 1.23.

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
