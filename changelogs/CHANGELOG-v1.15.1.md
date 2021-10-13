We are delighted to present version 1.15.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

# Fixes

Upgrades the default Envoy version to 1.18.3 for security and bug fixes. See the [Envoy 1.18.3 changelogs](https://www.envoyproxy.io/docs/envoy/v1.18.3/version_history/current) for more details.

- [CVE-2021-29492](https://github.com/envoyproxy/envoy/security/advisories/GHSA-4987-27fx-x6cf) (CVSS score 8.3, High): Envoy versions 1.18.2 and earlier does not decode escaped slash sequences %2F and %5C in HTTP URL paths. A remote attacker may craft a path with escaped slashes, e.g. /something%2F..%2Fadmin, to bypass access control, e.g. a block on /admin. A backend server could then decode slash sequences and normalize path which would provide an attacker access beyond the scope provided for by the access control policy.