## JWT Verification Support

Contour's HTTPProxy now supports configuring Envoy's [JSON Web Token (JWT) authentication filter](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/jwt_authn_filter), for verifying JWTs on incoming requests.

A root HTTPProxy can optionally define one or more JWT providers, each of which can define an issuer, audiences, and a JSON Web Key Set (JWKS) to use for verifying JWTs.

JWT providers can then be applied as requirements to routes on the HTTPProxy (or routes on [included HTTPProxies](https://projectcontour.io/docs/main/config/inclusion-delegation/)), either by setting one provider as the default, or by explicitly specifying a JWT provider to require for a given route.
Individual routes may also opt out of JWT verification if a default provider has been set for the HTTPProxy.

For more information, see:
- [JWT verification documentation](https://projectcontour.io/docs/main/config/jwt-verification)
- [JWTProvider API documentation](https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1.JWTProvider)
- [JWTVerificationPolicy API documentation](https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1.JWTVerificationPolicy)




