## Overview
Contour is an Ingress controller for Kubernetes that works by deploying the [Envoy proxy][1] as a reverse proxy and load balancer.
Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

Contour also introduces a new ingress API [HTTPProxy][2] which is implemented via a Custom Resource Definition (CRD).
Its goal is to expand upon the functionality of the Ingress API to allow for a richer user experience as well as solve shortcomings in the original design.

## Prerequisites
Contour is tested with Kubernetes clusters running version [1.15 and later][4], but should work with earlier versions where Custom Resource Definitions are supported (Kubernetes 1.7+).

RBAC must be enabled on your cluster.

## Get started
Getting started with Contour is as simple as one command.
See the [Getting Started][3] document.

[1]: https://www.envoyproxy.io/
[2]: httpproxy.md
[3]: {% link getting-started.md %}
[4]: {% link _resources/kubernetes.md %}