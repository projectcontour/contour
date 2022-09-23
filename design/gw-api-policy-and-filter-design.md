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
- [GatewayClass parameters](https://gateway-api.sigs.k8s.io/api-types/gatewayclass/#gatewayclass-parameters)

These configuration points are intended for different classes of configuration.
We will mainly focus on the first two above, though the third is worth mentioning as an option.
The Contour Gateway Provisioner uses GatewayClass parameters for passing general configuration for an instance of Contour.

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
- cookie reqriting
- direct HTTP responses
- etc.

Note: I think we should not consider these "Filters" equivalent to Envoy filters in the general sense, as Envoy filters are often associated with a single virtualhost, and these are route-level Filters.

#### Features that aid in operability, traffic distribution, authorization, access control
The above title is not an exhaustive list, but rather a bit more of a set of examples that help illustrate the category.
THey include configuration that needs to be applied to accomodate a specific workload, e.g. a timeout that needs to be applied to a particular deployment/service.
These types of features will be implemented via Policy resources that target specific Gateway API or Service resources.

Some examples and discussion of how they may work are below:
- TBD

### Status
While it is not explicitly statued in the Gateway API documentation, Contour should follow the conventions in setting Status Conditions being established by Gateway API.
We should utilize an `Attached` or `Accepted` Condition that signifies whether a Policy or Filter resource has been applied.
This encompasses validity, whether it is applied to an allowed resource type, and whether it is an allowed cross-namespace reference.
More detailed error conditions may be added as well to point out specific errors.

## Alternatives Considered
TBD

## Security Considerations

### Cross-namespace references
It is not specified explicitly in the Gateway API documentation, but if Policy or Filter custom resources live in a separate namespace from the resource it references/is referenced by, we will likely need to ensure the appropriate ReferenceGrant is present that allows the resources to be "attached."

## Compatibility
TBD

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
