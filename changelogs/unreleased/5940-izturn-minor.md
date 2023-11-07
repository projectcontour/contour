## HTTPRoute Timeouts

Contour now enables end-users to specify timeout by configuring the parameter `HTTPRouteRule.Timeouts`, Please ensure the value of `BackendRequest` must be <= the value of `Request` timeout.

