# HTTPProxy Conflict Resolution Design

Status: Draft

## Abstract

Configuration errors in HTTPProxy resources can result in objects being marked as invalid and consequently will not serve traffic.
These errors can come from invalid route configuration, missing service references, or invalid includes to other resources. 

## Background
HTTPProxy is designed as a multi-team aware Ingress, reduce annotation sprawl by providing a sensible home for configuration items.
Additionally, HTTPProxy offers a feedback loop to users via a `Status` field on the object.
Contour has further extended this to add `Conditions` which allows for a set of `Errors` and `Warnings` which inform users of errors in the HTTPProxy resource.

When the first error or warning is detected, these fields are updated and in the status of the HTTPProxy object, however if there are more errors, they are not all shown to the user, only the first.
There are ongoing efforts to change this logic to inform of all the errors or warnings dealing with a specific object.

Contour currently invalidates the entire resource if a configuration error is encountered which can cause downtime for specific routes. 

## Goals
- Serve valid traffic configurations if portions of the spec are invalid

## Non Goals
- Surface a complete set errors or warnings for a resource (this is a different issue)

## High-Level Design
Contour will set the error or warning condition when an error is encountered, but do its best to still serve valid configurations.

It's difficult to find errors and block them from breaking valid configurations due to how Contour currently processes objects.
Contour takes a passive look to the cluster by reacting to changes after they have changed. 
Due to this, Contour cannot block changes to resources before they are committed like an `Admission Controller` would be able to do (See alternatives: `HTTPProxyInstance`)

In general, Contour will set an object to be status `Error` if the request response to the user is changed from what is configured.
Contour will set a `Warning` when the object has an issue, but the response is not modified. 

## Detailed Design

### Invalid Include
Contour utilizes an inclusion model to allow for proper route/header delegation across namespaces and teams within a Kubernetes cluster.
If an include does not have a matching HTTPProxy.Name or HTTPProxy.Namespace, then Contour will set an error in the Conditions of the object.
Any requests matching the `Conditions` on the `Spec.Include` will return a `404` to the requester.

If the include was valid previously and is now no longer valid, the routes which the include previously enabled will no longer be valid.
All requests to the path specified in the `spec.conditions.prefix` will return a `404` HTTP status code. 

_Note: This is a use-case of an alternative model of implementing an `HTTPProxyInstance` resource which gives users a "last known good configuration" that can be reverted back to or an `Admission Controller`._

#### Include Summary

| Category           | Issue               | Request Response                               | Conditions |
| ------------------ | ------------------- | ---------------------------------------------- | ---------- |
| Parent Delegation  | Create Orphaned     | HTTP 404 For requests to `conditions.Prefix`   | Error      |
| Child Delegated    | Orphaned            | No response since requests can't route         | Warning    |

### Invalid Route
If a route contains configuration errors or warnings, the following table outlines how Contour will react and what response will be sent.  

#### Route Summary

| Category           | Issue               | Request Response                               | Conditions |
| ------------------ | ------------------- | ---------------------------------------------- | ---------- |
| Spec.Route.Conditions | Invalid `prefix` | HTTP 404                                       | Error      |
| Spec.Route.Conditions | Invalid `header` | HTTP 404                                       | Error      |
| Spec.Route.TimeoutPolicy | Invalid       | Request routes without timeoutpolicy set       | Warning      |
| Spec.Route.RetryPolicy | Invalid       | Request routes without retrypolicy set       | Warning      |
| Spec.Route.HTTPHealthCheckPolicy | Invalid       | Request routes without healthcheckpolicy set       | Warning      |
| Spec.Route.LoadBalancerPolicy | Invalid       | Request routes without loadbalancerpolicy set       | Warning      |
| Spec.Route.PathRewritePolicy | Invalid       | HTTP 404       | Error      |
| Spec.Route.RequestHeadersPolicy | Invalid       | Header Not Configured       | Warning      |
| Spec.Route.ResponseHeadersPolicy | Invalid       | Header Not Configured       | Warning      |

### Invalid Service
If a service contains configuration errors or warnings, the following table outlines how Contour will react and what response will be sent.

#### Single Service
For the case where only a single service is referenced from a `Spec.Route` of an HTTPProxy and has an `Error`, Contour will return a `503` status code and also set an error in the Conditions of the object.

#### Multiple Services
For the case where multiple services are referenced from a `Spec.Route` of an HTTPProxy and has an `Error`, Contour will set the invalid service weight to be `zero` so that no traffic is routed to the service in error and will also set an error in the Conditions of the object.

#### Service Summary

| Category           | Issue            | Request Response       | Conditions |
| ------------------ | ---------------- | ---------------------- | ---------- |
| Spec.Route.Service | Missing Service  | HTTP 503               | Error      |
| Spec.Route.Service | Invalid Protocol | HTTP 503               | Error      |
| Spec.Route.Service | Port Mismatch    | HTTP 503               | Error      |
| Spec.Route.Service.RequestHeadersPolicy | Invalid Header  | Header Not Configured | Warning    |
| Spec.Route.Service.ResponseHeadersPolicy | Invalid Header  | Header Not Configured | Warning    |
| Spec.Route.Service.UpstreamValidation | Invalid CACertificate | HTTP 503 | Error    |

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