### Gateway API: implement PathPrefix matching

Contour now implements Gateway API v1alpha2's "path prefix" matching for `HTTPRoutes`.
This is now the only native form of prefix matching supported by Gateway API, and is a change from v1alpha1.
Path prefix matching means that the prefix specified in an `HTTPRoute` rule must match entire segments of a request's path in order to match it, rather than just be a string prefix.
For example, the prefix `/foo` would match a request for the path `/foo/bar` but not `/foobar`.
For more information, see the [Gateway API documentation](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.PathMatchType).
