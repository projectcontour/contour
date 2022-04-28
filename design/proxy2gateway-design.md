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
- `--dry-run` (or `--audit` or `--report`): Flag to run the conversion process and report what resources can be converted, what are skipped, to give users the ability to assess how compatible their configuration is with Gateway API
- May add more to do with how to attach routes to listeners etc. (e.g. a flag option that generates a "strict" Listener AllowedRoutes policy vs. a more lax one)

### General flow
- Ingest resources via command line flag/files, unmarshal YAML
- Validate they are usable HTTPProxies/Gateway configuration
  - If using un-implemented features, skip and log warning
  - If in "audit" mode, record for report
- Convert usable resources to Gateway API resources
- Write to standard out (unless in "audit" mode, in this case we will write report)

### Listener vs route configuration
The main sticky wicket in converting HTTPproxy resources to Gateway API resources is that they do not convert 1-to-1.
Particularly hostname and TLS configuration is split over Gateway and route resources, whereas in HTTPProxy it is all in one resource type.
As such, it seems reasonable to try to generate as close as we can of a good Gateway setup as well as to generate route resources.

Hostnames to be used for HTTP host or TLS SNI matching are configured in HTTPProxy on a "root" HTTPProxy.
However, in the Gateway API they could be configured on a Gateway Listener *or* on an route.
From the Listener spec:
```
	// Hostname specifies the virtual hostname to match for protocol types that
	// define this concept. When unspecified, all hostnames are matched. This
	// field is ignored for protocols that don't require hostname based
	// matching.
	//
	// Implementations MUST apply Hostname matching appropriately for each of
	// the following protocols:
	//
	// * TLS: The Listener Hostname MUST match the SNI.
	// * HTTP: The Listener Hostname MUST match the Host header of the request.
	// * HTTPS: The Listener Hostname SHOULD match at both the TLS and HTTP
	//   protocol layers as described above. If an implementation does not
	//   ensure that both the SNI and Host header match the Listener hostname,
	//   it MUST clearly document that.
	//
	// For HTTPRoute and TLSRoute resources, there is an interaction with the
	// `spec.hostnames` array. When both listener and route specify hostnames,
	// there MUST be an intersection between the values for a Route to be
	// accepted. For more information, refer to the Route specific Hostnames
	// documentation.
	//
	// Support: Core
	//
	// +optional
	Hostname *Hostname `json:"hostname,omitempty"`
```

While we maybe could provide some optionality here, it seems like we should follow Contour's existing design philosophy around different personas involved in service networking and how hostname configuration is set up.
HTTPProxy (and IngressRoute before it) was designed with the model that the cluster operators would own the "root" HTTPProxy with FQDN and TLS configuration.
"Included" HTTPProxies should be owned by application teams.

By design, the Gateway API personas mirror this, with the Gateway and its Listeners intended to be owned by cluster operators and HTTPProxies owned by application teams.
We should stay with this in terms of how we configure hostnames on Gateways and routes to ensure the separation between cluster operator and application developer personas are preserved.
Hostnames should be set on Gateways *only* and not on routes generated by `proxy2gateway`.
This should prevent application teams from now having extra concerns when migrating to HTTPRoutes/TLSRoutes.

### Base Gateway configuration
Taking in a Gateway to base configuration on has the advantage of giving us the ability to adapt an existing Contour installation without having to reinstall it or install a second to use Gateway API (though this possibility could still be available to those who need it).
The user flow would possibly go something like:
- Given a Contour installation
- User ensures Contour is configured with Gateway API enabled
  - CRDs exist in cluster
  - GatewayClass controller string or Gateway name configured in Contour
  - Required GatewayClass and Gateway exist in cluster
  - Contour possibly created by a dynamic provisioning method
- At this point any existing HTTPProxy configuration can be run through `proxy2gateway` with the base Gateway to generate HTTP/TLSRoutes

The base Gateway that we ingest may be basic or "empty" or contain some existing listener configuration.
These should be taken into account and validated.
As we process HTTPProxy resources with TLS configuration, they should be added to either an existing matching Listener or possibly a newly created one that we add to match the correct hostname, port, protocol, TLS configuration etc.
We should do a similar thing with HTTPProxy resources with TCPProxy configuration.

Here is a [simple spike example](https://github.com/sunjayBhatia/proxy2gateway/tree/eaea819791426c8ee2af3134261cd8bdd793a2ff/internal/translate/testdata/basic_tcpproxy).
In this example, we have an empty Gateway and HTTPProxy with TCPRoute configuration that is intended to have TLS terminated at Envoy.
In the output Gateway, we have added a listener for port 443, the appropriate hostname, and tls Terminate mode with the appropriate secret configured.
We also output a TLSRoute with its ParentRef set to the Gateway, with appropriate BackendRef.

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

One possible downside of this approach could be that when this new generated Gateway is applied to a cluster, a user possibly using dynamic provisioning of Contour may get an additional install of Contour, rather than adapting the configuration of an existing instance.
This might not be the end of the world, but something to consider, how the user flow should work.

### Leave Gateways alone, only convert to routes
Instead of trying to generate a Gateway resource, only convert HTTPProxies to HTTPRoute or TLSRoute resources.
This seems not possible to do since in the Gateway API, TLS configuration and hostname configuration requires some involvement with the Gateway Listener configuration, it is not a route-only thing.

### Where hostnames are configured
A different way from our current opinionated/idiomatic way of creating Listeners based on existing HTTPProxies and where to set the hostname settings, on Listeners, HTTPRoutes, or both could be to use command line flags.
Some options are:
- A flag that signifies for each HTTPProxy with a unique FQDN, generate a new Listener with appropriate hostname, and assume Contour will coalesce them correctly between secure and insecure Envoy listeners
- A flag that signifies we will generate as general (in terms of hostname) of Listeners as possible, mirroring how we generate Envoy HTTPConnectionManagers

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
An end to end test that takes a simple HTTPProxies, converts it to Gateway API resources, and then checks the intended routing rules work as expected may be useful as well to ensure we are creating valid kubernetes resources and that we are applying the correct conversions.
A subset of this whole process that might be useful would be to apply some generated Gateway API resources to a cluster with the Gateway API validating webhook at the very least to ensure they are valid.

Common code in validating HTTPProxy resources would be very useful to use.
We may consider refactoring the Contour control-plane controller to pull out some of this in a shareable package.

Using Gateway API types esp. with pointers can be unwieldy, as we have seen in having to create this [helper package](https://github.com/projectcontour/contour/blob/e6ce1715c65493988e7d6c169e7d9bc8b555d769/internal/gatewayapi/helpers.go).
It would be great if this was public or if we could push this upstream, as using it in another codebase makes it more apparent how useful these helpers are to reduce duplication.

## Open Issues

### Use as a controller
Could this tool be useful as a controller, that dynamically converts HTTPProxy resources in-cluster?
Some thoughts how it would work:
- Inform on HTTPProxy resources, if valid and processed, add some status info on if the resource was successfully converted or not
- Modify a Gateway and HTTPRoute/TLSRoutes accordingly
- In order to "disable" the old HTTPProxy, maybe we could modify the HTTPRoute and add an invalid IngressClass name? (this aspect of not having conflicting resources in a cluster might be something to think about)
