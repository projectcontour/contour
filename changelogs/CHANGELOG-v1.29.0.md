We are delighted to present version v1.29.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Major Changes](#major-changes)
- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Docs Changes](#docs-changes)
- [Deprecations/Removals](#deprecation-and-removal-notices)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Major Changes

## Default xDS Server Implementation is now Envoy

As of this release, Contour now uses the `envoy` xDS server implementation by default.
This xDS server implementation is based on Envoy's [go-control-plane project](https://github.com/envoyproxy/go-control-plane) and will eventually be the only supported xDS server implementation in Contour.
This change is expected to be transparent to users.

### I'm seeing issues after upgrading, how to I revert to the contour xDS server?

If you encounter any issues, you can easily revert to the `contour` xDS server with the following configuration:

(if using Contour config file)
```yaml
server:
  xds-server-type: contour
```

(if using ContourConfiguration CRD)
```yaml
...
spec:
  xdsServer:
    type: contour
```

You will need to restart Contour for the changes to take effect.

(#6146, @skriss)

## Gateway API: Inform on v1 types

Contour no longer informs on v1beta1 resources that have graduated to v1.
This includes the "core" resources GatewayClass, Gateway, and HTTPRoute.
This means that users should ensure they have updated CRDs to Gateway API v1.0.0 or newer, which introduced the v1 version with compatibility with v1beta1.

(#6153, @sunjayBhatia)


# Minor Changes

## Use EndpointSlices by default

Contour now uses the Kubernetes EndpointSlices API by default to determine the endpoints to configure Envoy, instead of the Endpoints API.
Note: if you need to continue using the Endpoints API, you can disable the feature flag via `featureFlags: ["useEndpointSlices=false"]` in the Contour config file or ContourConfiguration CRD.

(#6149, @izturn)

## Gateway API: handle Route conflicts with HTTPRoute.Matches

It's possible that multiple HTTPRoutes will define the same Match conditions. In this case the following logic is applied to resolve the conflict:

- The oldest Route based on creation timestamp. For example, a Route with a creation timestamp of “2020-09-08 01:02:03” is given precedence over a Route with a creation timestamp of “2020-09-08 01:02:04”.
- The Route appearing first in alphabetical order (namespace/name) for example, foo/bar is given precedence over foo/baz.

With above ordering, any HTTPRoute that ranks lower, will be marked with below conditions accordingly
1. If only partial rules under this HTTPRoute are conflicted, it's marked with `Accepted: True` and `PartiallyInvalid: true` Conditions and Reason: `RuleMatchPartiallyConflict`.
2. If all the rules under this HTTPRoute are conflicted, it's marked with `Accepted: False` Condition and Reason `RuleMatchConflict`.

(#6188, @lubronzhan)

## Spawn Upstream Span is now enabled in tracing

As described in [Envoy documentations](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-tracing), ```spawn_upstream_span``` should be true when envoy is working as an independent proxy and from now on contour tracing spans will show up as a parent span to upstream spans.

(#6271, @SamMHD)


# Other Changes
- Fix data race in BackendTLSPolicy status update logic. (#6185, @sunjayBhatia)
- Fix for specifying a health check port with an ExternalName Service. (#6230, @yangyy93)
- Updates the example `envoyproxy/ratelimit` image tag to `19f2079f`, for multi-arch support and other improvements. (#6246, @skriss)
- In the `envoy` go-control-plane xDS server, use a separate snapshot cache for Endpoints, to minimize the amount of unnecessary xDS traffic generated. (#6250, @skriss)
- If there were no relevant resources for Contour in the watched namespaces during the startup of a follower instance of Contour, it did not reach a ready state. (#6295, @tsaarni)
- Added support for enabling circuit breaker statistics tracking. (#6297, @rajatvig)
- Updates to Go 1.22.2. See the [Go release notes](https://go.dev/doc/devel/release#go1.22.minor) for more information. (#6327, @skriss)
- Gateway API: add support for HTTPRoute's Timeouts.BackendRequest field. (#6335, @skriss)
- Updates Envoy to v1.30.1. See the v1.30.0 release notes [here](https://www.envoyproxy.io/docs/envoy/v1.30.1/version_history/v1.30/v1.30.0) and the v1.30.1 release notes [here](https://www.envoyproxy.io/docs/envoy/v1.30.1/version_history/v1.30/v1.30.1). (#6353, @tico88612)
- Gateway API: a timeout value of `0s` disables the timeout. (#6375, @skriss)
- Fix provisioner to use separate `--disable-feature` flags on Contour Deployment for each disabled feature. Previously a comma separated list was passed which was incorrect. (#6413, @sunjayBhatia)


# Deprecation and Removal Notices

## Configuring Contour with a GatewayClass controller name is no longer supported

Contour can no longer be configured with a GatewayClass controller name (gateway.controllerName in the config file or ContourConfiguration CRD), as the config field has been removed.
Instead, either use a specific Gateway reference (gateway.gatewayRef), or use the Gateway provisioner.

(#6145, @skriss)

## Contour xDS server implementation is now deprecated

As of this release, the `contour` xDS server implementation is now deprecated.
Once the go-control-plane based `envoy` xDS server has had sufficient production bake time, the `contour` implementation will be removed from Contour.
Notification of removal will occur at least one release in advance.

(#6146, @skriss)

## Use of Endpoints API is deprecated

Contour now uses the EndpointSlices API by default, and its usage of the Endpoints API is deprecated as of this release. Support for Endpoints, and the associated `useEndpointSlices` feature flag, will be removed in a future release.

(#6149, @izturn)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.29.0 is tested against Kubernetes 1.27 through 1.29.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @SamMHD
- @izturn
- @lubronzhan
- @rajatvig
- @tico88612
- @yangyy93


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
