# Envoy v2 gRPC API support

Contour currently supports the Envoy v1 JSON discovery APIs.
This document proposes a process to migrate to the upcoming v2 gRPC discovery APIs, and explains why it's worth doing.

## Goals

- Contour will offer full support for the v2 gRPC Envoy APIs.
- When deployed as an Ingress controller, each Envoy container will treat its Contour sidecar container as its [management server][0].

## Non-goals

- Removal of the v1 API support. This may happen at a later date, but is not part of this proposal.

## Background

Envoy supports calls to APIs for configuration of routes, clusters, cluster members, health checks, and so on.
Hereafter these APIs are collectively known by their colloquialism, xDS (for the amalgam of CDS, SDS, RDS, and so on).
When deployed as an Ingress controller, Contour acts as a [management server][0] for Envoy, providing the xDS APIs over REST/JSON.
These are known as the v1 APIs.

The combination of Contour as the v1 API server and Envoy as the forward proxy works well today.
Open design concerns remain, however, related to the polling based nature of the v1 API.
A poll from Envoy to Contour occurs at a fixed interval (plus some additional jitter) and is required for Envoy to learn changes to routes, services, and endpoints in Kubernetes.
The length of the polling interval directly affects the latency of service and endpoint changes in the Kubernetes API showing up in the Envoy configuration.
Too long, and Envoy lags behind Pod Endpoints scaling up and down. Ingress objects remain invisible for extended periods, eroding trust.
Too short, and resources are wasted as Contour repetitively constructs lengthy JSON documents that contain minimally changed information that Envoy must parse and discard.

The solution to the latency problem is the v2 gRPC APIs, which both provide a more efficient protocol buffer wire representation, and add the ability to stream changes to Envoy as they happen.

## High-Level Design

Each of the xDS APIs follows the same form: a Fetch operation and a Stream operation.
The Fetch operation is almost identical to its respective v1 JSON API.
The Stream operation is similar to the Kubernetes Watch API.

The high level process to add support for v2 gRPC:

1. Wire up a v2 version of a Fetch operation to a cache of Envoy gRPC types. This cache is populated by business logic attached to the Kubernetes API server watcher.
2. Replace the current hard coded cache walking logic in the v1 REST API server with:
   a. a call to the v2 gRPC Fetch operation of the same type
   b. a translation function to convert the v2 type into a v1 type
3. Wire up a v2 version of Stream that triggers a call to Fetch and iterates through the result whenever the relevant watcher receives a notification.

## Detailed Design

Both Fetch and Stream APIs _always_ transmit the full contents of the cache to Envoy.
This allows Envoy to adjust its configuration, adding or removing entries that are missing in either the current or the new configuration.

Currently the Kubernetes API watchers feed into a cache of their respective Kubernetes types.
When a v1 API call comes in, the API server iterates over all the objects in the cache for the API call, and converts the objects on the fly to the matching Envoy objects.
This is quite wasteful, because Envoy polls at a high rate, and almost always no data has changed since the last poll.

The solution, which also prepares for v2, is to move this cache from the Kubernetes side to the Envoy side. That is, the cache holds the results of translating the object from Kubernetes to v1 Envoy.
The v1 API call from Envoy is satisfied by simply dumping the cache over the wire in JSON format.

Placing the cache after the conversion logic permits smarter analysis. For example:

- Contour can avoid creating Envoy Cluster configurations for Ingress objects that are excluded with the `kubernetes.io/ingress.class` annotation.
- Contour can avoid creating duplicate Cluster configurations for named Service ports by canonicalizing the port name of any Service to its integer value.

From there, the population of a second cache of v2 gRPC Envoy types is straightforward.

These behaviors are not possible today because the v1 conversion routines have access to only a single Kubernetes object at a time.

### Fetch

Currently the v1 REST APIs iterate over a cache of values populated via the respective Kubernetes watchers; Ingress objects for RDS, Service objects for CDS, and so on. The logic for this is currently hard coded into the v1 REST API server.

Once a v2 Fetch gRPC implementation is available the v1 REST API should be rewritten in terms of the v2 Fetch API, then translated back to v1 JSON.
There is a small performance overhead in an extra layer of translation.
Given the goal is to move Contour off the v1 APIs, a modest overhead during the transition is a reasonable cost for most deployments.

The current end-to-end tests for the v1 REST API should be sufficient to ensure this change is backwards compatible.

### Stream

The Stream API can be thought of as a server-initiated Fetch response.

The Stream API returns full copies of its cache to the caller at a schedule determined by the Stream API implementation.
For example, a Stream API implementation that replies with the cache contents every 30 seconds would be compliant with the API.
This approach, however, just moves the polling from the client to the server, with limited benefit.

We propose to implement Stream like this (in peudocode):
```
func Stream() {
        for {
                select {
                case <- quit:
                        // clean up and return 
                case <- updatesignal:
                        req := new(FetchRequest)
                        res := Fetch(req)
                        Write(res)
                }
        }
}
```

## Alternatives Considered

An unexplored alternative to using the v2 gRPC APIs might be to add a hook on the Envoy management API to request a poll.
When Contour learns of changes in API server documents, it would ping the management API endpoint to request a poll.

We did not explore this option because the strong preference of the Envoy team is to pursue the gRPC API mechanism.

[0]: https://github.com/envoyproxy/data-plane-api/blob/master/XDS_PROTOCOL.md
