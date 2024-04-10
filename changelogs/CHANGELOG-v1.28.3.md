We are delighted to present version v1.28.3 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

## Update Envoy to v1.29.3

See the release notes for v1.29.3 [here](https://www.envoyproxy.io/docs/envoy/v1.29.3/version_history/v1.29/v1.29.3).

Note that this Envoy version retains the hop-by-hop TE header when set to `trailers`, fixing a regression seen in v1.29.0-v1.29.2 for HTTP/2, particularly gRPC.
However, this version of Contour continues to set the `envoy.reloadable_features.sanitize_te` Envoy runtime setting to `false` to ensure seamless upgrades.
This runtime setting will be removed in Contour v1.29.0.

## Update Go to v1.21.9

See the release notes for v1.21.9 [here](https://go.dev/doc/devel/release#go1.21.minor).

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.28.3 is tested against Kubernetes 1.27 through 1.29.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
