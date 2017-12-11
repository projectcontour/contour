# Heptio Contour

**Maintainers:** [Heptio][0]

[![Build Status][1]][2]

## Overview
Contour is an Ingress controller for Kubernetes that works by deploying the [Envoy proxy][13] as a reverse proxy and load balancer. Unlike other Ingress controllers, Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

This is an early release so that we can start sharing with the community. Check out [the roadmap][15] to see where we plan to go with the project.

And see [the launch blog post][17] for our vision of how Contour fits into the larger Kubernetes ecosystem.

## Prerequisites

Contour is tested with Kubernetes clusters running version 1.7 and later, but should work with earlier versions.

## Get started

You can try out Contour by creating a deployment from a hosted manifest -- no clone or local install necessary.

What you do need:

- A Kubernetes cluster that supports Service objects of `type: LoadBalancer` ([AWS Quickstart cluster][9] or Minikube, for example)
- `kubectl` configured with admin access to your cluster

See the [deployment documentation][10] for more deployment options if you don't meet these requirements.

### Add Contour to your cluster

Run:

```
$ kubectl apply -f https://j.hept.io/contour-deployment-rbac
```

If RBAC isn't enabled on your cluster (for example, if you're on GKE with legacy authorization), run:

```
$ kubectl apply -f https://j.hept.io/contour-deployment-norbac
```

This command creates:

- A new namespace `heptio-contour` with two instances of Contour in the namespace
- A Service of `type: LoadBalancer` that points to the Contour instances
- Depending on your configuration, new cloud resources -- for example, ELBs in AWS

#### Example workload

If you don't have an application ready to run with Contour, you can explore with [kuard][14].

Run:

```
$ kubectl apply -f https://j.hept.io/contour-kuard-example
```

This example specifies a default backend for all hosts, so that you can test your Contour install. It's recommended for exploration and testing only, however, because it responds to all requests regardless of the incoming DNS that is mapped. You probably want to run with specific Ingress rules for specific hostnames.

## Access your cluster

Now you can retrieve the external address of Contour's load balancer:

```
$ kubectl get -n heptio-contour service contour -o wide
NAME      CLUSTER-IP     EXTERNAL-IP                                                                    PORT(S)        AGE       SELECTOR
contour   10.106.53.14   a47761ccbb9ce11e7b27f023b7e83d33-2036788482.ap-southeast-2.elb.amazonaws.com   80:30274/TCP   3h        app=contour
```

On Minikube:
```
$ minikube service -n heptio-contour contour --url
http://192.168.99.100:30588
```

## Configuring DNS

How you configure DNS depends on your platform:

- On AWS, create a CNAME record that maps the host in your Ingress object to the ELB address.
- If you have an IP address instead (on GCE, for example), create an A record.
- On Minikube, you can fake DNS by editing `/etc/hosts`.

### More information and documentation

For more deployment options, including uninstalling Contour, see the [deployment documentation][10].

See also the Kubernetes documentation for [Services][11] and [Ingress][12].

The [detailed documentation][3] provides additional information, including an introduction to Envoy and an explanation of how Contour maps key Envoy concepts to Kubernetes.

We've also got [an FAQ][18] for short-answer questions and conceptual stuff that doesn't quite belong in the docs.

## Known limitations

* Contour does not yet support customizations with annotations.
* Contour does not yet support SSL/TLS.

## Troubleshooting

If you encounter any problems that the documentation does not address, [file an issue][4].

## Contributing

Thanks for taking the time to join our community and start contributing!

* Please familiarize yourself with the [Code of Conduct][8] before contributing.
* See [CONTRIBUTING.md][5] for information about setting up your environment, the workflow that we expect, and instructions on the developer certificate of origin that we require.
* Check out the [issues][4] and [our roadmap][15].

## Changelog

See [the list of releases][6] to find out about feature changes.

[0]: https://github.com/heptio
[1]: https://travis-ci.org/heptio/contour.svg?branch=master
[2]: https://travis-ci.org/heptio/contour
[3]: /docs
[4]: https://github.com/heptio/contour/issues
[5]: /CONTRIBUTING.md
[6]: /CHANGELOG.md
[8]: /CODE_OF_CONDUCT.md
[9]: https://aws.amazon.com/quickstart/architecture/heptio-kubernetes/
[10]: /docs/deploy-options.md
[11]: https://kubernetes.io/docs/concepts/services-networking/service/
[12]: https://kubernetes.io/docs/concepts/services-networking/ingress/
[13]: https://www.envoyproxy.io/
[14]: https://github.com/kubernetes-up-and-running/kuard
[15]: /design/roadmap.md
[16]: https://github.com/envoyproxy/envoy/issues/95
[17]: https://blog.heptio.com/making-it-easy-to-use-envoy-as-a-kubernetes-load-balancer-dde82959f171
[18]: /FAQ.md
