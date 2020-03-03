[![Build Status][1]][2] [![Go Report Card][3]][4] ![GitHub release][5] [![License][6]][7]

## Overview
Contour is an Ingress controller for Kubernetes that works by deploying the [Envoy proxy][8] as a reverse proxy and load balancer.
Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

Contour also introduces a new ingress API [HTTPProxy][9] which is implemented via a Custom Resource Definition (CRD).
Its goal is to expand upon the functionality of the Ingress API to allow for a richer user experience as well as solve shortcomings in the original design.

## Prerequisites
Contour is tested with Kubernetes clusters running version [1.15 and later][11], but should work with earlier versions where Custom Resource Definitions are supported (Kubernetes 1.7+).

RBAC must be enabled on your cluster.

## Get started
Getting started with Contour is as simple as one command.
See the [Getting Started][10] document.

[1]: https://travis-ci.org/projectcontour/contour.svg?branch={{page.version}}
[2]: https://travis-ci.org/projectcontour/contour
[3]: https://goreportcard.com/badge/github.com/projectcontour/contour
[4]: https://goreportcard.com/report/github.com/projectcontour/contour
[5]: https://img.shields.io/github/release/projectcontour/contour.svg
[6]: https://img.shields.io/badge/License-Apache%202.0-blue.svg
[7]: https://opensource.org/licenses/Apache-2.0
[8]: https://www.envoyproxy.io/
[9]: httpproxy.md
[10]: {% link getting-started.md %}
[11]: {% link _resources/kubernetes.md %}