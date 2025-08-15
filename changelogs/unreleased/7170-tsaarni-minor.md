## Distroless Envoy image

The Envoy image used in the example manifests and as the default image in the Gateway Provisioner has been switched to the [distroless](https://www.envoyproxy.io/docs/envoy/latest/start/install#image-variants) variant.

Previously, it was based on Ubuntu and included a minimal OS with a package manager.
The distroless variant contains only the files required to run Envoy, improving security.
