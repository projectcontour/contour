# Upgrading to version 2 bootstrap configuration

To support features such as streaming gRPC configuration and TLS, Contour has moved from Envoy's v1 JSON configuration format to the v2 YAML configuration format.

This document describes the changes you must make to your Deployment or Daemonset manifest for Contour 0.2 (or earlier) to support the new format in Contour 0.3.

### v1 JSON configuration will be removed in Contour 0.4

Contour 0.3 deprecates the JSON bootstrap configuration format.
Support for JSON configuration will be removed in Contour 0.4.

## Change `config.json` to `config.yaml`

In the `envoy` and `envoy-initconfig` container and init container respectively, change `/config/contour.json` to `/config/contour.yaml`.
This causes Contour to emit a YAML bootstrap configuration file with the version 2 syntax.
