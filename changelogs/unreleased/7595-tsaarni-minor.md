## Load balancer status source from Ingress objects

Contour can now watch an Ingress object's LoadBalancer status as the source for address propagation to `Ingress` and `HTTPProxy` status.
A new `--load-balancer-status` flag unifies address source configuration with support for `address:`, `service:`, and `ingress:` source kinds.
The same setting is available as `spec.envoy.loadBalancer` in the ContourConfiguration CRD or `load-balancer-status` in the config file.

(@hligit, @kahirokunn and @tsaarni)
