We are delighted to present version v1.24.4 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes
- Fix for bug in HTTPProxy duplicate include detection that caused memory usage spikes when root HTTPProxies with a large number of includes using header match conditions are present.
- Update to Envoy v1.25.6. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.25.6/version_history/v1.25/v1.25.6) for more information about the content of the release.


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.24.4 is tested against Kubernetes 1.24 through 1.26.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
