---
title: Migrating from IngressRoute to HTTPProxy
layout: page
---

This document describes the differences between IngressRoute and HTTPProxy.
It is intended for Contour users who have existing IngressRoute resources they wish to migrate to HTTPProxy.
It is not intended a comprehensive documentation of HTTPProxy, for that please see the [`HTTPProxy` documentation][1].

_Note: IngressRoute has been removed from Contour in v1.6._

## The easy way

The simplest way to migrate from IngressRoute to HTTPProxy is to use the `ir2proxy` tool.
Installation instructions are available at [its github repo](https://github.com/projectcontour/ir2proxy).
It's installable either by homebrew or downloading the binary.

This tool can automatically migrate most IngressRoutes to HTTPProxies.
However, due to the behavior changes around the move from delegation to inclusion, not all IngressRoutes can be translated without manual intervention.
The tool will tell you when manual intervention is required.


## Manual conversion notes

### Group, Version and Kind changes

`HTTPProxy` has moved to the `projectcontour.io` group.
The version is `v1`, with all the guarantees a GA API implies.


Before:

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
```
After:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
```

### TLS secrets

No change.

Before:

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: tls-example
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tlssecret
```

After:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: tls-example
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tlssecret
```

#### TLS Minimum protocol version

No change.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: tls-example
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tlssecret
      minimumProtocolVersion: "1.3"
```

#### Upstream TLS validation

No change.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: secure-backend
spec:
  virtualhost:
    fqdn: www.example.com
  routes:
    - match: /
      services:
        - name: service
          port: 8443
          validation:
            caSecret: my-certificate-authority
            subjectName: backend.example.com
```

#### TLS Certificate Delegation

The group and version of the TLSCertificateDelegation CRD have changed.
`contour.heptio.com/v1beta1.TLSCertificateDelegation` has been removed.

Before:

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: TLSCertificateDelegation
metadata:
  name: example-com-wildcard
  namespace: www-admin
spec:
  delegations:
    - secretName: example-com-wildcard
      targetNamespaces:
      - example-com
```
After:

```yaml
apiVersion: projectcontour.io/v1
kind: TLSCertificateDelegation
metadata:
  name: example-com-wildcard
  namespace: www-admin
spec:
  delegations:
    - secretName: example-com-wildcard
      targetNamespaces:
      - example-com
```

### Routing

HTTPProxy offers additional ways to match incoming requests to routes.
This document covers the conversion between the routing offered in IngressRoute and HTTPProxy.
For a broader discussion of HTTPProxy routing, see the [HTTPProxy documentation][1].

Before:

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: multiple-paths
  namespace: default
spec:
  virtualhost:
    fqdn: multi-path.bar.com
  routes:
    - match: / # matches everything else
      services:
        - name: s1
          port: 80
    - match: /blog # matches `multi-path.bar.com/blog` or `multi-path.bar.com/blog/*`
      services:
        - name: s2
          port: 80
```

After:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-paths
  namespace: default
spec:
  virtualhost:
    fqdn: multi-path.bar.com
  routes:
    - conditions: 
      - prefix: / # matches everything else
      services:
        - name: s1
          port: 80
    - conditions:
      - prefix: /blog # matches `multi-path.bar.com/blog` or `multi-path.bar.com/blog/*`
      services:
        - name: s2
          port: 80
```

#### Multiple services

No change.

#### Upstream weighting

No change.

#### Response timeout

`routes.timeoutPolicy.request` has been renamed to `routes.timeoutPolicy.response` to more accurately reflect is the timeout for the response.

Before:

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: request-timeout
  namespace: default
spec:
  virtualhost:
    fqdn: timeout.bar.com
  routes:
  - match: /
    timeoutPolicy:
      request: 1s
    services:
    - name: s1
      port: 80
```

After:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: request-timeout
  namespace: default
spec:
  virtualhost:
    fqdn: timeout.bar.com
  routes:
  - conditions:
    - prefix: /
    timeoutPolicy:
      response: 1s
    services:
    - name: s1
      port: 80
```

#### Prefix rewriting

Prefix rewriting is supported in HTTPProxy.

Before:
```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: app
  namespace: default
spec:
  virtualhost:
    fqdn: app.example.com
  routes:
    - match: /
      services:
        - name: app
          port: 80
    - match: /service2
      prefixRewrite: "/" # Setting this rewrites the request from `/service2` to `/`
      services:
        - name: app-service
          port: 80
```

After:
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: app
  namespace: default
spec:
  virtualhost:
    fqdn: app.example.com
  routes:
  - conditions:
    - prefix: "/"
    services:
    - name: app
      port: 80
  - conditions:
    - prefix: "/service2"
    pathRewritePolicy:
      replacePrefix:
      - replacement: / # app-service will see client requests to `/service2` coming in to `/`
    services:
    - name: app-service
      port: 80
```

### Load balancing strategies

Per service load balancing strategy has moved to a per route strategy that applies to all services for that route.

Before:

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: lb-strategy
  namespace: default
spec:
  virtualhost:
    fqdn: strategy.bar.com
  routes:
    - match: /
      services:
        - name: s1-strategy
          port: 80
          strategy: WeightedLeastRequest
        - name: s2-strategy
          port: 80
          strategy: WeightedLeastRequest
```

After:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: lb-strategy
  namespace: default
spec:
  virtualhost:
    fqdn: strategy.bar.com
  routes:
    - services:
        - name: s1-strategy
          port: 80
        - name: s2-strategy
          port: 80
      loadBalancerStrategy:
        strategy: WeightedLeastRequest
```

### Session affinity

See above.

### Per upstream health check

Per service health check has moved to a per route health check that applies to all services for that route.

Before:

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: health-check
  namespace: default
spec:
  virtualhost:
    fqdn: health.bar.com
  routes:
    - match: /
      services:
        - name: s1-health
          port: 80
          healthCheck:
            path: /healthy
            intervalSeconds: 5
            timeoutSeconds: 2
            unhealthyThresholdCount: 3
            healthyThresholdCount: 5
        - name: s2-health  # no health-check defined for this service
          port: 80
```

After:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: health-check
  namespace: default
spec:
  virtualhost:
    fqdn: health.bar.com
  routes:
    - conditions:
      - prefix: /
      healthCheckPolicy:
        path: /healthy
        intervalSeconds: 5
        timeoutSeconds: 2
        unhealthyThresholdCount: 3
        healthyThresholdCount: 5
      services:
        - name: s1-health
          port: 80
        - name: s2-health
          port: 80
```

### Websocket support

No change.

### External name support

No change.

## IngressRoute delegation

IngressRoute delegation has been replaced with HTTPProxy inclusion.
Functionally inclusion is similar to delegation.
In both scenarios the routes from one document are combined with those of the parent.

The key difference between IngressRoute's delegation and HTTPProxy's inclusion is the former has the appearance of being scoped by the path of the route of which it was attached.
As we explored the design of the next revision of IngressRoute the tight coupling of the properties of an incoming HTTP request; its path, its IP address, its headers, and so on--fundamentally run time concepts--with the inclusion of some configuration from another IngressRoute--which is definitely a configuration time concept--lead us to unanswerable questions like "what does it mean to delegate to an IngressRoute via a header".

This Gordian Knot was severed by decoupling the inclusion of one document into its parent from the facility to place restrictions on what route matching conditions could be specified in that document.
The former we call _inclusion_, the latter are known as _conditions_.
This section discusses conversion from delegation to inclusion, please see the [HTTPProxy documentation][1] for a discussion of conditions.

Before:

```yaml
# root.ingressroute.yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: delegation-root
  namespace: default
spec:
  virtualhost:
    fqdn: root.bar.com
  routes:
    - match: /
      services:
        - name: s1
          port: 80
    - match: /service2
      delegate:
        name: www
        namespace: www
---
# service2.ingressroute.yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: www
  namespace: www
spec:
  routes:
    - match: /service2
      services:
        - name: s2
          port: 80
    - match: /service2/blog
      services:
        - name: blog
          port: 80
```

After:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: delegation-root
  namespace: default
spec:
  virtualhost:
    fqdn: root.bar.com
  includes:
  - conditions:
    - prefix: /service2
    name: www
    namespace: www
  routes:
    - conditions: 
      - prefix: /
      services:
        - name: s1
          port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: www
  namespace: www
spec:
  routes:
    - conditions:
      - prefix: / # matches /service2
      services:
        - name: s2
          port: 80
    - conditions:
      - prefix: /blog # matches /service2/blog
      services:
        - name: blog
          port: 80
```

### Virtualhost aliases

No change.

### Inclusion across namespaces

As above. No change.

### Orphaned HTTPProxies

No change.
Orphaned status will be reported on _child_ HTTPProxy objects that are not included by any _root_ HTTPProxy records.

### Restricted root namespaces

The `--ingressroute-root-namespace` flag has been renamed to `--root-namespaces` for obvious reasons.
The old name has been removed.
See the [upgrading documentation]({% link _resources/upgrading.md %}) for more information on upgrading Contour.

### TCP Proxying

No change.

### TLS Termination

No change.

### TLS Passthrough

No change.

### Status reporting

Status reporting on HTTPProxy objects is similar in scope and function to IngressRoute status.

[1]: /docs/{{< param latest_version >}}/httpproxy
