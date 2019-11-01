---
title: Envoy compatibility matrix
layout: page
---

Due to the aggressive deprecation cycle of Envoy's xDS API, not all versions of Contour will work with all versions of Envoy, and vice versa.

This page describes the compatibility matrix of Contour and Envoy versions.

## Supported Envoy versions

|              | Contour 1.0.0 | 
| ------------ | :-----------:|
| Envoy 1.11.0 | Not supported<sup>1</sup> |
| Envoy 1.11.1 | Not supported<sup>2</sup> |
| Envoy 1.11.2 |  Supported | 

#### Notes

1. [CVE-2019-9512, CVE-2019-9513, CVE-2019-9514, CVE-2019-9515, CVE-2019-9518][1] 
2. [CVE-2019-15225, CVE-2019-15226][2]

## Envoy extensions

Contour requires the following extensions.
If you are using the image recommended in our [example deployment](https://github.com/projectcontour/contour/blog/{{ site.github.latest_release.tag_name }}/examples/contour) no action is required.
If you are providing your own Envoy it must be compiled with the following extensions:

- `access_loggers`: `envoy.file_access_log`,`envoy.http_grpc_access_log`,`envoy.tcp_grpc_access_log`
- `filters.http`: `envoy.buffer`,`envoy.cors`,`envoy.csrf`,`envoy.ext_authz`,`envoy.fault`,`envoy.filters.http.adaptive_concurrency`,`envoy.filters.http.dynamic_forward_proxy`,`envoy.filters.http.grpc_http1_reverse_bridge`,`envoy.filters.http.grpc_stats`,`envoy.filters.http.header_to_metadata`,`envoy.filters.http.jwt_authn`,`envoy.filters.http.original_src`,`envoy.filters.http.rbac`,`envoy.filters.http.tap`,`envoy.grpc_http1_bridge`,`envoy.grpc_json_transcoder`,`envoy.grpc_web`,`envoy.gzip`,`envoy.health_check`,`envoy.ip_tagging`,`envoy.rate_limit`,`envoy.router`,`envoy.squash`
- `filters.listener`: `envoy.listener.http_inspector`,`envoy.listener.original_dst`,`envoy.listener.original_src`,`envoy.listener.proxy_protocol`,`envoy.listener.tls_inspector`
- `filters.network`: `envoy.client_ssl_auth`,`envoy.echo`,`envoy.ext_authz`,`envoy.filters.network.sni_cluster`,`envoy.http_connection_manager`,`envoy.ratelimit`,`envoy.tcp_proxy`
- `stat_sinks`: `envoy.metrics_service`,`envoy.statsd`
- `transport_sockets.downstream`: `envoy.transport_sockets.alts`,`envoy.transport_sockets.raw_buffer`,`envoy.transport_sockets.tls`,`raw_buffer`,`tls`
- `transport_sockets.upstream`: `envoy.transport_sockets.alts`,`envoy.transport_sockets.raw_buffer`,`envoy.transport_sockets.tls`,`raw_buffer`,`tls`

[1]: https://groups.google.com/forum/#!topic/envoy-announce/ZLchtraPYVk
[2]: https://groups.google.com/forum/#!topic/envoy-announce/Zo3ZEFuPWec
