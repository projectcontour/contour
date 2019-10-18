---
layout: page
title: Getting Started
description: Install Contour in your cluster
id: getting-started
---

This page will help you get up and running with Contour.

## Before you start

Before you start you will need:

- A Kubernetes cluster that supports Service objects of `type: LoadBalancer` ([AWS Quickstart cluster](https://aws.amazon.com/quickstart/architecture/vmware-kubernetes/) or Minikube, for example)
- `kubectl` configured with admin access to your cluster
- RBAC must be enabled on your cluster.

## Add Contour to your cluster

Run:

```bash
$ kubectl apply -f {{ site.url }}/quickstart/contour.yaml
```

This command creates:

- A new namespace `projectcontour` with two instances of Contour in the namespace
- A Service of `type: LoadBalancer` that points to the Contour instances
- Depending on your configuration, new cloud resources -- for example, ELBs in AWS

See also [TLS support][7] for details on configuring TLS support for the services behind Contour.

For information on configuring TLS for gRPC between Contour and Envoy, see [our gRPC TLS documentation][8].

### Example workload

If you don't have an application ready to run with Contour, you can explore with [kuard](https://github.com/kubernetes-up-and-running/kuard).

Run:

```bash
$ kubectl apply -f {{ site.url }}/examples/kuard.yaml
```

This example specifies a default backend for all hosts, so that you can test your Contour install. It's recommended for exploration and testing only, however, because it responds to all requests regardless of the incoming DNS that is mapped. You probably want to run with specific Ingress rules for specific hostnames.

## Access your cluster

Now you can retrieve the external address of Contour's load balancer:

```bash
$ kubectl get -n projectcontour service contour -o wide
NAME      CLUSTER-IP     EXTERNAL-IP                                                                    PORT(S)        AGE       SELECTOR
contour   10.106.53.14   a47761ccbb9ce11e7b27f023b7e83d33-2036788482.ap-southeast-2.elb.amazonaws.com   80:30274/TCP   3h        app=contour
```

## Configuring DNS

How you configure DNS depends on your platform:

- On AWS, create a CNAME record that maps the host in your Ingress object to the ELB address.
- If you have an IP address instead (on GCE, for example), create an A record.

### What's next?

For more deployment options, including uninstalling Contour, see the [deployment documentation][1].

See also the Kubernetes documentation for [Services](https://kubernetes.io/docs/concepts/services-networking/service/), [Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/), and [HTTPProxy][2].

The [detailed documentation][3] provides additional information, including an introduction to Envoy and an explanation of how Contour maps key Envoy concepts to Kubernetes.

We've also got [a FAQ][4] for short-answer questions and conceptual stuff that doesn't quite belong in the docs.

## Troubleshooting

If you encounter issues, review the [troubleshooting docs][5], [file an issue][6], or talk to us on the [#contour channel](https://kubernetes.slack.com/messages/contour) on the Kubernetes Slack server.

[1]: {{ site.github.repository_url }}/tree/master/docs/deploy-options.md
[2]: {{ site.github.repository_url }}/tree/master/docs/httpproxy.md
[3]: {{ site.github.repository_url }}/tree/master/docs/
[4]: {{ site.github.repository_url }}/tree/master/docs/FAQ.md
[5]: {{ site.github.repository_url }}/tree/master/docs/troubleshooting.md
[6]: {{ site.github.repository_url }}/issues
[7]: {{ site.github.repository_url }}/tree/master/docs/tls.md
[8]: {{ site.github.repository_url }}/tree/master/docs/grpc-tls-howto.md
