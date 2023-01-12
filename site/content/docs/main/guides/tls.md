---
title: TLS support
layout: page
---

# TLS support

Contour supports HTTPS (TLS/SSL) ingress by integrating Envoy's SNI support.
Certificates must be provisioned which are saved as Kubernetes secrets and get passed to Envoy.
A common way to implement this is to use [JetStack's Cert Manager][3].

## Enabling TLS support

Enabling TLS support requires Contour version 0.3 or later. You must also add an [entry for port 443][1] to your `contour` service object.

## Configuring TLS with Contour on an ELB

If you deploy behind an AWS Elastic Load Balancer, see [EC2 ELB PROXY protocol support][2] for special instructions.

## TLS SNI name matching
Envoy SNI name matching during TLS handshake is case-sensitive.
For example, for a cert with common name foo.bar.com, requests to Foo.bar.com would not match.
Similarly, for cert with wildcard name \*.bar.com, only requests to lower case name will match.
Here is the [known issue][4] reported on Envoy.

[1]: {{< param github_url >}}/tree/{{< param latest_version >}}/examples/contour/03-contour.yaml/#L45
[2]: /guides/proxy-proto
[3]: /guides/cert-manager
[4]: https://github.com/envoyproxy/envoy/issues/6199
