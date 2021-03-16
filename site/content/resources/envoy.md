---
title: Envoy Support Matrix
layout: page
---

Due to the aggressive deprecation cycle of Envoy's xDS API, not all versions of Contour will work with all versions of Envoy, and vice versa.

This page describes the compatibility matrix of Contour and Envoy versions.

## Supported Contour/Envoy Versions

| Contour Version | Envoy Version        |
| --------------- | :------------------- |
| 1.11.0          | 1.16.2               |
| 1.10.1          | 1.16.2               |
| 1.10.0          | 1.16.0               |
| 1.9.0           | 1.15.1<sup>7</sup>   |
| 1.8.2           | 1.15.1<sup>7</sup>   |
| 1.8.1           | 1.15.0               |
| 1.8.0           | 1.15.0               |
| 1.7.0           | 1.15.0               |
| 1.6.1           | 1.14.3<sup>5</sup>   |
| 1.6.0           | 1.14.2<sup>5</sup>   |
| 1.5.1           | 1.14.2<sup>5</sup>   |
| 1.5.0           | 1.14.1               |
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
5. [CVE-2020-11080][6]
6. [CVE-2020-12603, CVE-2020-12605, CVE-2020-8663, CVE-2020-12604][7]
7. [CVE-2020-25017][8]

## Envoy extensions

Contour requires the following extensions.
If you are using the image recommended in our [example deployment][3] no action is required.
If you are providing your own Envoy it must be compiled with the following extensions:

- Access Loggers: 
  - envoy.access_loggers.file
  - envoy.access_loggers.http_grpc
  - envoy.access_loggers.tcp_grpc
  
- Compression:
  - envoy.compression.gzip.compressor
    
- HTTP Filters:
  - envoy.filters.http.compressor
  - envoy.filters.http.cors
  - envoy.filters.http.ext_authz
  - envoy.filters.http.grpc_stats
  - envoy.filters.http.grpc_web
  - envoy.filters.http.health_check
  - envoy.filters.http.lua
  - envoy.filters.http.router
   
- Listener filters
  - envoy.filters.listener.http_inspector
  - envoy.filters.listener.original_dst
  - envoy.filters.listener.proxy_protocol
  - envoy.filters.listener.tls_inspector

- Network filters
  - envoy.filters.network.client_ssl_auth
  - envoy.filters.network.ext_authz
  - envoy.filters.network.http_connection_manager
  - envoy.filters.network.tcp_proxy
  
- Transport sockets
  - envoy.transport_sockets.upstream_proxy_protocol
  - envoy.transport_sockets.raw_buffer
  
- Http Upstreams
  - envoy.upstreams.http.http
  - envoy.upstreams.http.tcp

__Note:__ These extensions are tested against Envoy v1.16.1.

[1]: https://groups.google.com/forum/#!topic/envoy-announce/Zo3ZEFuPWec
[2]: https://groups.google.com/d/msg/envoy-announce/3-8S992PUV4/t-egdelVDwAJ
[3]: {{< param github_url >}}/tree/{{< param latest_release_tag_name >}}/examples/contour
[4]: https://groups.google.com/d/msg/envoy-announce/BjgUTDTKAu8/DTfMMSyCAgAJ
[5]: https://groups.google.com/forum/#!msg/envoy-announce/sVqmxy0un2s/8aq430xiHAAJ
[6]: https://groups.google.com/d/msg/envoy-announce/y4C7hXH6WrU/eRoMZ6WaAgAJ
[7]: https://groups.google.com/d/msg/envoy-announce/qrrF8klFl-I/nz12XtqmAAAJ
[8]: https://groups.google.com/g/envoy-announce/c/5P0060xsRxc/m/dhIXZLjgCAAJ
