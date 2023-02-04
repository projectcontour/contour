# Reworking HTTPProxy duplicate route detection and reporting

Status: Draft

## Abstract
HTTPProxy include and route match conditions can have duplicates that are not detected by the current processing logic.
This design lays out a plan to change the UX of this feature as well as fix the detection and reporting of such errors to the user.

## Background
Currently HTTPProxy include match conditions are validated to prevent duplicates.
Recently we made changes to fix this validation logic as it has been incorrect for a while [in this PR](https://github.com/projectcontour/contour/pull/4931).
We found as a result of that change that [users are likely using the behavior we allowed by our logic being "wrong"](https://github.com/projectcontour/contour/issues/5014).
As a result [we had to make some changes made to loosen the validation of includes](https://github.com/projectcontour/contour/pull/5017).

Even after these changes, user configuration of routes can still generate duplicates, even if include conditions are validated.
Currently duplicate routes within an HTTPProxy or in a tree of includes will silently overwrite others they are processed after.

We realized we should ensure more consistent and correct handling of duplicate routes.
We can possibly provide a better UX if the total route is considered when validating duplicates, rather than individual segments.
However, this does come with more complications on how to report errors.
This design is an effort to rework things and present ideas to the community.

## Goals
- Establish consistent patterns for detecting duplicate route match configurations in a tree of HTTPProxies
- Establish status condition reporting pattern for notifying users of errors in their configuration
- Ensure all valid user configuration is still programmed, even if there are duplicate route errors in an HTTPProxy tree or individual HTTPProxy

## Non Goals
- Address or change the behavior of HTTPProxy routes being able to override Ingress routes

## High-Level Design
Rather than checking for duplicates at the include conditions level, we will do route match condition duplicate validation at the "leaves" of the include/route tree to consider a full route.
In the case of duplicate routes, precedence will be given to the route that comes "first", criteria described in more detail below.
A HTTPProxy that contains a duplicate route will have an error condition on it, be marked invalid, and warning conditions set on its parents, up to the root.
Any error/warning conditions will contain salient details for owners of the various affected resources to use to debug/remediate the issue.
We will attempt to program as much valid configuration as we can, i.e. don't short circuit processing an HTTPProxy because part of it is invalid.

## Detailed Design

### Single HTTPProxy

Currently the following HTTPProxy is allowed and considered valid:

```
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
spec:
  virtualhost:
    fqdn: example.com
  routes:
  - conditions:
    - prefix: /foo
    services:
    - name: s1
      port: 8080
  - conditions:
    - prefix: /foo
    services:
    - name: s2
      port: 8080
```

The actual route that is programmed will forward requests for `example.com/foo` to `s2`.

Instead, this design proposes that since the route to `s1` comes first, that route will be programmed and the second that routes to `s2` will be considered a duplicate.
Requests to `example.com/foo` will be routed to `s1`.
The `example` HTTPProxy will be set with Status below:

```
status:
  conditions:
  - errors:
    - message: duplicate match conditions defined on route matching path prefix: "/foo"
      reason: DuplicateMatchConditions
      status: "True"
      type: RouteError
    lastTransitionTime: ...
    message: At least one error present, see Errors for details
    observedGeneration: ...
    reason: ErrorPresent
    status: "False"
    type: Valid
  currentStatus: invalid
  description: At least one error present, see Errors for details
  ...
```

### Include tree

Currently this root HTTPProxy is deemed invalid due to duplicate include conditions:

```
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-root
spec:
  virtualhost:
    fqdn: example.com
  includes:
  - name: example-child1
    conditions:
    - prefix: /foo
  - name: example-child2
    conditions:
    - prefix: /foo
```

This design proposes this HTTPProxy itself is totally valid.

When paired with the following children:

```
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child1
spec:
  routes:
  - conditions:
    - prefix: /bar
    services:
    - name: s1
      port: 8080
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child2
spec:
  routes:
  - conditions:
    - prefix: /bar
    services:
    - name: s2
      port: 8080
```

Since `example-child1` is included first, it is totally valid.
`example-child2` is included second, and has a duplicate route for `example.com/foo/bar` so it is marked invalid with errors as described above.
`example-root` will be updated with Status below:

```
status:
  conditions:
  - lastTransitionTime: ...
    message: Valid HTTPProxy
    observedGeneration: ...
    reason: Valid
    status: "True"
    type: Valid
    warnings:
    - message: duplicate match conditions defined in included HTTPProxy "default/example-child2"
      reason: DuplicateMatchConditions
      status: "True"
      type: IncludeError
  currentStatus: invalid
  description: Valid HTTPProxy
  ...
```

Similarly, for the following root and children:

```
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-root
spec:
  virtualhost:
    fqdn: example.com
  includes:
  - name: example-child1
    conditions:
    - prefix: /foo
  - name: example-child2
    conditions:
    - prefix: /
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child1
spec:
  routes:
  - conditions:
    - prefix: /
    services:
    - name: s1
      port: 8080
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child2
spec:
  routes:
  - conditions:
    - prefix: /foo
    services:
    - name: s2
      port: 8080
```

`example-child2` will still be marked as a duplicate and invalid since the fully qualified match `example.com/foo` is realized by the route that is configured in both children.


**Note: The examples above all use path prefixes, but this applies to all other match condition types as well (header, query, etc.)**

### Reporting Duplicates via Status

## Alternatives Considered

### Do not set error/warning conditions on HTTPProxies with duplicate routes
Gateway API's HTTPRouteRule matches field defines it's semantics for matching precedence [here](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io%2fv1beta1.HTTPRouteRule).
In this API, there is no Status Condition set or error signalled to the user when there are duplicate route matches.
The documentation lays out the expectations on conflict resolution between different route resources and within an HTTPRoute.
We could take a similar documentation-only approach to route matching precedence, but it might leave some users with invalid configuration, confusion, etc.
This would make it simpler to report status as writing complex header/query/path match conditions in status messages can be unwieldy.

Note: this specification may change upstream in Gateway API, currently there are no conformance tests for it so as those are added we may see changes in this area.

### Error condition messages without match conditions
Instead of trying to enumerate exactly which set of match conditions caused the duplicate in the error message, we could use the json path syntax of the route to do so.
For example we could have an error message such as `Match conditions for spec.routes[2] are a duplicate of spec.routes[0]` for errors within an HTTPProxy.
Or: `Match conditions for spec.routes[2] are a duplicate of HTTPRoute foo/bar spec.routes[0]` for errors across HTTPProxies.

This might lead to complications in accounting where routes came from which could get unwieldy to implement, but some combination of this and the currently proposed solution could be used.

### Add a name field to routes
This would make generating the status messages for errors easier, we could refer to a named route instead of trying to stringify the match conditions.
However, if only used for this purpose, this might be heavyweight solution, since we would likely need to add uniqueness validation to some extent.

One point in favor of this in a general API design perspective is that Envoy has a name field for routes that is mostly used for debugging in a similar vein as this addition would be.

### Only update Status on HTTPProxy leaf that has an error
This would reduce the ability for operators/administrators to follow up on issues in their site across included HTTPProxies.
However this would reduce possible status update sprawl that may come from one bad actor/mistake/etc.

## Security Considerations

### Exposing match conditions from parent HTTPProxies down to child status
We propose adding details of the full set of duplicate match conditions to be included in Status Conditions detailing why a route is invalid.
This may be information not explicitly known/shared with the owner of the child HTTPProxy.
Since a backend configured on such a route will receive a request with all of that information, I do not think it is a security concern to include that detail in most cases, but something to think about in case any organizations have an issue with this.

## Compatibility

### Route precedence will change
Previously when duplicate route matches were present, the last one processed would overwrite any others.
With this change, the first one processed will take precedence.
This might be an unexpected change for some users and our documentation/release notes should be clear that this is the case.

I don't think there is any migration path really possible here.

### Differences in Status Conditions
Root HTTPProxies or others that have includes that were considered invalid due to duplicate include conditions previously will no longer be.
They will instead possibly have warning level Conditions set on them.
Child HTTPProxies referred to by duplicate includes will no longer be marked as orphans.
Instead child proxies that actually contain duplicate route conditions will be marked as invalid and include error conditions.

Expectations in monitoring/alerting systems may need to be adjusted and our documentation/release notes should make these changes clear.

## Implementation
The majority of changes should occur in the `HTTPProxyProcessor`.
There should not be much change to other parts of the DAG or other processing logic.

One possible implementation detail option could be to modify the `VirtualHost.AddRoute` method or add a new method that does not just overwrite routes when called, but will instead not overwrite what is there and let the caller know there was an existing route already in the DAG.
This will need some thinking how to implement if we would still like to keep our existing behavior that allows HTTPProxy routes to overwrite Ingress routes, and so on.

### Accounting for route source
If we do need to report in status what HTTPProxy a route originated from so users can sort out duplicates, we will need to add some more accounting information to the DAG, possibly on the `dag.Route` type.
This may help in other features, or be a pattern we carry forward for other resources.

### Testing
As usual we should include significant unit test coverage for these changes.
Coverage should ensure path conditions, header conditions, query conditions, etc. (in case anything is added to HTTPProxy) are all tested for duplicates.
In addition, we should add specific end to end coverage of the HTTPProxy include mechanisms, as we are lacking significant end to end testing of this area currently.

### Implement status update to children first, add parent status updates later
This is an incremental approach that would let us trial run the status update side of the feature with a smaller change at first.
If it is sufficient to only set errors on the invalid leaf HTTPProxies we may not have to continue with updating parent HTTPProxies with warnings.

### Documentation
This change should be accompanied with clear documentation detailing how the include and route match condition hierarchy works.
[This page](https://projectcontour.io/docs/v1.24.0/config/inclusion-delegation/) in particular should be updated.

## Open Issues

### Changes to child HTTPProxies will cause status updates to other resources
When a child HTTPProxy becomes invalid due to a duplicate route, it will cause its parents to receive status updates.
Additionally, if an HTTPProxy that comes earlier in processing (because it is part of an include branch that is earlier in a list of includes) changes, it can cause later HTTPProxies to become invalid and in turn cause status updates on their parents.

We will be in a state where a change to one resource could cause status updates to many others.
This could cause status update sprawl and performance concerns for deployments with a large number of HTTPProxies.
In general this is maybe not a recommended pattern, but in practice it seems unlikely to have too much of an impact.
If there are environments that have these sorts of invalid/duplicate route configurations widespread, that seems like more of a pressing issue because routing is not likely to be even working.
We should take a poll of how deep trees of HTTPProxy includes get in larger scale users, to see the impact of propagating warnings up the tree.

