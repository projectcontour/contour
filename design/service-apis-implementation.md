# Service-APIs implementation design

Status: Accepted

## Abstract
The Service-APIs are the evolution of the Kubernetes APIs that relate to `Services`, such as Ingress.
This document outlines what parts of these APIs Contour will implement, and how it will do so.

## Background
The Service-APIs are a subproject of Kubernetes SIG-Network, and are an attempt to re-do the mechanics around `Services` and `Ingress`, and how they interact.
They are aiming to cover the things currently implemented by Layer 4 load balancers that implement `Services` of type `LoadBalancer`, and ingress controllers (like Contour).

The Service APIs target three personas:
- Infrastructure provider: The infrastructure provider (infra) is responsible for the overall environment that the cluster(s) are operating in. Examples include: the cloud provider (AWS, Azure, GCP, ...), the PaaS provider in a company.
- Cluster operator: The cluster operator (ops) is responsible for administration of entire clusters. They manage policies, network access, application permissions.
- Application developer: The application developer (dev) is responsible for defining their application configuration (e.g. timeouts, request matching/filter) and Service composition (e.g. path routing to backends).

The cluster operator and application developer are basically the same as Contour's Cluster Administrator and Application Developer personas, which will be important for this design.

In terms of the APIs themsleves, the Service APIs have 3 primary API resources (taken from the service-apis docs site):

- GatewayClass defines a set of gateways with a common configuration and behavior.
- Gateway requests a point where traffic can be translated to Services within the cluster, using an internal Listener construct.
- Routes describe how traffic coming via the Gateway maps to the Services.

In Contour, we've previously solved a lot of the same problems with HTTPProxy (and IngressRoute before it).
That functionality can be described in the Service APIs by HTTPRoutes, HTTPSRoutes and TLSRoutes, as they describe Layer 7 ingress, as Contour does today.

Other types of routes include TCPRoutes and UDPRoutes, which are intended for Layer 4 load balancers. Implementations may also define their own Route objects, at the cost of interoperability.

In terms of its stated goals, Contour is aiming at being an ingress controller - that is, a Layer 7 proxy with some api gateway functions.
Currently, Contour provides "TCP Proxying" that allows the forwarding of TLS streams based on the SNI information, which is precisely what the Service APIs TLSRoute object is for.
If Project Contour (the organisation) does add support for TCP and UDP forwarding, it will not be in the `projectcontour/contour` repo, but will be a separate repo.

## Goals
- Define a data model and implementation for Contour's service-apis support.
- Layer 7 support only, which means HTTPRoutes, HTTPSRoutes, and TLSRoutes only.

## Non Goals
- No TCPRoute or UDPRoute support, that is, no support for arbitrary TCP/UDP forwarding.


## High-Level Design

### Service APIs implementation surface area

Contour's support for the Service APIs will include support for the HTTPRoute and TLSRoute objects only, not any other type of Route - this includes no TCPRoute or UDPRoute support.
Contour is a layer 7 ingress controller, and the layer 4 load balancing implies by TCPRoute and UDPRoute is out of scope for this tool (that is, `projectcontour/contour`).
Project Contour (the organisation) may investigate a Service-APIs based layer 4 solution in the future, but that effort will not be in `projectcontour/contour`.

### Contour Gateway model

Contour considers a GatewayClass and associated set of Gateways to describe a single Envoy deployment, deployed and managed by something outside of itself.
As such, Contour will error out if it is asked to watch more than one GatewayClass.
Inside its GatewayClass, Contour will merge Listeners within a single Gateway, or across Gateways in the GatewayClass. (See the detailed design for the exact rules Contour will use for this.)

### Interrelated watches

The Service APIs are a set of interrelated Kubernetes objects, where a change in one object can mean that the scope of objects Contour is interested in will change.
Because of this, Contour will watch all events associated with GatewayClass, Gateway, HTTPRoute, TLSRoute, and BackendPolicy objects, and filter the objects it takes action on internally, rather than using a filtered watch (as those are costly to set up and tear down).

### Combining Service APIs with other configuration

When it calculates the Envoy configuration using the DAG, Contour layers different types of configuration in order.
So, currently, Ingress is overwritten by HTTPProxy, so if an Ingress and a HTTPProxy specify the exact same route, the HTTPProxy will win.

Once we have the Service-APIs available as well, we have to choose the order.
In this design, I suggest having the order be Ingress is overwritten by HTTPProxy, is overwritten by the service-apis.
I could see reversing HTTPProxy and the Service APIs here, but I think this acknowledges that the Service APIs are really an evolution of the ideas in HTTPProxy, and will probably end up being the community standard.

### Status management

Each object in the Service APIs set has its own status definition. In general, Contour will update the status of any object that comes into scope with its details, ensuring that the `observedGeneration` field is set correctly.
When objects fall out of scope, any status set by Contour will not be removed.
It's expected that things that check the status will also check the `observedGeneration` to check if the status information is up-to-date with the current object generation.

