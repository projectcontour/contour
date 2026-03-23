We are delighted to present version v1.32.4 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

- Bumps to Envoy [v1.34.13](https://github.com/envoyproxy/envoy/releases/tag/v1.34.13) to address security vulnerabilities and improve stability.
- Updates `google.golang.org/grpc` to [v1.79.3](https://github.com/grpc/grpc-go/releases/tag/v1.79.3), which addresses [CVE-2026-33186](https://github.com/grpc/grpc-go/security/advisories/GHSA-p77j-4mvh-x3m3) (Contour is not affected).
- Removes Envoy metrics `hostPort: 8002` from example manifests. (#7476)

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Compatible Kubernetes Versions

Contour v1.32.4 is tested against Kubernetes 1.31 through 1.33.

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
