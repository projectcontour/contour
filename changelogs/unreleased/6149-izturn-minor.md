
## Use EndpointSlices by default

Enables contour to consume the kubernetes endpointslice API to determine the endpoints to configure Envoy with by default.
Note: if you need to continue using the old configuration, please set the flag: `useEndpointSlices=false` explicitly.
