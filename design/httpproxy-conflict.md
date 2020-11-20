# HTTPProxy Conflict Resolution Design

Status: Draft

## Abstract

Configuration errors in HTTPProxy resources can result in objects being marked as invalid and conversely will not serve traffic.
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

- If an invalid route is encountered, Contour will return a `404` HTTP status code to the requester
- If an invalid service is encountered,Contour will return a`503` HTTP status code to the requester

It's difficult to find errors and block them from breaking valid configurations due to how Contour currently processes objects.
Contour takes a passive look to the cluster by reacting to changes after they have changed. 
Due to this, Contour cannot block changes to resources before they are committed like an `Admission Controller` would be able to do (See alternatives: `HTTPProxyInstance`)

## Detailed Design

### Invalid Include
Contour utilizes an inclusion model to allow for proper route/header delegation across namespaces and teams within a Kubernetes cluster.
If an include does not have a matching HTTPProxy.Name or HTTPProxy.Namespace, then Contour will set an error in the Conditions of the object.
Any requests matching the `Conditions` on the `Spec.Include` will return a `404` to the requester.

If the include was valid previously and is now no longer valid, the routes will no longer be valid which the include previously enabled.

The logic described above should only apply if the object has a proper delegation chain to the defined `Conditions` on the include. 
If an object creates an `Include` to values that it does not have permission to, then it shouldn't create `404` responses.

_Note: This is a use-case of an alternative model of implementing an `HTTPProxyInstance` resource which gives users a "last known good configuration" that can be reverted back to or an `Admission Controller`._

### Invalid Route
If an HTTPProxy object references a `Conditions.Prefix` that it does not have a proper delegation chain from a `root` HTTPProxy, then that resource will be marked as `orphaned`.
Contour will set the Conditions with the orphaned status, ignore this route, but serve all other valid routes configured in the object.

### Invalid or Missing Kubernetes Service
From a `Route` in an HTTPProxy, services are referenced in a variety of places, but are primarily referenced from the `Spec.Routes` section.
Services could be configured in error by not matching a service name or by the service not existing at all when it might of previously existed.

_Note: Work has started to implementing this work in: https://github.com/projectcontour/contour/pull/3071 (Thanks @tsaarni!)_

#### Single Service
For the case where only a single service is referenced from a `Spec.Route` of an HTTPProxy, Contour will return a `503` status code and also set an error in the Conditions of the object.

#### Multiple Services
For the case where multiple services are referenced from a `Spec.Route` of an HTTPProxy, Contour will set the invalid service weight to be `zero` so that no traffic is routed to it and will also set an error in the Conditions of the object.

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