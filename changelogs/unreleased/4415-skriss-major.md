## Adds a `contour gateway-provisioner` command and deployment manifest for dynamically provisioning Gateways

Contour now has an optional Gateway provisioner, that watches for `Gateway` custom resources and provisions Contour + Envoy instances for them.
The provisioner is implemented as a new subcommand on the `contour` binary, `contour gateway-provisioner`.
The `examples/gateway-provisioner` directory contains the YAML manifests needed to run the provisioner as a Deployment in-cluster.

By default, the Gateway provisioner will process all `GatewayClasses` that have a controller string of `projectcontour.io/gateway-provisioner`, along with all Gateways for them.

The Gateway provisioner is useful for users who want to dynamically provision Contour + Envoy instances based on the `Gateway` CRD.
It is also necessary in order to have a fully conformant Gateway API implementation.