### Per-object design notes

#### GatewayClass

Contour will look for entries with `spec.controller` equal to `projectcontour.io/contour` by default.
Contour can change this value to any `projectcontour.io/<value>` with a config setting (similar to how `ingress.class` works today.)
Importantly, this means that changing this values requires a Contour restart; this means that the GatewayClass watcher can have this value baked in at runtime.

If Contour finds more than one matching GatewayClass, this is a conflict, and Contour will use the oldest GatewayClass (by creation timestamp).
All GatewayClasses will be updated with status conditions indicating if they are in use.

The output of this watcher is a list of GatewayClass name/namespace details.

#### Gateway
Contour will watch all Gateway objects, and filter for entries with known GatewayClass name/namespace details, from the previous watch.

Contour is not able to specify the address that the Envoy deployment will listen on, and so having any entries in Gateway's `spec.addresses` stanza will result in Contour marking those addresses as invalid in the Gateway's status.
Contour expects the `spec.addresses` field to be empty.

For Contour, the key Gateway section are the Listeners. These define how an implementation should listen for traffic described by Routes.

For Listeners:
- Listeners inside Gateways will be merged where possible, as will listeners across Gateways, in order to end up with two broad groups of listeners, secure and insecure.
- Conflicts within a Gateway will result in the relevant Listeners both being rejected.
- Conflicts between Gateways will result in the oldest Gateway (by creation timestamp) being kept.
- The rejected Listener will have its status updated with the reason and the name of the conflicting Gateway.
- Listeners are considered mergable if all the fields out of `hostname`, `port`, `protocol`, and `tls` match.
- Further merging rules are specified in the detailed design below.

The output of this watcher is both Envoy configuration, and a list of kind/name/namespace details for Routes to watch.

### HTTPRoute
Contour will watch all HTTPRoute objects, and filter for entries matching a label selector in a valid Gateway `spec.listeners[].routes`,
and will configure the associated routes.

Contour will only ever update the `status` of HTTPRoute objects.

Errors or conflicts here will render that section of the config invalid.
Other valid sections will still be passed to Envoy.
For each invalid section, Contour will update status information with the section and the reason.

The output of this watcher is Envoy configuration.

### TLSRoute
Contour will watch all TLSRoute objects, and filter for entries matching a label selector in a valid Gateway `spec.listeners[].routes`,
and will configure the associated routes.

Contour will only ever update the `status` of TLSRoute objects.

Errors or conflicts here will render that section of the config invalid.
Other valid sections will still be passed to Envoy.

The output of this watcher is Envoy configuration.

## Detailed Design

### Configuration
Contour will have an entry added to the config file for the GatewayClass string, `gatewayClassID`.
By default, this is set to `contour`.
This parameter is concatenated to `projectcontour.io/` to produce the string that Contour will check the `spec.controller` field of GatewayClass entries for.
So, by default, Contour will check for `projectcontour.io/contour`.


### Code changes
Contour already has support for importing the Service APIs objects into its Kubernetes cache.
However, for some types, we need to be able to keep some details - this design suggests making those details properties of the cache, as `IngressClass` is currently.

For ingestion, the general pattern currently is that the EventHandler in `internal/contour/handler.go` handles all objects, and calls out to the KubernetesCache from `internal/dag/cache.go`.
The current pattern across most of the objects is:
- check if the object is in scope
- add it to the cache
- indicate with a return value if the add should result in a DAG (and consequently an Envoy config) rebuild.

This pattern is applicable to all the Service APIs objects, with some differences for GatewayClass.

There are already internal fields in the cache to hold GatewayClass and some other service-apis objects.
We will use this scaffolding to build the actual functionality on top.

#### Hostname matching

In many of the Service APIs objects, Hostnames *may* be specified, and *may* be either "precise" (a domain name without the terminating dot of a network host), or "wildcard" (a domain name prefixed with a single wildcard label).
Per the API spec, only the first DNS label of the hostname may be a wildcard, and the wildcard must match only a single label.

Hostnames are considered to match if they exactly match, or if a precise hostname matches the suffix of a wildcard hostname (that is, if they match after the first DNS label is discarded.)

This DNS matching is referred to in this proposal as a "domain match", as opposed to an "exact match", which is string equality only.

### GatewayClass

When processing GatewayClass, Contour will use the configuration value `gatewayClassID` to build a controller URL of the form `projectcontour.io/<gatewayClassID>`.
By default, this value will be `projectcontour.io/contour`.

At startup, if this value is specified, Contour will fetch all GatewayClass objects and check the `spec.controller` field of each.
Contour will consider the oldest GatewayClass with a matching `spec.controller` field to be the canonical GatewayClass for that instance of Contour, and update its status.
Other GatewayClasses than the oldest will also have their status updated.
The status update will be adding a condition to the GatewayClass `spec.conditions` field with a type of `Accepted`, where the actually in use GatewayClass will be `Status: true` and any other GatewayClasses will be `status: false`.

