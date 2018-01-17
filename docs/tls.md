# TLS support

Contour 0.3 adds support for HTTPS (TLS/SSL) ingress by integrating Envoy's SNI support.

## Enabling TLS support

Enabling TLS support requires Contour version 0.3 or later. You must use the [YAML v2 bootstrap configuration][0].

You must also add an [entry for port 443][1] to your `contour` service object.

## Recovering the remote IP address with an ELB

If you deploy behind an AWS Elastic Load Balancer, make sure that your `contour` Service object is configured with `service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp`.
Otherwise, the ELB will do HTTP termination on your HTTPS port.
The downside of this configuration is the remote IP of your incoming connections will be the inside address of the ELB.

To recover the remote IP, switch the ELB into PROXY mode:
1. Add `service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: "*"` to your `contour` service object.
2. Add the `--use-proxy-protocol` flag to the flags for your `contour` Deployment or Daemonset.

[0]: upgrade.md
[1]: https://github.com/heptio/contour/blob/master/deployment/deployment-grpc-v2/03-service-tcp.yaml#L18
