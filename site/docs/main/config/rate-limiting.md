# Rate Limiting

## Local Rate Limiting

The `HTTPProxy` API supports defining local rate limit policies that can be applied to either individual routes or entire virtual hosts.
Local rate limit policies define a maximum number of requests per unit of time that an Envoy should proxy to the upstream service.
Requests beyond the defined limit will receive a `429 (Too Many Requests)` response by default.
Local rate limit policies program Envoy's [HTTP local rate limit filter](https://www.envoyproxy.io/docs/envoy/v1.17.0/configuration/http/http_filters/local_rate_limit_filter#config-http-filters-local-rate-limit).

It's important to note that local rate limit policies apply *per Envoy pod*.
For example, a local rate limit policy of 100 requests per second for a given route will result in *each Envoy pod* allowing up to 100 requests per second for that route.

By contrast, **global** rate limiting (which will be added in a future Contour release), uses a shared external rate limit service, allowing rate limits to apply across *all* Envoy pods.

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
