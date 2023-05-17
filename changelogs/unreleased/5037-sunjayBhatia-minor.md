## Contour now outputs metrics about status update load

Metrics on status update counts and duration are now output by the xDS server.
This should enable deployments at scale to diagnose delays in status updates and possibly tune the `--kubernetes-client-qps` and `--kubernetes-client-burst` flags.
