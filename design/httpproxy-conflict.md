# HTTPProxy Conflict Resolution Design

Status: Approved

## Abstract

Configuration errors in HTTPProxy resources can result in objects being marked as invalid and consequently will not serve traffic.
These errors can come from invalid route configuration, missing service references, or invalid includes to other resources.
To fix this, Contour will treat errors and warning differently and where possible, make a best effort to serve partially valid configurations.
Configurations that are in an Error or Warning status, will serve HTTP error codes or not be added to configuration passed down to Envoy.

## Personas

There are two main personas that interact with this conflict design document:

- User: A request to an application, routing through Envoy looking to access whatever resource an application running in Kubernetes should serve. This user is not aware of how the application is deployed or that it uses Kubernetes or Contour.
- ResourceOwner: Someone to manages an HTTPProxy resource inside the Kubernetes cluster configuring the Route/Service combination to allow users of the application to access.

## Background
HTTPProxy is designed as a multi-team aware Ingress that reduces annotation sprawl by providing a sensible home for configuration items.
Additionally, HTTPProxy offers a feedback loop to ResourceOwners (See #Personas section below) via a `Status` field on the object.
Contour has further extended this to add `Conditions` which allows for a set of `Errors` and `Warnings` which inform users of errors in the HTTPProxy resource.

It's difficult to find errors and block them from breaking valid configurations since Contour currently processes objects after they are committed to the API server.
Due to this, Contour cannot block changes to resources before they are committed like an `Admission Controller` would be able to do (See alternatives: `HTTPProxyInstance`).

Contour currently invalidates the entire resource if a configuration error is encountered which can cause downtime for specific routes or even the entire virtual host.

When processing HTTPProxy objects, some problems should not stop the rest of the object from being processed.
Contour will set an object to be status `Error` if the request response to the `User` is changed from what is configured.
Contour will set a `Warning` when the object has an issue, but the response a `User` sees when accessing the application for which the resource is configured is not modified.

## Goals
- Serve valid traffic configurations if portions of the spec are invalid
- Define and distinguish between fatal errors, non-fatal errors, and warnings

## Non Goals
- Change how Contour processes objects today from informers set against the Kubernetes API

## Definitions

This document will describe a Fatal & Non-Fatal Errors as well as Warnings.

### Fatal Error
An error encountered while processing an HTTPProxy spec where:

- Contour stops processing the rest of the spec
- Contour does not program any part of the HTTPProxy in Envoy
  
For example, an invalid FQDN.

### Non-Fatal Error
An error encountered while processing an HTTPProxy spec where:

- Contour continues processing the rest of the spec
- Contour does program the unaffected (valid) part of the HTTPProxy in Envoy
- Users may get a different response for a given route than what was configured (e.g. a 502 instead of the desired upstream service)

For example, a route to a nonexistent service (results in a 502 being programmed)

### Warning
An error encountered while processing an HTTPProxy spec where:

- Contour continues processing the rest of the spec
- Contour does program the unaffected part of the HTTPProxy in Envoy
- Users are routed to the correct destinations, but some aspects of the request handling may be different from desired (e.g. missing timeout or retry settings)

For example, invalid retry settings on a route (the route is still programmed, but without retry settings)

## High-Level Design
Contour will set the error or warning condition when a problem is encountered, but do its best to still serve valid configurations.
The `ResourceOwner` will understand there is an issue by looking at the object's `Status.Conditions.Errors` or `Status.Conditions.Warnings`. 

Details on how Conditions are implemented can be found in the [HTTPProxy Status Conditions Design Doc](https://github.com/projectcontour/contour/blob/main/design/httpproxy-status-conditions.md#high-level-design).

## Detailed Design

### Invalid Include
Contour utilizes an inclusion model to allow for proper route/header delegation across namespaces and teams within a Kubernetes cluster.
If an include does not have a matching HTTPProxy.Name or HTTPProxy.Namespace, then Contour will set an error in the Conditions of the object.
Any requests matching the `Conditions` on the `Spec.Include` will return a `502` to the requester.

If the include was valid previously and is now no longer valid, the routes which the include previously enabled will no longer be valid.
All requests to the path specified in the `spec.conditions.prefix` will return a `502` HTTP status code. 

#### Include Summary

| Category           | Issue               | Response                               | Conditions |
| ------------------ | ------------------- | ---------------------------------------------- | ---------- |
| Parent Delegation  | Create Orphaned     | HTTP 502 For requests to `conditions.Prefix`   | Error      |
| Child Delegated    | Orphaned            | No response since requests can't route         | Warning    |

### Invalid Route
If a route contains configuration errors or warnings, the following table outlines how Contour will react and what response will be sent.  

#### Route Summary

| Category           | Issue               | Response                               | Conditions |
| ------------------ | ------------------- | ---------------------------------------------- | ---------- |
| Spec.Route.Conditions | Invalid `prefix` | HTTP 502                                       | Error      |
| Spec.Route.Conditions | Invalid `header` | HTTP 502                                       | Error      |
| Spec.Route.TimeoutPolicy | Invalid       | Request routes without timeoutpolicy set       | Warning      |
| Spec.Route.RetryPolicy | Invalid       | Request routes without retrypolicy set       | Warning      |
| Spec.Route.HTTPHealthCheckPolicy | Invalid       | Request routes without healthcheckpolicy set       | Warning      |
| Spec.Route.LoadBalancerPolicy | Invalid       | Request routes without loadbalancerpolicy set       | Warning      |
| Spec.Route.PathRewritePolicy | Invalid       | HTTP 502       | Error      |
| Spec.Route.RequestHeadersPolicy | Invalid       | Header Not Configured       | Warning      |
| Spec.Route.ResponseHeadersPolicy | Invalid       | Header Not Configured       | Warning      |

### Invalid Service
If a service contains configuration errors or warnings, the following table outlines how Contour will react and what response will be sent.

#### Single Service
For the case where only a single service is referenced from a `Spec.Route` of an HTTPProxy and has an `Error`, Contour will return a `503` status code and also set an error in the Conditions of the object.

#### Multiple Services
For the case where multiple services are referenced from a `Spec.Route` of an HTTPProxy and has an `Error`, Contour will set the invalid service weight to be `zero` so that no traffic is routed to the service in error and will also set an error in the Conditions of the object.

#### Service Summary

| Category           | Issue            | Response       | Conditions |
| ------------------ | ---------------- | ---------------------- | ---------- |
| Spec.Route.Service | Missing Service  | HTTP 503               | Error      |
| Spec.Route.Service | Invalid Protocol | HTTP 503               | Error      |
| Spec.Route.Service | Port Mismatch    | HTTP 503               | Error      |
| Spec.Route.Service.RequestHeadersPolicy | Invalid Header  | Header Not Configured | Warning    |
| Spec.Route.Service.ResponseHeadersPolicy | Invalid Header  | Header Not Configured | Warning    |
| Spec.Route.Service.UpstreamValidation | Invalid CACertificate | HTTP 502 | Error    |

_Note: Work has started to implementing some of this work in: https://github.com/projectcontour/contour/pull/3071 (Thanks @tsaarni!)_

## Alternatives Considered

### Admission Controller
An admission controller could be utilized to do these checks before the object is written to API.
This would require a way to look at a DAG to verify status information and would require further thought into how to implement but would provide the best user experience.
This could also be later layered onto Contour without changing any logic defined in this proposal. 

### HTTPProxyInstance
Another alternative is to keep a copy of a "last know good configuration" for each HTTPProxy resource.
This would allow errors to be found and written to the HTTPProxy object, but not break any routing configuration.

// ref: https://github.com/projectcontour/contour/issues/2019#issuecomment-730551796

## Open Issues
- https://github.com/projectcontour/contour/issues/2019
- https://github.com/projectcontour/contour/pull/3071
- https://github.com/projectcontour/contour/issues/3039