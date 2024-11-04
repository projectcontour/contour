We are delighted to present version v1.30.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Deprecations/Removals](#deprecation-and-removal-notices)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Minor Changes

## Gateway API: Implement Listener/Route hostname isolation

Gateway API spec update in this [GEP](https://github.com/kubernetes-sigs/gateway-api/pull/2465).
Updates logic on finding intersecting route and Listener hostnames to factor in the other Listeners on a Gateway that the route in question may not actually be attached to.
Requests should be "isolated" to the most specific Listener and it's attached routes.

(#6162, @sunjayBhatia)

## Update examples for monitoring Contour and Envoy

Updates the [documentation](https://projectcontour.io/docs/main/guides/prometheus/) and examples for deploying a monitoring stack (Prometheus and Grafana) to scrape metrics from Contour and Envoy.
Adds a metrics port to the Envoy DaemonSet/Deployment in the example YAMLs to expose port `8002` so that `PodMonitor` resources can be used to find metrics endpoints.

(#6269, @sunjayBhatia)

## Update to Gateway API v1.1.0

Gateway API CRD compatibility has been updated to release v1.1.0.

Notable changes for Contour include:
- The `BackendTLSPolicy` resource has undergone some breaking changes and has been updated to the `v1alpha3` API version. This will require any existing users of this policy to uninstall the v1alpha2 version before installing this newer version.
- `GRPCRoute` has graduated to GA and is now in the `v1` API version.

Full release notes for this Gateway API release can be found [here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.1.0).

(#6398, @sunjayBhatia)

## Add Circuit Breaker support for Extension Services

This change enables the user to configure the Circuit breakers for extension services either via the global Contour config or on an individual Extension Service.

**NOTE**: The `PerHostMaxConnections` is now also configurable via the global settings.

(#6539, @clayton-gonsalves)

## Fallback Certificate: Add Global Ext Auth support

Applies Global Auth filters to Fallback certificate

(#6558, @erikflores7)

## Gateway API: handle Route conflicts with GRPCRoute.Matches

It's possible that multiple GRPCRoutes will define the same Match conditions. In this case the following logic is applied to resolve the conflict:

- The oldest Route based on creation timestamp. For example, a Route with a creation timestamp of “2020-09-08 01:02:03” is given precedence over a Route with a creation timestamp of “2020-09-08 01:02:04”.
- The Route appearing first in alphabetical order (namespace/name) for example, foo/bar is given precedence over foo/baz.

With above ordering, any GRPCRoute that ranks lower, will be marked with below conditions accordingly:
1. If only partial rules under this GRPCRoute are conflicted, it's marked with `Accepted: True` and `PartiallyInvalid: true` Conditions and Reason: `RuleMatchPartiallyConflict`.
2. If all the rules under this GRPCRoute are conflicted, it's marked with `Accepted: False` Condition and Reason `RuleMatchConflict`.

(#6566, @lubronzhan)


# Other Changes
- Fixes bug where external authorization policy was ignored on HTTPProxy direct response routes. (#6426, @shadialtarsha)
- Updates to Kubernetes 1.30. Supported/tested Kubernetes versions are now 1.28, 1.29, and 1.30. (#6444, @sunjayBhatia)
- Enforce `deny-by-default` approach on the `admin` listener by matching on exact paths and on `GET` requests (#6447, @davinci26)
- Add support for defining equal-preference cipher groups ([cipher1|cipher2|...]) and permit `ECDHE-ECDSA-CHACHA20-POLY1305` and `ECDHE-RSA-CHACHA20-POLY1305` to be used separately. (#6461, @tsaarni)
- allow `/stats/prometheus` route on the `admin` listener. (#6503, @clayton-gonsalves)
- Improve shutdown manager query to the Envoy stats endpoint for active connections by utilizing a regex filter query param. (#6523, @therealak12)
- Updates to Go 1.22.5. See the [Go release notes](https://go.dev/doc/devel/release#go1.22.minor) for more information. (#6563, @sunjayBhatia)
- Updates Envoy to v1.31.0. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.31.0/version_history/v1.31/v1.31.0) for more information about the content of the release. (#6569, @skriss)

# Deprecation and Removal Notices


## Contour sample YAML manifests no longer use `prometheus.io/` annotations

The annotations for notifying a Prometheus instance on how to scrape metrics from Contour and Envoy pods have been removed from the deployment YAMLs and the Gateway provisioner.
The suggested mechanism for doing so now is to use [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) and the [`PodMonitor`](https://prometheus-operator.dev/docs/operator/design/#podmonitor) resource.

(#6269, @sunjayBhatia)

## xDS server type fields in config file and ContourConfiguration CRD are deprecated

These fields are officially deprecated now that the `contour` xDS server implementation is deprecated.
They are planned to be removed in the 1.31 release, along with the `contour` xDS server implementation.

(#6561, @skriss)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.30.0 is tested against Kubernetes 1.28 through 1.30.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @clayton-gonsalves
- @davinci26
- @erikflores7
- @lubronzhan
- @shadialtarsha
- @therealak12


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
