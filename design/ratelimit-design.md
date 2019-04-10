# Envoy Rate Limiting Support

The rate limit service configuration specifies the global rate limit service Envoy should talk to when it needs to make global rate limit decisions.
If no rate limit service is configured, a “null” service will be used which will always return OK if called.

Envoy's support of RateLimiting requires a rate limiting service to be exposed and support the gRPC IDL specified in [rls.proto](https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/service/ratelimit/v2/rls.proto#envoy-api-file-envoy-service-ratelimit-v2-rls-proto).

## Goals

- Allow RateLimits to be applied to routes
- Use Lyft's [ratelimiting implementation](https://github.com/lyft/ratelimit)

## Non-goals

- Support additional RateLimiting implementations
- For the initial implementation, do not tightly integrate the RateLimiting service into Contour
- Contour will not write the RateLimiter configuration (future versions could to make the integration smoother)

## High-level Design

RateLimiting will be disabled by default and will be required to be enabled.
The RateLimiting implementation will determine if the request sent to Envoy should be serviced or not.
Contour will require configuration to enable the rate limiting HTTP filter as well as pointing Envoy to the rate limit implementation service.
The reference implementation from Lyft relies on an instance of Redis to be available. 
A simple example will be provided but it will be an integration item to determine how available the Redis cluster needs to be based upon business requirements.
Envoy v1.8.0 or higher will be required since it has functionality enabled for dynamic rate limiting configuration (envoyproxy/envoy#4669 & envoyproxy/envoy#5242).
The `IngressRoute` spec will be modified to allow for enabling rate limiting per Route.
In addition, a set of annotations will be added to allow for enabling rate limiting when using `Ingress` resource.

Initially support a subset of the rate limiting features available by looking to implement rate limiting via a `key` which will be automatically determined by Contour for that route.
Contour will support rate limiting via the `remote_address` presented to Envoy generically as well as allowing users to specify an IP address which will allow for blocking specific IPs (by defining a `requests_per_unit` of zero).

In the event a user defines `rateLimits` via IngressRoute, but are not enabled, the status field of that object will be updated to reflect the error.
If an Ingress object has the appropriate annotation, a log message will be written which will explain the error encountered.

## Detailed Design

### Lyft RateLimit Implementation

___Note: Much of the following documentation is from Lyft's implementation [github repo](https://github.com/lyft/ratelimit), but copied here for completeness of this design document.___

The rate limit service is a Go/gRPC service designed to enable generic rate limit scenarios from different types of applications. Applications request a rate limit decision based on a domain and a set of descriptors. The service reads the configuration from disk via runtime, composes a cache key, and talks to the Redis cache. A decision is then returned to the caller.

New arguments to Contour:

- Contour will configure the domain it uses to be `contour` by default, but a new argument will be added which will allow users to customize this value (`--rate-limit-domain`)
- RateLimit service dns or IP and port to use, see cluster example below (`--rate-limit-service-name` & `--rate-limit-service-port`)
- In the event the rate limiting service does not respond back. When set to true, allow traffic in case of communication failure between rate limiting service and the proxy (`--rate-limit-failure-mode-deny`)
- RateLimiting Stage to use, defaults to `0` (--rate-limit-stage`)


#### Example of cluster added for rate limit service configured via args above:
```
 - name: rate_limit_cluster
    type: strict_dns
    connect_timeout: 0.25s
    lb_policy: round_robin
    http2_protocol_options: {}
    hosts:
    - socket_address:
        address: 10.52.131.249
        port_value: 8081
```

#### Descriptor list definition

Each configuration contains a top level descriptor list and potentially multiple nested lists beneath that. The format is:

```
domain: <unique domain ID>
descriptors:
  - key: <rule key: required>
    value: <rule value: optional>
    rate_limit: (optional block)
      unit: <see below: required>
      requests_per_unit: <see below: required>
    descriptors: (optional block)
      - ... (nested repetition of above)
```

#### Rate limit definition
```
rate_limit:
  unit: <second, minute, hour, day>
  requests_per_unit: <uint>
```

The rate limit block specifies the actual rate limit that will be used when there is a match. Currently the service supports per second, minute, hour, and day limits.

#### Example Configurations

Limit all requests based upon a defined key:

```
---
domain: contour
descriptors:
  - key: generic_key
    value: apis  # This key will be configured in the annotation or IngressRoute
    rate_limit:
      unit: minute
      requests_per_unit: 1
```

Limit requests based upon the remote_address:
```
---
domain: contour
descriptors:
  - key: remote_address
    rate_limit:
      unit: second
      requests_per_unit: 10
```

Block all requests from a specific IP:
```
---
domain: contour
descriptors:
  - key: remote_address
    value: 50.0.0.5
    rate_limit:
      unit: second
      requests_per_unit: 0
```

### IngressRoute Design

The `IngressRoute` spec will be modified to allow specific routes to enable rate limiting.
A new struct will be added to allow users to define what type of rate limiting should be applied to the route.

The `rateLimit` struct will have a key/value list where users can customize whatever values are required by the RateLimit backend of their choice.

#### Configure rate limit via generic_key:
```
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: ratelimited-key
spec:
  virtualhost:
    fqdn: foo-basic.bar.com
  routes:
    - match: /
      rateLimit:
        - type: generic_key
        - value: apis
      services:
        - name: s1
          port: 80
```

#### Configure rate limit via remote_address:
```
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: ratelimited-ip
spec:
  virtualhost:
    fqdn: foo-basic.bar.com
  routes:
    - match: /
      rateLimit:
        - type: remote_address
      services:
        - name: s1
          port: 80
```

### Ingress Annotation

- `contour.heptio.com/rate-limit.generic_key: Specifies that a generic_key will be used. The annotation value is the `value` that should match the RateLimit service
- `contour.heptio.com/rate-limit.remote_address: Specifies that a generic_key will be used. The annotation value is the `value` that should match the RateLimit service
