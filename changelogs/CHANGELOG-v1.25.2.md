We are delighted to present version v1.25.2 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes
- Bumps [client-go](https://github.com/kubernetes/client-go) to v1.26.7. This ensures better compatibility with Kubernetes v1.27 clusters. See [this upstream issue](https://github.com/kubernetes/kubernetes/issues/118361) for more context on why this change is required. Many thanks to @chrism417 for bringing this to our attention.


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.25.2 is tested against Kubernetes 1.25 through 1.27.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