Inside Contour, the KubernetesCache will have a `GatewayClassDetails` field added, holding a struct that specifies the name of the active GatewayClass that Gateways may reference, and the creation timestamp for that object.
If this field is empty, then no GatewayClasses match, there is no active GatewayClass, and no Gateways will be checked.

Note that Contour's status update machinery should ensure that only status updates that are changes are actually applied.

A DAG rebuild should only be kicked off if the GatewayClass value in the KubernetesCache changed.

### Gateway

When ingesting Gateways, Contour will import all Gateways into the KubernetesCache, and determine whether they are in-scope as part of the DAG run.

#### Validity

Before processing Gateways, Contour will run a quick check across all gateways to look for conflicting or out-of-scope Listeners.

Contour will use the `GatewayClassDetails` field in the Kubernetes cache to check each object's `spec.gatewayClassName` field against the GatewayClass name.
If they do not match, the Gateway will be skipped from processing.
A debug-level log line should be output here to indicate the skip.

#### Secure and Insecure ports
Contour currently exposes only two ports on the Envoy proxies it controls, an insecure and a secure one.
This allows Contour to automatically register insecure to secure redirects unless told otherwise.

Because of this, the following rules around ports apply:
- Listeners that request a PortNumber that is not one of these two ports will be rejected.
- Listeners that request a HTTPRoute without TLS config on the secure port will be rejected.
- Listeners that request a HTTPRoute with TLS config on the insecure port will be rejected.
- Listeners that request a TLSRoute on the insecure port will be rejected.

#### Listener merging

Listener merging is performed by Contour to coalesce any set of valid Listeners into minimal configuration for Envoy.
Listener **conflict** is defined as two listeners that are not mergable for some reason.

Listeners that cannot be merged *within a Gateway* are invalid and will be rejected, and their status updated accordingly.

When Listeners cannot be merged *across Gateways* the **oldest** Gateway's Listeners (by creation timestamp) will be accepted, and other Listeners will be rejected.
An accepted Listener will also have a condition placd on it to indicate the Gateway that contains Listeners in conflict with it.

This behavior is similar to the HTTPProxy processor's `validHTTPProxies()` method.

#### Listener merging rules

TODO: The basic principle is to allow for people to specify lots of names across a set of Gateways, or within one Gateway.

In general, Listeners that match on ProtocolType and PortNumber can be merged, using Hostname as a discriminator.

Contour uses the following general rules for merging Listeners.

1. Contour's rules about Secure and Insecure ports must be satisfied.
1. A group is considered to be any set of Listeners that have the same values for the `ProtocolType` and `PortNumber` fields.
These Listeners may be within a single Gateway or selected across a set of Gateways.
1. Either each Listener within the group specifies the “HTTP” Protocol or each Listener within the group specifies either the “HTTPS” or “TLS” Protocol.
1. Each Listener within the group specifies a Hostname that is unique within the group.

TODO: This is similar to HTTPProxy behavior. Is what we want? This would seems to exclude the self-service use cases.

1. As a special case, one Listener within a group may omit Hostname, in which case this Listener matches when no other Listener matches.


Listeners that match on ProtocolType, PortNumber, and exactly match Hostname must also have matching TLS details (GatewayTLSConfig struct) to be merged.
Listeners that have different TLS config but the same other details are in conflict.

TODO: What happens if you have Listeners that cannot be merged, but point to the same TLS object? It seems like this should work (to allow default certs).

For "wildcard" hostnames, 
When merging wildcard hostnames, only exact hostname strings will be merged.

- 
- Listeners that match on ProtocolType, PortNumber, and Hostname, but have different GatewayTLSConfig structs (that is, the `tls` stanza is different) are in conflict.

TODO: There are some edge cases to check into here because TLS config can be specified both here and in the Route.
Also need to check for if you can mix-and-match between Routes and here, using here as a fallback?

### HTTPRoute

### TLSRoute



### Other concerns

TODO: What do we do about updating status information? Probably just rely on whatever provisions Envoy to put the reachability info in the Gateway Status.


For each object, I need to specify:
How the watch will work
Where the integration will be plumbed
How do we handle the inter-related dynamic configuration?

A detailed design describing how the changes to the product should be made.

The names of types, fields, interfaces, and methods should be agreed on here, not debated in code review.
The same applies to changes in CRDs, YAML examples, and so on.

Ideally the changes should be made in sequence so that the work required to implement this design can be done incrementally, possibly in parallel.

## Alternatives Considered
TODO: This will need further explanation of what the solution would look like if we didn't make one GatewayClass == One Contour.

TODO: Some discussion of why layer 7 only.

## Security Considerations
TODO: I can't think of any yet.

## Implementation
A description of the implementation, timelines, and any resources that have agreed to contribute.

## Open Issues
A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.
