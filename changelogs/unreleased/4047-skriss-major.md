## Gateway API v1alpha2 support

Contour now exclusively supports Gateway API v1alpha2, the latest available version.
This version of Gateway API has a number of breaking changes, which are detailed in [the Gateway API changelog](https://github.com/kubernetes-sigs/gateway-api/blob/master/CHANGELOG.md).
Contour currently supports a single `GatewayClass` and associated `Gateway`, and `HTTPRoutes` and `TLSRoutes` that attach to the `Gateway`. `TCPRoute` and `UDPRoute` are **not** supported.
As of this writing, `ReferencePolicy`, a new v1alpha2 resource, has not yet been implemented in Contour, but we will look to add support for it shortly.
For a list of other functionality that remains to be implemented, see Contour's [area/gateway-api](https://github.com/projectcontour/contour/labels/area%2Fgateway-api) label.

As part of this change, support for Gateway API v1alpha1 has been dropped, and any v1alpha1 resources **will not** be automatically converted to v1alpha2 resources because the API has moved to a different API group (from `networking.x-k8s.io` to `gateway.networking.k8s.io`).

