# Rate Limiting Support

Status: Accepted

## Abstract

This document proposes a design for supporting Envoy's L7 local and global rate limiting capabilities in Contour.

## Background

Rate limiting is a means of protecting backend services against unwanted traffic. This can be useful for a variety of different scenarios:

- Protecting against denial-of-service (DoS) attacks by malicious actors
- Protecting against DoS incidents due to bugs in client applications/services
- Enforcing usage quotas for different classes of clients, e.g. free vs. paid tiers
- Controlling resource consumption/cost

[Rate limiting support](https://github.com/projectcontour/contour/issues/370) is a common feature request for Contour.
An initial [design document](https://github.com/projectcontour/contour/blob/e19372e147ffeb8044173bf9d4bce5721a4cdbfe/design/ratelimit-design.md) was proposed and accepted in early 2019, and an [implementation](https://github.com/projectcontour/contour/pull/873) was drafted, but it was not merged due to competing priorities and became stale.
There was also a more recent [design update PR](https://github.com/projectcontour/contour/pull/2283) that was not merged.

Envoy supports both [local](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/other_features/local_rate_limiting) and [global](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/other_features/global_rate_limiting#arch-overview-global-rate-limit) rate limiting, at both the network and HTTP layers:

| Type | Network (L4) | HTTP (L7) |
| ---- | ------------ | --------- |
| Local | [envoy.filters.network.local_ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/local_rate_limit_filter#config-network-filters-local-rate-limit) | [envoy.filters.http.local_ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter#config-http-filters-local-rate-limit) |
| Global | [envoy.filters.network.ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/rate_limit_filter#config-network-filters-rate-limit) | [envoy.filters.http.ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rate_limit_filter#config-http-filters-rate-limit) |

This document focuses on the HTTP filters, as it's assumed that users will want L7 control over rate limiting.

## Goals
- Support Envoy's L7 **local** rate limiting filter.
- Support Envoy's L7 **global** rate limiting filter, with a "bring your own Rate Limit Service (RLS)" model.

## Non Goals
- L4 rate limiting support (local or global).
- Tight integration with a particular RLS.
- Supporting more than one global RLS (this may be revisited for TLS virtual hosts in the future, see Alternatives Considered).

## High-Level Design

Contour will add support for Envoy's local **and** global rate limiting.
Local rate limiting adds a lightweight, easy-to-configure way to prevent large overall spikes in traffic from degrading upstream services, that doesn't require deploying any additional services.
Global rate limiting provides much more fine-grained control over when and how rate limits are applied based on client IP, header values, etc., but requires a separate RLS to be deployed and configured alongside Envoy.
Local and global rate limiting differ significantly in functionality and deployment/operation, so Contour will not attempt to merge them into a single unified API.
Instead, each one will be exposed independently, and users can opt into one or both as needed.


A new type, `RateLimitPolicy`, will be defined as part of the `HTTPProxy` API.
A `RateLimitPolicy` can be defined for either a virtual host or a route.
The `RateLimitPolicy` defines parameters for local and/or global rate limiting.


For local rate limiting, the user defines the rate limit itself, as "requests per second" and "burst" parameters, which Contour translates into [token bucket](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/local_ratelimit/v3/local_rate_limit.proto#envoy-v3-api-field-extensions-filters-http-local-ratelimit-v3-localratelimit-token-bucket) settings.


For global rate limiting, the user defines the descriptors to be generated and sent to the external RLS.
Descriptors contain entries including things like: the client IP, the value of a particular header, the destination cluster, etc.
The external RLS makes the rate limit decision based on the descriptors, and returns either a 200 or a 429 to Envoy.
The operator of the external RLS must configure it with actual rate limits for different descriptors.
Each RLS implementation may have its own configuration format and mechanism for defining rate limits for descriptors, so Contour cannot provide a generic API for defining these.


For global rate limiting, an `ExtensionService` is defined to map to an external Rate Limit Service.
The RLS is defined in the Contour config file, which will be used by any `HTTPProxies` that define a global rate limit policy.

## Detailed Design

### RateLimitPolicy type

A sample `RateLimitPolicy` looks like:

```yaml
rateLimitPolicy:
  # local defines local rate limiting properties for the virtual host or route.
  local:
    # requests defines how many requests per unit of time to allow.
    # This programs the "tokens_per_fill" field on the Envoy local
    # rate limit filter.
    # See ref. https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/v3/token_bucket.proto#envoy-v3-api-msg-type-v3-tokenbucket.
    requests: 100
    # unit defines the period of time within which requests over the
    # limit will be rate limited. This programs the "fill_interval" field
    # on the Envoy local rate limiter.
    # See ref. https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/v3/token_bucket.proto#envoy-v3-api-msg-type-v3-tokenbucket.
    unit: second
    # burst defines how many additional requests above the baseline requests
    # are allowed in a short period of time. This, along with "requests", 
    # programs the "max_tokens" field on the Envoy local rate limit filter.
    # See ref. https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/v3/token_bucket.proto#envoy-v3-api-msg-type-v3-tokenbucket.
    burst: 20
  # global defines global rate limiting properties for the virtual host or route.
  global:
    # descriptors defines the lists of key-value pairs to be generated
    # and sent to the external Rate Limiting Service (RLS) for a rate
    # limit decision.
    descriptors:
    # This descriptor is generated only if the x-steve-ratelimit header
    # is present on the request.
    - items:
        # adds a descriptor entry with a key of "generic_key" and a value
        # of "s1".
        - genericKey:
            value: s1

        # adds a descriptor entry with a key of "remote_address" and a value
        # equal to the trusted address from x-forwarded-for.
        - remoteAddress: {}

        # adds a descriptor entry with a key of "steve-ratelimit"
        # and a value equal to the value of the "x-steve-ratelimit" header.
        - requestHeader:
            descriptorKey: steve-ratelimit
            headerName: x-steve-ratelimit
        
    # This descriptor is always generated since it's just a static value.
    - items:
        - genericKey:
            value: s1
```

This API maps closely to Envoy's rate limiting API.

For local rate limiting, the rate limit itself is defined inline in the `RateLimitPolicy`.
Since local rate limits apply *per Envoy*, each pod in the Envoy daemon set will have a token bucket with these properties.

For global rate limiting, per Envoy's API, the `RateLimitPolicy` only defines a list of descriptors to send to an external RLS for a given request.
All [descriptor entries defined by Envoy](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#config-route-v3-ratelimit-action) will be supported except `metadata` and `dynamic_metadata`.

### Global RLS ExtensionService
When using global rate limiting, first, an `ExtensionService` must be defined for the RLS with the cluster-level details for the RLS itself.
For example:
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
  timeoutPolicy:
    # sets  the "timeout" property on the Envoy rate limit service
    # config.
    response: 50ms
```

### HTTPProxy Changes

The HTTPProxy struct adds new fields `spec.virtualhost.rateLimitPolicy`, and `spec.routes[].rateLimitPolicy`:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: proxy
  namespace: projectcontour
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    tls:
      secretName: local-tls
    # rateLimitPolicy optionally defines local and global rate limit
    # parameters to apply to all requests for this virtual host.
    rateLimitPolicy:
      local:
        ...
      global:
        ...
  routes:
    - conditions:
        - prefix: /
      services:
        - name: s1
          port: 80
      # rateLimitPolicy optionally defines local and global rate limit
      # parameters to apply to all requests for this route.
      rateLimitPolicy:
        local:
          ...
        global:
          ...
```

### Contour Configuration
If using global rate limiting, anexternal RLS can be configured in the Contour config file.
This RLS will be used for all virtual hosts that defines a global rate limit policy.

```yaml
rateLimitServer:
  # extensionRef is an object reference to the RLS ExtensionService.
  extensionRef:
    name: ratelimit
  # domain is passed to the RLS for all rate limit requests.
  # Defaults to "contour".
  domain: contour
  # failOpen defines whether to fail open or closed. If false, then if the RLS
  # cannot be reached or does not return a valid rate limit decision within the
  # specified timeout, the client will receive a 429 response for their request.
  #
  # This sets the "denyOnFailure" field in the Envoy config.
  failOpen: true
```

Note that if an individual `HTTPProxy` does not define any global rate limit policies, then no calls to the RLS will occur.

### Rate Limit Flows

- First, Envoy applies local rate limiting to incoming requests.
- If there are no tokens left in the relevant token bucket(s), a `429 Too Many Requests` response is returned.
- If there was no local rate limiting defined, or there is a token available, Envoy proceeds to global rate limiting.
- 1+ descriptors, each of which is an ordered list of key/value pairs, are generated for the request based on the global rate limit configuration.
- The descriptors are sent to the external RLS via gRPC.
- The external RLS makes a rate limit decision for the request, based on the descriptors it receives.
- The external RLS returns a response indicating whether the client request should be rate-limited or not.
- If the request should be rate limited, a `429 Too Many Requests` response is returned to the client along with the `x-envoy-ratelimited` header.
- If the request should not be rate limited, it is routed to the appropriate upstream cluster and proceeds as normal.

### Rate Limit Status

Contour users will want to be able to observe the status of rate limiting: can the external RLS be connected to?
Are requests being rate-limited?

Envoy provides many statistics for observing the status of rate limiting.
Local rate limiting statistics are described [here](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter#statistics).
Global rate limiting statistics are described [here](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rate_limit_filter#statistics).

Like other Envoy statistics, these are exposed in Prometheus-compatible format and can be [scraped and visualized using Grafana/etc.](https://projectcontour.io/guides/prometheus/)

## Alternatives Considered

### Tight integration with Lyft RLS
Lyft has open-sourced a [reference external RLS implementation](https://github.com/envoyproxy/ratelimit/).
We considered tightly integrating with this specific RLS implementation, to provide a more streamlined UX for configuring rate limits.
A CRD could be used -- either `HTTPProxy` or a stand-alone one -- to define actual rate limits, and a controller could convert the CRDs into configuration in the Lyft RLS format.
This option was discarded (for now) because we have potential users who are interested in using other rate limiting services, so just providing an integration with the Lyft implementation is not sufficient. 
Future work could still be done to enable rate limits to be defined via CRD (as part of `HTTPProxy`, or stand-alone) and automatically configured with 1+ underlying RLS implementations.

### Contour as external RLS
Contour could function as an external RLS itself.
This would put Contour in the data path for requests.
It would simplify deployment and configuration for the user, at a cost of significant additional complexity for Contour.

### Unique RLS per TLS virtual host
It may be desirable for each virtual host to be able to configure a different RLS.
This is not currently possible for non-TLS virtual hosts, because they all share a single HTTP Connection Manager (HCM)/rate limit filter config.
However, this could be supported for TLS virtual hosts because they each have their own HCM and rate limit filter config.
To support this, the same `rateLimitServer` struct used in the config file could be added as a field to `HTTPProxy.Spec.VirtualHost`:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: proxy
  namespace: projectcontour
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    tls:
      secretName: local-tls
    # rateLimitServer optionally defines a non-default RLS to use
    # for this HTTPProxy. This field can only be specified for root
    # TLS-enabled HTTPProxies.
    rateLimitServer:
      # extensionRef is an object reference to the RLS ExtensionService.
      extensionRef:
        name: ratelimit
      # domain is passed to the RLS for all rate limit requests.
      # Defaults to "contour".
      domain: contour
      # failOpen defines whether to fail open or closed. If false, then if the RLS
      # cannot be reached or does not return a valid rate limit decision within the
      # specified timeout, the client will receive a 429 response for their request.
      #
      # This sets the "denyOnFailure" field in the Envoy config.
      failOpen: true
...
```

The `HTTPProxy` processor would ensure that the `rateLimitServer` field could only be specified for root TLS-enabled `HTTPProxies`.

We opted not to implement this for now because we haven't had users ask for the ability to have a different RLS per virtual host.
This alternative could be implemented as a new feature in the future, if we get information from users that this is necessary.

## Compatibility
Rate limiting (both local and global) will be an optional, opt-in feature for Contour users.

### Comparison with other Ingress controllers

#### Ambassador

Ambassador's OSS API Gateway supports global rate limiting, and follows a similar model to what is proposed here.
The user must deploy and configure their own external RLS.
A `RateLimitService` custom resource is then configured with Ambassador to identify the global RLS:
```yaml
apiVersion: getambassador.io/v2
kind:  RateLimitService
metadata:
  name:  ratelimit
spec:
  service: "example-rate-limit:5000"
```

Labels are then defined for requests:
```yaml
apiVersion: getambassador.io/v2
kind: Mapping
metadata:
  name: service-backend
spec:
  prefix: /backend/
  service: quote
  labels:
    ambassador:    
      - request_label_group:      
        - x-ambassador-test-allow:        
          header: "x-ambassador-test-allow"
          omit_if_not_present: true
```

These labels are sent to the global RLS as descriptors, for rate limiting decisions to be made.

The enterprise (paid) version of Ambassador packages an implementation of an RLS and provides a CRD-based API for configuring actual rate limits with it.

ref. https://www.getambassador.io/docs/latest/topics/running/services/rate-limit-service/

#### Gloo

Gloo's OSS Edge also supports global rate limiting, and follows a similar model.
The user must deploy and configure their own external RLS.
A `rateLimitServer` is configured in `Settings` to identify the global RLS:
```yaml
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  labels:
    app: gloo
    gloo: settings
  name: default
  namespace: gloo-system
spec:
  # ...
  
  ratelimitServer:
    ratelimitServerRef:
      name: ...        # rate-limit server upstream name
      namespace: ...   # rate-limit server upstream namespace
    requestTimeout: ...      # optional, default 100ms
    denyOnFail: ...          # optional, default false
    rateLimitBeforeAuth: ... # optional, default false
  
  # ...
```

Rate limit actions that generate descriptors are defined for `VirtualServices`:
```yaml

apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - '*'
    routes:
      - matchers:
          - prefix: /
        routeAction:
          single:
            upstream:
              name: default-example-80
              namespace: gloo-system
    options:
      ratelimit:
        rateLimits:
          - actions:
              - remoteAddress: {}
```

These descriptors are sent to the global RLS for rate limiting decisions to be made.

The enterprise (paid) version of Gloo packages an enhanced version of the Lyft RLS, and provides a simplified API for defining actual rate limits.

ref. https://docs.solo.io/gloo-edge/latest/guides/security/rate_limiting/

#### NGINX

The NGINX Ingress controller provides a set of annotations that can be used to define rate limits:

"These annotations define limits on connections and transmission rates. These can be used to mitigate DDoS Attacks.

nginx.ingress.kubernetes.io/limit-connections: number of concurrent connections allowed from a single IP address. A 503 error is returned when exceeding this limit.
nginx.ingress.kubernetes.io/limit-rps: number of requests accepted from a given IP each second. The burst limit is set to this limit multiplied by the burst multiplier, the default multiplier is 5. When clients exceed this limit, limit-req-status-code default: 503 is returned.
nginx.ingress.kubernetes.io/limit-rpm: number of requests accepted from a given IP each minute. The burst limit is set to this limit multiplied by the burst multiplier, the default multiplier is 5. When clients exceed this limit, limit-req-status-code default: 503 is returned.
nginx.ingress.kubernetes.io/limit-burst-multiplier: multiplier of the limit rate for burst size. The default burst multiplier is 5, this annotation override the default multiplier. When clients exceed this limit, limit-req-status-code default: 503 is returned.
nginx.ingress.kubernetes.io/limit-rate-after: initial number of kilobytes after which the further transmission of a response to a given connection will be rate limited. This feature must be used with proxy-buffering enabled.
nginx.ingress.kubernetes.io/limit-rate: number of kilobytes per second allowed to send to a given connection. The zero value disables rate limiting. This feature must be used with proxy-buffering enabled.
nginx.ingress.kubernetes.io/limit-whitelist: client IP source ranges to be excluded from rate-limiting. The value is a comma separated list of CIDRs.
If you specify multiple annotations in a single Ingress rule, limits are applied in the order limit-connections, limit-rpm, limit-rps.

To configure settings globally for all Ingress rules, the limit-rate-after and limit-rate values may be set in the NGINX ConfigMap. The value set in an Ingress annotation will override the global setting.

The client IP address will be set based on the use of PROXY protocol or from the X-Forwarded-For header value when use-forwarded-headers is enabled."

ref. https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/annotations/#rate-limiting

## Implementation
A tutorial-style guide will be written covering the feature, and showing a sample deployment of the Lyft RLS for global rate limiting.
This will be provided "as-is" and will not be considered an officially supported deployment/configuration of the Lyft RLS.

## Open Issues
- can a user define default rate limits?
- do we want to support rate limiting for Ingress?

## Appendix 1 - Example Rate Limit Configurations

### Global Rate Limiting

These examples show both `HTTPProxy` rate limit policies and corresponding [Lyft ratelimit service](https://github.com/envoyproxy/ratelimit) configs (as an example):

#### Limit each client to 100 requests per hour

The `HTTPProxy` rate limit policy:
```yaml
rateLimitPolicy:
  global:
    descriptors:
      - items:
          - remoteAddress: {}
```

The Lyft ratelimit service config:
```yaml
domain: contour
descriptors:
  - key: remote_address
    rate_limit:
      requests_per_unit: 100
      unit: hour
```

#### Limit each client to 5 requests per upstream cluster per minute

The `HTTPProxy` rate limit policy:
```yaml
rateLimitPolicy:
  global:
    descriptors:
      - items:
          - remoteAddress: {}
          - destinationCluster: {}
```

The Lyft ratelimit service config:
```yaml
domain: contour
descriptors:
  - key: remote_address
    descriptors:
      - key: destination_cluster
        rate_limit:
          requests_per_unit: 5
          unit: minute
```

#### Limit each client to 5 requests per per minute if they have the "os: linux" header, and 10 total requests per minute

The `HTTPProxy` rate limit policy:
```yaml
rateLimitPolicy:
  global:
    descriptors:
      - items:
          - remoteAddress: {}
          - headerValueMatch:
              headers:
                - name: os
                  exactMatch: linux
              descriptorValue: os=linux
      - items:
          - remoteAddress: {}
```

The Lyft ratelimit service config:
```yaml
domain: contour
descriptors:
  - key: header_match
    value: os=linux
    descriptors:
      - key: remote_address
        rate_limit:
          requests_per_unit: 5
          unit: minute
  - key: remote_address
    rate_limit:
      requests_per_unit: 10
      unit: minute
```
