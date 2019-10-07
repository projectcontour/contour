---
title: HTTPProxy in Action
image: /img/posts/kind-contour.png
excerpt: This blog post demonstrates how HTTPProxy works within a Kubernetes cluster for ingress resources.
author_name: Steve Sloka
author_avatar: /img/contributors/steve-sloka.png
categories: [kubernetes]
# Tag should match author to drive author pages
tags: ['Contour Team', 'Steve Sloka', 'httpproxy']
---

In the previous blog post, Dave Cheney walked through Contour’s evolution from `IngressRoute` to `HTTPProxy` and explained how & why the move happened.

Contour now allows for additional routing configuration outside of just supporting a `path prefix` which is enabled since the beta.1 release of Contour adds header routing capabilities.

This post looks to demonstrate a practical implementation of HTTPProxy and review some examples that explain how you can use in your cluster today! 

## Hello World Proxy
Every first example shows the “Hello World” example and this is no exception. Following is a simple example of how a user can route requests to `projectcontour.io` with two different paths. 

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: helloworld
  namespace: default
spec:
  virtualhost:
    fqdn: projectcontour.io
  routes:
  - condition:
      prefixPath: /blog
    services:
    - name: blogsite
      port: 80
 - condition:
      prefixPath: /payment
    services:
    - name: money
      port: 8080
 - services:
    - name: default
      port: 80
```

### Requests 
Requests to the following domain + path will be handled by a different service. 

GET projectcontour.io/ → `default:80` service
GET projectcontour.io/blog → `blogsite:80` service
GET projectcontour.io/payment → `money:8080` service
Demo Time!
Let’s walk through a simple scenario to quickly demonstrate how these new features work. We’ll start by creating a sample set of applications and services which we will use to set up some routing with HTTPProxy across two namespaces:

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/01-app.yaml 
```
