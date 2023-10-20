## HTTPProxy: Allow Host header rewrite with dynamic headers.

This Change allows the host header to be rewritten on requests using dynamic headers on the only route level.

#### Example
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: dynamic-host-header-rewrite
spec:
  fqdn: local.projectcontour.io
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
    - requestHeaderPolicy:
        set:
        - name: host
          value: "%REQ(x-rewrite-header)%"
```

