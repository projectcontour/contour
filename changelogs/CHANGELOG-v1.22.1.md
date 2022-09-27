We are delighted to present version v1.22.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [Changes](#changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Changes
- Updates Go to 1.19.0, see [release notes here](https://go.dev/doc/go1.19). (#4660, @sunjayBhatia)
- The global connect-timeout configuration value was not taking effect for routes that did not have timeoutPolicy set. (#4690, @tsaarni)
- Update Envoy to v1.23.1. This fixes an issue where the arm64 variant of the Envoy image was not built properly. See the [release notes](https://www.envoyproxy.io/docs/envoy/v1.23.1/version_history/v1.23/v1.23.1) for additional information. (#4691, @skriss)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.22.1 is tested against Kubernetes 1.22 through 1.24.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
