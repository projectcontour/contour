# Session Affinity

Status: _Draft_

Session Affinity, sometimes called _Sticky Sessions_, is a pattern whereby the incoming request indicates the backend server it wishes to handle the request.
Sticky sessions are very much not a cloud native pattern, however when migrating applications to Kubernetes it may be required to support session affinity if the application requires them.

This proposal describes how session affinity support will be added to Contour.

## Goals

- Support session affinity via a session cookie managed transparently by Contour.

## Non Goals

- Session Affinity via application supplied Header or Cookie.
- Session Affinity via client IP.

## Background

Session Affinity can be enabled in Envoy via three methods; header, cookie, and client IP.
Of these three methods, this proposal will only implement cookie based session affinity.

Header based affinity is similar to cookies--cookies are headers after all--however header based routing assumes the remote client supplies this header, which further assumes it has knowledge of the application deployment.
That is to say, for a remote client to request that a specific pod in a deployment handle its requests, that requires a priori knowledge of that pods existence.
A very uncloud native design.

If the IngressRoute author wants to route _via_ a header, we are working on that as part of the next round of routing improvements.

Client IP affinity will also not be offered.
This is because retaining the remote IP of a TCP connection once it has passed through load balancers, network translation, an so on is subtle, difficult to configure, and can break silently.
The end result is traffic which was expected to be hashed randomly ends up being hashed against the internal IP address of the next hop router.
Because of the difficulty in reliably preserving client IP addresses, and the unpredictable nature of the failure to do so, we won't be adding client IP affinity in this design.

## High-Level Design

### User interface

The user interface for session affinity is the `strategy` key in the IngressRoute spec.
To specify that cookie based session affinity is to be used the IngressRoute author will specify `strategy: cookie`.

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
      strategy: cookie
```

Multiple route backends can be specified and, providing they choose `strategy: cookie`, they will all be eligible for cookie based session affinity.

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
      strategy: cookie
      weight: 50
    - name: service-b
      port: 8080
      strategy: cookie
      weight: 50
```
However if a backing service does not specify `strategy: cookie` then requests landing on those backends will not receive a session cookie and will use the nominated strategy instead
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
      strategy: cookie
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

## Detailed Design

### `internal/dag`

The `route.service.strategy` key is already processed into a `dag.Cluster` entry during `internal/dag`'s build phase.
No change is necessary.

### `internal/envoy`

The `Cluster` helper which converts `dag.Cluster`'s to Envoy `v2.Clusters` should return a value of `v2.Cluster_RING_HASH` when presented with a `LoadBalancerStrategy` of `cookie`.
This is handled inside the `lbPolicy` helper.

The `Route` helper, when presented with a Route that has one or more Clusters with a `LoadBalancerStrategy` of `cookie` should add a HashPolicy to the Route Action.
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

- Name: `X-Contour-Session-Affinity`. Given there is no way to reuse a session cookie provided by the application (believe me, I spent days trying to do this) we always configure a cookie named X-Contour-Session-Affinity. The X-Contour prefix gives us a good guarantee that we're not conflicting with an application set value.
- Expires / Max-Age: not set, the cookie is a session cookie for the life of the browser session. This seems reasonable as the fragility with session affinity means there is little value in persisting this cookie for days or weeks -- it does not represent a login token -- only a handle to in memory state on the target backend.
- Path: `/`. The cookie applies to all routes on this virtual host in the hope that other `strategy: cookie` backends, assuming they dispatch to the same set of servers will share the same affinity cookie.

## Alternatives Considered

Most applications that require session affinity already use a session cookie.
Various other sticky session implementations, notably nginx and apache, were studied to see if it was possible to reuse the session cookie supplied by the application however this proved fruitless as: 

a. The Apache/Tomcat jvmroute session persistence method requires each backend application instance to know its own name and co-ordinate with the load balancer. It is infeasable to attempt this inside a Kubernetes cluster.
b. Using the session id or key as the ring hash suffered from the bootstrapping problem of the first request being routed to backend A, which sets a session cookie, however that session cookie will _not_ hash to backend A and cause subsequent requests to arrive at the wrong server, possibly repeating this behavior. Co-ordinating the session cookie creation between the backend application and Envoy is infeasble.

## Security Considerations

The `X-Contour-Session-Affinity` cookie contains no user identifiable data.
It is a random string generated on the first request and serves only as input to the ring hash algorithm.
Modifying the `X-Contour-Session-Affinity` cookie _could_ be used to route requests to a different pod in a service, but this is no different to presenting a request without a cookie.
