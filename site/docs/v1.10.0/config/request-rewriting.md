# Request Rewriting

## Path Rewriting

HTTPProxy supports rewriting the HTTP request URL path prior to delivering the request to the backend service.
Rewriting is performed after a routing decision has been made, and never changes the request destination.

The `pathRewritePolicy` field specifies how the path prefix should be rewritten.
The `replacePrefix` rewrite policy specifies a replacement string for a HTTP request path prefix match.
When this field is present, the path prefix that the request matched is replaced by the text specified in the `replacement` field.
If the HTTP request path is longer than the matched prefix, the remainder of the path is unchanged.

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
      - replacement: /new/prefix
```

The `replacePrefix` field accepts an array of possible replacements.
When more than one `replacePrefix` array element is present, the `prefix` field can be used to disambiguate which replacement to apply.

If no `prefix` field is present, the replacement is applied to all prefix matches made against the route.
If a `prefix` field is present, the replacement is applied only to routes that have an exactly matching prefix condition.
Specifying more than one `replacePrefix` entry is mainly useful when a HTTPProxy document is included into multiple parent documents.

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
    conditions:
    - prefix: /v1/api
    pathRewritePolicy:
      replacePrefix:
      - prefix: /v1/api
        replacement: /app/api/v1
      - prefix: /
        replacement: /app
```

### Header Rewriting

HTTPProxy supports rewriting HTTP request and response headers.
The `Set` operation sets a HTTP header value, creating it if it doesn't already exist or overwriting it if it does.
The `Remove` operation removes a HTTP header.
The `requestHeadersPolicy` field is used to rewrite headers on a HTTP request, and the `responseHeadersPolicy` is used to rewrite headers on a HTTP response.
These fields can be specified on a route or on a specific service, depending on the rewrite granularity you need.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: header-rewrite-example
spec:
  virtualhost:
    fqdn: header.bar.com
  routes:
  - services:
    - name: s1
      port: 80
    requestHeadersPolicy:
      set:
      - name: Host
        value: external.dev
      remove:
      - Some-Header
      - Some-Other-Header
```

Manipulating headers is also supported per-Service or per-Route.  Headers can be set or
removed from the request or response as follows:

per-Service:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: header-manipulation
  namespace: default
spec:
  virtualhost:
    fqdn: headers.bar.com
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
       responseHeadersPolicy:
         set:
         - name: X-Service-Name
           value: s1
         remove:
         - X-Internal-Secret
```

per-Route:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: header-manipulation
  namespace: default
spec:
  virtualhost:
    fqdn: headers.bar.com
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
    responseHeadersPolicy:
      set:
      - name: X-Service-Name
        value: s1
      remove:
      - X-Internal-Secret
```

In these examples we are setting the header `X-Foo` with value `baz` on requests
and stripping `X-Baz`.  We are then setting `X-Service-Name` on the response with
value `s1`, and removing `X-Internal-Secret`.
