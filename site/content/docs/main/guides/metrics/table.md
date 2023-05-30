| Name | Type | Labels | Description |
| ---- | ---- | ------ | ----------- |
| contour_build_info | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | branch, revision, version | Build information for Contour. Labels include the branch and git SHA that Contour was built from, and the Contour version. |
| contour_cachehandler_onupdate_duration_seconds | [SUMMARY](https://prometheus.io/docs/concepts/metric_types/#summary) |  | Histogram for the runtime of xDS cache regeneration. |
| contour_dag_cache_object | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | kind | Total number of items that are currently in the DAG cache. |
| contour_dagrebuild_seconds | [SUMMARY](https://prometheus.io/docs/concepts/metric_types/#summary) |  | Duration in seconds of DAG rebuilds |
| contour_dagrebuild_timestamp | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) |  | Timestamp of the last DAG rebuild. |
| contour_dagrebuild_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) |  | Total number of times DAG has been rebuilt since startup |
| contour_eventhandler_operation_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) | kind, op | Total number of Kubernetes object changes Contour has received by operation and object kind. |
| contour_httpproxy | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace | Total number of HTTPProxies that exist regardless of status. |
| contour_httpproxy_invalid | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace, vhost | Total number of invalid HTTPProxies. |
| contour_httpproxy_orphaned | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace | Total number of orphaned HTTPProxies which have no root delegating to them. |
| contour_httpproxy_root | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace | Total number of root HTTPProxies. Note there will only be a single root HTTPProxy per vhost. |
| contour_httpproxy_valid | [GAUGE](https://prometheus.io/docs/concepts/metric_types/#gauge) | namespace, vhost | Total number of valid HTTPProxies. |
| contour_status_update_conflict_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) | kind | Number of status update conflicts encountered by object kind. |
| contour_status_update_duration_seconds | [SUMMARY](https://prometheus.io/docs/concepts/metric_types/#summary) | error, kind | How long a status update takes to finish. |
| contour_status_update_failed_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) | kind | Number of status updates that failed by object kind. |
| contour_status_update_noop_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) | kind | Number of status updates that are no-ops by object kind. This is a subset of successful status updates. |
| contour_status_update_success_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) | kind | Number of status updates that succeeded by object kind. |
| contour_status_update_total | [COUNTER](https://prometheus.io/docs/concepts/metric_types/#counter) | kind | Total number of status updates by object kind. |
