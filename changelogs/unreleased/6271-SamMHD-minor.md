## Spawn Upstream Span is now enabled in tracing

As described in [Envoy documentations](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-tracing), ```spawn_upstream_span``` should be true when envoy is working as an independent proxy and from now on contour tracing spans will show up as a parent span to upstream spans.

