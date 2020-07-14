---
title: About Contour and Envoy
layout: page
---

At the highest level Contour is a proxy that fulfills the Envoy Cluster Discovery Service (CDS) API by translating Kubernetes Ingress objects and HTTPProxy objects into Envoy configuration objects.

## Introduction to Envoy

Envoy is a high performance, robust, flexible proxy from Lyft. Envoy was recently inducted into the CNCF alongside linkerd.

Envoy can be operated in many modes, and supports more than just HTTP/S, but Contour takes advantage of its suitability as a reverse proxy -- specifically, as an ingress controller.
Unlike other ingress controllers, Envoy supports continual reconfiguration without the requirement to do a hot reload of the process.

## How Contour works

Contour supports both Ingress objects and HTTPProxy objects which are ways to define traffic routes into a cluster. 
They are different in how the objects are structured as well as how they are implemented, however, are identical at their core intent, to configure the routing of ingress traffic. 
To make this document more clear, when we describe "Ingress", it will apply to both Ingress and HTTPProxy objects.

Generally, when Envoy is configured with the CDS endpoint, it polls the endpoint regularly, then merges the returned JSON fragment into its running configuration. If the cluster configuration returned to Envoy represents the current set of Ingress objects then Contour can be thought of as a translator from Ingress objects to Envoy cluster configurations. As Ingress objects come and go, Envoy adds and removes the relevant configuration without the need to constantly reload.

In practice, translating Ingress objects into Envoy configurations is a little more nuanced. Three Envoy configuration services must be mapped to Kubernetes: CDS, SDS, and RDS. At a minimum Contour must watch Ingress, Service, and Endpoint objects to construct responses for these services. We get these watchers more or less for free with client-go's cache/informer mechanisms, which provide edge triggered notification of addition, update, and removal of objects, as well as the lister mechanism to perform queries against the local cache of objects learnt by watching the API.

The objects collected are then processed into a directed acyclic graph (DAG) of the configuration for virtual hosts and their constituent routes.
This representation allows Contour to build up a top-level view of the routes and connect together corresponding services and TLS secrets within the cluster.
Once this new data structure is built, we can easily implement validation, authorization, as well as delegation for HTTPProxy objects.

The mapping between Envoy API calls and Kubernetes API resources is as follows:

- **CDS**: Cluster discovery service, maps closely to a Kubernetes Service, with some spillover onto an Ingress for TLS configuration (note: TLS is out of scope for the first release of Contour).
- **SDS**: Service discovery service, maps very well to an Endpoint for a Service. SDS is used by Envoy to automatically learn cluster members, which is a good match for the information contained in an Endpoint object. Envoy queries SDS with a name that Contour controls in the CDS response. Contour then uses the name as a selector to retrieve the correct set of Endpoint objects.
- **RDS**: Route discovery service, maps closest to Ingress with respect to RDS requiring virtual host names and providing prefix routing information.

## Mapping details

### CDS

CDS is most like a Kubernetes Service resource in that a Service is an abstract placeholder for concrete endpoints (pods), and an Envoy Cluster describes a set of upstreams that can have work routed to them (see RDS). There are some complications in that TLS configuration is part of CDS, and TLS information is provided by the Ingress resource in Kubernetes. TLS is out of scope for the first cut of Contour.

### SDS 

SDS is most like a Kubernetes Endpoint resource, and is probably the simplest to implement. It presents itself to Contour with an identifier we control, which maps to an Endpoint resource. Contour then transforms the Endpoint response objects into a `{ hosts: [] }` json fragment.

The identifier presented to SDS is the CDS clusters `service_name`, which we set as Service.ObjectMeta.Namespace + "/" + Sevice.ObjectMeta.Name.

### RDS

RDS is most like a Kubernetes Ingress resource. RDS routes one of prefix, path, or regex to an Envoy Cluster. The name of the Envoy cluster can be synthesized from the backend field in the IngressSpec, something like `namespace/serviceName_servicePort`, which, because it is a selector, matches the CDS object returned from transforming Service objects.

## More about mapping

We are exploring the fidelity of these translation, but because we can observe the Ingress, Service, and Endpoint objects directly, at least for straightforward cases we don't anticipate significant roadblocks. The translation functions are well isolated from the Kubernetes API and from the CDS endpoint, so we can perform comprehensive unit testing without the need for mocking. Starting with the simple case of exposing a service publicly, moving to complicated routing cases, and then to TLS, we should be able to iterate smoothly without major refactorings of the other components. This translation can be "lossy" to some degree because there is no requirement to roundtrip a RDS/CDS/SDS translation back to the original Kubernetes objects.

To respond to a CDS/RDS API request, Contour enumerates all Ingress objects in its cache. For each Ingress, we resolve the relevant Service and Endpoint objects, also from the cache. While Envoy's polling approach for CDS isn't awesome for performance, knowing that Envoy regularly polls Contour makes the job of dealing with incomplete Ingress/Service/Endpoint sets easier. If Contour doesn't have a cached value for the Ingress, Service or Endpoint during the enumeration, we ignore the Ingress (with the appropriate logging and Prometheus metrics). The intent is that either the data is available the next time Envoy polls, or the data will never be there (perhaps someone has forgotten the Service object).

All of the steps defined above are performed in response to an API request against Contour. Given we may have a high frequency poll rate (maybe even as low as 1s) there is some value in caching the output of the translation. The Add/Update/Delete events on Ingress/Service/Endpoint objects can be used as a signal to invalidate the cache.

## Terminology

- CDS. Envoy's Cluster Discovery Service. A polling based API where the Envoy process polls an HTTP endpoint for a JSON document that represents upstream clusters.
- Cluster. Envoy's word for a Kubernetes Service.

## Alternatives Considered

Envoy provides two mechanisms for reconfiguration: CDS and hot reload. Hot reload was rejected because this would leave us open to the controller "leaking" worker processes stuck holding up long running websocket connections.

## Security Concerns

Contour implicitly trusts any Ingress object on the API. We are considering a check that the Ingress and Service objects point to valid cluster resources -- in other words, that someone hasn't dumped an Ingress in there to siphon traffic out of the cluster.
