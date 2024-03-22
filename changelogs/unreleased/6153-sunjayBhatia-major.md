## Gateway API: Inform on v1 types

Contour no longer informs on v1beta1 resources that have graduated to v1.
This includes the "core" resources GatewayClass, Gateway, and HTTPRoute.
This means that users should ensure they have updated CRDs to Gateway API v1.0.0 or newer, which introduced the v1 version with compatibility with v1beta1.
