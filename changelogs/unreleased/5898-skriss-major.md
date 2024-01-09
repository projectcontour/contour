## Update to Gateway API 1.0

Contour now uses [Gateway API 1.0](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.0.0), which graduates the core resources GatewayClass, Gateway and HTTPRoute to the `v1` API version.

For backwards compatibility, this version of Contour continues to watch for `v1beta1` versions of these resources, to ease the migration process for users.
However, future versions of Contour will move to watching for `v1` versions of these resources.
Note that if you are using Gateway API 1.0 and the `v1` API group, the resources you create will also be available from the API server as `v1beta1` resources so Contour will correctly reconcile them as well.

