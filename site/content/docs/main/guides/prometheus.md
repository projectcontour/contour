---
title: Collecting Metrics with Prometheus
---

<div id="toc" class="navigation"></div>

## Envoy Metrics

Envoy typically [exposes metrics](https://www.envoyproxy.io/docs/envoy/v1.15.0/configuration/http/http_conn_man/stats#config-http-conn-man-stats) through an endpoint on its admin interface. To
avoid exposing the entire admin interface to Prometheus (and other workloads in
the cluster), Contour configures a static listener that sends traffic to the
stats endpoint and nowhere else.

Envoy supports Prometheus-compatible `/stats/prometheus` endpoint for metrics on
port `8002`.

## Contour Metrics

Contour exposes a Prometheus-compatible `/metrics` endpoint that defaults to listening on port 8000. This can be configured by using the `--http-address` and `--http-port` flags for the `serve` command.

**Note:** the `Service` deployment manifest when installing Contour must be updated to represent the same port as the configured flag.

**The metrics endpoint exposes the following metrics:**

{{% metrics-table %}}

## Deploy Sample Monitoring Stack

Follow the instructions [here][0] to install a monitoring stack to your cluster using the [kube-prometheus][1] project sample manifests.
These instructions install the [Prometheus Operator][2], a [Prometheus][3] instance, a [Grafana][4] `Deployment`, and other components.
Note that this is a quickstart installation, see documentation [here][5] for more details on customizing the installation for production usage.

The instructions above show how to access the Prometheus and Grafana web interfaces using `kubectl port-forward`.
Sample `HTTPProxy` resources in the `examples/` directory can also be used to access these through your Contour installation:

```sh
$ kubectl apply -f examples/prometheus/httpproxy.yaml
$ kubectl apply -f examples/grafana/httpproxy.yaml
```

### Scrape Contour and Envoy metrics

To enable Prometheus to scrape metrics from the Contour and Envoy pods, we can add some RBAC customizations with a `Role` and `RoleBinding` in the `projectcontour` namespace:

```sh
kubectl apply -f examples/prometheus/rbac.yaml
```

Now add [`PodMonitor`][6] resources for scraping metrics from Contour and Envoy pods in the `projectcontour` namespace:

```sh
kubectl apply -f examples/prometheus/podmonitors.yaml
```

You should now be able to browse Contour and Envoy Prometheus metrics in the Prometheus and Grafana web interfaces to create dashboards and alerts.

### Apply Contour and Envoy Grafana Dashboards

Some sample Grafana dashboards are provided as `ConfigMap` resources in the `examples/grafana` directory.
To use them with your Grafana installation, apply the resources:

```sh
$ kubectl apply -f examples/grafana/dashboards.yaml
```

And update the Grafana `Deployment`:

```sh
$ kubectl -n monitoring patch deployment grafana --type=json --patch-file examples/grafana/deployment-patch.json
```

You should now see dashboards for Contour and Envoy metrics available in the Grafana web interface.


[0]: https://prometheus-operator.dev/docs/prologue/quick-start/
[1]: https://github.com/prometheus-operator/kube-prometheus
[2]: https://github.com/prometheus-operator/prometheus-operator
[3]: https://prometheus.io/
[4]: https://grafana.com/
[5]: https://github.com/prometheus-operator/kube-prometheus?tab=readme-ov-file#getting-started
[6]: https://prometheus-operator.dev/docs/operator/design/#podmonitor
