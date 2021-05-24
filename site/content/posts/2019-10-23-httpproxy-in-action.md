---
title: HTTPProxy in Action
image: /img/posts/proxyinaction.png
excerpt: This blog post covers a practical demonstration of how HTTPProxy works.
author_name: Steve Sloka
author_avatar: /img/contributors/steve-sloka.png
categories: [kubernetes]
# Tag should match author to drive author pages
tags: ['Contour Team', 'Steve Sloka', 'tutorial']
date: 2019-10-23
slug: httpproxy-in-action
---

In our previous [blog post][1], Dave Cheney walked through Contour’s evolution from `IngressRoute` to `HTTPProxy` and explained how & why the move happened.

Now with `HTTPProxy`, Contour allows for additional routing configuration outside of just supporting a `path prefix`.

This post demonstrates a practical implementation of `HTTPProxy` and reviews some examples that explain how you can use it in your cluster today. 

[![img][2]][3]
*Here's a quick video demonstration walking through the rest of the blog post.*

## Prerequisites
If you’d like to follow along in your own cluster, you’ll need a working Kubernetes cluster as well as Contour deployed. There are a number of ways to get these up and working. A simple way to test this locally is to use Kubernetes in Docker (Kind); you can check out our previous blog post on how to get this up and running on your local machine: [https://projectcontour.io/kindly-running-contour/][4]

## Demo Time
Let’s walk through a simple scenario to quickly demonstrate how these new features work with `HTTPProxy`. This demo will progress through various features of `HTTPProxy` by starting off with a set of prerequisite services and deployments. Then it will move to implement `conditions` on routes to further specify request route matching. Finally, we’ll introduce `includes`, which will allow us to delegate path and header conditions to other `HTTPProxy` resources in different namespaces. 

## Setup and Prerequisites
We’ll start by creating a sample set of applications and services, which we will use to set up some routing with `HTTPProxy`:

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/01-prereq.yaml 
```

## Basic HTTPProxy
Next, we’ll apply our root HTTPProxy. This proxy is unique since it defines the `fqdn` for the requests. In our example, the FQDN is `local.projectcontour.io`. It configures two routes, one for `/` and another for `/secure`. 

_Note: If you’re running this in your own cluster, update the `fqdn` value in the spec.virtualhost section of the HTTPProxy before applying each HTTPProxy update. `local.projectcontour.io` points to 127.0.0.1 and works well if you’re running a `kind` setup._

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/02-proxy-basic.yaml

apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root
  namespace: projectcontour-roots
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - services:
    - name: rootapp
      port: 80
    conditions:
      - prefix: /
  - services:
    - name: secureapp-default
      port: 80
    conditions:
      - prefix: /secure
``` 

This proxy configures any request to `projectcontour.io/` to be handled by the service `rootapp`. Requests to `/secure` will be handled by the service `secureapp-default`.

At this point, the following requests map like this:

- GET projectcontour.io/ → `rootapp:80` service
- GET projectcontour.io/secure → `secureapp-default:80` service

### Sample Requests
```bash
$ curl http://local.projectcontour.io/ 

ECHO Request Server: 
--------------------
App: 
    This is the default app site!
Request: 
    http://local.projectcontour.io/

$ curl http://local.projectcontour.io/secure

ECHO Request Server: 
--------------------
App: 
    This is the secure app site!
Request: 
    http://local.projectcontour.io/secure
```
## Conditions
New in `HTTPProxy` is a concept called `conditions`, which allows you to define a set of request parameters that need to match for a route to receive requests. Contour allows for a `prefix` condition as well as a set of `header` conditions to be defined on requests. 

Let’s reconfigure the `root` proxy to handle requests with HTTP headers. We will target any request to `local.projectcontour.io/secure` that matches the header `User-Agent` containing the value of `Chrome` to route to the `secureapp` backend. Any other requests to `local.projectcontour.io/secure` that do not match the header we defined should route to `secureapp-default`.

In the previous example, we added `prefix` conditions to the route. In this example, we add a new type called `header`, which allows us to define a header key and value that must match the request. Header conditions can use `exact` to match  the value exactly, or  they can use `contains` to match  a specified value that exists somewhere in the header value. (We can also specify `notexact` and `notcontains`, which inverse the match.) 

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/03-proxy-conditions.yaml

apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root
  namespace: projectcontour-roots
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
    - services:
        - name: rootapp
          port: 80
      conditions:
        - prefix: /
    - services:
        - name: secureapp-default
          port: 80
      conditions:
        - prefix: /secure
    - services:
        - name: secureapp
          port: 80
      conditions:
        - prefix: /secure
        - header:
            name: User-Agent
            contains: Chrome
```

It’s important to call out that for a request to match a route, all `conditions` defined must match. In the example we just applied, for the request to route to the `secreapp` service, it must have a path prefix of `/secure` as well as have a header `User-Agent` containing the value `Chrome`. Adding additional header conditions would extend the match requirements for that route. 

At this point the following requests map as follows:

- GET local.projectcontour.io/ → `rootapp:80` service
- GET local.projectcontour.io/secure → `secureapp-default:80` service
- GET local.projectcontour.io/secure + Header[User-Agent: Chrome] → `secureapp:80` service

### Sample Requests
```
$ curl http://local.projectcontour.io/secure                                                                                                

ECHO Request Server: 
--------------------
App: 
    This is the DEFAULT secure app site!
Request: 
    http://local.projectcontour.io/secure

$ curl -H "User-Agent: Chrome" http://local.projectcontour.io/secure                                                                           

ECHO Request Server: 
--------------------
App: 
    This is the secure app site!
Request: 
    http://local.projectcontour.io/secure
```

## Includes across Namespaces
Also new in `HTTPProxy` is the concept of `includes` for a resource. Defining an `include` on an `HTTPProxy` causes Contour to prepend any `conditions` defined in the include to the conditions of the child proxy referenced. This can be used to delegate path prefixes to teams in different namespaces or require specific headers to be present on requests. There isn’t a limit to the number of times a proxy can reference an `include`.

First, let’s set up a new marketing team namespace allowing them to self-manage its `HTTPProxy` resources as well as deploy a sample app/service. The marketing team will be in charge of managing the `/blog` path from the  `local.projectcontour.io` domain.

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/04-marketing-prereq.yaml 
```

Next, let’s update our `root` proxy to pass off a path to the marketing team working in the `projectcontour-marketing` namespace. We’ll also create the marketing team’s `HTTPProxy`, which is referenced from the `root` HTTPProxy and will include the path condition of `/blog` to the marketing team’s proxy named `blogsite`.

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/04-proxy-include-basic.yaml

apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root
  namespace: projectcontour-roots
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  includes:
    - name: blogsite
      namespace: projectcontour-marketing
      conditions:
        - prefix: /blog
  routes:
    - services:
        - name: rootapp
          port: 80
      conditions:
        - prefix: /
    - services:
        - name: secureapp-default
          port: 80
      conditions:
        - prefix: /secure
    - services:
        - name: secureapp
          port: 80
      conditions:
        - prefix: /secure
        - header:
            name: User-Agent
            contains: Chrome
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: blogsite
  namespace: projectcontour-marketing
spec:
  routes:
    - services:
        - name: wwwblog
          port: 80
```

Since the `root` proxy included the path of `/blog`, requests that match `local.projectcontour.io/blog` will route to the marketing team’s service named `wwwblog` in the `projectcontour-marketing` namespace. You will notice we didn’t define any `conditions` on the marketing teams proxy. We don’t need to define them because the root proxy passed the path prefix condition to this HTTPProxy through the include. 

It’s important to note that no one else in the cluster can now utilize this path prefix. If another team would create an HTTPProxy referencing the same path, Contour would reject it because it is not part of a delegation chain. 

### Sample Requests
```
$ curl http://local.projectcontour.io/blog                                                                                                    

ECHO Request Server: 
--------------------
App: 
    This is the blog site!
Request: 
    http://local.projectcontour.io/blog
```

## Includes to the same Namespace
Just as we learned in the first example of how conditions are applied to routes, the same logic is applied to conditions on an include. Currently, the marketing team is responsible for the path `/blog`, but let’s add a few more requirements. 

The marketing team wants to create an `information` site, which will be served by the path `/blog/info`. We will create another HTTPProxy to define how the information application should be accessed. This new `HTTPProxy` will be included in the path `/info` from the `blogsite` `HTTPProxy` in the `projectcontour-marketing` namespace. 

Since `Conditions` are appended to the `HTTPProxy` when Contour processes them, the result for the information site will have the path `/blog/info`.   

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/05-proxy-include-info.yaml

apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: blogsite
  namespace: projectcontour-marketing
spec:
  includes:
    - name: infosite
      conditions:
      - prefix: /info
  routes:
    - services:
        - name: wwwblog
          port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: infosite
  namespace: projectcontour-marketing
spec:
  routes:
    - services:
        - name: info
          port: 80
```

### Sample Requests
```
$ curl http://local.projectcontour.io/blog/info                                                                                               

ECHO Request Server: 
--------------------
App: 
    This is the blog site!
Request: 
    http://local.projectcontour.io/blog/info
```

## Includes with Headers
Finally to complete our example, the administrative team now doesn’t want the marketing site to be served by Chrome or Firefox browsers. This rule needs to apply to all applications in the `projectcontour-marketing` namespace. 

We can easily implement this requirement by just adding another set of conditions to the `root` HTTPProxy. Once we add those, they will take effect across all the children of the root HTTPProxy defined in the `projectcontour-marketing` namespace. 

```bash
$ kubectl apply -f https://projectcontour.io/examples/proxydemo/06-proxy-include-headers.yaml

apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root
  namespace: projectcontour-roots
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  includes:
    - name: blogsite
      namespace: projectcontour-marketing
      conditions:
        - prefix: /blog
        - header:
            name: User-Agent
            notcontains: Chrome
        - header:
            name: User-Agent
            notcontains: Firefox
  routes:
    - services:
        - name: rootapp
          port: 80
      conditions:
        - prefix: /
    - services:
        - name: secureapp-default
          port: 80
      conditions:
        - prefix: /secure
    - services:
        - name: secureapp
          port: 80
      conditions:
        - prefix: /secure
        - header:
            name: User-Agent
            contains: Chrome
```

Requests to `local.projectcontour.io/blog/*` that do not match the `User-Agent` header of `Chrome` or `Firefox` will now route to the appropriate services in the marketing team’s namespace. Any other requests will be handled by the `rootapp` service because it matches the other requests. 

### Sample Requests
```bash
$ curl -H "User-Agent: Safari" http://local.projectcontour.io/blog/info                                                                       

ECHO Request Server: 
--------------------
App: 
    This is the INFO site!
Request: 
    http://local.projectcontour.io/blog/info

$ curl -H "User-Agent: Firefox" http://local.projectcontour.io/blog/info                                                                      

ECHO Request Server: 
--------------------
App: 
    This is the default app site!
Request: 
    http://local.projectcontour.io/blog/info
```

[1]: {% post_url 2019-09-27-from-ingressroute-to-httpproxy %}
[2]: {% link img/posts/kind-contour-video.png %}
[3]: https://youtu.be/YA82A4Rcs_A
[4]: {% post_url 2019-07-11-kindly-running-contour %}
