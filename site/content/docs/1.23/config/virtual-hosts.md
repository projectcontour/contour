# Virtual Hosts


Similar to Ingress, HTTPProxy support name-based virtual hosting.
Name-based virtual hosts use multiple host names with the same IP address.

```
foo.bar.com --|                 |-> foo.bar.com s1:80
              | 178.91.123.132  |
bar.foo.com --|                 |-> bar.foo.com s2:80
```

Unlike Ingress however, HTTPProxy only support a single root domain per HTTPProxy object.
As an example, this Ingress object:

```yaml
# ingress-name.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: name-example
spec:
  rules:
  - host: foo1.bar.com
    http:
      paths:
      - backend:
          service:
            name: s1
            port:
              number: 80
        pathType: Prefix              
  - host: bar1.bar.com
    http:
      paths:
      - backend:
          service:
            name: s2
            port:
              number: 80
        pathType: Prefix
```

must be represented by two different HTTPProxy objects:

```yaml
# httpproxy-name.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: name-example-foo
  namespace: default
spec:
  virtualhost:
    fqdn: foo1.bar.com
  routes:
    - services:
      - name: s1
        port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: name-example-bar
  namespace: default
spec:
  virtualhost:
    fqdn: bar1.bar.com
  routes:
    - services:
        - name: s2
          port: 80
```

A HTTPProxy object that contains a [`virtualhost`][2] field is known as a "root proxy".

## Virtualhost aliases

To present the same set of routes under multiple DNS entries (e.g. `www.example.com` and `example.com`), including a service with a `prefix` condition of `/` can be used.

```yaml
# httpproxy-inclusion-multipleroots.yaml
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-root
  namespace: default
spec:
  virtualhost:
    fqdn: bar.com
  includes:
  - name: main
    namespace: default
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-root-www
  namespace: default
spec:
  virtualhost:
    fqdn: www.bar.com
  includes:
  - name: main
    namespace: default
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: main
  namespace: default
spec:
  routes:
  - services:
    - name: s2
      port: 80
```

## Restricted root namespaces

HTTPProxy inclusion allows Administrators to limit which users/namespaces may configure routes for a given domain, but it does not restrict where root HTTPProxies may be created.
Contour has an enforcing mode which accepts a list of namespaces where root HTTPProxy are valid.
Only users permitted to operate in those namespaces can therefore create HTTPProxy with the [`virtualhost`] field ([see API docs][2]).

This restricted mode is enabled in Contour by specifying a command line flag, `--root-namespaces`, which will restrict Contour to only searching the defined namespaces for root HTTPProxy. This CLI flag accepts a comma separated list of namespaces where HTTPProxy are valid (e.g. `--root-namespaces=default,kube-system,my-admin-namespace`).

HTTPProxy with a defined [virtualhost][2] field that are not in one of the allowed root namespaces will be flagged as `invalid` and will be ignored by Contour.

Additionally, when defined, Contour will only watch for Kubernetes secrets in these namespaces ignoring changes in all other namespaces.
Proper RBAC rules should also be created to restrict what namespaces Contour has access matching the namespaces passed to the command line flag.
An example of this is included in the [examples directory][1] and shows how you might create a namespace called `root-httproxy`.

_**Note:** The restricted root namespace feature is only supported for HTTPProxy CRDs.
`--root-namespaces` does not affect the operation of Ingress objects._

[1]: {{< param github_url>}}/tree/{{< param branch >}}/examples/root-rbac
[2]: api/#projectcontour.io/v1.VirtualHost
