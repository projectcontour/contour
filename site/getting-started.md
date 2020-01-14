---
layout: page
title: Getting Started
description: Install Contour in your cluster
id: getting-started
---

This page will help you get up and running with Contour.

## Before you start

Before you start you will need:

- A Kubernetes cluster (See [Deployment Options][1] for provider specific details)
- `kubectl` configured with admin access to your cluster
- RBAC must be enabled on your cluster

## Add Contour to your cluster

Run:

```bash
$ kubectl apply -f {{ site.url }}/quickstart/contour.yaml
```

This command creates:

- A new namespace `projectcontour` 
- Two instances of Contour in the namespace
- A Kubernetes Daemonset running Envoy on each node in the cluster listening on host ports 80/443
- A Service of `type: LoadBalancer` that points to the Contour's Envoy instances
- Depending on your deployment environment, new cloud resources -- for example, a cloud load balancer

### Example workload

If you don't have an application ready to run with Contour, you can explore with [kuard][9].

Run:

```bash
$ kubectl apply -f {{ site.url }}/examples/kuard.yaml
```

This example specifies a default backend for all hosts, so that you can test your Contour install.
It's recommended for exploration and testing only, however, because it responds to all requests regardless of the incoming DNS that is mapped.

## Send requests to application

There are a number of ways to validate everything is working.
The first way is to use the external address of the Envoy service and the second is to port-forward to an Envoy pod:
 
### External Address

Retrieve the external address of Contour's Envoy load balancer:

```bash
$ kubectl get -n projectcontour service envoy -o wide
NAME    TYPE           CLUSTER-IP       EXTERNAL-IP                                                               PORT(S)
envoy   LoadBalancer   10.100.161.248   a9be.eu-west-1.elb.amazonaws.com   80:30724/TCP,443:32097/TCP   4m58s   app=envoy
```

How you configure DNS depends on your platform:

- On AWS, create a CNAME record that maps the host in your Ingress object to the ELB address.
- If you have an IP address instead (on GCE, for example), create an A record.

### Port-forward to an Envoy pod:

```bash
$ kubectl port-forward -n projectcontour svc/envoy 80:80
```

### What's next?

You probably want to run with specific Ingress rules for specific hostnames.

For more deployment options, see the [deployment documentation][1] which includes information about .

See also the Kubernetes documentation for [Services][10], [Ingress][11], and [HTTPProxy][2].

The [detailed documentation][3] provides additional information, including an introduction to Envoy and an explanation of how Contour maps key Envoy concepts to Kubernetes.

We've also got [a FAQ][4] for short-answer questions and conceptual stuff that doesn't quite belong in the docs.

## Troubleshooting

If you encounter issues, review the [troubleshooting docs][5], [file an issue][6], or talk to us on the [#contour channel][12] on the Kubernetes Slack server.

[0]: https://aws.amazon.com/quickstart/architecture/vmware-kubernetes
[1]: /docs/{{site.latest}}/deploy-options
[2]: /docs/{{site.latest}}/httpproxy
[3]: /docs/{{site.latest}}
[4]: {% link _resources/faq.md %}
[5]: /docs/{{site.latest}}/troubleshooting
[6]: {{site.github.repository_url}}/issues
[9]: https://github.com/kubernetes-up-and-running/kuard
[10]: https://kubernetes.io/docs/concepts/services-networking/service
[11]: https://kubernetes.io/docs/concepts/services-networking/ingress
[12]: {{site.footer_social_links.Slack.url}}
