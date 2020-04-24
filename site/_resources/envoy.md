---
title: Envoy Support Matrix
layout: page
---

Due to the aggressive deprecation cycle of Envoy's xDS API, not all versions of Contour will work with all versions of Envoy, and vice versa.

This page describes the compatibility matrix of Contour and Envoy versions.

## Supported Contour/Envoy Versions

| Contour Version | Envoy Version        |
| --------------- | :------------------- |
| 1.4.0           | 1.14.1               |
| 1.3.0           | 1.13.1<sup>4</sup>   |
| 1.2.1           | 1.13.1<sup>4</sup>   |
| 1.2.0           | 1.13.0               |
| 1.1.0           | 1.12.2<sup>2,3</sup> |
| 1.0.1           | 1.12.2<sup>2,3</sup> |
| 1.0.0           | 1.11.2<sup>1</sup>   |

<br>
#### Notes

1. [CVE-2019-15225, CVE-2019-15226][1]
2. [CVE-2019-18836][2]
3. [CVE-2019-18801. CVE-1019-18802, CVE-1019-18838][4]
4. [CVE-2020-8659, CVE-2020-8661, CVE-2020-8664, CVE-2020-8660][5]

## Envoy extensions

Contour requires the following extensions.
If you are using the image recommended in our [example deployment][3] no action is required.
If you are providing your own Envoy it must be compiled with the following extensions:

- `access_loggers`: `envoy.access_loggers.file`,`envoy.access_loggers.http_grpc`,`envoy.access_loggers.tcp_grpc`
- `filters.http`: `envoy.buffer`,`envoy.cors`,`envoy.csrf`,`envoy.fault`,`envoy.filters.http.adaptive_concurrency`,`envoy.filters.http.dynamic_forward_proxy`,`envoy.filters.http.grpc_http1_reverse_bridge`,`envoy.filters.http.grpc_stats`,`envoy.filters.http.header_to_metadata`,`envoy.filters.http.original_src`,`envoy.grpc_http1_bridge`,`envoy.grpc_json_transcoder`,`envoy.grpc_web`,`envoy.gzip`,`envoy.health_check`,`envoy.ip_tagging`,`envoy.router`
- `filters.listener`: `envoy.listener.http_inspector`,`envoy.listener.original_dst`,`envoy.listener.original_src`,`envoy.listener.proxy_protocol`,`envoy.listener.tls_inspector`
- `filters.network`: `envoy.echo`,`envoy.filters.network.sni_cluster`,`envoy.http_connection_manager`,`envoy.tcp_proxy`
- `stat_sinks`: `envoy.metrics_service`
- `transport_sockets`: `envoy.transport_sockets.alts`, `envoy.transport_sockets.raw_buffer`

[1]: https://groups.google.com/forum/#!topic/envoy-announce/Zo3ZEFuPWec
[2]: https://groups.google.com/d/msg/envoy-announce/3-8S992PUV4/t-egdelVDwAJ
[3]: {{site.github.repository_url}}/tree/{{site.github.latest_release.tag_name}}/examples/contour
[4]: https://groups.google.com/d/msg/envoy-announce/BjgUTDTKAu8/DTfMMSyCAgAJ
[5]: https://groups.google.com/forum/#!msg/envoy-announce/sVqmxy0un2s/8aq430xiHAAJ
