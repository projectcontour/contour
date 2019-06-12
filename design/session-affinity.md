# Session Affinity

Status: _Accepted_

This proposal describes how we will add session affinity support to Contour.

## Goals

- Support session affinity via a session cookie managed transparently by Contour.

## Non Goals

- Session Affinity via application supplied Header or Cookie.
- Session Affinity via client IP.

## Background

Session Affinity, sometimes called _Sticky Sessions_, is a pattern whereby the incoming request indicates the backend server it wishes to handle the request.
Sticky sessions are very much not a cloud native pattern, however when migrating applications to Kubernetes it may be required to support session affinity if the application requires them.

Session Affinity can be enabled in Envoy via three methods; header, cookie, and client IP.
Of these three methods, this proposal will only implement cookie based session affinity.

## High-Level Design

### User interface

The user interface for session affinity is the `strategy` key in the IngressRoute spec.
To specify that cookie based session affinity is to be used the IngressRoute author will specify `strategy: Cookie` on a per service basis.

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: httpbin
  namespace: default
spec:
  virtualhost:
    fqdn: httpbin2.davecheney.com
  routes:
  - match: /
    services:
    - name: httpbin
      port: 8080
      strategy: Cookie
```
As a route can specific multiple weighted backends, providing they choose `strategy: Cookie`, they will all be eligible for cookie based session affinity.
Once a request has been served from service-a (or service-b) subsequent requests carrying Contour's session affinity cookie will always return to their nominated server reguardless of weightings.
```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: httpbin
  namespace: default
spec:
  virtualhost:
    fqdn: httpbin2.davecheney.com
  routes:
  - match: /
    services:
    - name: service-a
      port: 8080
      strategy: Cookie
      weight: 50
    - name: service-b
      port: 8080
      strategy: Cookie
      weight: 50
```
An interesting situation occurs if multiple weighted backends choose disparate load balancing strategies: 
```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: httpbin
  namespace: default
spec:
  virtualhost:
    fqdn: httpbin2.davecheney.com
  routes:
  - match: /
    services:
    - name: service-a
      port: 8080
      strategy: Cookie
      weight: 50
    - name: service-b
      port: 8080
      strategy: random
      weight: 50
```
In this example there is a 50:50 chance that any initial request will land on service-a and be assigned a session cookie, or land on service-b and be assigned randomly to one of service-b's backends.
Statistically requests without a session cookie will eventually land on service-a, be assigned a session cookie and will exhibit an affinity towards one of service-a's members.

### Limitations

Session affinity is based on the premise that the backend servers are robust, do not change ordering, or grow and shrink according to load.
None of these properties are guaranteed by a Kubernetes cluster and will be visible to applications that rely heavily on session affinity.

Any pertibation in the set of pods backing a service risks redistributing backends around the hash ring.
This is an unavoidable consiquence of Envoy's session affinity implementation and the pods-as-cattle approach of Kubernetes.

## Detailed Design

### `internal/dag`

The `route.service.strategy` key is already processed into a `dag.Cluster` entry during `internal/dag`'s build phase.
No change is necessary.

### `internal/envoy`

The `Cluster` helper which converts `dag.Cluster`'s to Envoy `v2.Clusters` should return a value of `v2.Cluster_RING_HASH` when presented with a `LoadBalancerStrategy` of `Cookie`.
This is handled inside the `lbPolicy` helper.

The `Route` helper, when presented with a Route that dispatches to one or more Clusters with a `LoadBalancerStrategy` of `Cookie` should add a HashPolicy to the Route Action.
```go
               HashPolicy: []*route.RouteAction_HashPolicy{{
                       PolicySpecifier: &route.RouteAction_HashPolicy_Cookie_{
                               Cookie: &route.RouteAction_HashPolicy_Cookie{
                                       Name: "X-Contour-Session-Affinity",
                                       Ttl:  duration(0),
                                       Path: "/",
                               },
                       },
               }},
