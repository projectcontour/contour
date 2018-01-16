# Upgrading to version 2 bootstrap configuration

To support features such as streaming gRPC configuration and TLS, Contour has switched from Envoy's v1 JSON configuration format to the v2 YAML configuration format.

This document describes the change to your Contour 0.2 (or earlier) deployment or daemonset manifests.

### v1 JSON configuration will be removed on Contour 0.4

Contour 0.3 deprecates the JSON bootstrap configuration format.
JSON configuration will be removed in Contour 0.4.

## Switch from `config.json` to `config.yaml`

In the `envoy` and `envoy-initconfig` container and init container respectively, change `/config/contour.json` to `/config/contour.yaml`.
This will cause Contour to emit a YAML bootstrap configuration file using the version 2 syntax.

## Ensure `/config` is mounted in the `contour` container

Envoy has a spot in the API for passing certificate data inline in the gRPC response body, however this isn't [implemented yet][0].
In the mean time Contour writes certificate data to the shared `/config` volume and references that path in the Listener configuration streamed to Envoy.
To support this your should make sure the [`contour-config` volume mount][1] is present on the `contour` container.

[0]: https://github.com/envoyproxy/envoy/issues/1357
[1]: https://github.com/heptio/contour/blob/master/deployment/deployment-grpc-v2/02-contour.yaml#L36
