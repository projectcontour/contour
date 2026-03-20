## Update to Gateway API v1.4.1

Gateway API CRD compatibility has been updated to release v1.4.1.

Note: `BackendTLSPolicy`'s `v1alpha3` API version has been deprecated in favor of the `v1` API version. Contour will migrate to reconciling only `v1` API versions in a future release prior to the removal of `v1alpha3`.

Full release notes for this Gateway API release can be found [for v1.4.0 here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.4.0) and [for v1.4.1 here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.4.1).
