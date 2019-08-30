# examples

This directory contains example code for installing Contour and Envoy.

Most subdirectories contain a complete set of Kubernetes YAML that can be applied to a cluster.
This section describes the purpose of each subdirectory.

## [`contour`](./contour/README.md)

This is the recommended example installation of Contour.
It will deploy Contour into a Deployment, and Envoy into a Daemonset.
The gRPC communication is secured with certificates.
A `LoadBalancer` Service is created to expose Envoy to your cloud provider's load balancer.

## `common`

YAML fragments that are common across multiple examples. Not for applying to a Kubernetes cluster directly.

## `example-workload`

An example workload using Ingress ([kuard.yaml](./example-workload/kuard.yaml)) and one using IngressRoute ([kuard-ingressroute.yaml](./example-workload/kuard-ingressroute.yaml)).

There are also further IngressRoute examples under the `example-workload/ingressroute` directory. See the [README](./example-workload/ingressroute/README.md) for more details on each example.

## `grafana`, `prometheus`

Grafana and Prometheus examples, including the apps themselves, which can show the metrics that Contour exposes.

If you have your own Grafana and Prometheus deployment already, the supplied [ConfigMap](./grafana/02-grafana-configmap.yaml) contains a sample dashboard with Contour's metrics.

## `kind`, `root-rbac`

Both of these examples are fragments used in other documentation ([deploy-options](../docs/deploy-options.md) and [ingressroute](../docs/ingressroute.md) respectively.)

## `deployment-grpc-v2`

> This example is deprecated and will be removed as part of the Contour 1.0 release.

## `ds-grpc-v2`

> This example is deprecated and will be removed as part of the Contour 1.0 release.

## `ds-hostnet`

> This example is deprecated and will be removed as part of the Contour 1.0 release.

## `ds-hostnet-split`

> This example is deprecated and will be removed as part of the Contour 1.0 release.
