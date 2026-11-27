## Proxy `Expect: 100-continue` to upstream services

Contour now exposes Envoy's `proxy_100_continue` option as `listener.proxy-100-continue` in the configuration file and `spec.envoy.listener.proxy100Continue` in `ContourConfiguration`.
When enabled, Envoy forwards the `Expect: 100-continue` request header to the upstream service and passes its `100 Continue` response back to the client, so the upstream service decides whether the client should send the request body, for example to reject a large upload early.
The default is `false`, which keeps the current behavior: Envoy removes the `Expect: 100-continue` header and responds with `100 Continue` itself.
