## Gateway API: support for processing a specific Gateway

Contour can now optionally process a specific named `Gateway` and associated routes.
This is an alternate way to configure Contour, vs. the existing mode of specifying a `GatewayClass` controller string and having Contour process the first `GatewayClass` and associated `Gateway` for that controller string.
This new configuration option can be specified via:
```yaml
gateway:
  gatewayRef:
    namespace: gateway-namespace
    name: gateway-name
```
