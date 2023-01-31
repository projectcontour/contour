We are delighted to present version v1.24.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

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

## Fix handling of duplicate HTTPProxy Include Conditions

Duplicate include conditions are now correctly identified and HTTPProxies are marked with the condition `IncludeError` and reason `DuplicateMatchConditions`.
Previously the HTTPProxy processor was only comparing adjacent includes and comparing conditions element by element rather than as a whole, ANDed together.

In addition, the previous behavior when duplicate Include Conditions were identified was to throw out all routes, including valid ones, on the offending HTTPProxy.
Any referenced child HTTPProxies were marked as `Orphaned` as a result, even if they were included correctly.
With this change, all valid Includes and Route rules are processed and programmed in the data plane, which is a difference in behavior from previous releases.
An Include is deemed to be a duplicate if it has the exact same match Conditions as an Include that precedes it in the list.
Only child HTTPProxies that are referenced by a duplicate Include and not in any other valid Include are marked as `Orphaned`

### Caveat for empty or individual prefix path matches on `/`

A caveat to the above, is that an empty list of include conditions or a set of conditions that only consist of the prefix match on `/` are not treated as duplicates.

This special case has been added because many users rely on the behavior this enables and many Contour examples demonstrating inclusion actually use it.
For example:

```
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
spec:
  virtualhost:
    fqdn: foo-example.bar.com
  includes:
  - name: example-child1
  - name: example-child2
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child1
spec:
  routes:
    - conditions:
      - prefix: /
      services:
      - name: s1
        port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child2
spec:
  routes:
  - conditions:
    - prefix: /foo
    services:
    - name: s2
      port: 80
```

While the include conditions are equivalent, the resulting routing rules when the child routes are taken into account are distinct.

This special casing is a stop-gap for this release, to ensure we do not break user's configuration that is currently valid and working.

### Future changes to inclusion and route duplicate detection

Currently duplicate route conditions are not checked in an HTTPProxy include tree or within an individual HTTPProxy.
This means that you can have routes listed later in the list of routes on an HTTPProxy silently override others.
The same can happen if you have an include tree that generates duplicate routes based on the include conditions and route conditions.

If you are relying on this behavior, changes will be coming in the next Contour release.

We will be submitting a design document to address this as it will be a significant behavior change and encourage the community to weigh in.
The current plan is to fully validate duplicate route match conditions as they are generated from the tree of includes and routes.
There will likely be changes to status conditions set on HTTPRoutes to improve reporting such invalid configuration.

