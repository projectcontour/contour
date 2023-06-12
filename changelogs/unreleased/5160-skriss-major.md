## Support for Gateway Listeners on more than two ports

Contour now supports Gateway Listeners with many different ports.
Previously, Contour only allowed a single port for HTTP, and a single port for HTTPS/TLS.

As an example, the following Gateway, with two HTTP ports and two HTTPS ports, is now fully supported:

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  gatewayClassName: contour
  listeners:
    - name: http-1
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: Same
    - name: http-2
      protocol: HTTP
      port: 81
      allowedRoutes:
        namespaces:
          from: Same
    - name: https-1
      protocol: HTTPS
      port: 443
      allowedRoutes:
        namespaces:
          from: Same
      tls:
        mode: Terminate
        certificateRefs:
        - name: tls-cert-1
    - name: https-2
      protocol: HTTPS
      port: 444
      allowedRoutes:
        namespaces:
          from: Same
      tls:
        mode: Terminate
        certificateRefs:
        - name: tls-cert-2
```

If you are using the Contour Gateway Provisioner, ports for all valid Listeners will automatically be exposed via the Envoy service, and will update when any Listener changes are made.
If you are using static provisioning, you must keep the Service definition in sync with the Gateway spec manually.

Note that if you are using the Contour Gateway Provisioner along with HTTPProxy or Ingress for routing, then your Gateway must have exactly one HTTP Listener and one HTTPS Listener.
For this case, Contour supports a custom HTTPS Listener protocol value, to avoid having to specify TLS details in the Listener (since they're specified in the HTTPProxy or Ingress instead):

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour-with-httpproxy
spec:
  gatewayClassName: contour
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
    - name: https
      protocol: projectcontour.io/https
      port: 443
      allowedRoutes:
        namespaces:
          from: All
```