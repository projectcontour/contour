## Configuring Contour with a GatewayClass controller name is no longer supported

Contour can no longer be configured with a GatewayClass controller name (gateway.controllerName in the config file or ContourConfiguration CRD), as the config field has been removed.
Instead, either use a specific Gateway reference (gateway.gatewayRef), or use the Gateway provisioner.
