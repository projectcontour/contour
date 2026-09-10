# Troubleshooting with Envoy Statistics

Envoy statistics can help identify which stage of a request path is failing: accepting a connection, matching a route, selecting a healthy endpoint, connecting to a backend, or applying resource limits.
This guide is a starting point for investigating those stages in a Contour deployment.
For the complete catalog and exact semantics, use the statistics documentation for the Envoy version in your deployment.

Statistics belong to one Envoy process.
Counters restart when that process restarts, and a request may be handled by any Envoy pod.
Collect statistics from the affected pod when possible, or keep the pod identity when comparing data from multiple pods.

Statistics can expose Kubernetes namespace and Service names.
Restrict access to the statistics endpoints to trusted users and workloads.

## Collect a focused snapshot

By default, Contour exposes only Envoy's statistics and readiness endpoints on port `8002`.
For a standard Contour installation, select an Envoy pod and forward that port:

```sh
ENVOY_POD=$(kubectl -n projectcontour get pod -l app=envoy -o jsonpath='{.items[0].metadata.name}')
kubectl -n projectcontour port-forward "pod/${ENVOY_POD}" 8002:8002
```

Gateway-provisioned Envoy deployments use different pod labels.
Select a pod for the affected Gateway instead of using the `app=envoy` selector above.
If the metrics address, port, or TLS settings were customized, adjust the port-forward and client options to match.
When metrics TLS is configured, Contour serves metrics over HTTPS and uses a separate HTTP listener for readiness; a CA certificate and client certificate may also be required.

In another terminal, inspect the raw statistics:

```sh
# All currently available statistics.
curl -fsS http://127.0.0.1:8002/stats

# Only statistics that Envoy has used or updated.
curl -fsS 'http://127.0.0.1:8002/stats?usedonly'

# One HTTP connection manager.
curl -fsS --get \
  --data-urlencode 'filter=^http\.ingress_http\.' \
  http://127.0.0.1:8002/stats

# One backend Service port.
curl -fsS --get \
  --data-urlencode 'filter=^cluster\.default_backend_80\.' \
  http://127.0.0.1:8002/stats
```

The [`filter` parameter][11] accepts a regular expression and can be combined with `usedonly`.
Do not rely on `usedonly` when checking whether a zero-valued statistic exists: Envoy omits statistics that have not been used.

Use `/stats/prometheus` on the same port for Prometheus-formatted output.
See [Collecting Metrics with Prometheus][1] for scraping and dashboard setup.
The [read-only administration interface][2] also exposes statistics and provides `/listeners`, `/clusters`, and `/config_dump` for correlating a statistic with the active configuration.

## Understand Contour's statistic names

Start with raw `/stats` names because they show the hierarchy directly.
Contour configures the following prefixes:

In this guide, `<backend-prefix>` means `<namespace>_<service>_<numeric-port>`.

| Scope | Raw statistic prefix | Contour mapping |
| ----- | -------------------- | --------------- |
| Envoy listener | `listener.<address>.` | Contour does not set an Envoy listener statistic prefix, so discover the address value with a `^listener\.` filter. Do not substitute the listener's xDS name. |
| HTTP connection manager | `http.<listener-name>.` | Standard Contour listeners use `ingress_http` and `ingress_https`. Contour's Gateway provisioner uses a normalized `<protocol>-<port>` name, such as `http-80` or `https-443`, rather than the user-declared Gateway listener name. |
| TCP proxy | `tcp.<listener-name>.` | TCP proxy statistics use the same Contour listener name. |
| Kubernetes Service backend | `cluster.<backend-prefix>.` | Contour assigns this stable alternative statistic name even though the xDS cluster name also contains a configuration hash. |
| xDS updates | [`listener_manager.lds.`][13], [`cluster_manager.cds.`][14], and [`http.<listener-name>.rds.`][15] | These prefixes report whether listener, cluster, and route updates were accepted by Envoy. |

The backend cluster prefix represents a Kubernetes Service port, not an individual Pod.
Several xDS clusters for the same Service port can have different policies but share this alternative statistic name, so their statistics are aggregated.