```

### Cookie design

The cookie assigned by Contour will have the following properties; 

- Name: `X-Contour-Session-Affinity`.
Given there is no way to reuse a session cookie provided by the application (believe me, I spent days trying to do this) we always configure a cookie named `X-Contour-Session-Affinity`.
The `X-Contour` prefix gives us a reasonable guarantee that we're not conflicting with an application set value.

 The cookie name is not user configurable because we cannot reliably use a cookie supplied by the application.
See the following section on bootstrapping for more information.
- `Expires`/`Max-Age`: not set, the cookie is a session cookie for the life of the browser session.
This seems reasonable as the fragility with session affinity means there is little value in persisting this cookie for days or weeks -- it does not represent a login token -- only a handle to in memory state on the target backend.
Further, there is no reasonable `Expires` value as none are correct;
If the cookie expires too shortly then sessions will be abruptly lost.
If the cookie's expiry is too long then we risk imbalancing the backend as sessions will be naturally attracted to the longest living server in the group.
Making this value configurable simply pushes this insoluble problem to our users. 
- Path: `/`.
The cookie applies to all routes on this virtual host in the hope that other `strategy: Cookie` backends, assuming they dispatch to the same set of servers will share the same affinity cookie.
For example consider two routes, `/cart` and `/checkout` are served by the same Kubernetes service.
```yaml
  routes:
  - match: /
    services:
    - name: static-content
      port: 8000
  - match: /cart
    services:
    - name: ecommerce-pro
      port: 8080
      strategy: Cookie
  - match: /cheeckout
    - name: ecommerce-pro
      port: 8080
      strategy: Cookie
```
Given that both routes represent the same service with `static-content` overlayed to fill in the gaps, a session started on a backend of `ecommerce-pro` via `/cart` should land on the same `ecommerce-pro` backend when the request flow reaches `/checkout`.
Placing the cookie at the `/` path permits this with few negative side effects.

 The session affinity cookie is _not_ a login cookie.
It does not represent anything about the properties of the browser's session.
It is not important what value the session affinity cookie holds, only that it is unique.

## Alternatives Considered

There are two session affinity mechanisms that we chose not to implement in this proposal.
This section gives some details for that choice

### Header based affinity

Header based affinity is similar to cookie affinity--cookies are headers after all--however header based routing assumes the remote client supplies this header, which further assumes it has knowledge of the application deployment.
That is to say, for a remote client to request that a specific pod in a deployment handle its requests, that requires _a priori_ knowledge of that pods existence.
A very uncloud native design.

If the IngressRoute author wants to route _via_ a header, we are working on that as part of the next round of routing improvements.

#### Bootstrapping issues

A related problem to Header based affinity is reusing a session cookie provided by the end user application.
This is attractive as the application would normally be supplying its own cookie which we could treat as a input in the ring hash algorythm, however this suffers from two significant issues:

- Envoy consumes the whole cookie value, not part of it.
If the application supplied session cookie contains unrelated date like previously selected page, shopping cart information, etc, then the hash of that cookie will change resulting in the request being directed to the incorrect server.
- Using the session id or key as the ring hash suffered from the bootstrapping problem of the first request being routed to backend A, which sets a session cookie, however that session cookie will _not_ hash to backend A and cause subsequent requests to arrive at the wrong server, possibly repeating this behavior.
Co-ordinating the session cookie creation between the backend application and Envoy is infeasible.

### Client IP affinity

Client IP affinity uses the remote IP address of the end user as the hash key to choose a backend.
However, retaining the remote IP of a TCP connection once it has passed through load balancers, network translation, an so on is subtle, difficult to configure, and can break silently.
The end result is traffic which was expected to be hashed randomly ends up being hashed against the internal IP address of the next hop router.
Because of the difficulty in reliably preserving client IP addresses, and the unpredictable nature of the failure to do so, we won't be adding client IP affinity in this design.

## Security Considerations

The `X-Contour-Session-Affinity` cookie contains no user identifiable data.
It is a random string generated on the first request and serves only as input to the ring hash algorithm.
Modifying the `X-Contour-Session-Affinity` cookie _could_ be used to route requests to a different pod in a service, but this is no different to presenting a request without a cookie.
