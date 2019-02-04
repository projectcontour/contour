# Contour Deployment with Split Pods

This is an advanced deployment guide to configure Contour in a Deployment separate from Envoy which allows for easier scaling of each component.
This configuration has several advantages:

1. Envoy runs as a daemonset which allows for distributed scaling across workers in the cluster
2. Envoy runs on host networking which exposes Envoy directly without additional networking hops

## Moving parts

- Contour is run as Deployment and Envoy as a Daemonset
- Envoy runs on host networking
- Envoy runs on ports 80 & 443

## Deploy Contour

1. [Clone the Contour repository][1] and cd into the repo.
2. Run `kubectl apply -f deployment/ds-hostnet-split/`

**NOTE**: The current configuration exposes the `/stats` path from the Envoy Admin UI so that Prometheus can scrape for metrics.

# Test

1. Install a workload (see the kuard example in the [main deployment guide][2]).

[1]: ../CONTRIBUTING.md
[2]: deploy-options.md#test
