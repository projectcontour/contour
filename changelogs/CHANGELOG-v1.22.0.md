We are delighted to present version v1.22.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

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

## Update to Gateway API v0.5.0

Contour now supports Gateway API v0.5.0, including both the v1alpha2 and v1beta1 API versions.

With this update, Contour passes all of the Gateway API v0.5.0 conformance tests, which cover much of the core API surface (but are not yet 100% exhaustive).

For more information on the Gateway API v0.5.0 release, see [the release blog post](https://gateway-api.sigs.k8s.io/blog/2022/graduating-to-beta/).

For information on getting started with Contour and Gateway API, see the [Contour/Gateway API guide](https://projectcontour.io/guides/gateway-api/).

(#4617, @skriss)


# Minor Changes

## Update to Envoy 1.23.0

Contour now uses Envoy 1.23.0.
See [the Envoy changelog](https://www.envoyproxy.io/docs/envoy/v1.23.0/version_history/v1.23/v1.23.0) for more information on the contents of the release.

(#4621, @skriss)

## HTTPProxy Direct Response Policy

HTTPProxy.Route now has a HTTPDirectResponsePolicy which allows for routes to specify a DirectResponsePolicy.
This policy will allow a direct response to be configured for a specific set of Conditions within a single route.
The Policy can be configured with a `StatusCode`, `Body`. And the `StatusCode` is required.

It is important to note that one of route.services or route.requestRedirectPolicy or route.directResponsePolicy must be specified.

(#4526, @yangyy93)

## Validating revocation status of client certificates

It is now possible to enable revocation check for client certificates validation.
The CRL files must be provided in advance and configured as opaque Secret.
To enable the feature, `httpproxy.spec.virtualhost.tls.clientValidation.crlSecret` is set with the secret name.

(#4592, @tsaarni)

## Consolidate access logging and TLS cipher suite validation

Access log and TLS cipher suite configuration validation logic is now consolidated in the `apis/projectcontour/v1alpha1` package.
Existing exported elements of the `pkg/config` package are left untouched, though implementation logic now lives in `apis/projectcontour/v1alpha1`.

This should largely be a no-op for users however, as part of this cleanup, a few minor incompatible changes have been made:
- TLS cipher suite list elements will no longer be allowed to have leading or trailing whitespace
- The ContourConfiguration CRD field `spec.envoy.logging.jsonFields` has been renamed to `spec.envoy.logging.accessLogJSONFields`

(#4626, @sunjayBhatia)

## Gateway API: implement HTTP query parameter matching

Contour now implements Gateway API's [HTTP query parameter matching](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.HTTPQueryParamMatch).
Only `Exact` matching is supported.
For example, the following HTTPRoute will send a request with a query string of `?animal=whale` to `s1`, and a request with a querystring of `?animal=dolphin` to `s2`.

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: httproute-queryparam-matching
spec:
  parentRefs:
  - name: contour-gateway
  rules:
  - matches:
    - queryParams:
      - type: Exact
        name: animal
        value: whale
    backendRefs:
    - name: s1
  - matches:
    - queryParams:
      - type: Exact
        name: animal
        value: dolphin
    backendRefs:
    - name: s2
```

(#4588, @skriss)

## Gateway API: update handling of various invalid HTTPRoute/TLSRoute scenarios

Updates the handling of various invalid HTTPRoute/TLSRoute scenarios to be conformant with the Gateway API spec, including:

- Use a 500 response instead of a 404 when a route's backends are invalid
- The `Accepted` condition on a route only describes whether the route attached successfully to its parent, not whether it has any other errors
- Use the upstream reasons `InvalidKind` and `BackendNotFound` when a backend is not a Service or not found, respectively

(#4614, @skriss)

## Gateway API: enforce correct TLS modes for HTTPS and TLS listener protocols

Contour now enforces that the correct TLS modes are used for the HTTPS and TLS listener protocols.
For an HTTPS listener, the TLS mode "Terminate" must be used (this is compatible with HTTPRoutes).
For a TLS listener, the TLS mode "Passthrough" must be used (this is compatible with TLSRoutes).

(#4631, @skriss)

##  Bind create label operation for contour's deployment to the struct

There are now three places to create the same label(s), so let the operation to be a method of the Contour struct.

(#4585, @izturn)

## Use local variable to replace the long access chain of fields

The access chain of fields is too long, so use local variable to replace them.

(#4586, @izturn)


# Other Changes
- RTDS now serves dynamic runtime configuration layer which is requested by bootstrap configuration. In the future, contents of this runtime configuration will be made configurable by users. (#4387, @sunjayBhatia)
- internal/envoy: use Envoy's path-based prefix matching instead of regular expressions. (#4477, @mmalecki)
- Gateway API: compute Listener supported kinds sooner, so it's populated in all cases where it can be computed. (#4523, @skriss)
- When validating secrets, don't log an error for an Opaque secret that doesn't contain a `ca.crt` key. (#4528, @skriss)
- Removes the `DebugLogLevel` and `KubernetesDebugLogLevel` fields from the `ContourConfiguration` spec since they were unused and are required to be specified via CLI flag. (#4534, @skriss)
- Fixes TLS private key validation logic which previously ignored errors for PKCS1 and PKCS8 private keys. (#4544, @sunjayBhatia)
- Gateway API: return a 404 instead of a 503 when there are no valid backend refs for an HTTPRoute rule, to match the [revised Gateway API spec](https://github.com/kubernetes-sigs/gateway-api/pull/1151). (#4545, @skriss)
- Update supported Kubernetes versions to 1.22, 1.23 and 1.24. (#4546, @skriss)
- Changes the `contour envoy shutdown` command's `--check-delay` default to `0s` from `60s`, allowing Envoy pods to shut down more quickly when there are no open connections. (#4548, @skriss)
- Update gopkg.in/yaml.v3 to v3.0.1 to address CVE-2022-28948. (#4551, @tsaarni)
- Gateway API: adds support for the "RequestMirror" HTTPRoute filter type at the rule level. (#4557, @sepaper)
- Gateway API: fixes a bug where routes with multiple parent refs to listeners would not attach to all listeners correctly. (#4558, @skriss)
- Gateway API: wildcard hostnames can now match more than one DNS label, per https://github.com/kubernetes-sigs/gateway-api/pull/1173. (#4559, @skriss)
- Gateway API: adds support for ReferenceGrant, which was formerly known as ReferencePolicy. To ease migration, _both_ resources are supported for this release, but ReferencePolicy support will be removed next release. (#4580, @skriss)
- Envoy will now make requests to gRPC ExtensionServices with a sanitized `:authority` header, rather than just using the extension cluster name. (#4587, @sunjayBhatia)
- Gateway API: adds logic to only keep the first HTTP header match with a given name (case-insensitive) for each HTTP route match, per the Gateway API spec. (#4593, @skriss)
- Gateway API: replace usage of Contour-specific condition types and reasons with upstream Gateway API ones where possible (#4598, @skriss)
- `contour cli` commands have been updated with new logging and support for testing incremental (delta) xDS variants. (#4602, @youngnick)
- Gateway API: sets route parent status correctly when routes attach to specific Listeners. (#4604, @skriss)
- Updated the list of supported envoy log template keywords. (#4610, @yangyy93)
- Gateway API: set a Listener condition of `Ready: false` with reason `Invalid` when a Listener allows routes from a namespace selector but the selector is invalid. (#4615, @skriss)
- Adds support for access log operators introduced in Envoy 1.23.0. See [here](https://www.envoyproxy.io/docs/envoy/v1.23.0/version_history/v1.23/v1.23.0#new-features) for more details. (#4627, @sunjayBhatia)


# Docs Changes
- Updated SITE_CONTRIBUTION.md to reflect Hugo platform. (#4620, @gary-tai)
- Remove grey banner from main website page. (#4635, @gary-tai)


# Deprecation and Removal Notices

## Gateway API: ReferencePolicy is deprecated, will be removed next release

Gateway API has renamed ReferencePolicy to ReferenceGrant in the v0.5.0 release, while retaining the former for one release to ease migration.
Contour currently supports both, but will drop support for ReferencePolicy in the next release.
Users of ReferencePolicies must migrate their resources to ReferenceGrants ahead of the next Contour release.

(#4580, @skriss)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.22.0 is tested against Kubernetes 1.22 through 1.24.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @gary-tai
- @izturn
- @mmalecki
- @sepaper
- @yangyy93


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
