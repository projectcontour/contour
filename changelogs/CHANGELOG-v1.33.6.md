We are delighted to present version v1.33.6 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

## Security fix for [GHSA-cf57-xf33-fg5h](https://github.com/projectcontour/contour/security/advisories/GHSA-cf57-xf33-fg5h)

External authorization bypass when virtualhost authPolicy is disabled.

## Other Changes

- Updates Envoy to v1.38.3. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.38.3/version_history/v1.38/v1.38.3) for more information about the content of the release. Note: This bumps the Envoy minor version from 1.35 to 1.38 as Envoy 1.35 has reached end-of-life.
- Bumps Go to 1.25.12.
- Updated dependencies to fix CVEs.

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Compatible Kubernetes Versions

Contour v1.33.6 is tested against Kubernetes 1.32 through 1.34.

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
