## HTTPRoute Timeouts

Contour now enables end-users to specifys timeout by setting the [HTTPRouteRule.Timeouts](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteTimeouts) parameter, Please ensure the value of `BackendRequest` must be <= the value of `Request` timeout. otherwise `Request` will be set to the same value as `BackendRequest`