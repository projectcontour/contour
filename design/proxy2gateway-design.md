# proxy2gateway: Tool for converting HTTPProxy resources to Gateway API

Status: Draft

## Abstract
This design proposes a new command line tool `proxy2gateway` for converting Contour HTTPProxy resources to Gateway API resources (HTTPRoutes, TLSRoutes, and possibly Gateways).

## Background
As [Gateway API](https://gateway-api.sigs.k8s.io/) becomes more mature, moves to GA, is used more widely, and supports new and exciting features, Contour users will need support to convert their existing HTTPProxy resources to Gateway API resources.
The `proxy2gateway` tool intends to provide this functionality, in the same vein as [`ir2proxy`](https://github.com/projectcontour/ir2proxy) did for converting IngressRoute to HTTPProxy resources.
The Contour project supporting migration of HTTPProxy resources should help with adoption of Gateway API, the Contour data-plane controller, and the Contour Gateway provisioner as well as hopefully driving interoperability with other implementations of the Gateway API.

## Goals
- Provide a simple command line tool that users will be able to use to convert HTTPProxy resources equivalent to Gateway API resources that can be applied to a cluster and largely "just work"
- For users new to the Gateway API, set up converted resources in an idiomatic manner
- Start work on the `proxy2gateway` tool early so we have a working tool when Gateway API goes GA
- As the Gateway API spec expands to cover more features that HTTPProxy covers, `proxy2gateway` will stay up to date and release new versions to support these

## Non Goals
- `proxy2gateway` (at least to start) will not provide unlimited customizability for how routes and Gateways interact in order to reduce complexity
- `proxy2gateway` will not fetch resources directly from the API server

## High-Level Design
`proxy2gateway` will be a command line tool that converts HTTPProxy resources and some user input via command line options to produce the equivalent Gateway API route and possibly Gateway resources.
The tool will ingest HTTPProxy resources in a file and output the converted resources to standard output.
The tool may also take a base Gateway resource via a file or Gateway name via command line flag in order to set up the appropriate relationship between Gateways and routes.
This may include setting up Gateway Listeners to select routes and set up TLS.
`proxy2gateway` will output a Gateway that results from the input of base Gateway and HTTPProxies.

HTTPProxy resources using standard HTTP route mechanisms for configuring ingress traffic will be converted to Gateway API HTTPRoute resources.
HTTPProxy resources using TCPProxy configuration in passthrough or terminate modes will be converted to TLSRoute resources.
In the case that any HTTPProxy uses TLS, a secure Listener will be set up on the output Gateway.

HTTPProxies utilizing features that are not yet supported by Gateway API (e.g. using rate limiting, inclusion, or load balancing policies) will be ignored and a warning printed specifying the reason they were not converted.
For features that are supported in future versions of Contour and/or the Gateway API, newer versions of `proxy2gateway` will be released that support up to date conversions.
This may be a large set of features initially, but getting the groundwork started for this tool early should help the burden of work later.

Some validation of input HTTPProxy resources and base Gateway will be required, eventually hopefully in the form of a shared library from the main `projectcontour/contour` codebase and/or upstream Gateway API codebase.

## Detailed Design

### Command line flags
- `--http-proxies`: Flag for file path that contains HTTPProxy resource YAMLs
- `--base-gateway`: Flag for file path that contains Gateway API Gateway to use as base to attach routes to, set up listeners etc.
- May add more to do with how to attach routes to listeners etc. (e.g. a flag option that generates a "strict" Listener AllowedRoutes policy vs. a more lax one)

### General flow
- Ingest resources via command line flag/files, unmarshal YAML
- Validate they are usable HTTPProxies/Gateway configuration
  - If using un-implemented features, skip and log warning
- Convert usable resources to Gateway API resources
- Write to standard out

### Handling of base Gateway/Listener configuration
The main sticky wicket in converting HTTPproxy resources to Gateway API resources is that they do not convert 1-to-1.
Particularly hostname and TLS configuration is split over Gateway and route resources, whereas in HTTPProxy it is all in one resource type.
As such, it seems reasonable to try to generate as close as we can of a good Gateway setup as well as to generate route resources.

The base Gateway that we ingest may be "empty" or contain some existing listeners.
These should be taken into account and validated.
As we process HTTPProxy resources with TLS configuration, they should be added to either an existing matching Listener or possibly a newly created one that we add to match the correct hostname, port, protocol, TLS configuration etc.
We should do a similar thing with HTTPProxy resources with TCPProxy configuration.

Here is a [simple spike example](https://github.com/sunjayBhatia/proxy2gateway/tree/ec6e992138efab0c412f71434baf46e16988ff5c/internal/translate/testdata/basic_tcpproxy).
In this example, we have an empty Gateway and HTTPProxy with TCPRoute configuration that is intended to have TLS terminated at Envoy.
In the output Gateway, we have added a listener for port 443, the appropriate hostname, and tls Terminate mode with the appropriate secret configured.
We also output a TLSRoute with its ParentRef set to the Gateway, with appropriate hostname and BackendRef.

We may need to come up with our opinionated/idiomatic way of creating Listeners based on existing HTTPProxies.
This could maybe be controlled by future command line options.
Some options are:
- For each HTTPProxy with a unique FQDN, generate a new Listener with appropriate hostname, and assume Contour will coalesce them correctly between secure and insecure Envoy listeners
- Generate as general (in terms of hostname) of Listeners as possible, mirroring how we generate Envoy HTTPConnectionManagers
- etc.

### Initial conversions supported/not supported
This section lists some of the HTTPProxy features that will be supported to be converted by `proxy2gateway` to Gateway API route resources and what the resulting resource configuration may look like. Fields not mentioned will likely not be supported initially and any caveats in fields that do at least somewhat convert over are listed.

- HTTPProxy.Spec.VirtualHost.Fqdn: Converted to hostname on route and possibly Listener
  - Given a base Gateway and HTTProxy, will possibly need to add a new Listener to the Gateway
- HTTPProxy.Spec.VirtualHost.TLS: Secret set on TLS configuration on Gateway Listener
  - Most fields here initially not supported in Gateway Listener TLS configuration
  - Passthrough should be supported by setting on Gateway TLS configuration
- HTTPProxy.Spec.Routes.Conditions: These conditions are ANDed together so will be converted to a single HTTPRouteRule.Matches list
- HTTPProxy.Spec.Routes.Services: Services will be added to the aforementioned HTTPRouteRule.BackendRefs
- HTTPProxy.Spec.Routes.Services.[Request|Response]HeadersPolicy: Applied to the appropriate filter in HTTPRouteRule.BackendRefs.Filters
- HTTPProxy.Spec.Routes.[Request|Response]HeadersPolicy: Applied to the appropriate filter in HTTPRouteRule.Filters
- HTTPProxy.Spec.TCPProxy.Sevices: Services will be added to TLSRouteRule.BackendRefs
- TODO: make this more exhaustive

### Generated resource annotations
We should include the version of `proxy2gateway` that generated the resource, to ensure we can track down any bugs or issues that arise.
The annotation could be of the form: `projectcontour.io/proxy2gateway.version: v0.1.0`.

## Alternatives Considered

### Gateway name/GatewayClass name flags
These flags could be used to not require the user to provide a Gateway YAML and have it generated by `proxy2gateway`.
This would provide the tool the ultimate flexibility in generating an idiomatic Gateway and not require any Gateway API knowledge from the user initially.
These would be used separately from the `--base-gateway` flag and could be implemented instead of or in addition to this flag.

### Leave Gateways alone, only convert to routes
Instead of trying to generate a Gateway resource, only convert HTTPProxies to HTTPRoute or TLSRoute resources.
This seems not possible to do since in the Gateway API, TLS configuration and hostname configuration requires some involvement with the Gateway Listener configuration, it is not a route-only thing.

## Security Considerations

### Using converted resources as-is without inspection or testing
We should provide some disclaimer and guidance that converted Gateway API resources should be tested out in a staging environment to ensure the conversion was valid.
Converting routing rules and especially listener TLS configuration should be checked before usage in production.

## Compatibility

### Versioned CLI tool
We should be strict about versioning the tool.
As new features are added to Gateway API/HTTPProxy or new API revisions are released of either, we should use semver and be sure any changes are documented in release notes.
We may also decide to add an annotation to the resources we produce with the proxy2gateway version.
This should be easily implementable with a tool like [goreleaser](https://github.com/goreleaser/goreleaser).

## Implementation
See the quick spike [here](https://github.com/sunjayBhatia/proxy2gateway)

Testing is done in the same style as `ir2proxy`: test cases are generated with YAML files for input and output.

Common code in validating HTTPProxy resources would be very useful to use.
We may consider refactoring the Contour control-plane controller to pull out some of this in a shareable package.

Using Gateway API types esp. with pointers can be unwieldy, as we have seen in having to create this [helper package](https://github.com/projectcontour/contour/blob/e6ce1715c65493988e7d6c169e7d9bc8b555d769/internal/gatewayapi/helpers.go).
It would be great if this was public or if we could push this upstream, as using it in another codebase makes it more apparent how useful these helpers are to reduce duplication.

## Open Issues
None at the moment, besides strategies on coalescing Listeners as stated above.
