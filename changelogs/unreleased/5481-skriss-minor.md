## Gateway API: Support TLS termination with TLSRoute and TCPRoute

Contour now supports using TLSRoute and TCPRoute in combination with TLS termination.
To use this feature, create a Gateway with a Listener like the following:

```yaml
- name: tls-listener
  protocol: TLS
  port: 5000
  tls:
    mode: Terminate
    certificateRefs:
    - name: tls-cert-secret
  allowedRoutes:
    namespaces:
      from: All
---
```

It is then possible to attach either 1+ TLSRoutes, or a single TCPRoute, to this Listener.
If using TLSRoute, traffic can be routed to a different backend based on SNI.
If using TCPRoute, all traffic is forwarded to the backend referenced in the route.