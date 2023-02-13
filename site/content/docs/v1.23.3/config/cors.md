# CORS

A CORS (Cross-origin resource sharing) policy can be set for a HTTPProxy in order to allow cross-domain requests for trusted sources.
If a policy is set, it will be applied to all the routes of the virtual host.

Contour allows configuring the headers involved in responses to cross-domain requests.
These include the `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, `Access-Control-Expose-Headers`, `Access-Control-Max-Age`, and `Access-Control-Allow-Credentials` headers in responses.

In this example, cross-domain requests will be allowed for any domain (note the `*` value), with the methods `GET`, `POST`, or `OPTIONS`.
Headers `Authorization` and `Cache-Control` will be passed to the upstream server and headers `Content-Length` and `Content-Range` will be made available to the cross-origin request client.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: cors-example
spec:
  virtualhost:
    fqdn: www.example.com
    corsPolicy:
        allowCredentials: true
        allowOrigin:
          - "*" # allows any origin
        allowMethods:
          - GET
          - POST
          - OPTIONS
        allowHeaders:
          - authorization
          - cache-control
        exposeHeaders:
          - Content-Length
          - Content-Range
        maxAge: "10m" # preflight requests can be cached for 10 minutes.
  routes:
    - conditions:
      - prefix: /
      services:
        - name: cors-example
          port: 80
```

The `allowOrigin` list may also be configured with exact origin matches or regex patterns.
In the following example, cross-domain requests must originate from the domain `https://client.example.com` or domains that match the regex `http[s]?:\/\/some-site-[a-z0-9]+\.example\.com` (e.g. request with `Origin` header `https://some-site-abc456.example.com`)

*Note:* Patterns for matching `Origin` headers must be valid regex, simple "globbing" patterns (e.g. `*.foo.com`) will not be accepted or may produce incorrect matches.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: cors-example
spec:
  virtualhost:
    fqdn: www.example.com
    corsPolicy:
        allowCredentials: true
        allowOrigin:
          - https://client.example.com
          - http[s]?:\/\/some-site-[a-z0-9]+\.example\.com
        allowMethods:
          - GET
          - POST
          - OPTIONS
        allowHeaders:
          - authorization
          - cache-control
        exposeHeaders:
          - Content-Length
          - Content-Range
        maxAge: "10m"
  routes:
    - conditions:
      - prefix: /
      services:
        - name: cors-example
          port: 80
```

`MaxAge` durations are expressed in the Go [duration format](https://godoc.org/time#ParseDuration).
Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". Only positive values are allowed and 0 disables the cache requiring a preflight `OPTIONS` check for all cross-origin requests.
