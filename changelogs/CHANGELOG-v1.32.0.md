We are delighted to present version v1.32.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Changes](#changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)


# Changes
- Improve performance in clusters with a large number of endpoints by using go-control-plane LinearCache for EDS. (#6906, @tsaarni)
- Updates kind node image for e2e tests to Kubernetes 1.33. Supported/tested Kubernetes versions are now 1.33, 1.32 and 1.31. (#7020, @sunjayBhatia)
- Updates Envoy to v1.34.1. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.34.1/version_history/v1.34/v1.34.1) for more information about the content of the release. (#7033, @tsaarni)
- Fix DiscoveryRequests sent by cli command when a specific set of resources is requested. Previously only the first request was sending the requested resource names. (#7047, @sunjayBhatia)
- Update to Go 1.24.3. See the [Go release notes](https://go.dev/doc/devel/release#go1.24.minor) for more information. (#7051, @sunjayBhatia)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.32.0 is tested against Kubernetes 1.31 through 1.33.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
