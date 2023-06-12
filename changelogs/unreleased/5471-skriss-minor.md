## Gateway API: add TCPRoute support

Contour now supports Gateway API's [TCPRoute](https://gateway-api.sigs.k8s.io/guides/tcp/) resource.
This route type provides simple TCP forwarding for traffic received on a given Listener port.

This is a simple example of a Gateway and TCPRoute configuration:

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
  namespace: projectcontour
spec:
  gatewayClassName: contour
  listeners:
    - name: tcp-listener
      protocol: TCP
      port: 10000
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: echo-1
  namespace: default
spec:
  parentRefs:
  - namespace: projectcontour
    name: contour
    sectionName: tcp-listener
  rules:
  - backendRefs:
    - name: s1
      port: 80
```