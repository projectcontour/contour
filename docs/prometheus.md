# Prometheus

With Contour you can get metrics from Envoy. To do so you must expose the Envoy
admin socket and configure the Prometheus service discovery correctly.

Make the admin socket public:

```
sed 's#"bootstrap", "/config/contour.yaml"#"bootstrap", "/config/contour.yaml", "--admin-address", "0.0.0.0"#g' <your-contour-deployment>.yaml
```

Prometheus needs a configuration block that looks like this:

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

The main difference from the [official Prometheus Kubernetes sample config](https://github.com/prometheus/prometheus/blob/master/documentation/examples/prometheus-kubernetes.yml)
is the added interpretation of the `__meta_kubernetes_pod_annotation_prometheus_io_format` label, because Envoy
currently requires a [`format=prometheus` url parameter to return the stats in Prometheus format.](https://github.com/envoyproxy/envoy/issues/2182)

## Metrics

Metrics are essential to any system. Contour will expose a `/metrics` Prometheus endpoint with the following metrics:

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
