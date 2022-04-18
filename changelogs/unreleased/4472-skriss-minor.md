## ContourDeployment CRD now supports additional options

The `ContourDeployment` CRD, which can be used as parameters for a Contour-controlled `GatewayClass`, now supports additional options for customizing your Contour/Envoy installations:

- Contour deployment replica count
- Contour deployment node placement settings (node selectors and/or tolerations)
- Envoy workload type (daemonset or deployment)
- Envoy replica count (if using a deployment)
- Envoy service type and annotations
- Envoy node placement settings (node selectors and/or tolerations)
