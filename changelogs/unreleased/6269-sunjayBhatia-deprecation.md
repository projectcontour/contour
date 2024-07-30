## Contour sample YAML manifests no longer use `prometheus.io/` annotations

The annotations for notifying a Prometheus instance on how to scrape metrics from Contour and Envoy pods have been removed from the deployment YAMLs and the Gateway provisioner.
The suggested mechanism for doing so now is to use [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) and the [`PodMonitor`](https://prometheus-operator.dev/docs/operator/design/#podmonitor) resource.
