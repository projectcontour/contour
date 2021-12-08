We are delighted to present the first beta for Contour v1.20.0, our layer 7 HTTP reverse proxy for Kubernetes clusters.

**Please note that this is pre-release software**, and as such we do not recommend installing it in production environments.
Feedback and bug reports are welcome!

# Major Changes

## Gateway API v1alpha2 support

Contour now exclusively supports Gateway API v1alpha2, the latest available version.
This version of Gateway API has a number of breaking changes, which are detailed in [the Gateway API changelog](https://github.com/kubernetes-sigs/gateway-api/blob/master/CHANGELOG.md).
Contour currently supports a single `GatewayClass` and associated `Gateway`, and `HTTPRoutes` and `TLSRoutes` that attach to the `Gateway`. `TCPRoute` and `UDPRoute` are **not** supported.
For a list of other functionality that remains to be implemented, see Contour's [area/gateway-api](https://github.com/projectcontour/contour/labels/area%2Fgateway-api) label.

As part of this change, support for Gateway API v1alpha1 has been dropped, and any v1alpha1 resources **will not** be automatically converted to v1alpha2 resources because the API has moved to a different API group (from `networking.x-k8s.io` to `gateway.networking.k8s.io`).

(#4047, @skriss)

## xDS management connection between Contour and Envoy set to TLSv1.3

The minimum accepted TLS version for Contour xDS server is changed from TLSv1.2 to TLSv1.3.
Previously in Contour 1.19, the maximum accepted TLS version for Envoy xDS client was increased to TLSv1.3 which allows it to connect to Contour xDS server using TLSv1.3.

If upgrading from a version **prior to Contour 1.19**, the old Envoys will be unable to connect to new Contour until also Envoys are upgraded.
Until that, old Envoys are unable to receive new configuration data.

For further information, see [Contour architecture](https://projectcontour.io/docs/main/architecture/) and [xDS API](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol) in Envoy documentation.

(#4065, @tsaarni)

# Minor Changes

## Metrics over HTTPS

Both Envoy and Contour metrics can now be served over HTTPS.
Server can alternatively also require client to present certificate which is validated against configured CA certificate.
This feature makes it possible to limit the visibility of metrics to authorized clients.

(#3707, @tsaarni)

## Performance improvement for processing configuration

The performance of Contour's configuration processing has been made more efficient, particularly for clusters with large numbers (i.e. >1k) of HTTPProxies and/or Ingresses.
This means that there should be less of a delay between creating/updating an HTTPProxy/Ingress in Kubernetes, and having it reflected in Envoy's configuration.

(#4099, @skriss)

## Allow retry policy, num retries to be zero 

The field, NumRetries (e.g. count), in the RetryPolicy allows for a zero to be
specified, however Contour's internal logic would see that as "undefined"
and set it back to the Envoy default of 1. This would never allow the value of 
zero to be set. Users can set the value to be -1 which will represent disabling 
the retry count. If not specified or set to zero, then the Envoy default value 
of 1 is used.

(#4117, @stevesloka)

## Gateway API: implement PathPrefix matching

Contour now implements Gateway API v1alpha2's "path prefix" matching for `HTTPRoutes`.
This is now the only native form of prefix matching supported by Gateway API, and is a change from v1alpha1.
Path prefix matching means that the prefix specified in an `HTTPRoute` rule must match entire segments of a request's path in order to match it, rather than just be a string prefix.
For example, the prefix `/foo` would match a request for the path `/foo/bar` but not `/foobar`.
For more information, see the [Gateway API documentation](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.PathMatchType).

(#4119, @skriss)

## Gateway API: support ReferencePolicy

Contour now supports the `ReferencePolicy` CRD in Gateway API v1alpha2.
`ReferencePolicy` enables certain cross-namespace references to be allowed in Gateway API.
The primary use case is to enable routes (e.g. `HTTPRoutes`, `TLSRoutes`) to reference backend `Services` in different namespaces.
When Contour processes a route that references a service in a different namespace, it will check for a `ReferencePolicy` that applies to the route and service, and if one exists, it will allow the reference.

(#4138, @skriss)

## Gateway API: set Gateway Listener status fields

Contour now sets the `.status.listeners.supportedKinds` and `.status.listeners.attachedRoutes` fields on Gateways for Gateway API.
The first describes the list of route groups/kinds that the listener supports, and the second captures the total number of routes that are successfully attached to the listener.

(#4160, @skriss)

## Set Gateway listener conditions

Contour now sets various Gateway listener conditions as it processes them, including the "Ready", "Detached", and "ResolvedRefs" condition types, to provide more visibility to the user as to whether their listeners are defined correctly or not.

(#4186, @skriss)

## Default status on HTTPProxy resources

When a new HTTPProxy is created, if Contour isn't yet running or
functioning properly, then no status is set on the resource. 
Defaults of "NotReconciled/Waiting for controller" are now applied 
to any new object until an instance of Contour accepts the
object and updates the status.

(#4133, @stevesloka)

## Source IP hash based load balancing

Contour users can now configure their load balancing policies on `HTTPProxy` resources to hash the source IP of a client to ensure consistent routing to a backend service instance. Using this feature combined with header value hashing can implement advanced request routing and session affinity. Note that if you are using a load balancer to front your Envoy deployment, you will need to ensure it preserves client source IP addresses to ensure this feature is effective.

See [this page](https://projectcontour.io/docs/main/config/request-routing/#load-balancing-strategy) for more details on this feature.

(#4141, @sunjayBhatia)

## TLS Certificate validation updates

Contour now allows non-server certificates that do not have a CN or SAN set, which mostly fixes
[#2372](https://github.com/projectcontour/contour/issues/2372) and [#3889](https://github.com/projectcontour/contour/issues/3889).

TLS documentation has been updated to make the rules for Secrets holding TLS information clearer.

Those rules are:

For certificates that identify a server, they must:
- be `kubernetes.io/tls` type
- contain `tls.crt`, and `tls.key` keys with the server certificate and key respectively.
- have the first certificate in the `tls.crt` bundle have a CN or SAN field set.

They may:
- have the `tls.crt` key contain a certificate chain, as long as the first certificate in the chain is the server certificate.
- add a `ca.crt` key that contains a Certificate Authority (CA) certificate or certificates.

Certificates in the certificate chain that are not server certificates do not need to have a CN or SAN.

For CA secrets, they must:
- be `Opaque` type
- contain only a `ca.crt` key, not `tls.crt` or `tls.key`

The `ca.crt` key may contain one or more CA certificates, that do not need to have a CN or SAN.

(#4165, @youngnick)

## HTTPProxy TCPProxy service weights are now applied

Previously, Contour did not apply any service weights defined in an HTTPProxy's TCPProxy, and all services were equally weighted.
Now, if those weights are defined, they are applied so traffic is weighted appropriately across the services.
Note that if no TCPProxy service weights are defined, traffic continues to be equally spread across all services.

(#4169, @skriss)

## Leader Election Configuration

`contour serve` leader election configuration via config file has been deprecated.
The preferred way to configure leader election parameters is now via command line flags.
See [here](https://projectcontour.io/docs/main/configuration/#serve-flags) for more detail on the new leader election flags.

*Note:* If you are using the v1alpha1 ContourConfiguration CRD, leader election configuration has been removed from that CRD as well.
Leader election configuration is not something that will be dynamically configurable once Contour implements configuration reloading via that CRD.

(#4171, @sunjayBhatia)

## Transition to controller-runtime managed leader election

Contour now utilizes [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) Manager based leader election and coordination of subroutines.
With this change, Contour is also transitioning away from using a ConfigMap for leader election.
In this release, Contour now uses a combination of ConfigMap and Lease object.
A future release will remove usage of the ConfigMap resource for leader election.

This change should be a no-op for most users, however be sure to re-apply the relevant parts of your deployment for RBAC to ensure Contour has access to Lease and Event objects (this would be the ClusterRole in the provided example YAML).

(#4202, @sunjayBhatia)

# Other Changes
- Sets conditions of "Accepted: false" and "ValidBackendRefs: false" on `TLSRoutes` when all backend refs have a weight of 0 explicitly set. (#4027, @skriss)
- Fix panic in Contour startup when using `--root-namespaces` flag (#4110, @sunjayBhatia)
- Gateway API: adds support for HTTP method matching in `HTTPRoute` rules. See the [Gateway API documentation](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.HTTPRouteMatch) for more information. (#4120, @skriss)
- Gateway API: adds support for the "RequestRedirect" HTTPRoute filter type at the rule level. (#4123, @skriss)
- Update to using Envoy bootstrap Admin [`access_log` field](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/accesslog/v3/accesslog.proto#envoy-v3-api-msg-config-accesslog-v3-accesslog) instead of deprecated `access_log_path` (deprecated in Envoy v1.18.0) (#4142, @sunjayBhatia)
- Update to using Envoy [XFF Original IP Detection extension](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/http/original_ip_detection/xff/v3/xff.proto) instead of HTTPConnectionManager `xff_num_trusted_hops` field (deprecated in Envoy v1.19.0) (#4142, @sunjayBhatia)
- HTTPProxy resources now support wildcard fqdn's in the form `*.projectcontour.io`. (#4145, @stevesloka)
- Timeout for upstream network connection timeout increased from 250 msec to 2 seconds. (#4151, @tsaarni)
- Fix accidental negation of disableAllowChunkedLength configuration flag. (#4152, @sunjayBhatia)
- Replaces the use of the dynamic Kubernetes client with the controller-runtime client. (#4153, @skriss)
- Gateway API: Contour no longer sets up RBAC for TCPRoutes and UDPRoutes or watches them, since they are not supported. (#4166, @skriss)
- Very long (~100 characters) Ingress path prefix matches should now no longer be rejected by Envoy. See [this issue](https://github.com/projectcontour/contour/issues/4191) for context. (#4197, @sunjayBhatia)
- Removes spec.ttlSecondsAfterFinished from certgen job in versioned releases, as immediately deleting it upon completion will not be useful for most consumers. (#4200, @lrewega)
- Gateway API: set an HTTPRoute condition of "ValidMatches: false" when a path match does not start with '/' or contains consecutive '/' characters. (#4209, @skriss)
- Gateway API: allow Gateways to reference TLS certificates in other namespaces when an applicable ReferencePolicy is defined. See [the Gateway API documentation](https://gateway-api.sigs.k8s.io/v1alpha2/guides/tls/#cross-namespace-certificate-references) for more information. (#4212, @skriss)


# Docs Changes
- Pare down docs versions available in site dropdown. (#4020, @sunjayBhatia)
- Updates the cert-manager guide to use the latest versions of Contour and cert-manager as well as Ingress v1 resources. (#4115, @skriss)
- Adds a Gateway API v1alpha2 guide. (#4122, @skriss)
- The [Contour deprecation policy](https://projectcontour.io/resources/deprecation-policy/) for Alpha APIs has been updated to be explicitly more lenient in regards to behavior changes and field removal. A new API version is not strictly required when making such changes. (#4173, @sunjayBhatia)

# Installing

The simplest way to install v1.20.0-beta.1 is to apply one of the example configurations:

With Gateway API:
```bash
kubectl apply -f https://github.com/projectcontour/contour/blob/v1.20.0-beta.1/examples/render/contour-gateway.yaml
```

Without Gateway API:
```bash
kubectl apply -f https://github.com/projectcontour/contour/blob/v1.20.0-beta.1/examples/render/contour.yaml
```

## Compatible Kubernetes Versions

Contour v1.20.0-beta.1 is tested against Kubernetes 1.20 through 1.22

## Documentation

Documentation corresponding to `v1.20.0-beta.1` can be found at https://projectcontour.io/docs/main/.


# Are you a Contour user? We would love to know!

If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
