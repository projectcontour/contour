# CORS

A CORS (Cross-origin resource sharing) policy can be set for a HTTPProxy in order to allow cross-domain requests for trusted sources.
If a policy is set, it will be applied to all the routes of the virtual host.

Contour allows configuring the headers involved in cross-domain requests.
In this example, cross-domain requests will be allowed for any domain (note the `*` value).

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

In the following example, cross-domain requests are restricted to `https://client.example.com` only.

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
          - "https://client.example.com"
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
