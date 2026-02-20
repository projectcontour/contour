We are delighted to present version v1.33.2 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

- Updates Go to v1.25.7. See the [Go release notes](https://go.dev/doc/devel/release#go1.25.minor) for more information about the content of the release.
- Fixes load balancer status update failures caused by `HTTPProxy` CRD schema incorrectly marking `status.loadBalancer.ingress[].ports[].error` as a required field. (#7408)
- Increases CPU limit for the `shutdown-manager` container from `50m` to `200m` when using the Contour Gateway Provisioner, to prevent CPU throttling. (#7382)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.33.2 is tested against Kubernetes 1.32 through 1.34.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
