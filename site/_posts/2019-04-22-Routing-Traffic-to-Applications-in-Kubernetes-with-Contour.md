---
title: Routing Traffic to Applications in Kubernetes with Contour
image: /img/posts/image1.png
excerpt: One of the most critical needs in running workloads at scale with Kubernetes is efficient and smooth traffic ingress management at the Layer 7 level.
author_name: Contour Team
# author_avatar: https://placehold.it/64x64
categories: [kubernetes]
# Tag should match author to drive author pages
tags: ['Contour Team']
---
One of the most critical needs in running workloads at scale with Kubernetes is efficient and smooth traffic ingress management at the [Layer 7](https://en.wikipedia.org/wiki/OSI_model#Layer_7:_Application_Layer) level. Getting an application up and running is not always the entire story; it may still need a way for users to access it. Filling that operational gap is what Contour was designed to do by providing a way to allow users to access applications within a Kubernetes cluster.
Contour is an Ingress controller for Kubernetes that works by deploying the Envoy proxy as a reverse proxy and load balancer. Contour supports dynamic configuration updates out of the box while maintaining a lightweight profile.

Contour offers the following benefits for users:  

 - A simple installation mechanism to quickly deploy and integrate Envoy  
 - Safely support ingress in multi-team Kubernetes clusters  
 - Clean integration with the Kubernetes object model  
 - Dynamic updates to ingress configuration without dropped connections  

## What is Ingress?
[Kubernetes Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/) is a set of configurations that define how external traffic can be routed to an application inside a Kubernetes cluster. A controller (Contour) watches for changes to objects in the cluster, then wires together the configurations to create a data path for the request to be resolved, implementing the configurations defined. It makes decisions based on the request received (e.g., example.com/blog), provides TLS termination, and performs other functions.

Ingress is an important component of a cloud native system because it allows for a clean separation between the application and how it’s accessed. A cluster administrator deals with providing access to the controller, and the application engineer just deals with deploying the application. Ingress is the glue that ties the two together.

## Contour in Detail
Since it was added in Kubernetes 1.1, Ingress hasn’t gotten much attention but is still very popular in the community. Many controllers rely on annotations on the Ingress object to clarify, restrict, or augment the structure imposed by the Ingress object, which is no different from how Contour supports Ingress.

At the same time a number of web application deployment patterns, such as blue/green deployments, explicit load balancing strategies, and presenting more than one Kubernetes Service behind a single route, are difficult to achieve with Ingress as it stands today. Contour has introduced a new Custom Resource Definition (CRD) that allows for a new data model called `IngressRoute` and enhances what Ingress can do today by enabling new features not previously possible.

IngressRoute is designed to provide a sensible home for configuration parameters as well as to share an ingress controller across multiple namespaces and teams in the same Kubernetes cluster. We do this by using a process we call delegation. This delegation concept patterns off of the way a subdomain is delegated from one domain name server to another, and allows for teams to define and self-manage IngressRoute resources safely.

## Contour 0.10
Version 0.10 of Contour adds some exciting features to address TLS certificates and how they are referenced. This new version brings a feature called [TLS Certificate Delegation](https://github.com/heptio/contour/blob/master/design/tls-certificate-delegation.md). This facility makes it possible for an IngressRoute objects to reference, subject to the appropriate permissions, a Kubernetes Secret object in another namespace. The primary use case for this facility is to allow you, as an administrator, to place a TLS wildcard certificate in a secret object in your own namespace and delegate the permission for Contour to reference that secret from another namespace.

Much like how IngressRoute delegation can limit which namespaces can utilize a host plus path combination, this TLS cert delegation now similarly limits what certificates users can access, further enhancing Contour’s multi-team functionality.

## Future Plans
The Contour team would love to hear your feedback on your application requirements (not just how you do it today). This approach ensures that we don’t use a feature the wrong way to abuse how one is designed to solve other requirements.

Contour is also very community driven so please speak up! Many features today (including IngressRoute) were driven from users who needed a better way to solve their current problems.

If you are interested in contributing, a great place to start is to comment on one of the issues labeled with [Help Wanted](https://github.com/heptio/contour/issues?q=is%3Aopen+is%3Aissue+label%3A%22help+wanted%22) and work with the team on how to resolve them.

## Join the Contour Community!
* Get updates on Twitter ([@projectcontour](https://twitter.com/projectcontour))
* Chat with us on Slack ([#contour](https://kubernetes.slack.com/messages/C8XRH2R4J) on Kubernetes)
* Collaborate with us on GitHub: [github.com/heptio/contour](https://github.com/heptio/contour)

We’re immensely grateful for all of the community contributions that help make Contour even better! For version 0.10, special thanks go out to:
* @vaamarnath
* @256dpi

_Previously posted on <https://blogs.vmware.com/cloudnative/2019/03/08/routing-traffic-kubernetes-contour-0-10/>_
