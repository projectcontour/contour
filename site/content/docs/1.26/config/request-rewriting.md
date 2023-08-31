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

## Header Rewriting

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

### Dynamic Header Values

It is sometimes useful to set a header value using a dynamic value such as the
hostname where the Envoy Pod is running (`%HOSTNAME%`) or the subject of the
TLS client certificate (`%DOWNSTREAM_PEER_SUBJECT%`) or based on another header
(`%REQ(header)%`).

Examples:
```
    requestHeadersPolicy:
      set:
      - name: X-Envoy-Hostname
        value: "%HOSTNAME%"
      - name: X-Host-Protocol
        value: "%REQ(Host)% - %PROTOCOL%"
    responseHeadersPolicy:
      set:
      - name: X-Envoy-Response-Flags
        value: "%RESPONSE_FLAGS%"
```

Contour supports most of the custom request/response header variables offered
by Envoy - see the <a
href="https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#custom-request-response-headers">Envoy
documentation </a> for details of what each of these resolve to:

* `%DOWNSTREAM_REMOTE_ADDRESS%`
* `%DOWNSTREAM_REMOTE_ADDRESS_WITHOUT_PORT%`
* `%DOWNSTREAM_LOCAL_ADDRESS%`
* `%DOWNSTREAM_LOCAL_ADDRESS_WITHOUT_PORT%`
* `%DOWNSTREAM_LOCAL_PORT%`
* `%DOWNSTREAM_LOCAL_URI_SAN%`
* `%DOWNSTREAM_PEER_URI_SAN%`
* `%DOWNSTREAM_LOCAL_SUBJECT%`
* `%DOWNSTREAM_PEER_SUBJECT%`
* `%DOWNSTREAM_PEER_ISSUER%`
* `%DOWNSTREAM_TLS_SESSION_ID%`
* `%DOWNSTREAM_TLS_CIPHER%`
* `%DOWNSTREAM_TLS_VERSION%`
* `%DOWNSTREAM_PEER_FINGERPRINT_256%`
* `%DOWNSTREAM_PEER_FINGERPRINT_1%`
* `%DOWNSTREAM_PEER_SERIAL%`
* `%DOWNSTREAM_PEER_CERT%`
* `%DOWNSTREAM_PEER_CERT_V_START%`
* `%DOWNSTREAM_PEER_CERT_V_END%`
* `%HOSTNAME%`
* `%REQ(header-name)%`
* `%PROTOCOL%`
* `%RESPONSE_FLAGS%`
* `%RESPONSE_CODE_DETAILS%`
* `%UPSTREAM_REMOTE_ADDRESS%`

Note that Envoy passes variables that can't be expanded through unchanged or
skips them entirely - for example:
* `%UPSTREAM_REMOTE_ADDRESS%` as a request header remains as
  `%UPSTREAM_REMOTE_ADDRESS%` because as noted in the Envoy docs: "The upstream
  remote address cannot be added to request headers as the upstream host has not
  been selected when custom request headers are generated."
* `%DOWNSTREAM_TLS_VERSION%` is skipped if TLS is not in use
* Envoy ignores REQ headers that refer to an non-existent header - for example
  `%REQ(Host)%` works as expected but `%REQ(Missing-Header)%` is skipped

Contour already sets the `X-Request-Start` request header to
`t=%START_TIME(%s.%3f)%` which is the Unix epoch time when the request
started.

To enable setting header values based on the destination service Contour also supports:

* `%CONTOUR_NAMESPACE%`
* `%CONTOUR_SERVICE_NAME%`
* `%CONTOUR_SERVICE_PORT%`

For example, with the following HTTPProxy object that has a per-Service requestHeadersPolicy using these variables:
```
# httpproxy.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: basic
  namespace: myns
spec:
  virtualhost:
    fqdn: foo-basic.bar.com
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
          requestHeadersPolicy:
            set:
            - name: l5d-dst-override
              value: "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%"
```
the values would be:
* `CONTOUR_NAMESPACE: "myns"`
* `CONTOUR_SERVICE_NAME: "s1"`
* `CONTOUR_SERVICE_PORT: "80"`

and the `l5-dst-override` header would be set to `s1.myns.svc.cluster.local:80`.

For per-Route requestHeadersPolicy only `%CONTOUR_NAMESPACE%` is set and using
`%CONTOUR_SERVICE_NAME%` and `%CONTOUR_SERVICE_PORT%` will end up as the
literal values `%%CONTOUR_SERVICE_NAME%%` and `%%CONTOUR_SERVICE_PORT%%`,
respectively.
