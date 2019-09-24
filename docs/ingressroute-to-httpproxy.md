# IngressRoute to HTTPProxy

This document describes the differences between IngressRoute and HTTPProxy.
It is intended for Contour users who have existing IngressRoute resources they wish to migrate to HTTPProxy.
It is not intended a comprehensive documentation of HTTPProxy, for that please see the [`HTTPProxy` documentation](./httpproxy.md).

_Note: IngressRoute is deprecated and will be removed after Contour 1.0 ships in November._

## Group, Version and Kind changes

As part of the sunseting of the Heptio brand, `HTTPProxy` has moved to the `projectcontour.io` group.
For Contour 1.0.0-beta.1 the version is `v1alpha1`.
We expect this to change in forthcoming release candidates.

Before:
```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
```
After:
```yaml
apiVersion: projectcontour.io/v1alpha1
kind: HTTPProxy
```

## TLS secrets

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
apiVersion: projectcontour.io/v1alpha1
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

### TLS Minimum protocol version

No change.

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: HTTPProxy
metadata:
  name: tls-example
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tlssecret
      minimumProtocolVersion: 1.3
```

### Upstream TLS validation

No change.

```yaml
apiVersion: projectcontour.io/v1alpha1
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

### TLS Certificate Delegation

The group and version of the TLSCertificateDelegation CRD have changed.
`contour.heptio.com/v1beta1.TLSCertificateDelegation` will be removed after Contour 1.0 ships in November.

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
apiVersion: projectcontour.io/v1alpha1
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

## Routing

HTTPProxy offers additional ways to match incoming requests to routes.
This document covers the conversion between the routing offered in IngressRoute and HTTPProxy.
For a broader discussion of HTTPProxy routing, see the [Routing section of the HTTPProxy documentation](./httpproxy.md#Routing).

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
apiVersion: projectcontour.io/v1alpha1
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

### Multiple services

No change.

### Upstream weighting

No change.

### Response timeout

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
apiVersion: projectcontour.io/v1alpha1
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

### Load balancing strategies

No change.

### Session affinity

No change.

### Per upstream health check

No change.

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
This section discusses conversion from delegation to inclusion, please see the [`HTTPProxy` documentation](./httpproxy.md) for a discussion of conditions.

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
apiVersion: projectcontour.io/v1alpha1
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
apiVersion: projectcontour.io/v1alpha1
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
The old name is deprecated and will be removed after Contour 1.0 is released.
See the [upgrading documentation](./upgrading.md) for more information on upgrading from Contour 0.15.0 to 1.0.0-beta.1

### TCP Proxying

No change.

### TLS Termination

No change.

### TLS Passthrough

No change.

### Status reporting

Status reporting on HTTPProxy objects is similar in scope and function to IngressRoute status.
