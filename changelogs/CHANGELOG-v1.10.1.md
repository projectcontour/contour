We are delighted to present version 1.10.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

## Fixes

- Upgrades the default Envoy version from 1.16.0 to 1.16.2 for security and bug fixes. See the Envoy [1.16.1](https://www.envoyproxy.io/docs/envoy/v1.16.2/version_history/v1.16.1) and [1.16.2](https://www.envoyproxy.io/docs/envoy/v1.16.2/version_history/current) changelogs for details.
- Fixes a concurrent map access issue which could lead to Contour crashing/restarting (#3199).
