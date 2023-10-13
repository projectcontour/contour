## HTTP/2 max concurrent streams is configurable

This field can be used to limit the number of concurrent streams Envoy will allow on a single connection from a downstream peer.
It can be used to tune resource usage and as a mitigation for DOS attacks arising from vulnerabilities like CVE-2023-44487.

The Contour ConfigMap can be modified similar to the following (and Contour restarted) to set this value:

```
listener:
  http2-max-concurrent-streams: 50
```