(#4931, #5017, @sunjayBhatia)

## Contour supports Gateway API release v0.6.0

See [the Gateway API release notes](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.6.0) for more detail on the API changes.
This version of the API includes a few changes relevant to Contour users:
- The ReferenceGrant resource has been graduated to the v1beta1 API and ReferencePolicy removed from the API
- v1alpha2 versions of GatewayClass, Gateway, and HTTPRoute are deprecated
- There have been significant changes to status conditions on various resources for consistency:
  - Accepted and Programmed conditions have been added to Gateway and Gateway Listener
  - The Ready condition has been moved to "extended" conformance, at this moment Contour does not program this condition
  - The Scheduled condition has been deprecated on Gateway

(#4944, @sunjayBhatia)

## shutdown-manager sidecar container liveness probe removed

The liveness probe has been removed from the Envoy pods' shutdown-manager sidecar container.
This change is to mitigate a problem where when the liveness probe fails, the shutdown-manager container is restarted by itself.
This ultimately has the unintended effect of causing the envoy container to be stuck indefinitely in a "DRAINING" state and not serving traffic.
    
Overall, not having the liveness probe on the shutdown-manager container is less bad because envoy pods are less likely to get stuck in "DRAINING" indefinitely. 
In the worst case, during termination of an Envoy pod (due to upgrade, scaling, etc.), shutdown-manager is truly unresponsive, in which case the envoy container will simply terminate without first draining active connections.
If appropriate (i.e. during an upgrade), a new Envoy pod will then be created and re-added to the set of ready Envoys to load balance traffic to.

(#4967, @skriss)


# Minor Changes

## Add optional health check port for HTTP health check and TCP health check

HTTPProxy.Route.Service and HTTPProxy.TCPProxy.Service now has an optional `HealthPort` field which specifies a health check port that is different from the routing port. If not specified, the service `Port` field is used for healthchecking.

(#4761, @yangyy93)

## Secrets not relevant to Contour no longer validated

Contour no longer validates Secrets that are not used by an Ingress, HTTPProxy, Gateway, or Contour global config.
Validation is now performed as needed when a Secret is referenced.
This change also replaces misleading "Secret not found" error conditions with more specific errors when a Secret referenced by one of the above objects does exist, but is not valid.

(#4788, @skriss)

## Optional Client Certificate Validation

By default, when client certificate validation is configured, client certificates are required.
However, some applications might support different authentication schemes.
You can now set the `httpproxy.spec.virtualhost.tls.clientValidation.optionalClientCertificate` field to `true`. A client certificate will be requested, but the connection is allowed to continue if the client does not provide one.
If a client certificate is sent, it will be verified according to the other properties, which includes disabling validations if `httpproxy.spec.virtualhost.tls.clientValidation.skipClientCertValidation` is set.

(#4796, @gautierdelorme)

## Client Certificate Details Forwarding

HTTPProxy now supports passing certificate data through the `x-forwarded-client-cert` header to let applications use details from client certificates (e.g. Subject, SAN...).
Since the certificate (or the certificate chain) could exceed the web server header size limit, you have the ability to select what specific part of the certificate to expose in the header through the `httpproxy.spec.virtualhost.tls.clientValidation.forwardClientCertificate` field.
Read more about the supported values in the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#x-forwarded-client-cert).

(#4797, @gautierdelorme)

## Enable configuring Server header transformation 

Envoy's treatment of the Server header on responses can now be configured in the Contour config file or ContourConfiguration CRD.
When configured as `overwrite`, Envoy overwrites any Server header with "envoy".
When configured as `append_if_absent`, ⁣if a Server header is present, Envoy will pass it through, otherwise, it will set it to "envoy".
When configured as `pass_through`, Envoy passes through the value of the Server header and does not append a header if none is present.

(#4906, @Vishal-Chdhry)

## Added support for `ALL` DNS lookup family.

If `ALL` is specified, the DNS resolver will perform a lookup for both IPv4 and IPv6 families, and return all resolved addresses. When this is used, Happy Eyeballs will be enabled for upstream connections.

(#4909, @Vishal-Chdhry)

## Update Envoy to v1.25.0

Bumps Envoy to version 1.25.0.
See Envoy release notes [here](https://www.envoyproxy.io/docs/envoy/v1.25.0/version_history/v1.25/v1.25.0).

(#4988, @skriss)

## Kubernetes API client queries per second (QPS) and burst now configurable

Contour's Kubernetes API client defaults to allowing 5 requests per second, with a maximum of 10 over a short period.
These settings can now be configured, either by flag or by config file.
The `contour serve` flags are `--kubernetes-client-qps` and `--kubernetes-client-burst`.
The config file fields are `kubernetesClientQPS` and `kubernetesClientBurst`.

(#5003, @skriss)


# Other Changes
- Add (update)Strategy configurability to ContourDeployment resource for components. (#4713, @izturn)
- Don't trigger DAG rebuilds for updates/deletes of unrelated Secrets. (#4792, @skriss)
- Allow TLS certificate secrets to be of type `Opaque` as long as they have valid `tls.crt` and `tls.key` entries. (#4799, @skriss)
- Add Envoy log level configurability to ContourDeployment resource. (#4801, @izturn)
- Add Service/Envoy's ExternalTrafficPolicy configurability to ContourDeployment resource. (#4803, @izturn)
- Deprecated Envoy xDS APIs from Envoy 1.24.0 are no longer in use, see [here](https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.24/v1.24.0#deprecated) for details on their replacements. (#4822, @sunjayBhatia)
- Don't trigger DAG rebuilds for updates/deletes of unrelated Services. (#4827, @Vishal-Chdhry)
- Fixed bug where ExtensionServices were being updated continuously by Contour (#4846, @Vishal-Chdhry)
- Add `grpc_status_number` to the default JSON access log fields (#4880, @rajatvig)
- Implement support for Gateway API HTTPRoute ResponseHeaderModifier filter. (#4908, @sunjayBhatia)
- Don't trigger DAG rebuilds/xDS configuration updates for irrelevant HTTPRoutes and TLSRoutes. (#4912, @fangfpeng)
- Ensure changes to Services referenced by TLSRoute trigger xDS configuration updates. (#4915, @vmw-yingy)
- Supported/tested Kubernetes versions are now 1.24, 1.25, 1.26. (#4937, @skriss)
- Sort and ensure the option flags in lexicographic order, fix [#2397](https://github.com/projectcontour/contour/issues/2397) (#4958, @izturn)
- Gateway API: adds support for the [HTTPURLRewrite filter](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.HTTPURLRewriteFilter), which allows for rewriting the path or Host header as requests are being forwarded to the backend. (#4962, @skriss)
- expose configuration for envoy's RateLimitedAsResourceExhausted (#4971, @vroldanbet)
- Gateway API: when provisioning an Envoy `NodePortService`, use the Listeners' port numbers to populate the Service's node port values. (#4973, @izturn)
- Updates to Go 1.19.5. See the [Go release notes](https://go.dev/doc/devel/release#go1.19.minor) for more information. (#4980, @skriss)
- Improve xDS server logging on connection close to be less verbose by default. Previously all closed connections from Envoy xDS resource subscriptions were logged as errors. (#4993, @sunjayBhatia)
- Adds a new summary metric, `contour_dagrebuild_seconds`, to measure the duration of DAG rebuilds by quantile. (#5009, @skriss)
- Fixes a bug where the CLI did not have its Logger initialized which was resulting in a panic. (#5015, @skriss)


# Docs Changes
- Update [FIPS 140-2 in Contour](https://projectcontour.io/guides/fips/) for Go 1.19+. (#4813, @moeyui1)
- Added a section to the [Deployment Options](https://projectcontour.io/docs/main/deploy-options/) document describing how to deploy more than one Contour instance in a single cluster. (#4832, @skriss)
- Add get involved section in front page of the project contour. (#4847, @theVJagrawal)
- Gateway API: change the default controller name from `projectcontour.io/projectcontour/contour` to `projectcontour.io/gateway-controller` for static provisioning. (#4966, @izturn)
- Guides are now versioned along with the rest of Contour's documentation. You can find them listed in the menu on the left-hand side of https://projectcontour.io/docs. (#4977, @skriss)


# Deprecation and Removal Notices


# ContourDeployment.Spec.Contour.Replicas and ContourDeployment.Spec.Envoy.Replicas are deprecated

- `ContourDeployment.Spec.Contour.Replicas` is deprecated and has been replaced by `ContourDeployment.Spec.Contour.Deployment.Replicas`. Users should switch to using the new field. The deprecated field will be removed in a future release. See #4713 for additional details.

- `ContourDeployment.Spec.Envoy.Replicas` is deprecated and has been replaced by `ContourDeployment.Spec.Envoy.Deployment.Replicas`. Users should switch to using the new field. The deprecated field will be removed in a future release. See #4713 for additional details.

(#4713, @izturn)

## Gateway API: ReferencePolicy no longer supported (use ReferenceGrant instead)

In Gateway API, ReferencePolicy's rename to ReferenceGrant has been fully completed.
Contour now only supports ReferenceGrant, and does not support ReferencePolicy resources in any way.

(#4830, @skriss)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.24.0 is tested against Kubernetes 1.24 through 1.26.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @Vishal-Chdhry
- @fangfpeng
- @gautierdelorme
- @izturn
- @moeyui1
- @rajatvig
- @theVJagrawal
- @vmw-yingy
- @vroldanbet
- @yangyy93


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
