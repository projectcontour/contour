---
title: Header and Host Rewrite with Contour 1.1
excerpt: Our latest release, Contour 1.1, now includes request and response header manipulation as well as host rewriting to external domains.
author_name: Steve Sloka
author_avatar: /img/contributors/steve-sloka.png
categories: [kubernetes]
# Tag should match author to drive author pages
tags: ['Contour Team', 'Steve Sloka', 'release']
date: 2020-01-16
slug: announcing-contour-1.1
---

Contour continues to take shape with new features. Our latest release, [Contour 1.1](https://github.com/projectcontour/contour/releases/tag/v1.1.0), now includes request and response header manipulation as well as host rewriting to external domains. Contour 1.1 also lets you specify a service’s protocol in HTTPProxy and adds back prefix rewrite support, which was the last feature blocking many users from migrating from IngressRoute to HTTPProxy.

## Header Manipulation

Manipulating request and response headers are supported per-Service or per-Route. A `HeaderRequestPolicy` can be defined for both request and response requests.

The header request policy has the following configuration:

* **Set**: Takes a name-value pair and will create a header if it does not exist or update the value of the header specified by the key
* **Remove**: Takes the name of a header to remove

The following example takes requests from `headers.projectcontour.io/` and applies the following logic:

* Adds the header `X-Foo: bar` to any request before it is proxied to the Kubernetes service named `s1` and removes the header `X-Baz`
* After the request is processed by service `s1`, the response back to the requester will have the header `X-Service-Name: s1` added and will remove the header `X-Internal-Secret`

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: header-manipulation
  namespace: default
spec:
  virtualhost:
    fqdn: headers.projectcontour.io
  routes:
    - services:
        - name: s1
          port: 80
          requestHeadersPolicy:
            set:
              - name: X-Foo
                value: bar
            remove:
              - X-Baz
          responseHeaderPolicy:
            set:
              - name: X-Service-Name
                value: s1
            remove:
              - X-Internal-Secret
```

## Prefix Rewrite Support

Path prefix rewrite was a feature in IngressRoute that got removed right before Contour 1.0 was released. Now in Contour 1.1, HTTPProxy supports rewriting the HTTP request URL path prior to delivering the request to the backend service. Rewriting, which is performed after a routing decision has been made, never changes the request destination.

The pathRewritePolicy field specifies how the path prefix should be rewritten. The replacePrefix rewrite policy specifies a replacement string for a HTTP request path prefix match. When this field is present, the path prefix that the request matched is replaced by the text specified in the replacement field. If the HTTP request path is longer than the matched prefix, the remainder of the path is unchanged.

The following example will replace the prefix `/v1/api` with `/app/api/v1` on the request:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: rewrite-example
  namespace: default
spec:
  virtualhost:
    fqdn: rewrite.bar.com
  routes:
  - services:
    - name: s1
      port: 80
    pathRewritePolicy:
      replacePrefix:
      - prefix: /v1/api
        replacement: /app/api/v1
```

For more information, see the documentation on [`Path Rewriting`](https://projectcontour.io/docs/v1.1.0/httpproxy/#path-rewriting).

## Host Rewrite

Contour supports routing traffic to `ExternalName` service types. This kind of traffic routing allows users to proxy traffic to resources that aren’t running in the same Kubernetes cluster. You could, for example, proxy traffic to an external object storage bucket.

Some users encountered a problem with this feature, in that the host header that the externalName service received was the same host header as in the original request. This problem potentially leads to routing in the external name service to fail. 

To solve this issue, you can set a `requestHeadersPolicy` and define the `Host` header to match the value of the externalName; here’s an example:  

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: header-rewrite-example
spec:
  virtualhost:
    fqdn: header.projectcontour.io
  routes:
  - services:
    - name: s1
      port: 80
    requestHeadersPolicy:
      set:
      - name: Host
        value: external.dev
```

Now requests to `header.projectcontour.io` will proxy to `external.dev` with the header `Host: external.dev`. 

## IngressRoute Deprecation

Since the release of Contour 1.0, `HTTPProxy` became the successor of `IngressRoute` going forward. One struggle for users wanting to migrate is a way to convert IngressRoute resources to HTTPProxy resources.

A new tool named [`ir2proxy`](https://github.com/projectcontour/ir2proxy) will take an `IngressRoute` object and migrate it to an HTTPProxy.

```yaml
$ ir2proxy basic.ingressroute.yaml
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: basic
  namespace: default
spec:
  routes:
  - conditions:
    - prefix: /
    services:
    - name: s1
      port: 80
  virtualhost:
    fqdn: foo-basic.bar.com
status: {}
```

Ir2proxy can be installed from the [releases](https://github.com/projectcontour/ir2proxy/releases) page or via [homebrew](https://github.com/projectcontour/ir2proxy#homebrew).

## Future Plans

The Contour team would love to hear your feedback! Many of the features in this release were driven by users who needed a better way to solve their problems. We’re working hard to add features to Contour, especially in expanding how we approach routing.

We recommend reading the full release notes for [Contour 1.1](https://github.com/projectcontour/contour/releases/tag/v1.1.0) as well as digging into the [upgrade guide](https://projectcontour.io/resources/upgrading/), which outlines the changes to be aware of when moving to version 1.1.

If you are interested in contributing, a great place to start is to comment on one of the issues labeled with [Help Wanted](https://github.com/projectcontour/contour/issues?q=is%3Aopen+is%3Aissue+label%3A%22help+wanted%22) and work with the team on how to resolve them.

## Thank you!

We’re immensely grateful for all the community contributions that help make Contour even better! For version 1.1, special thanks go out to the following people:

[@alvaroaleman](https://github.com/alvaroaleman)  
[@SDBrett](https://github.com/SDBrett)  
[@dhxgit](https://github.com/dhxgit)  
[@mattmoor](https://github.com/mattmoor)  
[@stefanprodan](https://github.com/stefanprodan)  
[@surajssd](https://github.com/surajssd)  
[@tsaarni](https://github.com/tsaarni)  
[@masa213f](https://github.com/masa213f)  
