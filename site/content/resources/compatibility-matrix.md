---
title: Compatibility Matrix
layout: page
---

This page documents the compatibility matrix of versions of Contour, Envoy, Kubernetes, and Gateway API.
These combinations of versions are specifically tested in CI and supported by the Contour maintainers.

## Compatibility Matrix

| Contour Version | Envoy Version        | Kubernetes Versions | Gateway API Version |
| --------------- | :------------------- | ------------------- | --------------------|
| main            | [1.33.1][66]         | 1.32, 1.31, 1.30    | [1.2.1][112]        |
| 1.30.3          | [1.31.6][71]         | 1.30, 1.29, 1.28    | [1.1.0][111]        |
| 1.30.2          | [1.31.5][69]         | 1.30, 1.29, 1.28    | [1.1.0][111]        |
| 1.30.1          | [1.31.3][64]         | 1.30, 1.29, 1.28    | [1.1.0][111]        |
| 1.30.0          | [1.31.0][60]         | 1.30, 1.29, 1.28    | [1.1.0][111]        |
| 1.29.5          | [1.30.10][70]        | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.29.4          | [1.30.9][68]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.29.3          | [1.30.7][63]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.29.2          | [1.30.4][59]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.29.1          | [1.30.2][56]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.29.0          | [1.30.1][53]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.8          | [1.29.12][67]        | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.7          | [1.29.10][62]        | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.6          | [1.29.7][61]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.5          | [1.29.5][57]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.4          | [1.29.4][55]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.3          | [1.29.3][50]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.2          | [1.29.2][49]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.1          | [1.29.1][46]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.28.0          | [1.29.1][46]         | 1.29, 1.28, 1.27    | [1.0.0][110]        |
| 1.27.4          | [1.28.4][58]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.27.3          | [1.28.3][54]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.27.2          | [1.28.2][52]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.27.1          | [1.28.1][47]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.27.0          | [1.28.0][45]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.26.3          | [1.27.4][51]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.26.2          | [1.27.3][48]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.26.1          | [1.27.2][42]         | 1.28, 1.27, 1.26    | [0.8.1][109]        |
| 1.26.0          | [1.27.0][41]         | 1.28, 1.27, 1.26    | [0.8.0][108]        |
| 1.25.3          | [1.26.6][43]         | 1.27, 1.26, 1.25    | [0.6.2][107]        |
| 1.25.2          | [1.26.4][40]         | 1.27, 1.26, 1.25    | [0.6.2][107]        |
| 1.25.1          | [1.26.4][40]         | 1.27, 1.26, 1.25    | [0.6.2][107]        |
| 1.25.0          | [1.26.1][35]         | 1.27, 1.26, 1.25    | [0.6.2][107]        |
| 1.24.6          | [1.25.11][44]        | 1.26, 1.25, 1.24    | [0.6.0][106]        |
| 1.24.5          | [1.25.9][39]         | 1.26, 1.25, 1.24    | [0.6.0][106]        |
| 1.24.4          | [1.25.6][36]         | 1.26, 1.25, 1.24    | [0.6.0][106]        |
| 1.24.3          | [1.25.4][32]         | 1.26, 1.25, 1.24    | [0.6.0][106]        |
| 1.24.2          | [1.25.2][31]         | 1.26, 1.25, 1.24    | [0.6.0][106]        |
| 1.24.1          | [1.25.1][28]         | 1.26, 1.25, 1.24    | [0.6.0][106]        |
| 1.24.0          | [1.25.0][25]         | 1.26, 1.25, 1.24    | [0.6.0][106]        |
| 1.23.6          | [1.24.10][38]        | 1.25, 1.24, 1.23    | [0.5.1][105]        |
| 1.23.5          | [1.24.5][33]         | 1.25, 1.24, 1.23    | [0.5.1][105]        |
| 1.23.4          | [1.24.3][30]         | 1.25, 1.24, 1.23    | [0.5.1][105]        |
| 1.23.3          | [1.24.2][27]         | 1.25, 1.24, 1.23    | [0.5.1][105]        |
| 1.23.2          | [1.24.1][24]         | 1.25, 1.24, 1.23    | [0.5.1][105]        |
| 1.23.1          | [1.24.1][24]         | 1.25, 1.24, 1.23    | [0.5.1][105]        |
| 1.23.0          | [1.24.0][21]         | 1.25, 1.24, 1.23    | [0.5.1][105]        |
| 1.22.6          | [1.23.7][34]         | 1.24, 1.23, 1.22    | [0.5.0][104]        |
| 1.22.5          | [1.23.5][29]         | 1.24, 1.23, 1.22    | [0.5.0][104]        |
| 1.22.4          | [1.23.4][26]         | 1.24, 1.23, 1.22    | [0.5.0][104]        |
| 1.22.3          | [1.23.3][23]         | 1.24, 1.23, 1.22    | [0.5.0][104]        |
| 1.22.2          | [1.23.3][23]         | 1.24, 1.23, 1.22    | [0.5.0][104]        |
| 1.22.1          | [1.23.1][20]         | 1.24, 1.23, 1.22    | [0.5.0][104]        |
| 1.22.0          | [1.23.0][19]         | 1.24, 1.23, 1.22    | [0.5.0][104]        |
| 1.21.3          | [1.22.6][22]         | 1.23, 1.22, 1.21    | [0.4.3][103]        |
| 1.21.2          | [1.22.6][22]         | 1.23, 1.22, 1.21    | [0.4.3][103]        |
| 1.21.1          | [1.22.2][17]         | 1.23, 1.22, 1.21    | [0.4.3][103]        |
| 1.21.0          | [1.22.0][16]         | 1.23, 1.22, 1.21    | [0.4.3][103]        |
| 1.20.2          | [1.21.3][18]         | 1.23, 1.22, 1.21    | [0.4.1][102]        |
| 1.20.1          | [1.21.1][15]         | 1.23, 1.22, 1.21    | [0.4.1][102]        |
| 1.20.0          | [1.21.0][14]         | 1.23, 1.22, 1.21    | [0.4.1][102]        |
| 1.19.1          | [1.19.1][13]         | 1.22, 1.21, 1.20    | [0.3.0][101]        |
| 1.19.0          | [1.19.1][13]         | 1.22, 1.21, 1.20    | [0.3.0][101]        |
| 1.18.3          | [1.19.1][13]         | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.18.2          | [1.19.1][13]         | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.18.1          | [1.19.1][13]         | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.18.0          | [1.19.0][10]         | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.17.2          | [1.18.4][12]         | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.17.1          | [1.18.3][9]          | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.17.0          | [1.18.3][9]          | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.16.1          | [1.18.4][12]         | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.16.0          | [1.18.3][9]          | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.15.2          | [1.18.4][12]         | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.15.1          | [1.18.3][9]          | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.15.0          | [1.18.2][8]          | 1.21, 1.20, 1.19    | [0.3.0][101]        |
| 1.14.2          | [1.17.4][11]         | 1.20, 1.19, 1.18    | [0.2.0][100]        |
| 1.14.1          | [1.17.2][7]          | 1.20, 1.19, 1.18    | [0.2.0][100]        |
| 1.14.0          | [1.17.1][6]          | 1.20, 1.19, 1.18    | [0.2.0][100]        |
| 1.13.1          | [1.17.1][6]          | 1.20, 1.19, 1.18    | [0.2.0][100]        |
| 1.13.0          | [1.17.0][5]          | 1.20, 1.19, 1.18    | [0.2.0][100]        |
| 1.12.0          | [1.17.0][5]          | 1.19, 1.18, 1.17    | N/A                 |
| 1.11.0          | [1.16.2][4]          | 1.19, 1.18, 1.17    | N/A                 |
| 1.10.1          | [1.16.2][4]          | 1.19, 1.18, 1.17    | N/A                 |
| 1.10.0          | [1.16.0][3]          | 1.19, 1.18, 1.17    | N/A                 |
| 1.9.0           | [1.15.1][2]          | 1.18, 1.17, 1.16    | N/A                 |

