# HTTPProxy Inclusion

HTTPProxy permits the splitting of a system's configuration into separate HTTPProxy instances using **inclusion**.

Inclusion, as the name implies, allows for one HTTPProxy object to be included in another, optionally with some conditions inherited from the parent.
Contour reads the inclusion tree and merges the included routes into one big object internally before rendering Envoy config.
Importantly, the included HTTPProxy objects do not have to be in the same namespace.

Each tree of HTTPProxy starts with a root, the top level object of the configuration for a particular virtual host.
Each root HTTPProxy defines a `virtualhost` key, which describes properties such as the fully qualified name of the virtual host, TLS configuration, etc.

HTTPProxies included from the root must not contain a virtualhost key.
Root objects cannot include other roots either transitively or directly.
This permits the owner of an HTTPProxy root to allow the inclusion of a portion of the route space inside a virtual host, and to allow that route space to be further subdivided with inclusions.
Because the path is not necessarily used as the only key, the route space can be multi-dimensional.

## Conditions and Inclusion

Like Routes, Inclusion may specify a set of [conditions][1].
These conditions are added to any conditions on the routes included.
This process is recursive.

Conditions are sets of individual condition statements, for example `prefix: /blog` is the condition that the matching request's path must start with `/blog`.
When conditions are combined through inclusion Contour merges the conditions inherited via inclusion with any conditions specified on the route.
This may result in duplicates, for example two `prefix:` conditions, or two header match conditions with the same name and value.
To resolve this Contour applies the following logic.

- `prefix:` conditions are concatenated together in the order they were applied from the root object. For example the conditions, `prefix: /api`, `prefix: /v1` becomes a single `prefix: /api/v1` conditions. Note: Multiple prefixes cannot be supplied on a single set of Route conditions.
- Proxies with repeated identical `header:` conditions of type "exact match" (the same header keys exactly) are marked as "Invalid" since they create an un-routable configuration.

## Configuring Inclusion

Inclusion is a top-level field in the HTTPProxy [spec][2] element.
It requires one field, `name`, and has two optional fields:

- `namespace`. This will assume the included HTTPProxy is in the same namespace if it's not specified.
- a `conditions` block.

## Inclusion Within the Same Namespace

HTTPProxies can include other HTTPProxy objects in the namespace by specifying the name of the object and its namespace in the top-level `includes` block.
Note that `includes` is a list, and so it must use the YAML list construct.

In this example, the HTTPProxy `include-root` has included the configuration for paths matching `/service2` from the HTTPProxy named `service2` in the same namespace as `include-root` (the `default` namespace).
It's important to note that `service2` HTTPProxy has not defined a `virtualhost` property as it is NOT a root HTTPProxy.

```yaml
# httpproxy-inclusion-samenamespace.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: include-root
  namespace: default
spec:
  virtualhost:
    fqdn: root.bar.com
  includes:
  # Includes the /service2 path from service2 in the same namespace
  - name: service2
    namespace: default
    conditions:
    - prefix: /service2
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
  name: service2
  namespace: default
spec:
  routes:
    - services: # matches /service2
        - name: s2
          port: 80
    - conditions:
      - prefix: /blog # matches /service2/blog
      services:
        - name: blog
          port: 80
```

## Inclusion Across Namespaces

Inclusion can also happen across Namespaces by specifying a `namespace` in the `inclusion`.
This is a particularly powerful paradigm for enabling multi-team Ingress management.

In this example, the root HTTPProxy has included configuration for paths matching `/blog` to the `blog` HTTPProxy object in the `marketing` namespace.

```yaml
# httpproxy-inclusion-across-namespaces.yaml
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: namespace-include-root
  namespace: default
spec:
  virtualhost:
    fqdn: ns-root.bar.com
  includes:
  # delegate the subpath, `/blog` to the HTTPProxy object in the marketing namespace with the name `blog`
  - name: blog
    namespace: marketing
    conditions:
    - prefix: /blog
  routes:
    - services:
        - name: s1
          port: 80

---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: blog
  namespace: marketing
spec:
  routes:
    - services:
        - name: s2
          port: 80
```

## Orphaned HTTPProxy children

It is possible for HTTPProxy objects to exist that have not been delegated to by another HTTPProxy.
These objects are considered "orphaned" and will be ignored by Contour in determining ingress configuration.

[1]: request-routing#conditions
[2]: api/#projectcontour.io/v1.HTTPProxySpec
