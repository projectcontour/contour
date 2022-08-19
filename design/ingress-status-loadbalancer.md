# Ingress Status Loadbalancer Support

Status: _Draft_

This proposal describes how to add support for the `ingress.status.loadBalancer` field to Contour.

## Goals

- Populate the status.loadbalancer field in all `networking.k8s.io/v1beta.Ingress` objects managed by Contour.
- Contour pass the Ingress compliance tests for `networking.k8s.io/v1beta.Ingress` objects.

## Non Goals

- Similar functionality for IngressRoute (deprecated) and HTTPProxy are out of scope.

## Background

The Kubernetes Ingress object provides a field on the object's status to record address details for how access the ingresses virtualhosts from outside the cluster.
This is known as the status loadbalancer information after the name of the fields on the Ingress Object.

Details from the Kubernetes spec [are here](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#loadbalancerstatus-v1-core).

## High-Level Design

There are two components to this solution

1. Discovering the loadbalancer information
2. Writing the loadbalancer information to the ingress objects.

The former, the discovery will need to support

1. Hard coded IP/address details (actually a list, and it can be an address or an IP)
2. Discovering information from the service document.

### Hard coded load balancer addresses

While the hard coded name approach sounds simpler, and is, it will likely not be the common use case, nor will it be the default.
This is for several reasons.

- The hard coded address model implies either the cluster is running on prem, or using host networking.
- The hard coded IP model requires the administrator to know the IP addresses of the _envoy_ service (not the contour service) which may change over time.

The configuration will likely go into Contour's configuration file.

These restrictions (which are administrative, not ones we've placed on the solution) make this bookkeeping expensive, prohibitively in the case of a highly dynamic cluster.

### Dynamic load balancer address discovery

The far more common case will be dynamic load balancer discovery.
In this model Contour will track a service document (default to projectcontour/envoy, but configurable) and use those details as the list of IP addresses to write back to the ingress' status.

The nginx controller goes to some lengths to try to dynamically figure out what it's status document as well as other side channels to discover the address if that fails.
For this first implementation I propose we stick to the discovery methods outlined above.

### Status updater

The latter, writing the status to valid ingress objects, waits to be elected the leader then updates the status document on all of the ingress objects within Contour's class.

As new ingress objects arrive they will need to be updated.

Note: there appears to be no requirement that if Contour is shutting down we delete the status field on the ingress on the way out.
Initially this feels wrong but consider the scenarios:

- Leadership has deposed the current Contour causing it to shut down; the new leader will be responsible for updating status. Best that during this transition observers of the Ingress document do not see status disappear and reappear quickly.
- Contour is shut down for a long period of time. Yes, the status information will be incorrect, but there is no correct value, no ingress controller is serving that vhost.
- Contour is being replaced with something else, then the responsibility to _overwrite_ the status information falls to Contour's replacement.

## Detailed Design

I think the design should involve two goroutines added to cmd/contour's workgroup.
The first will be responsible for load balancer discovery, the second will be responsible for updating status documents.

### Discovery worker

The discovery worker should look something like this pseudocode:

1. worker starts up, blocks on leadership election.
2. when elected leader it checks the mode.
    * if discovery is hard coded in the config file it transmits these addresses over a channel to the other worker, then goes to sleep until deposed and/or shutdown.
    * if discovery is dynamic it opens a watcher on the specified (defaults, config file, cli flags, in that order) service document and each time it changes sends the address details to the other worker until deposed or shutdown.

Open question: if the service document goes away, should we transmit an empty set of addresses to the status worker to indicate it should remove the status.loadbalancer details?

The send from the discovery worker to the status worker will be over an unbuffered channel so the discovery worker cannot send until the status worker has been elected leader and is ready to receive IP details.
Care should be taken to ensure the discovery worker can respond to the group's shutdown signal.

### Status worker

The status worker should look something like this pseudocode:

1. the worker starts up, blocks on leadership election
2. once elected the worker blocks until it has received a set of addresses
3. the worker starts a watch on all ingress objects.
4. for each ingress observed, if the even is an add or update, we patch it with the latest set of addresses. For deletes, we do nothing.

The open question is how to deal with the discovery worker sending a new set of addresses after the initial pass.
It feels like we need to keep a side cache of ingress objects we have previously seen to go back and bulk update all the ingresses' seen so far.

## Security Considerations

n/a
