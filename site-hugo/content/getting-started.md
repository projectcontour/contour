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

### Option 1: Quickstart

As the name suggests, use this option for a quick start with reasonable defaults. If
you have specific requirements to customize Contour for environment, see Options 2 and 3.

Run:

```bash
$ kubectl apply -f {{ site.url }}/quickstart/contour.yaml
```

### Option 2: Install using Operator

__FEATURE STATE:__ Contour v1.11.0 [alpha][13]

Deploy Contour using the [operator][14]. First deploy the operator:

```bash
$ kubectl apply -f {{ site.url }}/quickstart/operator.yaml
```

This command creates:

- Namespace `contour-operator` to run the operator.
- Operator and Contour CRDs
- Operator RBAC resources
- A Deployment to manage the operator
- A Service to front-end the operator's metrics endpoint.

Then create an instance of the Contour custom resource:

```bash
$ kubectl apply -f {{ site.url }}/quickstart/contour-custom-resource.yaml
```

Deploying Contour using either option creates:

- Namespace `projectcontour`
- Two instances of Contour in the namespace
- A Kubernetes Daemonset running Envoy on each node in the cluster listening on host ports 80/443
- A Service of `type: LoadBalancer` that points to the Contour's Envoy instances
- Depending on your deployment environment, new cloud resources -- for example, a cloud load balancer

### Option 3: Install using Helm

The [Contour Helm chart][15] contains a large set of configuration options for customizing
your Contour deployment.

Run:

```bash
$ helm repo add bitnami https://charts.bitnami.com/bitnami
$ helm install my-release bitnami/contour
```

NOTE: As of 13 Nov 2020, [Helm 2 support has ended and is obsolete][16]. Please ensure you use Helm 3.

## Example workload

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

If you encounter issues, review the Troubleshooting section of [the docs][3], [file an issue][6], or talk to us on the [#contour channel][12] on the Kubernetes Slack server.

[0]: https://aws.amazon.com/quickstart/architecture/vmware-kubernetes
[1]: /docs/{{site.latest}}/deploy-options
[2]: /docs/{{site.latest}}/config/fundamentals
[3]: /docs/{{site.latest}}
[4]: {% link _resources/faq.md %}
[6]: {{site.github.repository_url}}/issues
[9]: https://github.com/kubernetes-up-and-running/kuard
[10]: https://kubernetes.io/docs/concepts/services-networking/service
[11]: https://kubernetes.io/docs/concepts/services-networking/ingress
[12]: {{site.footer_social_links.Slack.url}}
[13]: https://projectcontour.io/resources/deprecation-policy/
[14]: https://github.com/projectcontour/contour-operator/blob/main/README.md
[15]: https://github.com/bitnami/charts/tree/master/bitnami/contour
[16]: https://github.com/helm/charts#%EF%B8%8F-deprecation-and-archive-notice