<br />

## Notes on Compatibility

**As of Contour version 1.16.0, Contour only subscribes to Ingress v1 resources (and no longer falls back to Ingress v1beta1). The minimum compatible Kubernetes version for Contour 1.16.0 and above is Kubernetes 1.19.**

Contour utilizes [client-go][98] to watch for resources in a Kubernetes cluster.
We depend on the latest version of the library and by extension only support the latest versions of Kubernetes.
While the `client-go` [compatibility matrix][99] may list older versions of Kubernetes as being compatible and supported by upstream, the Contour project only tests a given version of Contour against the versions listed in the table above.
Combinations not listed are not tested, guaranteed to work, or supported by the Contour maintainers.

## Envoy Extensions
Contour requires the following Envoy extensions.
If you are using the image recommended in our [example deployment][1] no action is required.
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

__Note:__ This list of extensions was last verified to be complete with Envoy v1.16.1.


[1]: {{< param github_url >}}/tree/{{< param latest_version >}}/examples/contour

[2]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.15.1
[3]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.16.0
[4]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.16.2
[5]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.0
[6]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.1
[7]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.2
[8]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.18.2
[9]: https://www.envoyproxy.io/docs/envoy/v1.18.3/version_history/current
[10]: https://www.envoyproxy.io/docs/envoy/v1.19.0/version_history/current
[11]: https://www.envoyproxy.io/docs/envoy/v1.17.4/version_history/current
[12]: https://www.envoyproxy.io/docs/envoy/v1.18.4/version_history/current
[13]: https://www.envoyproxy.io/docs/envoy/v1.19.1/version_history/current
[14]: https://www.envoyproxy.io/docs/envoy/v1.21.0/version_history/current
[15]: https://www.envoyproxy.io/docs/envoy/v1.21.1/version_history/current
[16]: https://www.envoyproxy.io/docs/envoy/v1.22.0/version_history/current
[17]: https://www.envoyproxy.io/docs/envoy/v1.22.2/version_history/current
[18]: https://www.envoyproxy.io/docs/envoy/v1.21.3/version_history/current
[19]: https://www.envoyproxy.io/docs/envoy/v1.23.0/version_history/v1.23/v1.23.0
[20]: https://www.envoyproxy.io/docs/envoy/v1.23.1/version_history/v1.23/v1.23.1
[21]: https://www.envoyproxy.io/docs/envoy/v1.24.0/version_history/v1.24/v1.24.0
[22]: https://www.envoyproxy.io/docs/envoy/v1.22.6/version_history/current
[23]: https://www.envoyproxy.io/docs/envoy/v1.23.3/version_history/v1.23/v1.23.3
[24]: https://www.envoyproxy.io/docs/envoy/v1.24.1/version_history/v1.24/v1.24.1
[25]: https://www.envoyproxy.io/docs/envoy/v1.25.0/version_history/v1.25/v1.25.0
[26]: https://www.envoyproxy.io/docs/envoy/v1.23.4/version_history/v1.23/v1.23.4
[27]: https://www.envoyproxy.io/docs/envoy/v1.24.2/version_history/v1.24/v1.24.2
[28]: https://www.envoyproxy.io/docs/envoy/v1.25.1/version_history/v1.25/v1.25.1
[29]: https://www.envoyproxy.io/docs/envoy/v1.23.5/version_history/v1.23/v1.23.5
[30]: https://www.envoyproxy.io/docs/envoy/v1.24.3/version_history/v1.24/v1.24.3
[31]: https://www.envoyproxy.io/docs/envoy/v1.25.2/version_history/v1.25/v1.25.2
[32]: https://www.envoyproxy.io/docs/envoy/v1.25.4/version_history/v1.25/v1.25.4
[33]: https://www.envoyproxy.io/docs/envoy/v1.24.5/version_history/v1.24/v1.24.5
[34]: https://www.envoyproxy.io/docs/envoy/v1.23.7/version_history/v1.23/v1.23.7
[35]: https://www.envoyproxy.io/docs/envoy/v1.26.1/version_history/v1.26/v1.26.1
[36]: https://www.envoyproxy.io/docs/envoy/v1.25.6/version_history/v1.25/v1.25.6
[37]: https://www.envoyproxy.io/docs/envoy/v1.26.2/version_history/v1.26/v1.26.2
[38]: https://www.envoyproxy.io/docs/envoy/v1.24.10/version_history/v1.24/v1.24.10
[39]: https://www.envoyproxy.io/docs/envoy/v1.25.9/version_history/v1.25/v1.25.9
[40]: https://www.envoyproxy.io/docs/envoy/v1.26.4/version_history/v1.26/v1.26.4
[41]: https://www.envoyproxy.io/docs/envoy/v1.27.0/version_history/v1.27/v1.27.0
[42]: https://www.envoyproxy.io/docs/envoy/v1.27.2/version_history/v1.27/v1.27.2
[43]: https://www.envoyproxy.io/docs/envoy/v1.26.6/version_history/v1.26/v1.26.6
[44]: https://www.envoyproxy.io/docs/envoy/v1.25.11/version_history/v1.25/v1.25.11
[45]: https://www.envoyproxy.io/docs/envoy/v1.28.0/version_history/v1.28/v1.28.0
[46]: https://www.envoyproxy.io/docs/envoy/v1.29.1/version_history/v1.29/v1.29.1
[47]: https://www.envoyproxy.io/docs/envoy/v1.28.1/version_history/v1.28/v1.28.1
[48]: https://www.envoyproxy.io/docs/envoy/v1.27.3/version_history/v1.27/v1.27.3
[49]: https://www.envoyproxy.io/docs/envoy/v1.29.2/version_history/v1.29/v1.29.2
[50]: https://www.envoyproxy.io/docs/envoy/v1.29.3/version_history/v1.29/v1.29.3
[51]: https://www.envoyproxy.io/docs/envoy/v1.27.4/version_history/v1.27/v1.27.4
[52]: https://www.envoyproxy.io/docs/envoy/v1.28.2/version_history/v1.28/v1.28.2
[53]: https://www.envoyproxy.io/docs/envoy/v1.30.1/version_history/v1.30/v1.30.1
[54]: https://www.envoyproxy.io/docs/envoy/v1.28.3/version_history/v1.28/v1.28.3
[55]: https://www.envoyproxy.io/docs/envoy/v1.29.4/version_history/v1.29/v1.29.4
[56]: https://www.envoyproxy.io/docs/envoy/v1.30.2/version_history/v1.30/v1.30.2
[57]: https://www.envoyproxy.io/docs/envoy/v1.29.5/version_history/v1.29/v1.29.5
[58]: https://www.envoyproxy.io/docs/envoy/v1.28.4/version_history/v1.28/v1.28.4
[59]: https://www.envoyproxy.io/docs/envoy/v1.30.4/version_history/v1.30/v1.30.4
[60]: https://www.envoyproxy.io/docs/envoy/v1.31.0/version_history/v1.31/v1.31.0
[61]: https://www.envoyproxy.io/docs/envoy/v1.29.7/version_history/v1.29/v1.29.7
[62]: https://www.envoyproxy.io/docs/envoy/v1.29.10/version_history/v1.29/v1.29
[63]: https://www.envoyproxy.io/docs/envoy/v1.30.7/version_history/v1.30/v1.30
[64]: https://www.envoyproxy.io/docs/envoy/v1.31.3/version_history/v1.31/v1.31
[65]: https://www.envoyproxy.io/docs/envoy/v1.32.0/version_history/v1.32/v1.32
[66]: https://www.envoyproxy.io/docs/envoy/v1.33.1/version_history/v1.33/v1.33.1
[67]: https://www.envoyproxy.io/docs/envoy/v1.29.12/version_history/v1.29/v1.29.12
[68]: https://www.envoyproxy.io/docs/envoy/v1.30.9/version_history/v1.30/v1.30.9
[69]: https://www.envoyproxy.io/docs/envoy/v1.31.5/version_history/v1.31/v1.31.5
[70]: https://www.envoyproxy.io/docs/envoy/v1.30.10/version_history/v1.30/v1.30.10
[71]: https://www.envoyproxy.io/docs/envoy/v1.31.6/version_history/v1.31/v1.31.6

[98]: https://github.com/kubernetes/client-go
[99]: https://github.com/kubernetes/client-go#compatibility-matrix

[100]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.2.0
[101]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.3.0
[102]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.4.1
[103]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.4.3
[104]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.5.0
[105]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.5.1
[106]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.6.0
[107]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.6.2
[108]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.8.0
[109]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.8.1
[110]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.0.0
[111]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.1.0
[112]: https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.2.1
