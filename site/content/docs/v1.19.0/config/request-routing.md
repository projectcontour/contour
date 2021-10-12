# Request Routing

A HTTPProxy object must have at least one route or include defined.
In this example, any requests to `multi-path.bar.com/blog` or `multi-path.bar.com/blog/*` will be routed to the Service `s2`.
All other requests to the host `multi-path.bar.com` will be routed to the Service `s1`.

```yaml
# httpproxy-multiple-paths.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-paths
  namespace: default
spec:
  virtualhost:
    fqdn: multi-path.bar.com
  routes:
    - conditions:
      - prefix: / # matches everything else
      services:
        - name: s1
          port: 80
    - conditions:
      - prefix: /blog # matches `multi-path.bar.com/blog` or `multi-path.bar.com/blog/*`
      services:
        - name: s2
          port: 80
```

In the following example, we match on headers and send to different services, with a default route if those do not match.

```yaml
# httpproxy-multiple-headers.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-paths
  namespace: default
spec:
  virtualhost:
    fqdn: multi-path.bar.com
  routes:
    - conditions:
      - header:
          name: x-os
          contains: ios
      services:
        - name: s1
          port: 80
    - conditions:
      - header:
          name: x-os
          contains: android
      services:
        - name: s2
          port: 80
    - services:
        - name: s3
          port: 80
```

## Conditions

Each Route entry in a HTTPProxy **may** contain one or more conditions.
These conditions are combined with an AND operator on the route passed to Envoy.
Conditions can be either a `prefix` or a `header` condition.

#### Prefix conditions

Paths defined are matched using prefix conditions.
Up to one prefix condition may be present in any condition block.

Prefix conditions **must** start with a `/` if they are present.

#### Header conditions

For `header` conditions there is one required field, `name`, and six operator fields: `present`, `notpresent`, `contains`, `notcontains`, `exact`, and `notexact`.

- `present` is a boolean and checks that the header is present. The value will not be checked.

- `notpresent` similarly checks that the header is *not* present.

- `contains` is a string, and checks that the header contains the string. `notcontains` similarly checks that the header does *not* contain the string.

- `exact` is a string, and checks that the header exactly matches the whole string. `notexact` checks that the header does *not* exactly match the whole string.

## Multiple Upstreams

One of the key HTTPProxy features is the ability to support multiple services for a given path:

```yaml
# httpproxy-multiple-upstreams.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-upstreams
  namespace: default
spec:
  virtualhost:
    fqdn: multi.bar.com
  routes:
    - services:
        - name: s1
          port: 80
        - name: s2
          port: 80
```

In this example, requests for `multi.bar.com/` will be load balanced across two Kubernetes Services, `s1`, and `s2`.
This is helpful when you need to split traffic for a given URL across two different versions of an application.

### Upstream Weighting

Building on multiple upstreams is the ability to define relative weights for upstream Services.
This is commonly used for canary testing of new versions of an application when you want to send a small fraction of traffic to a specific Service.

```yaml
# httpproxy-weight-shfiting.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: weight-shifting
  namespace: default
spec:
  virtualhost:
    fqdn: weights.bar.com
  routes:
    - services:
        - name: s1
          port: 80
          weight: 10
        - name: s2
          port: 80
          weight: 90
```

In this example, we are sending 10% of the traffic to Service `s1`, while Service `s2` receives the remaining 90% of traffic.

HTTPProxy weighting follows some specific rules:

- If no weights are specified for a given route, it's assumed even distribution across the Services.
- Weights are relative and do not need to add up to 100. If all weights for a route are specified, then the "total" weight is the sum of those specified. As an example, if weights are 20, 30, 20 for three upstreams, the total weight would be 70. In this example, a weight of 30 would receive approximately 42.9% of traffic (30/70 = .4285).
- If some weights are specified but others are not, then it's assumed that upstreams without weights have an implicit weight of zero, and thus will not receive traffic.

### Traffic mirroring

Per route,  a service can be nominated as a mirror.
The mirror service will receive a copy of the read traffic sent to any non mirror service.
The mirror traffic is considered _read only_, any response by the mirror will be discarded.

This service can be useful for recording traffic for later replay or for smoke testing new deployments.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: traffic-mirror
  namespace: default
spec:
  virtualhost:
    fqdn: www.example.com
  routes:
    - conditions:
      - prefix: /
      services:
        - name: www
          port: 80
        - name: www-mirror
          port: 80
          mirror: true
```

## Response Timeouts

Each Route can be configured to have a timeout policy and a retry policy as shown:

```yaml
# httpproxy-response-timeout.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: response-timeout
  namespace: default
spec:
  virtualhost:
    fqdn: timeout.bar.com
  routes:
  - timeoutPolicy:
      response: 1s
      idle: 10s
    retryPolicy:
      count: 3
      perTryTimeout: 150ms
    services:
    - name: s1
      port: 80
```

In this example, requests to `timeout.bar.com/` will have a response timeout policy of 1s.
This refers to the time that spans between the point at which complete client request has been processed by the proxy, and when the response from the server has been completely processed.

- `timeoutPolicy.response` Timeout for receiving a response from the server after processing a request from client.
If not supplied, Envoy's default value of 15s applies.
More information can be found in [Envoy's documentation][4].
- `timeoutPolicy.idle` Timeout for how long the proxy should wait while there is no activity during single request/response (for HTTP/1.1) or stream (for HTTP/2).
Timeout will not trigger while HTTP/1.1 connection is idle between two consecutive requests.
If not specified, there is no per-route idle timeout, though a connection manager-wide stream idle timeout default of 5m still applies.
More information can be found in [Envoy's documentation][6].

TimeoutPolicy durations are expressed in the Go [Duration format][5].
Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
The string "infinity" is also a valid input and specifies no timeout.
A value of "0s" will be treated as if the field were not set, i.e. by using Envoy's default behavior.
Example input values: "300ms", "5s", "1m".

- `retryPolicy`: A retry will be attempted if the server returns an error code in the 5xx range, or if the server takes more than `retryPolicy.perTryTimeout` to process a request.

- `retryPolicy.count` specifies the maximum number of retries allowed. This parameter is optional and defaults to 1.

- `retryPolicy.perTryTimeout` specifies the timeout per retry. If this field is greater than the request timeout, it is ignored. This parameter is optional.
  If left unspecified, `timeoutPolicy.request` will be used.

## Load Balancing Strategy

Each route can have a load balancing strategy applied to determine which of its Endpoints is selected for the request.
The following list are the options available to choose from:

- `RoundRobin`: Each healthy upstream Endpoint is selected in round robin order (Default strategy if none selected).
- `WeightedLeastRequest`:  The least request load balancer uses different algorithms depending on whether hosts have the same or different weights in an attempt to route traffic based upon the number of active requests or the load at the time of selection. 
- `Random`: The random strategy selects a random healthy Endpoints.
- `RequestHash`: The request hashing strategy allows for load balancing based on request attributes. An upstream Endpoint is selected based on the hash of an element of a request. Requests that contain a consistent value in a HTTP request header for example will be routed to the same upstream Endpoint. Currently only hashing of HTTP request headers is supported.
- `Cookie`: The cookie load balancing strategy is similar to the request hash strategy and is a convenience feature to implement session affinity, as described below.

More information on the load balancing strategy can be found in [Envoy's documentation][7].

The following example defines the strategy for the route `/` as `WeightedLeastRequest`.

```yaml
# httpproxy-lb-strategy.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: lb-strategy
  namespace: default
spec:
  virtualhost:
    fqdn: strategy.bar.com
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1-strategy
          port: 80
        - name: s2-strategy
          port: 80
      loadBalancerPolicy:
        strategy: WeightedLeastRequest
```

The below example demonstrates how header hash load balancing policies can be configured:

```yaml
# httpproxy-lb-request-hash.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: lb-request-hash 
  namespace: default
spec:
  virtualhost:
    fqdn: request-hash.bar.com
  routes:
  - conditions:
    - prefix: /
    services:
    - name: httpbin
      port: 8080
    loadBalancerPolicy:
      strategy: RequestHash
      requestHashPolicies:
      - headerHashOptions:
          headerName: X-Some-Header
        terminal: true
      - headerHashOptions:
          headerName: User-Agent
```

In this example, if a client request contains the `X-Some-Header` header, the value of the header will be hashed and used to route to an upstream Endpoint. This could be used to implement a similar workflow to cookie-based session affinity by passing a consistent value for this header. If it is present, because it is set as a `terminal` hash option, Envoy will not continue on to process to `User-Agent` header to calculate a hash. If `X-Some-Header` is not present, Envoy will use the `User-Agent` header value to make a routing decision.

## Session Affinity

Session affinity, also known as _sticky sessions_, is a load balancing strategy whereby a sequence of requests from a single client are consistently routed to the same application backend.
Contour supports session affinity on a per route basis with `loadBalancerPolicy` `strategy: Cookie`.

```yaml
# httpproxy-sticky-sessions.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpbin
  namespace: default
spec:
  virtualhost:
    fqdn: httpbin.davecheney.com
  routes:
  - services:
    - name: httpbin
      port: 8080
    loadBalancerPolicy:
      strategy: Cookie
```

Session affinity is based on the premise that the backend servers are robust, do not change ordering, or grow and shrink according to load.
None of these properties are guaranteed by a Kubernetes cluster and will be visible to applications that rely heavily on session affinity.

Any perturbation in the set of pods backing a service risks redistributing backends around the hash ring.

[4]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-timeout
[5]: https://godoc.org/time#ParseDuration
[6]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-idle-timeout
[7]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/overview
