We are delighted to present version v1.21.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)


# Minor Changes

## Bump Envoy to v1.22.2

Bumps Envoy to security patch version 1.22.2.
Envoy CI had a few issues releasing 1.22.1 so a subsequent patch, 1.22.2 was released.
Envoy announcement [here](https://groups.google.com/g/envoy-announce/c/QxI6z6wdL7M).
See Envoy release notes [for 1.22.1 here](https://www.envoyproxy.io/docs/envoy/v1.22.2/version_history/v1.22.1) and [1.22.2 here](https://www.envoyproxy.io/docs/envoy/v1.22.2/version_history/current).

(#4573, @sunjayBhatia)


# Other Changes
- When validating secrets, don't log an error for an Opaque secret that doesn't contain a `ca.crt` key. (#4528, @skriss)
- Fixes TLS private key validation logic which previously ignored errors for PKCS1 and PKCS8 private keys. (#4544, @sunjayBhatia)
- Update gopkg.in/yaml.v3 to v3.0.1 to address CVE-2022-28948. (#4551, @tsaarni)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.21.1 is tested against Kubernetes 1.21 through 1.23.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better!


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
