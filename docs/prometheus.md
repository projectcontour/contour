# Prometheus

Contour and Envoy expose metrics that can be scraped with Prometheus.

## Envoy Metrics

Envoy typically exposes metrics through an endpoint on its admin interface. To
avoid exposing the entire admin interface to Prometheus (and other workloads in
the cluster), Contour configures a static listener that sends traffic to the
stats endpoint and nowhere else.

To enable the static listener, set the `--statsd-enabled` flag.
By default, Envoy's stats will be exposed over `0.0.0.0:8002` but can be overridden setting the `--stats-address` and `--stats-port` flags in Contour.

Envoy supports Prometheus-compatible `/stats/prometheus` endpoint for metrics. 

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
