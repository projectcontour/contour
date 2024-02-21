# Contour

![GitHub release](https://img.shields.io/github/release/projectcontour/contour.svg) [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![Slack](https://img.shields.io/badge/slack-join%20chat-e01563.svg?logo=slack)](https://kubernetes.slack.com/messages/contour)

![Build and Test Pull Request](https://github.com/projectcontour/contour/workflows/Build%20and%20Test%20Pull%20Request/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/projectcontour/contour)](https://goreportcard.com/report/github.com/projectcontour/contour) [![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/projectcontour/contour/badge)](https://securityscorecards.dev/viewer/?uri=github.com/projectcontour/contour) [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4141/badge)](https://bestpractices.coreinfrastructure.org/projects/4141)


![Contour is fun at parties!](contour.png)

## Overview

Contour is an [ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) for Kubernetes that works by deploying the [Envoy proxy](https://www.envoyproxy.io/) as a reverse proxy and load balancer.
Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

Contour supports multiple configuration APIs in order to meet the needs of as many users as possible:

- **[Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/)** - A stable upstream API that enables basic ingress use cases.
- **[HTTPProxy](https://projectcontour.io/docs/main/config/fundamentals/)** - Contour's Custom Resource Definition (CRD) which expands upon the functionality of the Ingress API to allow for a richer user experience as well as solve shortcomings in the original design.
- **[Gateway API](https://gateway-api.sigs.k8s.io/)** - A new CRD-based API managed by the [Kubernetes SIG-Network community](https://github.com/kubernetes/community/tree/master/sig-network) that aims to evolve Kubernetes service networking APIs in a vendor-neutral way.

## Prerequisites

See the [compatibility matrix](https://projectcontour.io/resources/compatibility-matrix/) for the Kubernetes versions Contour is supported with.

RBAC must be enabled on your cluster.

## Get started

Getting started with Contour is as simple as one command.
See the [Getting Started](https://projectcontour.io/getting-started) document.

## Troubleshooting

If you encounter issues, review the Troubleshooting section of [the docs](https://projectcontour.io/docs), [file an issue](https://github.com/projectcontour/contour/issue), or talk to us on the [#contour channel](https://kubernetes.slack.com/messages/contour) on the Kubernetes Slack server.

## Contributing

Thanks for taking the time to join our community and start contributing!

- Please familiarize yourself with the [Code of Conduct](/CODE_OF_CONDUCT.md) before contributing.
- See [CONTRIBUTING.md](/CONTRIBUTING.md) for information about setting up your environment, the workflow that we expect, and instructions on the developer certificate of origin that we require.
- Check out the [open issues](https://github.com/projectcontour/contour/issues).
- Join our Kubernetes Slack channel: [#contour](https://kubernetes.slack.com/messages/contour/)
- Join the **Contour Community Meetings** - [schedule, notes, and recordings can be found here](https://projectcontour.io/community)
- Find GOVERNANCE in our [Community repo](https://github.com/projectcontour/community)
## Roadmap

See [Contour's roadmap](https://github.com/projectcontour/community/blob/main/ROADMAP.md) to learn more about where we are headed.

## Security

### Security Audit

A third party security audit was performed by Cure53 in December of 2020. You can see the full report [here](Contour_Security_Audit_Dec2020.pdf).

### Reporting security vulnerabilities

If you've found a security related issue, a vulnerability, or a potential vulnerability in Contour please let the [Contour Security Team](mailto:cncf-contour-maintainers@lists.cncf.io) know with the details of the vulnerability. We'll send a confirmation email to acknowledge your report, and we'll send an additional email when we've identified the issue positively or negatively.

For further details please see our [security policy](SECURITY.md).

## Changelog

See [the list of releases](https://github.com/projectcontour/contour/releases) to find out about feature changes.