Contour does not configure per-HTTPProxy or per-route statistic prefixes.
HTTP connection-manager and router statistics are aggregated by listener, while upstream statistics are aggregated by Service port.
Use [access logs][3] and `/config_dump` when a route or virtual host must be isolated.

Prometheus exposition flattens the raw hierarchy into `envoy_*` metric names and extracts some dynamic values into labels.
For example, HTTP metrics use the `envoy_http_conn_manager_prefix` label, and cluster metrics use the `envoy_cluster_name` label.
Inspect `/stats/prometheus` in the running Envoy rather than assuming that every raw name has a direct Prometheus spelling.

## Follow the request path

Compare counter changes during a short reproduction window.
The following sequence starts at the downstream listener and follows the request toward the backend:

| Question | Statistics to inspect | What a change suggests |
| -------- | --------------------- | ---------------------- |
| Did traffic reach this Envoy pod? | `listener.<address>.downstream_cx_total`, `listener.<address>.downstream_cx_active`, `http.<prefix>.downstream_rq_total`, `http.<prefix>.downstream_rq_active` | A listener connection without an HTTP request can point to a TLS, protocol, or filter-chain problem. No change may mean that traffic reached another Envoy pod or did not reach Envoy. |
| Did TLS, HTTP parsing, and route matching succeed? | `listener.<address>.no_filter_chain_match`, `listener.<address>.ssl.connection_error`, `listener.<address>.ssl.fail_verify_error`, `listener.<address>.ssl.fail_verify_san`, `listener.<address>.ssl.fail_verify_no_cert`, `http.<prefix>.downstream_cx_protocol_error`, `http.<prefix>.no_route` | These counters narrow the problem to filter-chain selection, the TLS handshake or certificate verification, HTTP parsing, or route matching. TLS statistics appear only on TLS listeners. |
| Did requests produce HTTP errors or resets? | `http.<prefix>.downstream_rq_4xx`, `http.<prefix>.downstream_rq_5xx`, `http.<prefix>.downstream_rq_rx_reset`, `http.<prefix>.downstream_rq_tx_reset` | These counters identify the class of response or reset, but not its root cause. Check the corresponding access-log response flags and details. |
| Did Envoy have a usable endpoint? | `cluster.<backend-prefix>.membership_total`, `cluster.<backend-prefix>.membership_healthy`, `cluster.<backend-prefix>.upstream_cx_none_healthy` | `membership_total` equal to zero means that no endpoints were delivered to Envoy. A positive total with zero healthy membership means that endpoints are present but unhealthy. Check the Service, EndpointSlices, Pod readiness, and `/clusters`. |
| Could Envoy communicate with the backend? | `cluster.<backend-prefix>.upstream_cx_connect_fail`, `cluster.<backend-prefix>.upstream_cx_connect_timeout`, `cluster.<backend-prefix>.upstream_rq_rx_reset`, `cluster.<backend-prefix>.upstream_rq_tx_reset`, `cluster.<backend-prefix>.ssl.connection_error`, `cluster.<backend-prefix>.ssl.fail_verify_error` | Increases point toward connection, reset, or upstream TLS failures. The TLS statistics exist only for TLS-enabled upstream clusters. |
| Were requests slow, retried, or capacity-limited? | `http.<prefix>.downstream_rq_time`, `http.<prefix>.downstream_rq_timeout`, `cluster.<backend-prefix>.upstream_rq_time`, `cluster.<backend-prefix>.upstream_rq_timeout`, `cluster.<backend-prefix>.upstream_rq_per_try_timeout`, `cluster.<backend-prefix>.upstream_rq_retry`, `cluster.<backend-prefix>.upstream_rq_retry_success`, `cluster.<backend-prefix>.upstream_rq_retry_limit_exceeded`, `cluster.<backend-prefix>.upstream_rq_retry_overflow`, `cluster.<backend-prefix>.upstream_cx_overflow`, `cluster.<backend-prefix>.upstream_rq_pending_overflow` | Histograms show latency distributions; timeout, retry, and overflow counters help distinguish slow backends from exhausted retry or circuit-breaker capacity. A successful retry is an upstream attempt, not an end-to-end success ratio. |
| Did Envoy reject a configuration update? | `listener_manager.lds.update_rejected`, `cluster_manager.cds.update_rejected`, `http.<prefix>.rds.<route-config>.update_rejected`, `listener_manager.total_listeners_warming`, `cluster_manager.warming_clusters` | Rejections and resources that remain warming point toward xDS configuration or dependency problems. Compare them with Contour logs and [Contour's xDS resources][4]. |
| Was Envoy under memory or connection pressure? | `server.memory_allocated`, `server.memory_heap_size`, `http.<prefix>.downstream_rq_overload_close`, `listener.<address>.downstream_global_cx_overflow`, and `overload.*` | Overload statistics are meaningful only when the corresponding [overload manager][5] or global connection limit is configured. Confirm the configured thresholds before interpreting them. |

The `upstream_rq_*`, retry, and request-time statistics apply to HTTP upstream clusters.
For TCP proxy traffic, start with `tcp.<listener-name>.*` and the cluster's `upstream_cx_*` statistics.

If a backend has an active `healthCheckPolicy`, Envoy also emits statistics under `cluster.<backend-prefix>.health_check.`, including `attempt`, `success`, `failure`, `network_failure`, and `healthy`.
Without that policy, Kubernetes endpoint readiness is represented by the cluster membership statistics, but there are no active health-check results.

The full statistic families are documented in Envoy's [listener statistics][6], [HTTP connection manager statistics][7], [router statistics][12], [cluster statistics][8], and [overload manager statistics][9].

## Interpret changes safely

* **Counters** increase for the lifetime of one Envoy process.
  Compare a rate or a before-and-after delta over the same reproduction window, and account for pod restarts.
* **Gauges** are point-in-time values such as active connections or healthy membership.
  Compare snapshots taken at approximately the same time.
* **Histograms** describe a distribution, such as request duration.
  Use their interval or cumulative distributions instead of treating them as simple totals.
* **Missing statistics** are not proof that an event did not occur.
  Some statistics are created only for a configured feature, protocol, or observed event, and `usedonly` hides untouched values.
* **HTTP status counters** record what Envoy observed, not necessarily who generated the response.
  For example, an increase in `downstream_rq_5xx` does not prove that Envoy generated the `5xx` response.

Statistics identify a stage of the request path, but usually do not identify the root cause alone.
Correlate the same time window and Envoy pod with [response flags and access logs][10], Kubernetes events and EndpointSlices, Contour logs, and the read-only administration endpoints.
See Envoy's [statistics overview][16] for more detail about statistic types, storage, and histogram output.

[1]: /docs/{{< param version >}}/guides/prometheus/
[2]: /docs/{{< param version >}}/troubleshooting/envoy-admin-interface/
[3]: /docs/{{< param version >}}/config/access-logging/
[4]: /docs/{{< param version >}}/troubleshooting/contour-xds-resources/
[5]: /docs/{{< param version >}}/config/overload-manager/
[6]: https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/stats
[7]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/stats
[8]: https://www.envoyproxy.io/docs/envoy/latest/configuration/upstream/cluster_manager/cluster_stats
[9]: https://www.envoyproxy.io/docs/envoy/latest/configuration/operations/overload_manager/overload_manager#statistics
[10]: /docs/{{< param version >}}/troubleshooting/common-proxy-errors/
[11]: https://www.envoyproxy.io/docs/envoy/latest/operations/admin.html#operations-admin-interface-stats
[12]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter.html#config-http-filters-router-stats
[13]: https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/lds.html#statistics
[14]: https://www.envoyproxy.io/docs/envoy/latest/configuration/upstream/cluster_manager/cds.html#statistics
[15]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/rds.html#statistics
[16]: https://www.envoyproxy.io/docs/envoy/latest/operations/stats_overview
