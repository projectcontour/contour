# Rate Limiting

- [Overview](#overview)
- [Local Rate Limiting](#local-rate-limiting)
- [Global Rate Limiting](#global-rate-limiting)

## Overview

Rate limiting is a means of protecting backend services against unwanted traffic.
This can be useful for a variety of different scenarios:

- Protecting against denial-of-service (DoS) attacks by malicious actors
- Protecting against DoS incidents due to bugs in client applications/services
- Enforcing usage quotas for different classes of clients, e.g. free vs. paid tiers
- Controlling resource consumption/cost

Envoy supports two forms of HTTP rate limiting: **local** and **global**.

In local rate limiting, rate limits are enforced by each Envoy instance, without any communication with other Envoys or any external service.

In global rate limiting, an external rate limit service (RLS) is queried by each Envoy via gRPC for rate limit decisions.

Contour supports both forms of Envoy's rate limiting.

## Local Rate Limiting

The `HTTPProxy` API supports defining local rate limit policies that can be applied to either individual routes or entire virtual hosts.
Local rate limit policies define a maximum number of requests per unit of time that an Envoy should proxy to the upstream service.
Requests beyond the defined limit will receive a `429 (Too Many Requests)` response by default.
Local rate limit policies program Envoy's [HTTP local rate limit filter][1].

It's important to note that local rate limit policies apply *per Envoy pod*.
For example, a local rate limit policy of 100 requests per second for a given route will result in *each Envoy pod* allowing up to 100 requests per second for that route.

### Defining a local rate limit

Local rate limit policies can be defined for either routes or virtual hosts. A local rate limit policy requires a `requests` and a `units` field, defining the *number of requests per unit of time* that are allowed. `Requests` must be a positive integer, and `units` can be `second`, `minute`, or `hour`. Optionally, a `burst` parameter can also be provided, defining the number of requests above the baseline rate that are allowed in a short period of time. This would allow occasional larger bursts of traffic not to be rate limited.

Local rate limiting for the virtual host:
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  namespace: default
  name: ratelimited-vhost
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    rateLimitPolicy:
      local:
        requests: 100
        unit: hour
        burst: 20
  routes:
  - conditions:
    - prefix: /s1
    services:
    - name: s1
      port: 80
  - conditions:
    - prefix: /s2
    services:
    - name: s2
      port: 80
```

Local rate limiting for the route:
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  namespace: default
  name: ratelimited-route
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - conditions:
    - prefix: /s1
    services:
    - name: s1
      port: 80
    rateLimitPolicy:
      local:
        requests: 20
        unit: minute
  - conditions:
    - prefix: /s2
    services:
    - name: s2
      port: 80
```

### Customizing the response

#### Response code

By default, Envoy returns a `429 (Too Many Requests)` when a request is rate limited.
A non-default response code can optionally be configured as part of the local rate limit policy, in the `responseStatusCode` field.
The value must be in the 400-599 range.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  namespace: default
  name: custom-ratelimit-response
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - conditions:
    - prefix: /s1
    services:
    - name: s1
      port: 80
    rateLimitPolicy:
      local:
        requests: 20
        unit: minute
        responseStatusCode: 503 # Service Unavailable 
```

#### Headers

Headers can optionally be added to rate limited responses, by configuring the `responseHeadersToAdd` field.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  namespace: default
  name: custom-ratelimit-response
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - conditions:
    - prefix: /s1
    services:
    - name: s1
      port: 80
    rateLimitPolicy:
      local:
        requests: 20
        unit: minute
        responseHeadersToAdd:
        - name: x-contour-ratelimited
          value: "true"
```

## Global Rate Limiting

The `HTTPProxy` API also supports defining global rate limit policies on routes and virtual hosts.

In order to use global rate limiting, you must first select and deploy an external rate limit service (RLS).
There is an [Envoy rate limit service implementation][2], but any service that implements the [RateLimitService gRPC interface][3] is supported.

### Configuring an external RLS with Contour

Once you have deployed your RLS, you must configure it with Contour.

Define an extension service for it (substituting values as appropriate):
```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ExtensionService
metadata:
  namespace: projectcontour
  name: ratelimit
spec:
  protocol: h2
  services:
    - name: ratelimit
      port: 8081
```

Now add a reference to it in the Contour config file:
```yaml
rateLimitService:
  # The namespace/name of the extension service.
  extensionService: projectcontour/ratelimit
  # The domain value to pass to the RLS for all rate limit
  # requests. Acts as a container for a set of rate limit
  # definitions within the RLS.
  domain: contour
  # Whether to allow requests to proceed when the rate limit
  # service fails to respond with a valid rate limit decision
  # within the timeout defined on the extension service.
  failOpen: true
```

### Defining a global rate limit policy

Global rate limit policies can be defined for either routes or virtual hosts. Unlike local rate limit policies, global rate limit policies do not directly define a rate limit. Instead, they define a set of request descriptors that will be generated and sent to the external RLS for each request. The external RLS then makes the rate limit decision based on the descriptors and returns a response to Envoy.

A global rate limit policy for the virtual host:
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  namespace: default
  name: ratelimited-vhost
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    rateLimitPolicy:
      global:
        descriptors:
          # the first descriptor has a single key-value pair:
          # [ remote_address=<client IP> ].
          - entries:
              - remoteAddress: {}
          # the second descriptor has two key-value pairs:
          # [ remote_address=<client IP>, vhost=local.projectcontour.io ].
          - entries:
              - remoteAddress: {}
              - genericKey:
                  key: vhost
                  value: local.projectcontour.io
  routes:
  - conditions:
    - prefix: /s1
    services:
    - name: s1
      port: 80
  - conditions:
    - prefix: /s2
    services:
    - name: s2
      port: 80
```

A global rate limit policy for the route:
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  namespace: default
  name: ratelimited-route
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - conditions:
    - prefix: /s1
    services:
    - name: s1
      port: 80
    rateLimitPolicy:
      global:
        descriptors:
          # the first descriptor has a single key-value pair:
          # [ remote_address=<client IP> ].
          - entries:
              - remoteAddress: {}
          # the second descriptor has two key-value pairs:
          # [ remote_address=<client IP>, prefix=/s1 ].
          - entries:
              - remoteAddress: {}
              - genericKey:
                  key: prefix
                  value: /s1
  - conditions:
    - prefix: /s2
    services:
    - name: s2
      port: 80
```

[1]: https://www.envoyproxy.io/docs/envoy/v1.17.0/configuration/http/http_filters/local_rate_limit_filter#config-http-filters-local-rate-limit
[2]: https://github.com/envoyproxy/ratelimit
[3]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ratelimit/v3/rls.proto
