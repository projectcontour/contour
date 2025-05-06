We are delighted to present version v1.31.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Deprecations/Removals](#deprecation-and-removal-notices)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)


# Minor Changes

## Disable ExtAuth by default if GlobalExtAuth.AuthPolicy.Disabled is set

Global external authorization can now be disabled by default and enabled by overriding the vhost and route level auth policies.
This is achieved by setting the `globalExtAuth.authPolicy.disabled` in the configuration file or `ContourConfiguration` CRD to `true`, and setting the `authPolicy.disabled` to `false` in the vhost and route level auth policies.
The final authorization state is determined by the most specific policy applied at the route level.

## Disable External Authorization in HTTPS Upgrade

When external authorization is enabled, no authorization check will be performed for HTTP to HTTPS redirection.
Previously, external authorization was checked before redirection, which could result in a 401 Unauthorized error instead of a 301 Moved Permanently status code.

(#6661, @SamMHD)

## Overload Manager - Max Global Connections

Introduces an envoy bootstrap flag to enable the [global downstream connection limit overload manager resource monitors](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/resource_monitors/downstream_connections/v3/downstream_connections.proto#envoy-v3-api-msg-extensions-resource-monitors-downstream-connections-v3-downstreamconnectionsconfig).

The new flag can be passed as an integer flag to the contour bootstrap subcommand, `overload-downstream-max-conn`.

```sh
contour bootstrap --help
INFO[0000] maxprocs: Leaving GOMAXPROCS=10: CPU quota undefined
usage: contour bootstrap [<flags>] <path>

Generate bootstrap configuration.


Flags:
  -h, --[no-]help                Show context-sensitive help (also try --help-long and --help-man).
      --log-format=text          Log output format for Contour. Either text or json.
      --admin-address="/admin/admin.sock"
                                 Path to Envoy admin unix domain socket.
      --admin-port=ADMIN-PORT    DEPRECATED: Envoy admin interface port.
      --dns-lookup-family=DNS-LOOKUP-FAMILY
                                 Defines what DNS Resolution Policy to use for Envoy -> Contour cluster name lookup. Either v4, v6, auto, or all.
      --envoy-cafile=ENVOY-CAFILE
                                 CA Filename for Envoy secure xDS gRPC communication. ($ENVOY_CAFILE)
      --envoy-cert-file=ENVOY-CERT-FILE
                                 Client certificate filename for Envoy secure xDS gRPC communication. ($ENVOY_CERT_FILE)
      --envoy-key-file=ENVOY-KEY-FILE
                                 Client key filename for Envoy secure xDS gRPC communication. ($ENVOY_KEY_FILE)
      --namespace="projectcontour"
                                 The namespace the Envoy container will run in. ($CONTOUR_NAMESPACE)
      --overload-downstream-max-conn=OVERLOAD-DOWNSTREAM-MAX-CONN
                                 Defines the Envoy global downstream connection limit
      --overload-max-heap=OVERLOAD-MAX-HEAP
                                 Defines the maximum heap size in bytes until overload manager stops accepting new connections.
      --resources-dir=RESOURCES-DIR
                                 Directory where configuration files will be written to.
      --xds-address=XDS-ADDRESS  xDS gRPC API address.
      --xds-port=XDS-PORT        xDS gRPC API port.
      --xds-resource-version="v3"
                                 The versions of the xDS resources to request from Contour.

Args:
  <path>  Configuration file ('-' for standard output).
```

As part of this change, we also set the `ignore_global_conn_limit` flag to `true` on the existing admin listeners such
that envoy remains live, ready, and serving stats even though it is rejecting downstream connections.

To add some flexibility for health checks, in addition to adding a new bootstrap flag, there is a new configuration
option for the envoy health config to enforce the envoy overload manager actions, namely rejecting requests. This
"advanced" configuration gives the operator the ability to configure readiness and liveness to handle taking pods out
of the pool of pods that can serve traffic.

(#6794, @seth-epps)


## Update to Gateway API v1.2.1

Gateway API CRD compatibility has been updated to release v1.2.1.

Full release notes for Gateway API v1.2.0 can be found [here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.2.0), and v1.2.1 [here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.2.1).


# Other Changes
- The HTTP compression algorithm can now be configured using the `compression.algorithm` field in the configuration file or the `spec.envoy.listener.compression.algorithm` field in the `ContourConfiguration` CRD. The available values are `gzip` (default), `brotli`, `zstd`, and `disabled`. (#6546, @chaosbox)
- Fixed a bug where follower Contour instance occasionally got stuck in a non-ready state when using `--watch-namespaces` flag. (#6614, @tsaarni)
- Contour, support http and https as AppProtocol in k8s' services (#6616, @Krast76)
- Added conditions `reset-before-request`, `envoy-ratelimited` and `http3-post-connect-failure` for `httpproxy.spec.routes.retryPolicy.retryOn`, see Envoy [documentation](https://www.envoyproxy.io/docs/envoy/v1.32.0/configuration/http/http_filters/router_filter#config-http-filters-router-x-envoy-retry-on) for more details. (#6772, @tsaarni)
- `HTTPProxy.spec.routes.requestRedirectPolicy.statusCode` now supports 303, 307 and 308 redirect status codes in addition to 301 and 302. (#6789, @billyjs)
- Adds a new configuration option `strip-trailing-host-dot` which defines if trailing dot of the host should be removed from host/authority header before any processing of request by HTTP filters or routing. (#6792, @saley89)
- Updates kind node image for e2e tests to Kubernetes 1.32. Supported/tested Kubernetes versions are now 1.32, 1.31 and 1.30. (#6834, @skriss)
- Fixed a memory leak in Contour follower instance due to unprocessed LoadBalancer status updates. (#6872, @tsaarni)
- Add support for the SECP521R1 curve, enabling the use of EC certificates with 521-bit private keys in the xDS gRPC interface between Envoy and Contour. (#6996, @tsaarni)
- Updates Envoy to v1.34.0. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.34.0/version_history/v1.34/v1.34) for more information about the content of the release. (#7003, @tsaarni)
- Update to Go 1.24.2. See the [Go release notes](https://go.dev/doc/devel/release#go1.24.0) for more information. (#7019, @sunjayBhatia)


# Deprecation and Removal Notices


## xDS server type fields in config file and ContourConfiguration CRD and legacy `contour` xDS server are removed

Contour now uses a go-control-plane-based xDS server.
The legacy `contour` xDS server that pre-dates `go-control-plane` has been removed.
Since there is now only one supported xDS server, the config fields for selecting an xDS server implementation have been removed.

(#6568, @skriss)

## useEndpointSlices feature flag removed

As of v1.29.0, Contour has used the Kubernetes EndpointSlices API by default to determine the endpoints to configure Envoy with, instead of the Endpoints API.
The Endpoints API is also deprecated in upstream Kubernetes as of v1.33 (see announcement [here](https://kubernetes.io/blog/2025/04/24/endpoints-deprecation/)).
EndpointSlice support is now stable in Contour and the remaining Endpoint handling code, along with the associated `useEndpointSlices` feature flag, has been removed.
This should be a no-op change for most users, only affecting those that opted into continuing to use the Endpoints API and possibly also disabled EndpointSlice mirroring of Endpoints.

(#7008, @sunjayBhatia)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.31.0 is tested against Kubernetes 1.30 through 1.32.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @Krast76
- @SamMHD
- @billyjs
- @chaosbox
- @saley89
- @seth-epps


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
