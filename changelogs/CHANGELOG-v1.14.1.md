We are delighted to present version 1.14.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

# Fixes

Upgrades the default Envoy version to 1.17.2 for security and bug fixes. See the [Envoy 1.17.2 changelogs](https://www.envoyproxy.io/docs/envoy/v1.17.2/version_history/current) for more details.

- CVE-2021-28682 (CVSS score 7.5, High): Envoy through 1.17.1, 1.16.2, 1.15.3, and 1.14.6 contains a remotely exploitable integer overflow via a very large grpc-timeout value causes undefined behavior.
- CVE-2021-28683 (CVSS score 7.5, High): Envoy through 1.17.1 and 1.16.2 contains a remotely exploitable crash in TLS when an unknown TLS alert code is received.
- CVE-2021-29258 (CVSS score 7.5, High): Envoy through 1.17.1, 1.16.2, 1.15.3, and 1.14.6 contains a remotely exploitable crash in Envoy's HTTP2 Metadata, when an empty METADATA map is sent.