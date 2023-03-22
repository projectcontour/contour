---
cascade:
  layout: docs
  version: main
  branch: main
---

## Overview
Contour is an Ingress controller for Kubernetes that works by deploying the [Envoy proxy][1] as a reverse proxy and load balancer.
Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

## Philosophy
- Follow an opinionated approach which allows us to better serve most users
- Design Contour to serve both the cluster administrator and the application developer
- Use our experience with ingress to define reasonable defaults for both cluster administrators and application developers.
- Meet users where they are by understanding and adapting Contour to their use cases

See the full [Contour Philosophy][8] page.

## Why Contour?
Contour bridges other solution gaps in several ways:
- Dynamically update the ingress configuration with minimal dropped connections
- Safely support multiple types of ingress config in multi-team Kubernetes clusters
  - [Ingress/v1][10]
  - [HTTPProxy (Contour custom resource)][2]
  - [Gateway API][9]
- Cleanly integrate with the Kubernetes object model

## Prerequisites
Contour is tested with Kubernetes clusters running version [1.21 and later][4].

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
[8]: /resources/philosophy
[9]: guides/gateway-api
[10]: /docs/{{< param version >}}/config/ingress
