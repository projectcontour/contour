# TLS Support

Contour 0.3 adds support for HTTPS (TLS/SSL) ingress via Envoy's SNI support.

## Enabling TLS Support

Enabling TLS support requires Contour 0.3 or later and a switch to the [YAML v2 bootstrap configuration][0].

Add an [entry for port 443][1] to your `contour` service object.

## Recovering the remote IP

If you are deploying behind an AWS Elastic Load Balancer you must ensure your `contour` service objects is configured with `service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp`.
Otherwise, the ELB will do HTTP termination on your HTTPS port.
The downside of this configuration is the remote IP of your incoming connections will be the inside address of the ELB.

To recover the remote IP, switch the ELB into PROXY mode:
1. Add `service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: "*"` to your `contour` service object.
2. Add the `--use-proxy-protocol` flag to your `contour` deployment or daemonset's container's flags.

[0]: upgrade.md
[1]: https://github.com/heptio/contour/blob/master/deployment/deployment-grpc-v2/03-service-tcp.yaml#L18
