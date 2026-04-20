We are delighted to present version v1.31.6 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

## Security fix for CVE-2026-41246

This release fixes [CVE-2026-41246](https://github.com/projectcontour/contour/security/advisories/GHSA-x4mj-7f9g-29h4), a Lua code injection vulnerability in Contour's [Cookie Rewriting](https://projectcontour.io/docs/1.31/config/cookie-rewriting/) feature.

An attacker with RBAC permissions to create or modify HTTPProxy resources could craft a malicious `cookieRewritePolicies[].pathRewrite.value` that results in arbitrary code execution in the Envoy proxy. Since Envoy runs as shared infrastructure, the injected code could read Envoy's xDS client credentials from the filesystem or cause denial of service for other tenants sharing the Envoy instance.

The fix escapes user-provided values before interpolation into Lua code.

## Other Changes

- Bumps to Envoy [v1.34.14](https://github.com/envoyproxy/envoy/releases/tag/v1.34.14).

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Compatible Kubernetes Versions

Contour v1.31.6 is tested against Kubernetes 1.30 through 1.32.

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
