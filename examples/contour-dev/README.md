# Contour-dev Installation

This is an installation guide to configure Contour in a standalone Daemonset for easy trialling or development.

## Moving parts

- Contour and Envoy are run as in a Daemonset as Sidecars.
- Envoy runs on ports 80 & 443

## Install

Either run:

```bash
kubectl apply -f https://raw.githubusercontent.com/heptio/contour/master/examples/render/contour-dv.yaml
```

or:
Clone or fork the repository, change directory to `examples/contour-dev`, then run:

```bash
kubectl apply -f .
```

Contour is now deployed. Depending on your cloud provider, it may take some time to configure the load balancer.

## Test

1. Install a workload (see the kuard example in the [main deployment guide](../../docs/deploy-options.md#test-with-ingressroute)).
