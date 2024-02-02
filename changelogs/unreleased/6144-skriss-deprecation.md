## Configuring Contour with a GatewayClass controller name is deprecated

Contour should no longer be configured with a GatewayClass controller name (`gateway.controllerName` in the config file or ContourConfiguration CRD).
Instead, either use a specific Gateway reference (`gateway.gatewayRef`), or use the Gateway provisioner.
`gateway.controllerName` will be removed in a future release.