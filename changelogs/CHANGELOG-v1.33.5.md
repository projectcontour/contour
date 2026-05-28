We are delighted to present version v1.33.5 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

## Security fix for [GHSA-g3xr-5w5j-w4q4](https://github.com/projectcontour/contour/security/advisories/GHSA-g3xr-5w5j-w4q4)

Fixes a bug where configuring fallback certificate with JWT verification in `HTTPProxy` allowed requests without TLS SNI or with unrecognized SNI to bypass JWT verification. Contour now rejects this invalid configuration.

## Other Changes

- Bumps Go to 1.25.10.
- Bumps golang.org/x/net to v0.55.0.

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Compatible Kubernetes Versions

Contour v1.33.5 is tested against Kubernetes 1.32 through 1.34.

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
