---
title: Contour Philosophy
layout: page
# NOTE(jpeach): we have to use 'h1,h2' here, because the TOC is not generated at all if we just use 'h1'.
toc: h1,h2
---

<!-- NOTE: this document should be formatted with one sentence per line to made reviewing easier. -->

Contour is a Layer 7 HTTP middleware reverse proxy for enabling ingress to Kubernetes clusters.

## Non Goals
Contour is not a service mesh, nor is it the intention for Contour to expose all of the features or configuration options of Envoy. 

Contour is not intended for proxying layer 4 protocols such as TCP or UDP except insofar as they are needed to deliver HTTP.
So, TCP proxying is available, but expects the proxying to be used for passing through HTTPS.
In the future, some UDP support may be added so that we can support QUIC, which uses UDP as a transport.
Using Contour to proxy raw TCP or UDP traffic may work, but is not the intended usage.

## User Personas

Contour defines two end user personas.
The first is the administrator or operator of a cluster.
The second is the application developer.

### Cluster administrator

Cluster administrators are charged with the health and operation of a Kubernetes cluster.
They are responsible for, amongst other things, connectivity to the outside world, TLS secret management, and DNS.
They are responsible for installing Contour and Envoy, the software lifecycle for those applications, and for Contour application configuration.

### Application developer

An Application Developer is a person who wants to deploy a web application or microservice workload on a Kubernetes cluster, and who has less access to the cluster than the administrator.
They interact with Contour by creating Kubernetes objects of types that Contour can understand, and have no interaction with Contour itself aside from the effects it has on those objects.

## Opinions

Contour is an opinionated project.
We believe, above all, that a large part of the value of Contour is its opinionated approach.
We believe that this opinionated approach allows us to write better software that meets the needs of the maintainers and the users.

### Sensible defaults

Many projects in the networking space provide wide scope for configuration.
In general, this is a good thing.
Most networking projects are operated by teams who have little agency to change the code they run; therefore configuration must be possible at runtime.
However, this practice has evolved into a position of if something could be configurable, it should be configurable.
Contour rejects this position.

In general, making something configurable implies taking no position on sensible values within its range.
For example, many projects allow network buffer sizes to be configured arbitrarily, not just within the sensible range from a page to a few dozen pages, but often as low as 1 byte and as high as gigabytes.
In providing the tools, but little or no guidance on how they should be used, infinitely tweakable knobs forces the burden of maining the high dimensional configuration space onto the end users of the product.
The secondary impact on support teams and the upstream development team should not be discounted, both in terms of difficulty in diagnosing problems, and the possibility of configuration drift between environments.

Contour takes the position that when there is a sensible default value for an Envoy configuration parameters, Contour will apply it unconditionally.
We’ve used this position in the past by compressing HTTP response bodies unconditionally, disallowing TLS/1.0, choosing an aggressive cipher suite, and so on.

Contour will provide a method for the administrator or developer to supply their chosen value as a last resort, in the event that we cannot discover a universally acceptable value.
In the history of the project this second scenario has rarely occurred.
More often than not, the discussion sparked by the desire to change a particular parameter has led to a deeper understanding of the ways in which Contour is being used which would not have otherwise occurred.

### Limited feature scope

The Contour project has a well defined scope: a reverse proxy implementation for HTTP workloads on Kubernetes clusters.
Contour’s limited support for TCP proxying is intended solely for Contour to to support web applications which desire to handle TLS directly.

### Every feature is supportable by the application developer or cluster administrator

For every feature we add, we have to have an answer for the question -- can the end user debug a failure in this feature without having to escalate to the Contour maintainers?

If there is a third party component involved, how can we connect the application developer to the component that failed in such a way that they are aware of each other as first parties without us having to mediate?

This means we prefer to avoid adding features that the customer cannot debug themselves -- even if it is their system that is at fault.

When adding validation features we err on the side of the design space that gives the customer feedback as soon as possible even if it is not the most complete; i.e., CRD validation enforced by the api server is preferable to status fields on objects.

When designing Kubernetes objects, we try to expose information as close as possible to the object that needs it.
For example, we will ensure that HTTPProxy objects have status conditions that tell the user that created them if there is a problem, rather than just logging that information from Contour itself.

### We meet users where they are
Contour currently supports Ingress v1beta1, HTTPProxy and IngressRoute.
In the near future, we’ll add support for Ingress v1 and after that Ingress v2.
We don’t ask users to choose which ingress API they want to use, instead we will consider providing support for any requested types to meet users wherever they are.

This goal is in conflict with the goal of a minimum surface area, but we realize that channeling all our users to an API which is only implemented in Contour is bad for their interoperability and limits our total addressable market.
The idea for this goal is that we will thoughtfully consider new ingress types as they become available, and add them in if we believe it is a good idea.

By closely tracking the upcoming Ingress V1 and V2 specifications we actively contribute to the broader Kubernetes community as early adopters.

## Our pledge to our users

We promise that:
- We will treat all your interactions with us in accordance with our Code of Conduct.
- We will consider each request on its merits.
- In the event that we cannot accommodate a request, we will provide a reasonable explanation, with reference to the principles embodied in this document.
