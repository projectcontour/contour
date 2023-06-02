# HTTPProxy Fundamentals

The [Ingress][1] object was added to Kubernetes in version 1.1 to describe properties of a cluster-wide reverse HTTP proxy.
Since that time, the Ingress API has remained relatively unchanged, and the need to express implementation-specific capabilities has inspired an [explosion of annotations][2].

The goal of the HTTPProxy Custom Resource Definition (CRD) is to expand upon the functionality of the Ingress API to allow for a richer user experience as well addressing the limitations of the latter's use in multi tenant environments.

## Key HTTPProxy Benefits

- Safely supports multi-team Kubernetes clusters, with the ability to limit which Namespaces may configure virtual hosts and TLS credentials.
- Enables including of routing configuration for a path or domain from another HTTPProxy, possibly in another Namespace.
- Accepts multiple services within a single route and load balances traffic across them.
- Natively allows defining service weighting and load balancing strategy without annotations.
- Validation of HTTPProxy objects at creation time and status reporting for post-creation validity.

## Ingress to HTTPProxy

A minimal Ingress object might look like:

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: basic
spec:
  rules:
  - host: foo-basic.bar.com
    http:
      paths:
      - backend:
          service:
            name: s1
            port:
              number: 80
        pathType: Prefix
```

This Ingress object, named `basic`, will route incoming HTTP traffic with a `Host:` header for `foo-basic.bar.com` to a Service named `s1` on port `80`.
Implementing similar behavior using an HTTPProxy looks like this:

```yaml
# httpproxy.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: basic
spec:
  virtualhost:
    fqdn: foo-basic.bar.com
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
```

**Lines 1-5**: As with all other Kubernetes objects, an HTTPProxy needs apiVersion, kind, and metadata fields.

**Lines 7-8**: The presence of the `virtualhost` field indicates that this is a root HTTPProxy that is the top level entry point for this domain.


## Interacting with HTTPProxies

As with all Kubernetes objects, you can use `kubectl` to create, list, describe, edit, and delete HTTPProxy CRDs.

Creating an HTTPProxy:

```bash
$ kubectl create -f basic.httpproxy.yaml
httpproxy "basic" created
```

Listing HTTPProxies:

```bash
$ kubectl get httpproxy
NAME      AGE
basic     24s
```

Describing HTTPProxy:

```bash
$ kubectl describe httpproxy basic
Name:         basic
Namespace:    default
Labels:       <none>
API Version:  projectcontour.io/v1
Kind:         HTTPProxy
Metadata:
  Cluster Name:
  Creation Timestamp:  2019-07-05T19:26:54Z
  Resource Version:    19373717
  Self Link:           /apis/projectcontour.io/v1/namespaces/default/httpproxy/basic
  UID:                 6036a9d7-8089-11e8-ab00-f80f4182762e
Spec:
  Routes:
    Conditions:
      Prefix: /
    Services:
      Name:  s1
      Port:  80
  Virtualhost:
    Fqdn:  foo-basic.bar.com
Events:    <none>
```

Deleting HTTPProxies:

```bash
$ kubectl delete httpproxy basic
httpproxy "basic" deleted
```

## Status Reporting

There are many misconfigurations that could cause an HTTPProxy or delegation to be invalid.
Contour will make its best effort to process even partially valid configuration and allow traffic to be served for the valid parts.
To aid users in resolving any issues, Contour updates a `status` field in all HTTPProxy objects.

If an HTTPProxy object is valid, it will have a status property that looks like this:

```yaml
status:
  currentStatus: valid
  description: valid HTTPProxy
```

If the HTTPProxy is invalid, the `currentStatus` field will be `invalid` and the `description` field will provide a description of the issue.

As an example, if an HTTPProxy object has specified a negative value for weighting, the HTTPProxy status will be:

```yaml
status:
  currentStatus: invalid
  description: "route '/foo': service 'home': weight must be greater than or equal to zero"
```

Some examples of invalid configurations that Contour provides statuses for:

- Negative weight provided in the route definition.
- Invalid port number provided for service.
- Prefix in parent does not match route in delegated route.
- Root HTTPProxy created in a namespace other than the allowed root namespaces.
- A given Route of an HTTPProxy both delegates to another HTTPProxy and has a list of services.
- Orphaned route.
- Delegation chain produces a cycle.
- Root HTTPProxy does not specify fqdn.
- Multiple prefixes cannot be specified on the same set of route conditions.
- Multiple header conditions of type "exact match" with the same header key.
- Contradictory header conditions on a route, e.g. a "contains" and "notcontains" condition for the same header and value.

Invalid configuration is ignored and will be not used in the ingress routing configuration.
Envoy will respond with an error when HTTP request is received on route with invalid configuration on following cases:

* `502 Bad Gateway` response is sent when HTTPProxy has an include that refers to an HTTPProxy that does not exist.
* `503 Service Unavailable` response is sent when HTTPProxy refers to a service that does not exist.

### Example

Following example has two routes: the first one is valid, the second one refers to a service that does not exist.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-routes-with-a-missing-service
spec:
  virtualhost:
    fqdn: www.example.com
  routes:
    - conditions:
      - prefix: /
      services:
        - name: valid-service
          port: 80
    - conditions:
      - prefix: /subpage
      services:
        - name: service-that-does-not-exist
          port: 80
```

The `HTTPProxy` will have condition `Valid=false` with detailed error message: `Spec.Routes unresolved service reference: service "default/service-that-does-not-exist" not found`.
Requests received for `http://www.example.com/` will be forwarded to `valid-service` but requests received for `http://www.example.com/subpage` will result in error `503 Service Unavailable` response from Envoy.

## HTTPProxy API Specification

The full HTTPProxy specification is described in detail in the [API documentation][4].
There are a number of working examples of HTTPProxy objects in the [`examples/example-workload`][3] directory of the Contour Github repository.

 [1]: https://kubernetes.io/docs/concepts/services-networking/ingress/
 [2]: https://github.com/kubernetes/ingress-nginx/blob/master/docs/user-guide/nginx-configuration/annotations.md
 [3]: {{< param github_url>}}/tree/{{< param branch >}}/examples/example-workload/httpproxy
 [4]: api.md
