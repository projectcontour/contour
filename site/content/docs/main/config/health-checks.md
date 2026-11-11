# Upstream Health Checks

## HTTP Proxy Health Checking

Active health checking can be configured on a per route basis.
Contour supports HTTP health checking and can be configured with various settings to tune the behavior.

During HTTP health checking Envoy will send an HTTP request to the upstream Endpoints.
It expects a 200 response by default if the host is healthy (see `expectedStatuses` below for configuring the "healthy" status codes).
The upstream host can return 503 if it wants to immediately notify Envoy to no longer forward traffic to it.
It is important to note that these are health checks which Envoy implements and are separate from any other system such as those that exist in Kubernetes.

```yaml
# httpproxy-health-checks.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: health-check
  namespace: default
spec:
  virtualhost:
    fqdn: health.bar.com
  routes:
  - conditions:
    - prefix: /
    healthCheckPolicy:
      path: /healthy
      intervalSeconds: 5
      timeoutSeconds: 2
      unhealthyThresholdCount: 3
      healthyThresholdCount: 5
    services:
      - name: s1-health
        port: 80
      - name: s2-health
        port: 80
```

Health check configuration parameters:

- `path`: HTTP endpoint used to perform health checks on upstream service (e.g. `/healthz`). It expects a 200 response if the host is healthy. The upstream host can return 503 if it wants to immediately notify downstream hosts to no longer forward traffic to it.
- `host`: The value of the host header in the HTTP health check request. If left empty (default value), the name "contour-envoy-healthcheck" will be used.
- `intervalSeconds`: The interval (seconds) between health checks. Defaults to 5 seconds if not set.
- `timeoutSeconds`: The time to wait (seconds) for a health check response. If the timeout is reached the health check attempt will be considered a failure. Defaults to 2 seconds if not set.
- `unhealthyThresholdCount`: The number of unhealthy health checks required before a host is marked unhealthy. Note that for http health checking if a host responds with 503 this threshold is ignored and the host is considered unhealthy immediately. Defaults to 3 if not defined.
- `healthyThresholdCount`: The number of healthy health checks required before a host is marked healthy. Note that during startup, only a single successful health check is required to mark a host healthy.
- `expectedStatuses`: An optional list of HTTP status ranges that are considered healthy. Ranges follow half-open semantics, meaning the start is inclusive and the end is exclusive. Statuses must be between 100 (inclusive) and 600 (exclusive).
- `panicThreshold`: The healthy panic threshold, as a percentage between 0 and 100. When the percentage of healthy hosts drops below this value, Envoy ignores health check results and balances traffic across all hosts. Defaults to 0, which disables panic mode. See [Panic threshold](#panic-threshold) below.

### Non-default expected statuses

By default, only responses with a 200 status code will be considered healthy.
The set of response codes considered healthy can be customized by specifying ranges in `expectedStatuses`.
Ranges follow half-open semantics, meaning the start is inclusive and the end is exclusive. 
Statuses must be between 100 (inclusive) and 600 (exclusive).
For example:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: health-check
  namespace: default
spec:
  virtualhost:
    fqdn: health.bar.com
  routes:
  - conditions:
    - prefix: /
    healthCheckPolicy:
      path: /healthy
      intervalSeconds: 5
      timeoutSeconds: 2
      unhealthyThresholdCount: 3
      healthyThresholdCount: 5
      # Status codes 200 and 250-299 will be considered healthy.
      expectedStatuses:
      - start: 200
        end: 201
      - start: 250
        end: 300
    services:
      - name: s1-health
        port: 80
      - name: s2-health
        port: 80
```

Note that if `expectedStatuses` is specified, `200` must be explicitly included in one of the specified ranges if it is desired as a healthy status code.

### Panic threshold

By default, Contour disables Envoy's [panic mode][1].
If health checks mark every upstream host as unhealthy, Envoy stops forwarding traffic to them and responds with a 503.

Setting `panicThreshold` to a value between 1 and 100 enables panic mode.
When the percentage of healthy hosts drops below the threshold, Envoy ignores the health check results and balances traffic across all hosts, including the ones that failed their health checks.
This can be useful when hosts may fail their health checks while still being able to serve traffic, and sending requests to them is preferable to returning errors to clients.
For example:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: health-check
  namespace: default
spec:
  virtualhost:
    fqdn: health.bar.com
  routes:
  - conditions:
    - prefix: /
    healthCheckPolicy:
      path: /healthy
      # Balance traffic across all hosts if less than 50% of them are healthy.
      panicThreshold: 50
    services:
      - name: s1-health
        port: 80
      - name: s2-health
        port: 80
```

With a threshold of 50, if only one of four hosts passes its health check (25% healthy), Envoy balances traffic across all four hosts.

## TCP Proxy Health Checking

Contour also supports TCP health checking and can be configured with various settings to tune the behavior.

During TCP health checking Envoy will send a connect-only health check to the upstream Endpoints.
It is important to note that these are health checks which Envoy implements and are separate from any
other system such as those that exist in Kubernetes.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: tcp-health-check
  namespace: default
spec:
  virtualhost:
    fqdn: health.bar.com
  tcpproxy:
    healthCheckPolicy:
      intervalSeconds: 5
      timeoutSeconds: 2
      unhealthyThresholdCount: 3
      healthyThresholdCount: 5
    services:
      - name: s1-health
        port: 80
      - name: s2-health
        port: 80
```

TCP Health check policy configuration parameters:

- `intervalSeconds`: The interval (seconds) between health checks. Defaults to 5 seconds if not set.
- `timeoutSeconds`: The time to wait (seconds) for a health check response. If the timeout is reached the health check attempt will be considered a failure. Defaults to 2 seconds if not set.
- `unhealthyThresholdCount`: The number of unhealthy health checks required before a host is marked unhealthy. Note that for http health checking if a host responds with 503 this threshold is ignored and the host is considered unhealthy immediately. Defaults to 3 if not defined.
- `healthyThresholdCount`: The number of healthy health checks required before a host is marked healthy. Note that during startup, only a single successful health check is required to mark a host healthy.
- `panicThreshold`: The healthy panic threshold, as a percentage between 0 and 100. When the percentage of healthy hosts drops below this value, Envoy ignores health check results and balances connections across all hosts. Defaults to 0, which disables panic mode. See [Panic threshold](#panic-threshold) above.

## Specify the service health check port

contour supports configuring an optional health check port for services.

By default, the service's health check port is the same as the service's routing port.
If the service's health check port and routing port are different, you can configure the health check port separately.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: health-check
  namespace: default
spec:
  virtualhost:
    fqdn: health.bar.com
  routes:
  - conditions:
    - prefix: /
    healthCheckPolicy:
      path: /healthy
      intervalSeconds: 5
      timeoutSeconds: 2
      unhealthyThresholdCount: 3
      healthyThresholdCount: 5
    services:
      - name: s1-health
        port: 80
        healthPort: 8998
      - name: s2-health
        port: 80
```

In this example, envoy will send a health check request to port `8998` of the `s1-health` service and port `80` of the `s2-health` service respectively . If the host is healthy, envoy will forward traffic to the `s1-health` service on port `80` and to the `s2-health` service on port `80`.

[1]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/panic_threshold
