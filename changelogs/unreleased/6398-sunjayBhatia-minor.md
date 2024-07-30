## Update to Gateway API v1.1.0

Gateway API CRD compatibility has been updated to release v1.1.0.

Notable changes for Contour include:
- The `BackendTLSPolicy` resource has undergone some breaking changes and has been updated to the `v1alpha3` API version. This will require any existing users of this policy to uninstall the v1alpha2 version before installing this newer version.
- `GRPCRoute` has graduated to GA and is now in the `v1` API version.

Full release notes for this Gateway API release can be found [here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.1.0).
