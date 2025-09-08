We are delighted to present version v1.33.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)


# Minor Changes

## Distroless Envoy image

The Envoy image used in the example manifests and as the default image in the Gateway Provisioner has been switched to the [distroless](https://www.envoyproxy.io/docs/envoy/latest/start/install#image-variants) variant.

Previously, it was based on Ubuntu and included a minimal OS with a package manager.
The distroless variant contains only the files required to run Envoy, improving security.

(#7170, @tsaarni)


## Update to Gateway API v1.3.0

Gateway API CRD compatibility has been updated to release v1.3.0.

Full release notes for Gateway API v1.3.0 can be found [here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.3.0).


# Other Changes
- Update to Go 1.25.0. See the [Go release notes](https://go.dev/doc/go1.25) for more information. (#7168, @sunjayBhatia)
- Updates Envoy to v1.35.2. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.35.2/version_history/v1.35/v1.35) for more information about the content of the release. (#7197, @sunjayBhatia)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.33.0 is tested against Kubernetes 1.32 through 1.34.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:



# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
