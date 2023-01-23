## Contour supports Gateway API release v0.6.0

See [the Gateway API release notes](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.6.0) for more detail on the API changes.
This version of the API includes a few changes relevant to Contour users:
- The ReferenceGrant resource has been graduated to the v1beta1 API and ReferencePolicy removed from the API
- v1alpha2 versions of GatewayClass, Gateway, and HTTPRoute are deprecated
- There have been significant changes to status conditions on various resources for consistency:
  - Accepted and Programmed conditions have been added to Gateway and Gateway Listener
  - The Ready condition has been moved to "extended" conformance, at this moment Contour does not program this condition
  - The Scheduled condition has been deprecated on Gateway
