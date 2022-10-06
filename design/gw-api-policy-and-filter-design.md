# Implementing advanced features in Gateway API with Policy Attachment and Route Filters

Status: Draft

## Abstract
This design describes how Contour may implement features in Gateway API outside the "core" or "extended" features using the well defined extension points in the API: Policy attachment, route level filters, and GatewayClass parameters.
We will also attempt to categorize existing and new Contour features to specify which extension mechanism should be used to express them in Contour's implementation of Gateway API.

## Background
Contour is up to date with Gateway API conformance and will continue to be, however there are many features Contour supports that Gateway API does not.
Even as the Gateway API spec moves to GA, not everything Contour implements will be available, since the Gateway API is a more general API, with core and extended functionality intended to be supported by many implementations.
For example, things needed for access control, authorization, "day two operations", observability, and more fine grained control over HTTP middleware are still needed and not yet expressed in Gateway API.
We've outlined the feature parity gap between Contour's HTTPProxy and the existing core and extended Gateway API [here](https://docs.google.com/document/d/1F2o7R10H5ZDG9rW3m1K2BVkl-SwhTp-EShylMW1wlBQ/).

Gateway API does offer a few extension points intended to support many of the features Contour and other ingress controllers provide:
- [Policy Attachment](https://gateway-api.sigs.k8s.io/references/policy-attachment/)
- [HTTPRoute filters](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.HTTPRouteFilter)
- Using a different type than Service in a `BackendRef`
- [GatewayClass parameters](https://gateway-api.sigs.k8s.io/api-types/gatewayclass/#gatewayclass-parameters)

These configuration points are intended for different classes of configuration.
We will mainly focus on the first two above, though the next two are worth mentioning as options.

The Contour Gateway Provisioner uses GatewayClass parameters for passing general configuration for an instance of Contour and is not really a focus of this design.

It is also possible for us to use a different type than Service as the target of a `BackendRef` [`BackendObjectReference`](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.BackendObjectReference).
For example, this may be a "wrapper" type around a Service that offers additional configuration Service does not have.
The current [Backend Capabilities GEP](https://github.com/kubernetes-sigs/gateway-api/pull/1430) seems to be using this method to configuring things like TLS between a proxy and backend (which is a bit out of scope of this design, but possible relevant).

Policy attachment is described in detail [here](https://gateway-api.sigs.k8s.io/references/policy-attachment/).
Policy resources must follow a standard structure, referencing the resource they apply to via a [`targetRef`](https://gateway-api.sigs.k8s.io/references/policy-attachment/#target-reference-api) field.
They could be "attached" to a Gateway, Route, Service (or other backend reference resource), or even a Namespace (howeer the resources a particular policy is allowed to attach to is implementation-dependent).
A key feature of Policy attachment is the idea that Policies can be applied with a hierarchy.
[Overrides and Defaults can be cascaded up/down the hierarchy of Gateway API resources](https://gateway-api.sigs.k8s.io/references/policy-attachment/#hierarchy).

(Note that Policies could be attached to GatewayClasses, however it is [more complicated and probably not recommended](https://gateway-api.sigs.k8s.io/references/policy-attachment/#attaching-policy-to-gatewayclass))

HTTPRoute also supports a per-route or per-backend [`HTTPRouteFilter`](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.HTTPRouteFilter) type.
This provides built-in HTTP request modification/redirection capabilities (varying between core and extended conformance levels).
It also supports an `ExtensionRef` filter type, which can be to reference a custom resource for implementation-specific configuration.

Policies and Filters represent extension points that could overlap in scope.
Upstream Gateway API documentation provides some guidance [here](https://gateway-api.sigs.k8s.io/references/policy-attachment/#interaction-with-custom-route-filters).
In particular the community recommends [differentiating what configurations are supported by each method](https://gateway-api.sigs.k8s.io/references/policy-attachment/#2-custom-filters-and-policies-should-not-overlap)

## Goals
- Differentiating which Contour features should be included in each configuration/extension type
- Give maintainers/contributors/users an agreed upon framework/pattern in which to think about extension points in Gateway API so we can make the implementation and API consistent
- Use abstractions/conventions of Gateway API as much as possible to avoid possible future of piecemeal attaching new features/configurations

## Non Goals
- Cover configuration of upstream TLS or other "backend protocol details"
  - These will hopefully be covered by [this GEP](https://gateway-api.sigs.k8s.io/geps/gep-1282/) and subsequent features
- Contour flags we've added for smaller surface area coverage or convention etc.
  - e.g. `contourconfiguration.spec.envoy.listener.disableAllowChunkedLength`

## High-Level Design
Summary TBD

## Detailed Design

### Categorizing features between Policies or Filters
Here is a list of features we want to configure likely outside scope of the Gateway API standardized features that are already in Contour:
- rate limiting
- auth (external auth and/or built-in Envoy filters)
- load balancing between endpoints of a service
- CORS configuration
- cookie rewriting
- direct HTTP response configuration (HTTPProxy directResponsePolicy, could also be contributed to GW API upstream?)

Things not in Contour yet:
- tracing
- IP allow/block lists
- etc.

We can categorize these a few ways, but to differentiate what should be configured in a Gateway API Policy vs. Filter, I propose that we consider two categories:

#### Features that modify content in the data path
These features should be considered ones that we use HTTPRoute Filters to implement.
Similar to the existing Filter types that modify the data path, we can use custom Filters to implement things like:
- cookie rewriting
- direct HTTP responses
- etc.

Note: I think we should not consider these "Filters" equivalent to Envoy filters in the general sense, as Envoy filters are often associated with a single virtualhost, and these are route-level Filters.

Another way to think about this could be that it applies to configuration that is likely to be specific to an individual route, and unlikely to generally apply across an entire Listener/Gateway.

#### Features that aid in operability, traffic distribution, authorization, access control
The above title is not an exhaustive list, but rather a bit more of a set of examples that help illustrate the category.
They include configuration that needs to be applied to accomodate a specific workload, e.g. a timeout that needs to be applied to a particular deployment/service.
These types of features will be implemented via Policy resources that target specific Gateway API or Service resources.

Some examples and discussion of how they may work are below:
- TBD

### Status
While it is not explicitly statued in the Gateway API documentation, Contour should follow the conventions in setting Status Conditions being established by Gateway API.
We should utilize an `Attached` or `Accepted` Condition that signifies whether a Policy or Filter resource has been applied.
This encompasses validity, whether it is applied to an allowed resource type, and whether it is an allowed cross-namespace reference.
More detailed error conditions may be added as well to point out specific errors.

## Alternatives Considered

### Filter vs. Policy Examples
In these examples we consider a feature that can be configured using different mechanisms and the merits or drawbacks of each.

In particular, using rate limiting as a case study is quite interesting because of how it can be categorized as a feature and how it ultimately must be configured in Envoy.
Envoy provides per-route rate limiting settings, not per-cluster or per-virtualhost.

#### Rate Limiting: Filter
[Here](https://github.com/projectcontour/contour/pull/4775) is an example spike on configuring rate limiting via HTTPRoute filter `extensionRef`.
It adds a new CRD `RateLimitFilter` that currently only allows configuring a "local" rate limiting policy.
The resource can be applied per-route (not per-backendRef in this case) and by nature of the `extensionRef` spec, must live in the *same* namespace as the referencing HTTPRoute.

The relevant sections of the changes to how this would be configured are below:

```yaml
---
kind: RateLimitFilter
apiVersion: projectcontour.io/v1alpha1
metadata:
  name: local-ratelimit-example
  namespace: demo
spec:
  local:
    requests: 10
    unit: minute
---
kind: HTTPRoute
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: echo
  namespace: demo
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: example
    namespace: demo
  hostnames:
  - "filter-example.com"
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - kind: Service
      name: echo
      port: 80
    filters:
    - type: ExtensionRef
      extensionRef:
        group: projectcontour.io
        kind: RateLimitFilter
        name: local-ratelimit-example
```

[This comment](https://github.com/projectcontour/contour/pull/4775#issuecomment-1268982038) around this implementation option is quite relevant.
Implementing features such as this as a Filter feels quite straightforward, especially if users don't ultimately want/need the flexibility/defaulting etc. that a Policy setup would give.

#### Rate Limiting: Policy
[Here](https://github.com/projectcontour/contour/pull/4776) is an example spike on configuring rate limiting via Policy resource.
It adds a new CRD `RateLimitPolicy` that currently only allows configuring a "local" rate limiting policy.

In the example YAML in the PR linked, we have a Gateway with a RateLimitPolicy attached as well as two HTTPRoutes, one of which has a RateLimitPolicy attached.
We can see how different settings can be overridden/defaulted in a hierarchy, with some settings defaulted at the Gateway level that can be overridden by more specific resources.
Policies cannot yet target specific route rules, however, to achieve the equivalent of the Filter case, where a rate limit applies to a single route rule, you can follow the example and create a Policy that targets an HTTPRoute which has a single route rule.

This potentially gives users a huge amount of flexibility *and* operators control over critical settings.
We can concievably have a kubectl plugin or other visualization that shows exactly what settings are enabled on a particular route.

Cross-namespace references also are possible with Policies, though not implemented in this example, but possible in the future with Policy resources accompanied by ReferenceGrants.

Overall this option has more moving pieces, but is very powerful.
In addition, it does not directly affect the core Gateway API resources, but rather is a meta-resource on top of them that adds additional functionality.
This may be desirable for some.

It is also worth noting, we don't necessarily have to allow a Policy to target all resources, we can constrain this and as mentioned in implementation notes, it might be a lot easier to iterate this way.

#### Rate Limiting: Custom BackendObjectReference type
This option involves wrapping a Service that requires a rate limit with a resource that specifies a rate limit.

I'm not entirely sure this will properly work, as we can't today implement per-cluster rate limits in Envoy, though there may be a clever way in the API we could get it to work.

Implementing features using this method (and with Filters to be fair) does mean we affect the core resources, making it a bit harder to swap one's configuration between Gateway API implementations.

### Idea: Replace parts of Contour global configuration with Policies
An option for simplifying the ContourConfiguration and ContourDeployment CRDs could be to split some of the relevant fields it contains out into Policy resources that are applicable to a Gateway.

This way, there is a more succinct and focused place for users to apply configuration, and we can acheive some goals of having more dynamic configuration for some Contour settings.
The configuration file/configuration CRDs should end up just being what is needed to get Contour/Envoy started.

The Gateway Provisioner could give you a base/default Policy if none existed or users could come with their own.

Thinking out loud, this might alleviate some of the weirdness around how GatewayClass parameters maybe shouldn't be reconciled when things change.

Some examples of things that might be applicable here to remove from the global config and move to a Gateway policy are:
- Timeout policy
- HTTP header policy
- TLS parameters
- Envoy "Cluster" settings
- Envoy "Listener" settings
- etc.

## Security Considerations

### Cross-namespace references
It is not specified explicitly in the Gateway API documentation, but if Policy or Filter custom resources live in a separate namespace from the resource it references/is referenced by, we will likely need to ensure the appropriate ReferenceGrant is present that allows the resources to be "attached."

## Compatibility

### Portability between Gateway API implementations
Policy resources reference core resources, whereas Filters or BackendObjectResource references are referenced by core resources.
Policies have the advantage of likely not affecting core functionality (e.g. routing rules and other features that are standardized), since swapping between implementations will not mean the new implementation will encounter references to resource kinds it does not know how to process.

## Implementation

### Reconciling Policy resources
While the Policy resource `targetRef` allows referencing any resource, we should be judicious about what resources a Policy can attach to, especially initially.
This is to ensure clarity for implementers and more importantly users as to which Policy is in effect.
This should allow us to incrementally grow our implementation and API surface area.
For example, rather than initally allow a Policy to reference *any* resource, we may restrict it to reference only a Gateway or only an HTTPProxy.

Contour will add informers for each Policy resource we end up creating, collect and caching them as we do with other resources.
Typically we cache resources by namespace and name of the resource.
As an optimization we may consider caching Policy resources by the namespace, name, and kind of the resource they reference instead.
This may aid in lookup of applicable Policies when processing Gateways, HTTPRoutes, and Services, and make it easier to construct the final values we must apply for some configuration setting as the number of Policies in a cluster grows.

### Reconciling Filter resources
Reconciling Filters should be simpler than Policies, as they can only be referenced in route rules or as part of a backend configuration.
Contour will add informers for each Filter resource we end up creating, collect and caching them as we do with other resources.
When processing HTTPRoutes, any referenced Filter resources will be looked up in the cache.
Status should be set accordingly on the referencing HTTPRoute and Filter if invalid.

## Open Issues
N/A for now
