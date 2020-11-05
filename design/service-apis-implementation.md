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
- Gateway requests a point where traffic can be translated to Services within the cluster.
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

### Contour watches GatewayClass objects
Looking for entries with `spec.controller` equal to `projectcontour.io/contour` by default.
Contour can change this value to any `projectcontour.io/<value>` with a config setting (similar to how `ingress.class` works today.)
Importantly, this means that changing this values requires a Contour restart; this means that the GatewayClass watcher can have this value baked in at runtime.
The output of this watcher is a list of GatewayClass name/namespace details.

### Contour watches Gateway objects
Looking for entries with known GatewayClass name/namespace details, from the previous watch.

Listeners inside Gateways will be merged where possible, as will listeners across Gateways.
Conflicts within a Gateway will result in the relevant Listeners both being rejected.
Conflicts between Gateways will result in the oldest Gateway (by creation timestamp) being kept.
The rejected Listener will have its status updated with the reason and the name of the conflicting Gateway.

The output of this watcher is both Envoy configuration, and a list of kind/name/namespace details for Routes to watch.

### Contour watches HTTPRoutes
HTTPRoutes refereenced in Gateways will be watched, and associated routes configured.
Errors or conflicts here will render that section of the config invalid.
Other valid sections will still be passed to Envoy.

The output of this watcher is Envoy configuration.

### Contour watches TLSRoutes
TLSRoutes referenced in Gateways will be watched and associated routes configured in Contour.
Errors or conflicts here will render that section of the config invalid.
Other valid sections will still be passed to Envoy.

The output of this watcher is Envoy configuration.

### Interrelated watches

TODO: Design how the information flow will work. What watches? How does the GatewayClass pass object names to the Gateway, for example?

In general, because of the dynamic, interrelated nature of these resources, Contour will need to watch all events associated with all objects, and filter internally, rather than using a filtered watch (as those are costly to set up and tear down).


## Detailed Design

For each object, I need to specify:
How the watch will work
Where the integration will be plumbed
How do we handle the inter-related dynamic configuration?

A detailed design describing how the changes to the product should be made.

The names of types, fields, interfaces, and methods should be agreed on here, not debated in code review.
The same applies to changes in CRDs, YAML examples, and so on.

Ideally the changes should be made in sequence so that the work required to implement this design can be done incrementally, possibly in parallel.

## Alternatives Considered
If there are alternative high level or detailed designs that were not pursued they should be called out here with a brief explanation of why they were not pursued.

## Security Considerations
If this proposal has an impact to the security of the product, its users, or data stored or transmitted via the product, they must be addressed here.

## Compatibility
A discussion of any compatibility issues that need to be considered

## Implementation
A description of the implementation, timelines, and any resources that have agreed to contribute.

## Open Issues
A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.
