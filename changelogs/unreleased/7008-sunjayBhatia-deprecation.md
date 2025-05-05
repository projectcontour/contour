## useEndpointSlices feature flag removed

As of v1.29.0, Contour has used the Kubernetes EndpointSlices API by default to determine the endpoints to configure Envoy with, instead of the Endpoints API.
The Endpoints API is also deprecated in upstream Kubernetes as of v1.33 (see announcement [here](https://kubernetes.io/blog/2025/04/24/endpoints-deprecation/)).
EndpointSlice support is now stable in Contour and the remaining Endpoint handling code, along with the associated `useEndpointSlices` feature flag, has been removed.
This should be a no-op change for most users, only affecting those that opted into continuing to use the Endpoints API and possibly also disabled EndpointSlice mirroring of Endpoints.
