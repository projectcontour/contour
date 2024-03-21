We are delighted to present version v1.28.2 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

## Update Envoy to v1.29.2

See the release notes [here](https://www.envoyproxy.io/docs/envoy/v1.29.2/version_history/v1.29/v1.29.2).

Note that this Envoy version reverts the HTTP/2 codec back to `nghttp2` from `oghttp2`.

## Disable Envoy removing TE header

As of version v1.29.0, Envoy removes the hop-by-hop TE header.
However, this causes issues with HTTP/2, particularly gRPC, with implementations expecting the header to be present (and set to `trailers`).
Contour disables this via Envoy runtime setting and reverts to the v1.28.x and prior behavior of allowing the header to be proxied.

Once [this Envoy PR that enables the TE header including `trailers` to be forwarded](https://github.com/envoyproxy/envoy/pull/32255) is backported to a release or a new minor is cut, Contour will no longer set the aforementioned runtime key.

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.28.2 is tested against Kubernetes 1.27 through 1.29.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
