We are delighted to present version v1.20.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Changes](#changes)
- [Docs Changes](#docs-changes)
- [Deprecations/Removals](#deprecation-and-removal-notices)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)


# Changes
- Fixed a bug where upstream TLS SNI (`HTTProxy.spec.routes.requestHeadersPolicy` `Host` key) and protocol fields might not take effect when e.g. two `HTTPProxies` were otherwise equal but differed only on those fields. (#4350, @tsaarni)
- Update github.com/prometheus/client_golang to v1.11.1 to address CVE-2022-21698. (#4361, @tsaarni)
- Updates Envoy to v1.21.1. See the [Envoy changelog](https://www.envoyproxy.io/docs/envoy/v1.21.1/version_history/current) for details. (#4365, @skriss)


# Docs Changes
- Added documentation for HTTPProxy request redirection. (#4367, @sunjayBhatia)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.20.1 is tested against Kubernetes 1.21 through 1.23.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
