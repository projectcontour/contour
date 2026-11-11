## Configurable healthy panic threshold for HTTPProxy health checks

HTTPProxy `healthCheckPolicy` and `tcpproxy.healthCheckPolicy` now support a `panicThreshold` field, which sets Envoy's healthy panic threshold for the upstream cluster as a percentage between 0 and 100.
When the percentage of healthy hosts drops below the threshold, Envoy ignores health check results and balances traffic across all hosts instead of only the healthy ones.
The default of 0 keeps panic mode disabled, which is the existing behavior.
See the [health checks documentation](https://projectcontour.io/docs/main/config/health-checks/#panic-threshold) for details.
