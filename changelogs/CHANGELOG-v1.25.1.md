We are delighted to present version v1.25.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes
- Update to Envoy v1.26.4. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.26.4/version_history/v1.26/v1.26.4) for more information about the content of the release.
- Update to Go v1.20.6. See the [Go release notes](https://go.dev/doc/devel/release#go1.20.minor) for more information.
- Failure to automatically set GOMAXPROCS using the [automaxprocs](https://github.com/uber-go/automaxprocs) library is no longer fatal. Contour will now simply log the error and continue with the automatic GOMAXPROCS detection ignored.


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.25.1 is tested against Kubernetes 1.25 through 1.27.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
