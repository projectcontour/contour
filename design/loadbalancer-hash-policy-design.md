# Adding Support for Header Hash Load Balancing

Status: Accepted

## Abstract
This document hopes to describe the API changes needed to support header hash policy based load balancing in Contour.
The proposed design should be flexible and extensible enough to enable adding additional hash policy configuration in Contour's load balancing API.

## Background
Contour currently supports configuring different [load balancing policies](https://projectcontour.io/docs/v1.11.0/config/api/#projectcontour.io/v1.LoadBalancerPolicy) for balancing traffic between the various members of a cluster.
Load balancing requests can be used to distribute requests evenly across instances of a service, segment certain types of requests to be processed by a subset of service instances, or even target specific instances to handle a sequence of client requests in a stateful application.
In Contour, traffic to be load balanced may be targeted at an upstream service as configured by a [`Route`](https://projectcontour.io/docs/v1.11.0/config/api/#projectcontour.io/v1.Route) or [`TCPRoute`](https://projectcontour.io/docs/v1.11.0/config/api/#projectcontour.io/v1.TCPProxy) on an `HTTPProxy` or a gRPC [`ExtensionService`](https://projectcontour.io/docs/v1.11.0/config/api/#projectcontour.io/v1alpha1.ExtensionServiceSpec) cluster used for advanced Envoy configuration.
This design will be primarily focused on the additional load balancing capabilities Contour can provide using request property hashing, specifically targeting improvements for load balancing client HTTP requests to backend services rather than improving the load balancing experience to an `ExtensionService` or `TCPRoute`.

Contour takes advantage of the load balancing features Envoy provides to offer a few different out of the box load balancing strategies that can be chosen from.
The [`Random` load balancing strategy](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/load_balancers#random) configures Envoy to select a random available host.
[`RoundRobin` load balancing](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/load_balancers#weighted-round-robin) ensures upstream hosts are selected in round-robin order, with endpoint weight taken into account.
The [`WeightedLeastRequest` option](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/load_balancers#weighted-least-request) uses different algorithms to route requests to upstream hosts based on the relative weights and the number of active requests to that instance.
Envoy also provides a [ring hash](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/load_balancers#ring-hash) load balancing policy which hashes some property of a request in order to select an upstream host.
Envoy produces a consistent hash based on an [attribute of a request](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy) to allow clients to effectively select a consistent backend instance requests will be sent to.
Contour currently only exposes the [cookie value](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy-cookie) based flavor as the `Cookie` load balancing strategy in order to give an out of the box experience that provides the ability to implement [session affinity](session-affinity.md) to a particular upstream host.

In order to cater to more advanced load balancing use cases or offer more granular control, Contour can provide additional mechanisms for configuring Envoy's load balancing hash policy.
Specifically, [hashing HTTP request headers](https://github.com/projectcontour/contour/issues/3099) can help to route client requests to a service instance, a feature that may be useful for stateful applications being transformed to run in Kubernetes or monolithic applications in the process of being split into microservices.
A client may send a consistent identifier in a header value in order to specify the same upstream service instance should handle all requests with that specific value.
In addition, [supporting multiple hash policies](https://github.com/projectcontour/contour/issues/3044) can give even more flexibility.
If a particular load balancing attribute is not available on a request, users can specify fallback policies on additional attributes to still segment traffic between backends, rather than having Envoy continue on to route requests using the round-robin or random mechanisms.

## Goals
- Contour should offer the ability to load balance requests based on HTTP user specified request headers
- The design should offer the ability to configure multiple hash policies (of which specific new types may be implemented separately from this design)

## Non Goals
- Provide advanced load balancing configuration for `ExtensionService` or `TCPRoute`
- Provide users direct access to Envoy configuration fields/structures

## High-Level Design
In order to support multiple hash policies on different attributes of a request, we will add support for a new load balancing strategy on the `LoadBalancerPolicy` object that allows users creating `Route` entries on `HTTPProxy` resources to specify a list of hash policies.
This new strategy will only be usable for `LoadBalancerPolicy` objects on `HTTPProxy` resources and will be replaced with the default strategy when selected on `TCPProxy` and `ExtensionService` resources.
Initially, we will only support the option to configure hashing of HTTP request headers selected by name but this may be expanded on in the future as user requests come in.

## Detailed Design

### Changes to `LoadBalancerPolicy`
The `Strategy` field of the `LoadBalancerPolicy` object will support a new value, `RequestHash` which will denote that request attributes will be hashed by Envoy to make a decision about an upstream cluster instance to route a request to.
If the `RequestHash` strategy is chosen, Contour will inspect the new `RequestHashPolicies` list field of the `LoadBalancerPolicy` object to build the Envoy hash policy configuration.

```
type LoadBalancerPolicy struct {
  // Now also supports `RequestHash`
  Strategy string `json:"strategy,omitempty"`

  RequestHashPolicies []RequestHashPolicy `json:"requestHashPolicies,omitempty"`
}
```

This field will be a list of `RequestHashPolicy` objects with each element holding configuration for an individual hash policy.
Each list element will allow users configuring a `Route` on a `HTTPProxy` to specify a separate request attribute for Envoy to hash in order to make an upstream load balancing decision.
Users may configure multiple elements in order to ensure Envoy calculates a hash based on a collection of request attributes, for example a tuple of headers that may be present on a request.

The `Terminal` field denotes if the attribute specified (e.g. header name) is found, Envoy should stop processing any subsequent hash policies in the list it is provided.
This is a performance optimization and can provide a speedup in hash calculation if set and the attribute to hash on is found.
An example of how this field may be used from the Envoy docs is [here](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-hashpolicy-terminal).

The remaining fields will be request attribute specific configuration options, for example `HeaderHashOptions` corresponding with a desire to generate a hash based on a request header.
These attribute specific fields may include what an individual element of a set of properties on the request to use as an identifier to hash (for example the header or cookie name to hash the value of) and/or additional parameters to pass to Envoy to calculate a hash with (for example the TTL on a generated cookie).
Exactly one hash option field must be set in a `RequestHashPolicy` element, otherwise it will be ignored and a warning set on the `HTTPProxy` resource it is part of.

```
type RequestHashPolicy struct {
  Terminal          bool               `json:"terminal,omitempty"`
  HeaderHashOptions *HeaderHashOptions `json:"headerHashOptions,omitempty"`
  // Possible future field
  CookieHashOptions *CookieHashOptions `json:"cookieHashOptions,omitempty"`
}
```

We would also add an additional type for each type of hash option we supported, for example for header hashing:

```
type HeaderHashOptions struct {
  HeaderName string `json:"headerName,omitempty"`
  // Possible future fields
  ValueRewriteRegex       string `json:"valueRewriteRegex,omitempty"`
  ValueRewriteReplacement string `json:"valueRewriteReplacement"`
}
```

This solution was chosen to ensure ease of validation of user provided configuration using typed structs as opposed to a more generic data structure.
The set of request hash policies likely bounded and there are only so many attributes that Envoy can reasonably hash an HTTP request on so the risk of an explosion of structs in this space is low.
In addition, it will allow us to use `kubebuilder` annotations for validation and be able to use more nested data types as needed if configuration needs arise.

### HTTPProxy YAML Example

An example of how this feature would be used to hash on a specific header value follows below:

```
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
spec:
  virtualhost:
    fqdn: example.projectcontour.io
  routes:
  - services:
    - name: example-app
      port: 80
    loadBalancerPolicy:
      strategy: RequestHash
      requestHashPolicies:
      - headerHashOptions:
          headerName: X-Some-Header
        terminal: true
      - headerHashOptions:
          headerName: User-Agent
```

The value of the `X-Some-Header` header will be hashed by Envoy if present and used to make a load balancing decision.
Consistent values in this header would lead to Envoy routing to the same backend service instance.
If the header is present, Envoy will *not* move on to attempting to hash the `User-Agent` header value.

### Handling Invalid Strategy Choice
As per the documentation of the [`LoadBalancerPolicy` field on the `ExtensionService` object](https://projectcontour.io/docs/v1.11.0/config/api/#projectcontour.io/v1alpha1.ExtensionService), the `Cookie` load balancing strategy is invalid for use with the `ExtensionService`.
Currently, that restriction does not seem to be enforced (or on the `TCPRoute` object either, for which the `Cookie` strategy is not compatible).
This design proposes that we start to enforce these strategy restrictions and also restrict the new `RequestHash` strategy to only valid on `Route` objects.
If specified on `ExtensionService` or `TCPRoute` objects, it will be overridden to the default `RoundRobin` strategy and a warning condition will be added to the status of the object.
Overriding rather than throwing an error is a deliberate attempt to continue to allow the configured `HTTPProxy` or configured extension to function, albeit with basic load balancing enabled.

## Alternatives Considered

### Using `map[string]string` for `HashOptions` Type
This alternative would change `RequestHashPolicy` to the below:

```
type RequestHashPolicy struct {
  RequestAttribute  string            `json:"requestAttribute,omitempty"`
  Terminal          bool              `json:"terminal,omitempty"`
  HashOptions map[string]string       `json:"headerHashOptions,omitempty"`
}
```

`RequestHashPolicy` contains a field `RequestAttribute` specifying which type of request attribute it is targeting.
If `RequestAttribute` is empty or an unknown value, this hash policy entry will be ignored and a warning set on the containing resource.
Initially, only `Header` will be supported as a value for `RequestAttribute` though in the future, Contour may support the `Cookie` attribute, or others that Envoy makes configurable.
`HashOptions` is a generic `map[string]string` field that may contain options specific to the requested attribute.
Depending on the `RequestAttribute`, some fields of this generic map may be required, an if missing, the hash policy excluded.
For example, to implement header hashing, the `headerName` field would be required.

This option was not chosen as it would require all future configuration fields to fit into a `string` value, making the possibility for more complex data types more difficult to use and validate.
Strongly typed structs also match better with other configuration patterns we have used in Contour.

### Using `map[string]interface{}` for `HashOptions` Type
This approach does have the benefit of not restricting hash options to string values if we choose to support a wide range of options in the future.
This option was considered and not chosen because using `interface{}` as the value type of the options map out of the box does not work with the deep copy infrastructure we have.
We would likely have to implement a type alias to `interface{}` and write some custom deep copy code to support this rather than rely on types that work with out of the box tooling.

### Individual Hash Policy Fields
Looking through the various existing [Hash Policy configurations available](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy), there are a limited set of fields we may even desire to expose to users per hash policy.
It is feasible that we could add individual fields in order to add constrained configurability without an explosion of structs added to the API, we would instead see a growth in fields on the `RequestHashPolicy` struct.

For example, for [header based hashing](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy-header), it is likely we would only want to allow the `header_name` field to be configurable as we have yet to see requests for `regex_rewrite` configuration and it may introduce a significant amount of complexity to validate user provided configuration.
Similar logic could apply if we were to expand to supporting more configuration of [cookie hashing](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy-cookie) or to supporting the other existing hashing policies ([connection properties](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy-connectionproperties), [query parameter](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy-queryparameter), [filter state](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction-hashpolicy-filterstate)) that only have one configurable field.

We could inline the fields in the `RequestHashPolicy` struct:

```
type RequestHashPolicy struct {
  RequestAttribute string `json:"requestAttribute,omitempty"`
  HeaderName       string `json:"headerName,omitempty"`
  Terminal         bool   `json:"terminal,omitempty"`
}
```

While the number of options to support is unlikely to grow quickly, this option was not chosen as it is possible hashing attributes that require multiple fields of configurability would make this solution messy and unweildy.

### Only Address Header Hashing, Not Multiple Hash Options
In order to deliver solely header hashing functionality, we could instead add an additional load balancing strategy `HeaderHash` and a configuration to supply a HTTP header name to hash on.
This option was not chosen as it does not address [this issue](https://github.com/projectcontour/contour/issues/3044).

## Security Considerations
Somewhat arbitrary user input in the `HashOptions` field needs to be validated rigorously, especially if we intend to allow options like header value regex rewriting in the future.

## Compatibility
The feature discussed in this design is an opt-in feature, users employing other existing load balancing strategies or none at all will not be affected.

The existing `Cookie` load balancing strategy will be retained as an out of the box solution for session affinity.
Users expecting to use this feature along with other request hashing policies will need to duplicate the existing cookie hashing logic themselves once more generic cookie hashing is supported.
Documentation for this should be provided as part of the implementation.

## Implementation
Documentation of the feature will be provided along with the implementation.
