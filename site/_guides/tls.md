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

[1]: {{site.github.repository_url}}/blob/{{site.github.latest_release.tag_name}}/examples/contour/03-contour.yaml/#L45
[2]: {% link _guides/proxy-proto.md %}
[3]: {% link _guides/cert-manager.md %}