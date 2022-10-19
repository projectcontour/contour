# Upstream Health Checks

## HTTP Proxy Health Checking

Active health checking can be configured on a per route basis.
Contour supports HTTP health checking and can be configured with various settings to tune the behavior.

During HTTP health checking Envoy will send an HTTP request to the upstream Endpoints.
It expects a 200 response if the host is healthy.
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
