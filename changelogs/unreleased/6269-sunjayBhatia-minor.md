## Update examples for monitoring Contour and Envoy

Updates the [documentation](https://projectcontour.io/docs/main/guides/prometheus/) and examples for deploying a monitoring stack (Prometheus and Grafana) to scrape metrics from Contour and Envoy.
Adds a metrics port to the Envoy DaemonSet/Deployment in the example YAMLs to expose port `8002` so that `PodMonitor` resources can be used to find metrics endpoints.
