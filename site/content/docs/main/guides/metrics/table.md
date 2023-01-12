| Name | Type | Labels | Description |
| ---- | ---- | ------ | ----------- |
| contour_build_info | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | branch, revision, version | Build information for Contour. Labels include the branch and git SHA that Contour was built from, and the Contour version. |
| contour_cachehandler_onupdate_duration_seconds | [SUMMARY](https://prometheus.io/docs/concepts/metric_types/#summary) |  | Histogram for the runtime of xDS cache regeneration. |
| contour_dagrebuild_timestamp | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) |  | Timestamp of the last DAG rebuild. |
| contour_dagrebuild_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) |  | Total number of times DAG has been rebuilt since startup |
| contour_eventhandler_operation_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) | kind, op | Total number of Kubernetes object changes Contour has received by operation and object kind. |
| contour_httpproxy | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace | Total number of HTTPProxies that exist regardless of status. |
| contour_httpproxy_invalid | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace, vhost | Total number of invalid HTTPProxies. |
| contour_httpproxy_orphaned | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace | Total number of orphaned HTTPProxies which have no root delegating to them. |
| contour_httpproxy_root | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace | Total number of root HTTPProxies. Note there will only be a single root HTTPProxy per vhost. |
| contour_httpproxy_valid | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace, vhost | Total number of valid HTTPProxies. |
