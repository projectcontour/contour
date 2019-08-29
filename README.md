# Contour [![Build Status](https://travis-ci.org/heptio/contour.svg?branch=master)](https://travis-ci.org/heptio/contour) [![Go Report Card](https://goreportcard.com/badge/github.com/heptio/contour)](https://goreportcard.com/report/github.com/heptio/contour) ![GitHub release](https://img.shields.io/github/release/heptio/contour.svg) [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

![Contour is fun at parties!](contour.png)

## Overview

Contour is an Ingress controller for Kubernetes that works by deploying the [Envoy proxy](https://www.envoyproxy.io/) as a reverse proxy and load balancer.
Unlike other Ingress controllers, Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

Contour also introduces a new ingress API ([IngressRoute][2]) which is implemented via a Custom Resource Definition (CRD).
Its goal is to expand upon the functionality of the Ingress API to allow for a richer user experience as well as solve shortcomings in the original design.

## Prerequisites

Contour is tested with Kubernetes clusters running version 1.10 and later, but should work with earlier versions where Custom Resource Definitions are supported (Kubernetes 1.7+).

RBAC must be enabled on your cluster.

## Get started

You can try out Contour by creating a deployment from a hosted manifest -- no clone or local install necessary.

What you do need:

- A Kubernetes cluster that supports Service objects of `type: LoadBalancer` ([AWS Quickstart cluster](https://aws.amazon.com/quickstart/architecture/vmware-kubernetes/) or Minikube, for example)
- `kubectl` configured with admin access to your cluster

See the [deployment documentation][1] for more deployment options if you don't meet these requirements.

### Add Contour to your cluster

Run:

```bash
kubectl apply -f https://raw.githubusercontent.com/heptio/contour/master/examples/render/contour.yaml
```

This command creates:

- A new namespace `heptio-contour` with two instances of Contour in the namespace
- A Service of `type: LoadBalancer` that points to the Contour instances
- Depending on your configuration, new cloud resources -- for example, ELBs in AWS

See also [TLS support](docs/tls.md) for details on configuring TLS support for the services behind Contour.

For information on configuring TLS for gRPC between Contour and Envoy, see [grpc-tls-howto.md](docs/grpc-tls-howto.md)

#### Example workload

If you don't have an application ready to run with Contour, you can explore with [kuard](https://github.com/kubernetes-up-and-running/kuard).

Run:

```bash
kubectl apply -f https://raw.githubusercontent.com/heptio/contour/master/examples/example-workload/kuard.yaml
```

This example specifies a default backend for all hosts, so that you can test your Contour install. It's recommended for exploration and testing only, however, because it responds to all requests regardless of the incoming DNS that is mapped. You probably want to run with specific Ingress rules for specific hostnames.

## Access your cluster

Now you can retrieve the external address of Contour's load balancer:

```bash
$ kubectl get -n heptio-contour service contour -o wide
NAME      CLUSTER-IP     EXTERNAL-IP                                                                    PORT(S)        AGE       SELECTOR
contour   10.106.53.14   a47761ccbb9ce11e7b27f023b7e83d33-2036788482.ap-southeast-2.elb.amazonaws.com   80:30274/TCP   3h        app=contour
```

## Configuring DNS

How you configure DNS depends on your platform:

- On AWS, create a CNAME record that maps the host in your Ingress object to the ELB address.
- If you have an IP address instead (on GCE, for example), create an A record.

### More information and documentation

For more deployment options, including uninstalling Contour, see the [deployment documentation][1].

See also the Kubernetes documentation for [Services](https://kubernetes.io/docs/concepts/services-networking/service/), [Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/), and [IngressRoutes][2].

The [detailed documentation](/docs) provides additional information, including an introduction to Envoy and an explanation of how Contour maps key Envoy concepts to Kubernetes.

We've also got [an FAQ](/FAQ.md) for short-answer questions and conceptual stuff that doesn't quite belong in the docs.

## Troubleshooting

If you encounter issues, review the [troubleshooting docs](/docs/troubleshooting.md), [file an issue][3], or talk to us on the [#contour channel](https://kubernetes.slack.com/messages/contour) on the Kubernetes Slack server.

## Contributing

Thanks for taking the time to join our community and start contributing!

- Please familiarize yourself with the [Code of Conduct](/CODE_OF_CONDUCT.md) before contributing.
- See [CONTRIBUTING.md](/CONTRIBUTING.md) for information about setting up your environment, the workflow that we expect, and instructions on the developer certificate of origin that we require.
- Check out the [open issues][3].

## Changelog

See [the list of releases](https://github.com/heptio/contour/releases) to find out about feature changes.

[1]: /docs/deploy-options.md
[2]: /docs/ingressroute.md
[3]: https://github.com/heptio/contour/issues
