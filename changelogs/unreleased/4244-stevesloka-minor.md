## HTTPRedirectPolicy allows for Path rewriting

Adds a `Path` field to the `HTTPProxy.Spec.Route.RequestRedirectPolicy` which allows
for redirects to also specify the path to redirect to. When specified, an
HTTP 302 response will be sent to the requestor with the new path specified.

Sample HTTPProxy: 

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: rewrite-path
spec:
  virtualhost:
    fqdn: rewrite.projectcontour.io
  routes:
    - conditions:
        - prefix: /blog
      services:
        - name: blogservice
          port: 80
      requestRedirectPolicy:
        path: /blog/site
```

Request: 
```bash
$ curl -i http://rewrite.projectcontour.io/blog                                                                                                

HTTP/2 302 
location: http://rewrite.projectcontour.io/blog/site
vary: Accept-Encoding
date: Wed, 15 Dec 2021 20:42:04 GMT
server: envoy
```