## Overview
Contour is an Ingress controller for Kubernetes that works by deploying the [Envoy proxy][1] as a reverse proxy and load balancer.
Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

Contour also introduces a new ingress API [HTTPProxy][2] which is implemented via a Custom Resource Definition (CRD).
Its goal is to expand upon the functionality of the Ingress API to allow for a richer user experience as well as solve shortcomings in the original design.

## Prerequisites
Contour is tested with Kubernetes clusters running version [1.19 and later][4].

RBAC must be enabled on your cluster.

## Get started
Getting started with Contour is as simple as one command.
See the [Getting Started][3] document.

## Troubleshooting
If you encounter issues review the [troubleshooting][5] page, [file an issue][6], or talk to us on the [#contour channel][7] on Kubernetes slack.

[1]: https://www.envoyproxy.io/
[2]: config/fundamentals.md
[3]: /getting-started
[4]: /resources/compatibility-matrix.md
[5]: /docs/main/troubleshooting
[6]: https://github.com/projectcontour/contour/issues
[7]: https://kubernetes.slack.com/messages/contour