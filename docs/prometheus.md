# Prometheus

Contour and Envoy expose metrics that can be scraped with Prometheus.

## Envoy Metrics

Envoy typically exposes metrics through an endpoint on its admin interface. To
avoid exposing the entire admin interface to Prometheus (and other workloads in
the cluster), Contour configures a static listener that sends traffic to the
stats endpoint and nowhere else.

To enable the static listener, set the `--statsd-enabled` flag.
By default, Envoy's stats will be exposed over `0.0.0.0:8002` but can be overridden setting the `--stats-address` and `--stats-port` flags in Contour.

### Configuration Prometheus

The Envoy stats endpoint returns the metrics in statsd format by default. To get
the metrics in Prometheus format, requests to the endpoint must be made with a
URL parameter: `format=prometheus` ([This requirement will go away in a newer
version of Envoy](https://github.com/envoyproxy/envoy/issues/2182)).

Because of this Envoy requirement, the Prometheus scraping configuration must be
tweaked to include the following:

```yaml
- source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_format]
  action: replace
  target_label: __param_format
  regex: (.+)
```

The following is the official Prometheus Kubernetes example configuration, with
the addition of the tweak mentioned above:

```yaml
    - job_name: 'kubernetes-pods'
      kubernetes_sd_configs:
      - role: pod
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_format]
        action: replace
        target_label: __param_format
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target_label: __address__
      - action: labelmap
        regex: __meta_kubernetes_pod_label_(.+)
      - source_labels: [__meta_kubernetes_namespace]
        action: replace
        target_label: kubernetes_namespace
      - source_labels: [__meta_kubernetes_pod_name]
        action: replace
        target_label: kubernetes_pod_name
```

## Contour Metrics

Contour exposes a Prometheus-compatible `/metrics` endpoint with the following metrics:

- **contour_ingressroute_total (gauge):** Total number of IngressRoutes objects that exist regardless of status (i.e. Valid / Invalid / Orphaned, etc). This metric should match the sum of `Orphaned` + `Valid` + `Invalid` IngressRoutes.
  - namespace
- **contour_ingressroute_orphaned_total (gauge):**  Number of `Orphaned` IngressRoute objects which have no root delegating to them
  - namespace
- **contour_ingressroute_root_total (gauge):**  Number of `Root` IngressRoute objects (Note: There will only be a single `Root` IngressRoute per vhost)
  - namespace
- **contour_ingressroute_valid_total (gauge):**  Number of `Valid` IngressRoute objects
  - namespace
  - vhost
- **contour_ingressroute_invalid_total (gauge):**  Number of `Invalid` IngressRoute objects
  - namespace
  - vhost
- **contour_ingressroute_dagrebuild_timestamp (gauge):** Timestamp of the last DAG rebuild
