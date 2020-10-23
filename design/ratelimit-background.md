# Rate Limiting Support

[Rate limiting support](https://github.com/projectcontour/contour/issues/370) is a common feature request for Contour. An initial [design document](https://github.com/projectcontour/contour/blob/main/design/ratelimit-design.md) was proposed and accepted in early 2019, and an [implementation](https://github.com/projectcontour/contour/pull/873) was drafted, but it was not merged due to competing priorities and became stale. There was also a more recent [design update PR](https://github.com/projectcontour/contour/pull/2283) that was not merged.

The purpose of this document is to solicit further input on the topic from interested parties, in order to have enough information to successfully design and implement the feature in Contour. It presents background information on Envoy's rate limiting support, a survey of other ingress controllers' rate limiting support, and some possible approaches for Contour to take.

## Rate Limiting Goals

Rate limiting is a means of protecting backend services against unwanted traffic. This can be useful for a variety of different scenarios:

- Protecting against denial-of-service (DoS) attacks by malicious actors
- Protecting against DoS incidents due to bugs in client applications/services
- Enforcing usage quotas for different classes of clients, e.g. free vs. paid tiers
- Controlling resource consumption/cost

## Rate Limiting in Envoy

Envoy supports both [local](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/other_features/local_rate_limiting) and [global](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/other_features/global_rate_limiting#arch-overview-global-rate-limit) rate limiting, at both the network and HTTP layers:

| Type | Network (L4) | HTTP (L7) |
| ---- | ------------ | --------- |
| Local | [envoy.filters.network.local_ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/local_rate_limit_filter#config-network-filters-local-rate-limit) | [envoy.filters.http.local_ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter#config-http-filters-local-rate-limit) |
| Global | [envoy.filters.network.ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/rate_limit_filter#config-network-filters-rate-limit) | [envoy.filters.http.ratelimit](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rate_limit_filter#config-http-filters-rate-limit) |

This document focuses on the HTTP filters, as it's assumed that users will want L7 control over rate limiting.

### Local
Local rate limiting enables per-Envoy-process rate limits to be applied to requests. In the context of Contour, where Envoy is run as a daemon set by default, this means that each Envoy pod has its own [rate limit token buckets](https://en.wikipedia.org/wiki/Token_bucket) which are not shared with the other Envoy pods, making rate limit decisions local to the pod that is proxying the request.

Local rate limiting can be configured for specific routes or virtual hosts, or across all requests handled by the Envoy process.

Local rate limiting uses the token bucket model. In this model, a bucket is filled with an initial set of tokens, which are replenished at a constant rate. Each incoming request takes one token out of the bucket. If there are no tokens left in the bucket, the request is rate-limited by sending a `429 (Too Many Requests)` response to the client. Note that while each route can have its own token bucket, it's not possible to bucket requests in more complex ways, such as by client, by header value, etc.

A local rate limit config for a particular route looks like (copied from https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter):

```yaml
route_config:
  name: local_route
  virtual_hosts:
  - name: local_service
    domains: ["*"]
    routes:
    - match: { prefix: "/path/with/rate/limit" }
      route: { cluster: service_protected_by_rate_limit }
      typed_per_filter_config:
        envoy.filters.http.local_ratelimit:
          "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
          token_bucket:
            max_tokens: 10000
            tokens_per_fill: 1000
            fill_interval: 1s
          filter_enabled:
            runtime_key: local_rate_limit_enabled
            default_value:
              numerator: 100
              denominator: HUNDRED
          filter_enforced:
            runtime_key: local_rate_limit_enforced
            default_value:
              numerator: 100
              denominator: HUNDRED
          response_headers_to_add:
            - append: false
              header:
                key: x-local-rate-limit
                value: 'true'
    - match: { prefix: "/" }
      route: { cluster: default_service }
```

Local rate limiting is relatively simple to configure and operate, and offers some utility, but is likely not powerful enough to satisfy all users' needs.

### Global  
Global rate limiting allows rate limit decisions to be delegated to an external service, called the Rate Limit Service (RLS). In this approach, all Envoy processes consult the same RLS to determine whether a request should be rate-limited or not, making the rate limit decisions global.

In global rate limiting, each request [generates 1+ descriptors](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rate_limit_filter#composing-actions), which are sets of key-value pairs describing attributes of the request (e.g. the client IP, the destination cluster, a specific header's value, etc.). The specifics of which items are included in the descriptor are configurable by the Envoy administrator on a per-vhost or per-route basis. Envoy sends the descriptor(s) to the RLS, and the RLS makes a rate-limit decision based on the descriptor contents. The RLS returns the decision to Envoy, in the form of an HTTP response code, where `429 (Too Many Requests)` means the request should be rate-limited.

The details of *how* to make rate-limit decisions are left up to the RLS implementation. Lyft provides a [reference RLS implementation](https://github.com/envoyproxy/ratelimit), which uses a [YAML configuration file](https://github.com/envoyproxy/ratelimit#configuration) to define different rate limits that should be applied to different descriptors. However, any RLS implementation that conforms to the [RLS interface](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ratelimit/v3/rls.proto#envoy-v3-api-file-envoy-service-ratelimit-v3-rls-proto) can be used.

The descriptor model provides significant flexibility for configuring rate limiting based on client IP, destination cluster, headers, and more. Multiple rate limit policies can apply to a single request - for example, each client could be limited to 100 requests per hour across all routes, as well as to 10 requests per hour for a particular route.

To configure global rate limiting, first, a cluster must be defined representing the rate limit service:

```yaml
clusters:
- name: ratelimit
  type: STRICT_DNS
  connect_timeout: 1s
  lb_policy: ROUND_ROBIN
  protocol_selection: USE_CONFIGURED_PROTOCOL
  http2_protocol_options: {}
  load_assignment:
    cluster_name: ratelimit
    endpoints:
      - lb_endpoints:
          - endpoint:
              address:
                socket_address:
                  address: ratelimit
                  port_value: 8081
```

The HTTP filter must be configured, referencing the cluster:

```yaml
http_filters:
- name: envoy.filters.http.ratelimit
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit
    domain: steve
    timeout:
      seconds: 1
    failure_mode_deny: true
    rate_limit_service:
      grpc_service:
        envoy_grpc:
          cluster_name: ratelimit
      transport_api_version: v3
```

Rate limit actions must be configured for virtual hosts or routes (here, a virtual host's config is shown):

```yaml
virtual_hosts:
- name: local_service
  domains: ["*"]
  routes:
  - match:
      prefix: "/"
    route:
      host_rewrite_literal: contour.stevekriss.com
      cluster: contour_stevekriss_com
  rate_limits:
    # if x-steve-ratelimit: please is not set,
    # then this rate limit config will not generate
    # a descriptor at all
    - actions:
      - header_value_match:
          descriptor_value: "yes"
          headers:
            - name: x-steve-ratelimit
              exact_match: please
      - generic_key: 
          descriptor_value: steve
    
    # always generate this descriptor
    - actions:
      - generic_key: 
          descriptor_value: steve
```

Finally, corresponding rate limit rules must be defined in the RLS itself. An example configuration for the Lyft RLS is shown here:

```yaml
domain: steve
descriptors:
  # this rule defines a rate limit of 1 request per minute
  # for descriptors with generic_key=steve and header_match=yes.
  - key: generic_key
    value: steve
    descriptors:
      - key: header_match
        value: yes
        rate_limit:
          unit: minute
          requests_per_unit: 1
  # this rule defines a rate limit of 5 requests per minute
  # for descriptors with only generic_key=steve.
  - key: generic_key
    value: steve
    rate_limit:
      unit: minute
      requests_per_unit: 5
```

Global rate limiting is much more powerful than local, at the cost of added deployment, configuration, and operational complexity.

It's worth noting that, per Envoy, the two can be used in conjunction:

>Local rate limiting can be used in conjunction with global rate limiting to reduce load on the global rate limit service. For example, a local token bucket rate limit can absorb very large bursts in load that might otherwise overwhelm a global rate limit service. Thus, the rate limit is applied in two stages. The initial coarse grained limiting is performed by the token bucket limit before a fine grained global limit finishes the job. 

(source: [Envoy docs](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/other_features/global_rate_limiting#arch-overview-global-rate-limit))

## Survey of other Ingress Controllers

### Ambassador

(Envoy-based)

[API Gateway (OSS)](https://www.getambassador.io/docs/latest/howtos/rate-limiting-tutorial/) supports Envoy's global rate limiting, via a `RateLimitService` CRD and fields in the `Mapping` CRD to generate descriptors for requests. Deployment and configuration of the RLS is an exercise left to the reader.

[Edge Stack (freemium)](https://www.getambassador.io/docs/latest/howtos/advanced-rate-limiting/) ships with a built-in RLS, that can be configured with rate limit policies via the `RateLimit` CRD.

### Gloo

(Envoy-based)

Similar to Ambassador:

[Gloo OSS](https://docs.solo.io/gloo/latest/guides/security/rate_limiting/#rate-limiting-in-gloo) supports Envoy's global rate limiting. RLS is configured in the `Settings` CRD, and the [API corresponds very closely to Envoy's](https://docs.solo.io/gloo/latest/guides/security/rate_limiting/envoy/). Deployment and configuration of the RLS is an exercise left to the reader.

Gloo Enterprise ships with a built-in RLS. It supports a [RateLimitConfig](https://docs.solo.io/gloo/latest/guides/security/rate_limiting/crds/) API, as well as a [Gloo API](https://docs.solo.io/gloo/latest/guides/security/rate_limiting/simple/) for simplified rate limit policy definition.


### Kong

- Supports both a local and global mode.
- Can rate-limit by consumer, credential, ip, service, header.

[Documentation](https://docs.konghq.com/hub/kong-inc/rate-limiting).

### NGINX

Supports annotations specifying rate limits *per client IP* for the given `Ingress`:
- `nginx.ingress.kubernetes.io/limit-rps`
- `nginx.ingress.kubernetes.io/limit-rpm`
- etc.

[Documentation](https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/annotations/#rate-limiting).

### Traefik

- Supports rate-limiting by source IP, header, host.

[Documentation](https://doc.traefik.io/traefik/middlewares/ratelimit/).

## Sketch of Possible Approaches

### Local rate limiting only

In this approach, Contour only exposes Envoy's local rate limiting capability, meaning each Envoy pod independently applies rate limiting to the requests it is proxying. Rate-limiting token buckets can be defined for all requests, for an entire virtual host, or for a particular route. It's not possible to define per-client rate limits.

The Contour API could look something like:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: default
  namespace: default
spec:
  virtualhost:
    fqdn: foo.bar.com
    # rate limit policy can be supplied for the vhost or for a specific route
    rateLimitPolicy:
      local:
        tokenBucket:
          maxTokens: 100
          tokensPerFill: 10
          fillInterval: 1s
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
      # rate limit policy can be supplied for the vhost or for a specific route
      rateLimitPolicy:
        local:
          tokenBucket:
            maxTokens: 10
            tokensPerFill: 1
            fillInterval: 5s
```

### Global rate limiting

#### Bring your own RLS

- Support defining an RLS to use, via the `ExtensionService` API
- Contour's API allows rate-limit descriptors to be defined for virtual hosts and routes
- Contour will provide a guide on how to deploy and configure the Lyft reference RLS implementation

The Contour API for defining descriptors could look something like:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: default
  namespace: default
spec:
  virtualhost:
    fqdn: foo.bar.com
    # rate limit policy can be supplied for the vhost or for a specific route
    rateLimitPolicy:
      global:
        descriptors:
          # generates a descriptor containing both the remote address (source IP)
          # and the destination cluster, to limit requests by a particular client
          # to a particular upstream service.
          - entries:
              - remoteAddress: {}
              - destinationCluster: {}
          # generates a descriptor containing only the remote address (source IP)
          # to limit total requests by a particular client.
          - entries:
              - remoteAddress: {}
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
      # rate limit policy can be supplied for the vhost or for a specific route
      rateLimitPolicy:
        global:
          ...
```

The administrator would then need to define corresponding configuration for the RLS, to apply actual rate limits for these descriptors.
For example, if using the Lyft RLS, the config file could look like:

```yaml
domain: contour
descriptors:
  # Only allow 5 requests per day from each client to each target cluster
  - key: remote_address
    descriptors:
      - key: destination_cluster
        rate_limit:
          unit: day
          requests_per_unit: 5
  # Only allow 20 requests per day total from each client
  - key: remote_address
    rate_limit:
      unit: day
      requests_per_unit: 20
```

Pros:
- similar to Contour's approach to external auth
- similar to other OSS Envoy-based ingress controllers
- flexibility for users to use whatever RLS implementation they want

Cons:
- deployment/configuration/operation of the RLS is entirely up to the user
- rate limit config is split across Contour and the RLS
- potentially more difficult to diagnose problems

#### Contour-bundled RLS

- Contour's example YAML can include a manifest for the Lyft RLS, and an `ExtensionService` for it
- Contour exposes an API for defining rate limit policies, either as new fields on `HTTPProxy`, or as an entirely new CRD (`RateLimitPolicy` or similar). Contour translates these into the Lyft RLS config file format.

Pros:
- tighter integration between Contour and Lyft RLS makes configuration easier for users
- CRD-based configuration for RLS is potentially easier for users

Cons:
- possibly excludes alternate RLS implementations from being plugged in (depends on implementation choices)
- Contour development team is likely responsible for at least some support of the Lyft RLS/integration

#### Lyft RLS operator/CRD

- As an alternative to the previous option, an operator for the Lyft RLS could be written that defines a `RateLimitPolicy` CRD and translates it into the Lyft config file format. This would be independent of Contour.
- Contour users would either use the `RateLimitPolicy` API directly to define their rate limits, or Contour could expose its own API (as new `HTTPProxy`fields or a new CRD) that would be translated into the operator's `RateLimitPolicy` API.
- Contour would support configuring a target RLS and defining rate-limit descriptors for virtual hosts and routes

Pros:
- similar benefits to previous option
- code could potentially be upstreamed so it wouldn't have to be solely maintained by the Contour team

Cons:
- Contour team may end up solely supporting it

#### Operating a Rate Limit Service

- Deployment
- Latency
- High-availability
- Etc.

## Questions

- Are folks interested in local rate limiting, global rate limiting, or both?
- For global rate limiting, do folks want to use their own RLS implementation, or Lyft's reference implementation?
- Should the definition of rate limit policies be something Contour exposes, or should it be an exercise left to the reader?
- If Contour exposes an API for defining rate limits, should it match the [Envoy descriptor API](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#config-route-v3-ratelimit-action) ~as-is, or provide an abstraction of it?
