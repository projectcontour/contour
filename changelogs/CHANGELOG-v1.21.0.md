We are delighted to present version v1.21.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

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

## Contour leader election resource RBAC moved to namespaced Role

Previously, in our example deployment YAML, RBAC for Contour access to resources used for leader election was contained in a ClusterRole, meaning that Contour required cluster-wide access to ConfigMap resources.
This release also requires Contour access to Events and Leases which would require cluster-wide access (see [this PR](https://github.com/projectcontour/contour/pull/4202)).

In this release, we have moved the RBAC rules for leader election resources to a namespaced Role in the example Contour deployment.
This change should limit Contour's default required access footprint.
A corresponding namespaced RoleBinding has been added as well.

### Required actions

If you are using the example deployment YAML to deploy Contour, be sure to examine and re-apply the resources in `examples/contour/02-rbac.yaml` and `examples/contour/02-role-contour.yaml`.
If you have deployed Contour in a namespace other than the example `projectcontour`, be sure to modify the `contour` Role and `contour-rolebinding` RoleBinding resources accordingly.
Similarly, if you are using the `--leader-election-resource-namespace` flag to customize where Contour's leader election resources reside, you must customize the new Role and RoleBinding accordingly.

(#4204, @sunjayBhatia)

## Container Images Now Exclusively Published on GitHub Container Registry (GHCR) 

Contour's container images are now exclusively published [on GHCR](https://github.com/projectcontour/contour/pkgs/container/contour). They are no longer being pushed to Docker Hub (past images have been left on Docker Hub for posterity.)

(#4314, @skriss)

## Adds a `contour gateway-provisioner` command and deployment manifest for dynamically provisioning Gateways

Contour now has an optional Gateway provisioner, that watches for `Gateway` custom resources and provisions Contour + Envoy instances for them.
The provisioner is implemented as a new subcommand on the `contour` binary, `contour gateway-provisioner`.
The `examples/gateway-provisioner` directory contains the YAML manifests needed to run the provisioner as a Deployment in-cluster.

By default, the Gateway provisioner will process all `GatewayClasses` that have a controller string of `projectcontour.io/gateway-controller`, along with all Gateways for them.

The Gateway provisioner is useful for users who want to dynamically provision Contour + Envoy instances based on the `Gateway` CRD.
It is also necessary in order to have a fully conformant Gateway API implementation.

(#4415, @skriss)


# Minor Changes

## Configurable access log level

The verbosity of HTTP and HTTPS access logs can now be configured to one of: `info` (default), `error`, `disabled`.
The verbosity level is set with `accesslog-level` field in the [configuration file](https://projectcontour.io/docs/main/configuration/#configuration-file) or `spec.envoy.logging.accessLogLevel` field in [`ContourConfiguration`](https://projectcontour.io/docs/main/config/api/).

(#4331, @tsaarni)

## Leader election now only uses Lease object

Contour now only uses the Lease object to coordinate leader election.
RBAC in example manifests has been updated accordingly.

**Note:** Upgrading to this version of Contour will explicitly require you to upgrade to Contour v1.20.0 *first* to ensure proper migration of leader election coordination resources.

(#4332, @sunjayBhatia)

## Re-increase maximum allowed regex program size

Regex patterns Contour configures in Envoy (for path matching etc.) currently have a limited "program size" (approximate cost) of 100.
This was inadvertently set back to the Envoy default, from the intended 1048576 (2^20) when moving away from using deprecated API fields.
Note: regex program size is a feature of the regex library Envoy uses, [Google RE2](https://github.com/google/re2).

This limit has now been reset to the intended value and an additional program size warning threshold of 1000 has been configured.

Operators concerned with performance implications of allowing large regex programs can monitor Envoy memory usage and regex statistics.
Envoy offers two statistics for monitoring regex program size, `re2.program_size` and `re2.exceeded_warn_level`.
See [this documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/matcher/v3/regex.proto.html?highlight=warn_level#type-matcher-v3-regexmatcher-googlere2) for more detail.
Future versions of Contour may allow configuration of regex program size thresholds via RTDS (Runtime Discovery Service).

(#4379, @sunjayBhatia)

## Gateway API: support for processing a specific Gateway

Contour can now optionally process a specific named `Gateway` and associated routes.
This is an alternate way to configure Contour, vs. the existing mode of specifying a `GatewayClass` controller string and having Contour process the first `GatewayClass` and associated `Gateway` for that controller string.
This new configuration option can be specified via:
```yaml
gateway:
  gatewayRef:
    namespace: gateway-namespace
    name: gateway-name
```

(#4410, @skriss)

## Gateway provisioner: add support for more than one Gateway/Contour instance per namespace

The Gateway provisioner now supports having more than one Gateway/Contour instance per namespace.
All resource names now include a `-<gateway-name>` suffix to avoid conflicts (cluster-scoped resources also include the namespace as part of the resource name).
Contour instances are always provisioned in the namespace of the Gateway custom resource itself.

(#4426, @skriss)

## Gateway provisioner: generate xDS TLS certs directly

The Gateway provisioner now generates xDS TLS certificates directly, rather than using a "certgen" job to trigger certificate generation.
This simplifies operations and reduces the RBAC permissions that the provisioner requires.
Certificates will still be rotated each time the provisioner is upgraded to a new version.

(#4432, @skriss)

## Gateway provisioner: support requesting a specific address

The Gateway provisioner now supports requesting a specific Gateway address, via the Gateway's `spec.addresses` field.
Only one address is supported, and it must be either an `IPAddress` or `Hostname` type.
The value of this address will be used to set the provisioned Envoy service's `spec.loadBalancerIP` field.
If for any reason, the requested address is not assigned to the Gateway, the Gateway will have a condition of "Ready: false" with a reason of `AddressesNotAssigned`.

If no address is requested, no value will be specified in the provisioned Envoy service's `spec.loadBalancerIP` field, and an address will be assigned by the load balancer provider.

(#4443, @skriss)

## All ContourConfiguration CRD fields are now optional

To better manage configuration defaults, all `ContourConfiguration` CRD fields are now optional without defaults.
Instead, Contour itself will apply defaults to any relevant fields that have not been specified by the user when it starts up, similarly to how processing of the Contour `ConfigMap` works today.
The default values that Contour uses are documented in the `ContourConfiguration` CRD's API documentation.

(#4451, @skriss)

## ContourDeployment CRD now supports additional options

The `ContourDeployment` CRD, which can be used as parameters for a Contour-controlled `GatewayClass`, now supports additional options for customizing your Contour/Envoy installations:

- Contour deployment replica count
- Contour deployment node placement settings (node selectors and/or tolerations)
- Envoy workload type (daemonset or deployment)
- Envoy replica count (if using a deployment)
- Envoy service type and annotations
- Envoy node placement settings (node selectors and/or tolerations)

(#4472, @skriss)

## Query parameter hash based load balancing

Contour users can now configure their load balancing policies on `HTTPProxy` resources to hash the query parameter on a request to ensure consistent routing to a backend service instance.

See [this page](https://projectcontour.io/docs/v1.21.0/config/request-routing/#load-balancing-strategy) for more details on this feature.

Credit to @pkit for implementing this feature!

(#4508, @sunjayBhatia)


# Other Changes
- Allow the contour --ingress-class-name value to be a comma-separated list of classes to match against.  Contour will process Ingress and HTTPProxy objects with any of the specified ingress classes. (Note that the alpha ContourConfiguration CRD has also been changed to use a ClassNames array field instead of a scalar ClassName field.) (#4109, @erwbgy)
- Don't check for or log errors for unsupported annotations on objects that Contour doesn't care about (e.g. ingresses for a different class than Contour's). (#4304, @skriss)
- Explicitly disable controller-runtime manager metrics and health listeners. (#4312, @sunjayBhatia)
- Removed code duplication for the secret validation in the dag package. (#4316, @alessandroargentieri)
- Node labels in `localhost:6060/debug/dag` troubleshooting API are sanitized by html-escaping user fields. (#4323, @kb000)
- Upstream TCP connection timeout is now configurable in [configuration file](https://projectcontour.io/docs/main/configuration/#timeout-configuration) and in [`ContourConfiguration`](https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1alpha1.TimeoutParameters). (#4326, @tsaarni)
- Drops RBAC and caching for the `networking.k8s.io/IngressClass` resource as it's not used by Contour. (#4329, @skriss)
- Fixed a bug where upstream TLS SNI (`HTTProxy.spec.routes.requestHeadersPolicy` `Host` key) and protocol fields might not take effect when e.g. two `HTTPProxies` were otherwise equal but differed only on those fields. (#4350, @tsaarni)
- New field `HTTPProxy.spec.routes.timeoutPolicy.idleConnection` was added. The field sets timeout for how long the upstream connection will be kept idle between requests before disconnecting it. (#4356, @tsaarni)
- Update github.com/prometheus/client_golang to v1.11.1 to address CVE-2022-21698. (#4361, @tsaarni)
- Envoy's [`merge_slashes`](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-merge-slashes) option that enables
a non-standard path transformation option to replace multiple consecutive slashes in an URL path with a single slash can now be disabled by setting the `DisableMergeSlashes` option in the Contour config file or ContourConfiguration custom resource. (#4363, @mszabo-wikia)
- Updates Envoy to v1.21.1. See the [Envoy changelog](https://www.envoyproxy.io/docs/envoy/v1.21.1/version_history/current) for details. (#4365, @skriss)
- Add base implementation for RTDS (Runtime Discovery Service). This will be used to enable dynamic configuration of Envoy Runtime settings. (#4380, @sunjayBhatia)
- Ensure controller-runtime logging is properly configured to log to Contour's logrus Logger instance. (#4391, @sunjayBhatia)
- Adds an optional `--name-prefix` flag to the `contour certgen` command which, if specified, will be added as a prefix to the names of the generated Kubernetes secrets (e.g. `myprefix-contourcert` and `myprefix-envoycert`). (#4394, @skriss)
- Moved all usages of header_match and exact_match with string_match (#4397, @rajatvig)
- Use the protocol field from the Cluster when performing the health check (#4398, @rajatvig)
- Removed the hack for ImagePullPolicy for certgen (#4402, @rajatvig)
- internal/envoy: Enable gzip compression for grpc-web content types. (#4403, @bourquep)
- In the example manifests, leave `imagePullPolicy` as `Always` on main branch and only change to `IfNotPresent` on release branches/release-tagged manifests. (#4406, @rajatvig)
- Upgrade to Go 1.18.0. (#4412, @skriss)
- Add grpc_stats filter for Envoy
Add the ability to log "grpc_status" to the Envoy access log (#4424, @rajatvig)
- Gateway provisioner: set the GatewayClass "Accepted" condition based on the validity of its parametersRef, if it has one. Also only reconciles Gateways for GatewayClasses with "Accepted: true". (#4440, @skriss)
- The Gateway provisioner now provisions a `ContourConfiguration` resource instead of a `ConfigMap` for describing Contour's configuration. (#4454, @skriss)
- Uses the `ContourConfigurationSpec` defined as part of a `GatewayClass's` `ContourDeployment` parameters when provisioning a `ContourConfiguration` for a `Gateway`. (#4459, @skriss)
- Gateway API: set appropriate conditions on Listeners if they don't specify the same port as other Listeners for their protocol group (i.e. HTTP, or HTTPS/TLS) or don't have a unique hostname within their group. (#4462, @skriss)
- Add a example to show how to do blue-green deployment under Gateway-API mode (#4466, @izturn)
- Fix improper use of OriginalIPDetectionFilter in HTTPConnectionManager. Reverts back to XffNumTrustedHops setting which was un-deprecated in Envoy 1.20. (#4470, @sunjayBhatia)
- Gateway provisioner: change default controller name to `projectcontour.io/gateway-controller`. (#4474, @skriss)
- Gateway API: when an `HTTPRoute` or `TLSRoute` has a cross-namespace backend ref that's not permitted by a `ReferencePolicy`, set the reason for the `ResolvedRefs: false` condition to `RefNotPermitted` instead of `Degraded`. (#4482, @skriss)
- Add support for Contour to produce logs in JSON format by specifying `--log-format=json` command line switch. (#4486, @tsaarni)
- Use typed config for all Envoy extensions in place of well-known names or internal type URL constants, for consistency and forwards-compatibility. (#4487, @skriss)
- Updates to Envoy 1.22.0. See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.22.0/version_history/current) for more information. (#4488, @skriss)
- Updates Gateway API to v0.4.3 and adds the Gateway API validating webhook to Contour's Gateway API example YAML. (#4489, @skriss)
- Gateway API: adjusts logic for finding intersecting hostnames between a Listener and a Route to ignore non-matching hosts rather than reporting an error for them. (#4505, @skriss)
- Upgrade to Go 1.18.1. (#4509, @sunjayBhatia)
- Remove `ContourConfiguration` kubebuilder enum validations, and add equivalent validations in Contour code. (#4511, @skriss)
- Gateway API: fixes a bug where a route would be marked "Accepted: false" with reason "NoIntersectingHostnames" if it did not have intersecting hostnames with _every_ Listener. Now, as long as the route's hostnames intersect with at least one Listener, it's accepted. (#4512, @skriss)


# Docs Changes
- The AWS NLB deployment guide has been updated, and the annotations `service.beta.kubernetes.io/aws-load-balancer-type` has been change to `external`. It should now work correctly with the given YAMLs. (#4347, @yankay)
- Added documentation for HTTPProxy request redirection. (#4367, @sunjayBhatia)
- Add `pathType` field to Ingress resource. (#4446, @lou-lan)


# Deprecation and Removal Notices


## Remove leader election configuration from configuration file

Leader election configuration via configuration file was deprecated in Contour v1.20.0.
Configuration of leader election lease details and resource must now be done via command line flag.

(#4340, @sunjayBhatia)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.21.0 is tested against Kubernetes 1.21 through 1.23.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @alessandroargentieri
- @bourquep
- @erwbgy
- @izturn
- @kb000
- @lou-lan
- @mszabo-wikia
- @rajatvig
- @yankay


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
