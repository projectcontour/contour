We are delighted to present version v1.33.8 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

## Security fix for [GHSA-899w-6852-7fr3](https://github.com/projectcontour/contour/security/advisories/GHSA-899w-6852-7fr3)

Crash loop when an Ingress has a resource backend.

## Security fix for [GHSA-9x3c-68m7-f3x4](https://github.com/projectcontour/contour/security/advisories/GHSA-9x3c-68m7-f3x4)

HTTPProxy JWKS settings can be replaced by another HTTPProxy that uses the same JWKS host.

## Security fix for [GHSA-g6qh-g3pq-5cpm](https://github.com/projectcontour/contour/security/advisories/GHSA-g6qh-g3pq-5cpm)

Included HTTPProxy can receive requests outside its prefix by using regex.

## Other Changes

- Updates Envoy to v1.38.9. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.38.9/version_history/v1.38/v1.38.9) for more information about the content of the release.
- Bumps Go to 1.26.10.
- Updated dependencies to fix CVEs.

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Compatible Kubernetes Versions

Contour v1.33.8 is tested against Kubernetes 1.35 through 1.37.

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
