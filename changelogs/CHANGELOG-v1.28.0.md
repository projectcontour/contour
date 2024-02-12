We are delighted to present version v1.28.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

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

## Upstream TLS now supports TLS 1.3 and TLS parameters can be configured

The default maximum TLS version for upstream connections is now 1.3, instead of the Envoy default of 1.2.

In a similar way to how Contour users can configure Min/Max TLS version and
Cipher Suites for Envoy's listeners, users can now specify the
same information for upstream connections. In the ContourConfiguration, this is
available under `spec.envoy.cluster.upstreamTLS`. The equivalent config file
parameter is `cluster.upstream-tls`.

(#5828, @KauzClay)

## Update to Gateway API 1.0

Contour now uses [Gateway API 1.0](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.0.0), which graduates the core resources GatewayClass, Gateway and HTTPRoute to the `v1` API version.

For backwards compatibility, this version of Contour continues to watch for `v1beta1` versions of these resources, to ease the migration process for users.
However, future versions of Contour will move to watching for `v1` versions of these resources.
Note that if you are using Gateway API 1.0 and the `v1` API group, the resources you create will also be available from the API server as `v1beta1` resources so Contour will correctly reconcile them as well.

(#5898, @skriss)

## Support for Gateway API BackendTLSPolicy

The BackendTLSPolicy CRD can now be used with HTTPRoute to configure a Contour gateway to connect to a backend Service with TLS. This will give users the ability to use Gateway API to configure their routes to securely connect to backends that use TLS with Contour.

The BackendTLSPolicy spec requires you to specify a `targetRef`, which can currently only be a Kubernetes Service within the same namespace as the BackendTLSPolicy. The targetRef is what Service should be watched to apply the BackendTLSPolicy to. A `SectionName` can also be configured to the port name of a Service to reference a specific section of the Service.

The spec also requires you to specify `caCertRefs`, which can either be a ConfigMap or Secret with a `ca.crt` key in the data map containing a PEM-encoded TLS certificate. The CA certificates referenced will be configured to be used by the gateway to perform TLS to the backend Service. You will also need to specify a `Hostname`, which will be used to configure the SNI the gateway will use for the connection.

See Gateway API's [GEP-1897](https://gateway-api.sigs.k8s.io/geps/gep-1897) for the proposal for BackendTLSPolicy.

(#6119, @flawedmatrix, @christianang)


# Minor Changes

## JWT Authentication happens before External Authorization

Fixes a bug where when the external authorization filter and JWT authentication filter were both configured, the external authorization filter was executed _before_ the JWT authentication filter.  Now, JWT authentication happens before external authorization when they are both configured.

(#5840, @izturn)

## Allow Multiple SANs in Upstream Validation section of HTTPProxy

This change introduces a max length of 250 characters to the field `subjectName` in the UpstreamValidation block.

Allow multiple SANs in Upstream Validation by adding a new field `subjectNames` to the UpstreamValidtion block. This will exist side by side with the previous `subjectName` field. Using CEL validation, we can enforce that when both are present, the first entry in `subjectNames` must match the value of `subjectName`.

(#5849, @KauzClay)

## Gateway API Backend Protocol Selection

For Gateway API, Contour now enables end-users to specify backend protocols by setting the backend Service's [ServicePort.AppProtocol](https://kubernetes.io/docs/concepts/services-networking/service/#application-protocol) parameter. The accepted values are `kubernetes.io/h2c` and `kubernetes.io/ws`. Note that websocket upgrades are already enabled by default for Gateway API. If `AppProtocol` is set, any other configurations, such as the annotation: `projectcontour.io/upstream-protocol.{protocol}` will be disregarded.

(#5934, @izturn)

## Gateway API: support HTTPRoute request timeouts

Contour now enables end-users to specify request timeouts by setting the [HTTPRouteRule.Timeouts.Request](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteTimeouts) parameter. Note that `BackendRequest` is not yet implemented because without Gateway API support for retries, it's functionally equivalent to `Request`.

(#5997, @izturn)

## Support for Global Circuit Breaker Policy

The way [circuit-breaker-annotations](https://projectcontour.io/docs/1.27/config/annotations/) work currently is that when not present they are being defaulted to Envoy defaults. The Envoy defaults can be quite low for larger clusters with more traffic so if a user accidentally deletes them or unset them this cause an issue. With this change we are providing contour administrators the ability to provide global defaults that are good. In that case even if the user forgets to set them or deletes them they can have the safety net of good defaults. They can be configured via [cluster.circuit-breakers](https://projectcontour.io/docs/1.28/configuration/#circuit-breakers)  or via `ContourConfiguration`` CRD in [spec.envoy.cluster.circuitBreakers](https://projectcontour.io/docs/1.28/config/api/#projectcontour.io/v1alpha1.GlobalCircuitBreakerDefaults)

(#6013, @davinci26)

## Allow setting connection limit per listener

Adds a `listeners.max-connections-per-listener` config option to Contour config file and `spec.envoy.listener.maxConnectionsPerListener` to the ContourConfiguration CRD.

Setting the max connection limit per listener field limits the number of active connections to a listener. The default, if unset, is unlimited.

(#6058, @flawedmatrix, @christianang)

## Upstream TLS validation and client certificate for TCPProxy

TCPProxy now supports validating server certificate and using client certificate for upstream TLS connections.
Set `httpproxy.spec.tcpproxy.services.validation.caSecret` and `subjectName` to enable optional validation and `tls.envoy-client-certificate` configuration file field or `ContourConfiguration.spec.envoy.clientCertificate` to set the optional client certificate.

(#6079, @tsaarni)

## Remove Contour container readiness probe initial delay

The Contour Deployment Contour server container previously had its readiness probe `initialDelaySeconds` field set to 15.
This has been removed from the example YAML manifests and Gateway Provisioner generated Contour Deployment since as of [PR #5672](https://github.com/projectcontour/contour/pull/5672) Contour's xDS server will not start or serve any configuration (and the readiness probe will not succeed) until the existing state of the cluster is synced.
In clusters with few resources this will improve the Contour Deployment's update/rollout time as initial startup time should be low.

(#6099, @sunjayBhatia)

## Add anti-affinity rule for envoy deployed by provisioner

The envoy deployment created by the gateway provisioner now includes a default anti-affinity rule. The anti-affinity rule in the [example envoy deployment manifest](https://github.com/projectcontour/contour/blob/main/examples/deployment/03-envoy-deployment.yaml) is also updated to `preferredDuringSchedulingIgnoredDuringExecution` to be consistent with the contour deployment and the gateway provisioner anti-affinity rule.

(#6148, @lubronzhan)

## Add DisabledFeatures to ContourDeployment for gateway provisioner

A new flag DisabledFeatures is added to ContourDeployment so that user can configure contour which is deployed by the provisioner to skip reconciling CRDs which are specified inside the flag.

Accepted values are `grpcroutes|tlsroutes|extensionservices|backendtlspolicies`.

(#6152, @lubronzhan)


# Other Changes
- For Gateway API v1.0, the successful attachment of a Route to a Listener is based solely on the combination of the AllowedRoutes field on the corresponding Listener and the Route's ParentRefs field. (#5961, @izturn)
- Gateway API: adds support for [Gateway infrastructure labels and annotations](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.GatewayInfrastructure)``. (#5968, @skriss)
- Gateway API: add the `gateway.networking.k8s.io/gateway-name` label to generated resources. (#5969, @skriss)
- Fixes a bug with the `envoy` xDS server where at startup, xDS configuration would not be generated and served until a subsequent configuration change. (#5972, @skriss)
- Envoy: Adds support for setting [per-host circuit breaker max-connections threshold](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/circuit_breaker.proto#envoy-v3-api-field-config-cluster-v3-circuitbreakers-per-host-thresholds) using a new service-level annotation: `projectcontour.io/per-host-max-connections`. (#6016, @relu)
- Updates to Kubernetes 1.29. Supported/tested Kubernetes versions are now 1.27, 1.28 and 1.29. (#6031, @skriss)
- Remove static base runtime layer from bootstrap (#6063, @lubronzhan)
- Updates to Go 1.21.6. See the [Go release notes](https://go.dev/doc/devel/release#go1.21.minor) for more information. (#6070, @sunjayBhatia)
- Allow gatewayProvisioner to create contour that only watch limited namespaces of resources (#6073, @lubronzhan)
- Access Log: Contour excludes empty fields in Envoy JSON based access logs by default. (#6077, @abbas-gheydi)
- Updates HTTP filter names to match between the HTTP connection manager and per-filter config on virtual hosts/routes, and to use canonical names. (#6124, @skriss)
- Gateway API provisioner now checks `gateway.networking.k8s.io/bundle-version` annotation on Gateway CRDs and sets SupportedVersion status condition on GatewayClass if annotation value matches supported Gateway API version. Best-effort support is provided if version does not match. (#6147, @sunjayBhatia)
- For Gateway API, add "Accepted" condition to BackendTLSPolicy. If the condition is true the BackendTLSPolicy was accepted by the Gateway and if false a reason will be stated on the policy as to why it wasn't accepted. (#6151, @christianang)
- Updates Envoy to v1.29.1. See the release notes [here](https://www.envoyproxy.io/docs/envoy/v1.29.1/version_history/v1.29/v1.29.1). (#6164, @sunjayBhatia)


# Docs Changes
- Document that Gateway names should be 63 characters or shorter to avoid issues with generating dependent resources when using the Gateway provisioner. (#6143, @skriss)
- Add troubleshooting guide for general app traffic errors. (#6161, @sunjayBhatia)


# Deprecation and Removal Notices


## Deprecate `subjectName` field on UpstreamValidation

The `subjectName` field is being deprecated in favor of `subjectNames`, which is
an list of subjectNames. `subjectName` will continue to behave as it has. If
using `subjectNames`, the first entry in `subjectNames` must match the value of
`subjectName`. this will be enforced by CEL validation.

(#5849, @KauzClay)

## ContourDeployment.Spec.ResourceLabels is deprecated

The `ContourDeployment.Spec.ResourceLabels` field is now deprecated. You should use `Gateway.Spec.Infrastructure.Labels` instead. The `ResourceLabels` field will be removed in a future release.

(#5968, @skriss)

## Configuring Contour with a GatewayClass controller name is deprecated

Contour should no longer be configured with a GatewayClass controller name (`gateway.controllerName` in the config file or ContourConfiguration CRD).
Instead, either use a specific Gateway reference (`gateway.gatewayRef`), or use the Gateway provisioner.
`gateway.controllerName` will be removed in a future release.

(#6144, @skriss)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.28.0 is tested against Kubernetes 1.27 through 1.29.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @KauzClay
- @abbas-gheydi
- @christianang
- @davinci26
- @flawedmatrix
- @izturn
- @lubronzhan
- @relu


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
