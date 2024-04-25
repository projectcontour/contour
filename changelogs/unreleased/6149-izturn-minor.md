
## Use EndpointSlices by default

Contour now uses the Kubernetes EndpointSlices API by default to determine the endpoints to configure Envoy, instead of the Endpoints API.
Note: if you need to continue using the Endpoints API, you can disable the feature flag via `featureFlags: ["useEndpointSlices=false"]` in the Contour config file or ContourConfiguration CRD.
