## HTTPProxy: Allow custom host header on HttpProxy level.

This Change allows you set custom host headers on httpProxy level, Please note headers are appended to requests/responses in the following order: weighted cluster level headers, route level headers, virtual host level headers and finally global level headers.

#### Example
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: custom-host-header
spec:
  fqdn: local.projectcontour.io
  requestHeadersPolicy:
    set:
      - name: x-header
        value: somevalue
  responseHeadersPolicy:
    set:
      - name: x-powered-by
        value: contour
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
```