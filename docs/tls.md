# TLS support

Contour 0.3 adds support for HTTPS (TLS/SSL) ingress by integrating Envoy's SNI support.

## Enabling TLS support

Enabling TLS support requires Contour version 0.3 or later. You must also add an [entry for port 443][1] to your `contour` service object.

## Configuring TLS with Contour on an ELB

If you deploy behind an AWS Elastic Load Balancer, see [EC2 ELB PROXY protocol support](proxy-proto.md) for special instructions.

[1]: https://github.com/heptio/contour/blob/master/examples/common/service.yaml#L20-L23
