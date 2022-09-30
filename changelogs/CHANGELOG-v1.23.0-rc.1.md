We are delighted to present version v1.23.0-rc.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

**Please note that this is pre-release software**, and as such we do not recommend installing it in production environments.
Feedback and bug reports are welcome!


- [Major Changes](#major-changes)
- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Docs Changes](#docs-changes)
- [Deprecations/Removals](#deprecation-and-removal-notices)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Major Changes

## Overload Manager

It is now possible to enable Envoy overload manager to avoid traffic disturbances when Envoy process allocates too much heap and is terminated by the Linux out-of-memory manager.
The feature is disabled by default and can be enabled by following [instructions here](https://projectcontour.io/docs/main/config/overload-manager/).

(#4597, @tsaarni)

## JWT Verification Support

Contour's HTTPProxy now supports configuring Envoy's [JSON Web Token (JWT) authentication filter](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/jwt_authn_filter), for verifying JWTs on incoming requests.

A root HTTPProxy can optionally define one or more JWT providers, each of which can define an issuer, audiences, and a JSON Web Key Set (JWKS) to use for verifying JWTs.

JWT providers can then be applied as requirements to routes on the HTTPProxy (or routes on [included HTTPProxies](https://projectcontour.io/docs/main/config/inclusion-delegation/)), either by setting one provider as the default, or by explicitly specifying a JWT provider to require for a given route.
Individual routes may also opt out of JWT verification if a default provider has been set for the HTTPProxy.

For more information, see:
- [JWT verification documentation](https://projectcontour.io/docs/main/config/jwt-verification)
- [JWTProvider API documentation](https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1.JWTProvider)
- [JWTVerificationPolicy API documentation](https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1.JWTVerificationPolicy)

(#4723, @skriss)


# Minor Changes

## HTTPProxy CORS policy supports regex matching on Allowed Origins

The AllowOrigin field of the HTTPProxy CORSPolicy can be configured as a regex to enable more flexibility for users.
More advanced matching can now be performed on the `Origin` header of HTTP requests, instead of restricting users to allow all origins, or enumerating all possible values.

(#4710, @sunjayBhatia)


# Other Changes
- Transition to `default_source_code` Lua filter field from deprecated `inline_string` field for specifying Lua scripts. (#4622, @sunjayBhatia)
- There are so many EnsureXDeleted in the sub-packages of objects , so unify them to objects/EnsureObjectDelete (#4630, @izturn)
- Transition to using new bootstrap field `default_regex_engine` instead of deprecated per-regex match engine selection. (#4652, @sunjayBhatia)
- Gateway Listeners with Secret references whose namespace is not covered by a ReferenceGrant should have their status reason set to RefNotPermitted. (#4664, @sunjayBhatia)
- Add a new flag `leader-election-namespace` for gateway-provisioner (#4669, @izturn)
- Add Contour log level configurability to ContourDeployment resource. (#4676, @izturn)
- Add Kubernetes client debug log level configurability to ContourDeployment resource. (#4677, @izturn)
- Gateway API: status-only updates to resources no longer trigger DAG reprocessing and xDS updates. (#4744, @skriss)
- Gateway API: don't make status update calls to the API server if status has not changed on the resource. (#4745, @skriss)
- Updates to Gateway API v0.5.1. (#4755, @skriss)
- Update supported Kubernetes versions to 1.23, 1.24, and 1.25. (#4757, @sunjayBhatia)


# Docs Changes
- Added guide for configuring gRPC routes. (#4725, @sunjayBhatia)


# Installing and Upgrading

The simplest way to install v1.23.0-rc.1 is to apply one of the example configurations:

With Gateway API:
```bash
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/v1.23.0-rc.1/examples/render/contour-gateway.yaml
```

Without Gateway API:
```bash
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/v1.23.0-rc.1/examples/render/contour.yaml
```


# Compatible Kubernetes Versions

Contour v1.23.0-rc.1 is tested against Kubernetes 1.23 through 1.25.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @izturn


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
